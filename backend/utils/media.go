package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"backend/config"
	"backend/models"
	"gorm.io/gorm"
)

const MaxVideoSize = 500 * 1024 * 1024

var mediaDir string

func InitMedia() error {
	dir, err := filepath.Abs(config.Value("CWATCH_MEDIA_DIR", "../data/media"))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0750); err != nil {
		return err
	}
	mediaDir = dir
	return initMinIO()
}

func VideoDir(id uint) string {
	return filepath.Join(mediaDir, strconv.FormatUint(uint64(id), 10))
}

func MediaPath(id uint, name string) (string, error) {
	if !filepath.IsLocal(name) || strings.Contains(name, "\\") || filepath.ToSlash(filepath.Clean(name)) != name {
		return "", errors.New("无效的文件路径")
	}
	return filepath.Join(VideoDir(id), filepath.FromSlash(name)), nil
}

func MediaURL(id uint, name string) string {
	return fmt.Sprintf("/media/%d/%s", id, name)
}

func OpenVideoMedia(ctx context.Context, video *models.Video, name string) (io.ReadSeekCloser, time.Time, error) {
	path, err := MediaPath(video.ID, name)
	if err != nil {
		return nil, time.Time{}, err
	}
	if video.Storage == models.StorageMinIO {
		return getMinIOObject(ctx, video, name)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, time.Time{}, err
		}
		return nil, time.Time{}, os.ErrNotExist
	}
	return file, info.ModTime(), nil
}

func PrepareTranscodeFiles(ctx context.Context, video *models.Video, token string) (dir, input string, err error) {
	if video.Storage == models.StorageMinIO {
		dir, err := os.MkdirTemp(mediaDir, ".transcode-")
		if err != nil {
			return "", "", err
		}
		input, err := downloadMinIOSource(ctx, video, dir)
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", "", err
		}
		return dir, input, nil
	}
	input, err = MediaPath(video.ID, video.FileName)
	if err != nil {
		return "", "", err
	}
	if _, err := os.Stat(input); err != nil {
		return "", "", err
	}
	dir, err = MediaPath(video.ID, token)
	if err != nil {
		return "", "", err
	}
	return dir, input, os.Mkdir(dir, 0750)
}

func StoreTranscodeArtifact(ctx context.Context, video *models.Video, token, name, dir, contentType string) error {
	if video.Storage != models.StorageMinIO {
		return nil
	}
	if objectStorage == nil {
		return ErrMediaUnavailable
	}
	return objectStorage.upload(ctx, videoObjectPrefix(video)+token+"/"+name, filepath.Join(dir, name), contentType)
}

func DiscardTranscodeFiles(video *models.Video, token, dir string) {
	_ = os.RemoveAll(dir)
	if video.Storage == models.StorageMinIO && objectStorage != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = objectStorage.deletePrefix(ctx, videoObjectPrefix(video)+token+"/")
	}
}

func CleanupVideoMedia(ctx context.Context, task models.VideoTask) error {
	var video models.Video
	err := db.WithContext(ctx).Unscoped().First(&video, task.VideoID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (!video.DeletedAt.Valid || video.FileName != task.Source) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	if video.Storage == models.StorageMinIO {
		if objectStorage == nil {
			return ErrMediaUnavailable
		}
		return objectStorage.deletePrefix(ctx, videoObjectPrefix(&video))
	}
	return os.RemoveAll(VideoDir(task.VideoID))
}

func StoreUpload(ctx context.Context, userID, id uint, body io.Reader) error {
	video, err := GetVideoByID(ctx, id)
	if err != nil || video.UserID != userID || video.Status != models.VideoStatusUploading ||
		video.SourceSize < 1 || video.SourceSize > MaxVideoSize || time.Since(video.CreatedAt) > 24*time.Hour {
		return ErrVideoUnavailable
	}
	target, err := MediaPath(id, video.FileName)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(mediaDir, ".upload-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	size, copyErr := io.Copy(file, io.LimitReader(body, video.SourceSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if size != video.SourceSize {
		return errors.New("实际文件大小与声明不符")
	}
	if video.Storage == models.StorageMinIO {
		return storeMinIOUpload(ctx, video, file.Name())
	}

	return transaction(ctx, func(tx *gorm.DB) error {
		current, err := lockVideo(tx, id, "UPDATE")
		if err != nil {
			return err
		}
		if !sameUpload(current, video) {
			return ErrVideoUnavailable
		}
		if _, err := os.Stat(target); err == nil {
			return nil // The first completed PUT fixes this source version.
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(VideoDir(id), 0750); err != nil {
			return err
		}
		return os.Rename(file.Name(), target)
	})
}

func sameUpload(current models.Video, source *models.Video) bool {
	return current.UserID == source.UserID && current.Status == models.VideoStatusUploading &&
		current.FileName == source.FileName && current.Storage == source.Storage && current.SourceSize == source.SourceSize
}

func ConfirmUpload(ctx context.Context, userID, id uint) (string, error) {
	video, err := GetVideoByID(ctx, id)
	if err != nil || video.UserID != userID {
		return "", ErrVideoUnavailable
	}
	if video.Status == models.VideoStatusUploading {
		size, err := sourceSize(ctx, video)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err != nil || size != video.SourceSize || size == 0 {
			return "", errors.New("文件未上传成功，请重新上传")
		}
	}

	var url string
	var changed bool
	err = transaction(ctx, func(tx *gorm.DB) error {
		changed = false
		current, err := lockVideo(tx, id, "UPDATE")
		if err != nil || current.UserID != userID || current.FileName != video.FileName {
			return ErrVideoUnavailable
		}
		if current.Status == models.VideoStatusUploaded {
			url = current.URL
			return nil
		}
		if current.Status != models.VideoStatusUploading {
			return ErrVideoUnavailable
		}
		if !sameUpload(current, video) || current.SourceObject != video.SourceObject {
			return ErrVideoUnavailable
		}
		url = MediaURL(id, current.FileName)
		if err := tx.Model(&current).Updates(map[string]any{"status": models.VideoStatusUploaded, "url": url}).Error; err != nil {
			return err
		}
		changed = true
		return tx.Create(&models.VideoTask{VideoID: id, Source: current.FileName, Kind: models.TaskTranscode,
			State: models.TaskPending, RunAt: time.Now().UTC()}).Error
	})
	if err == nil && changed {
		InvalidateVideoCache()
	}
	return url, err
}
