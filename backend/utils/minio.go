package utils

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"backend/config"
	"backend/models"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"gorm.io/gorm"
)

var ErrMediaUnavailable = errors.New("MinIO 暂时不可用")

type minioStorage struct {
	client *minio.Client
	bucket string
}

var objectStorage *minioStorage

func initMinIO() error {
	endpoint := config.Value("CWATCH_MINIO_ENDPOINT", "")
	if endpoint == "" {
		objectStorage = nil
		return nil
	}
	address, err := url.Parse(endpoint)
	if err != nil || address.Host == "" || address.User != nil || address.RawQuery != "" ||
		address.Fragment != "" || (address.Path != "" && address.Path != "/") ||
		(address.Scheme != "http" && address.Scheme != "https") {
		return errors.New("CWATCH_MINIO_ENDPOINT 必须是带 http:// 或 https:// 的 MinIO API 地址")
	}
	accessKey := config.Value("CWATCH_MINIO_ACCESS_KEY", "")
	secretKey := config.Value("CWATCH_MINIO_SECRET_KEY", "")
	if accessKey == "" || secretKey == "" {
		return errors.New("请配置 CWATCH_MINIO_ACCESS_KEY 和 CWATCH_MINIO_SECRET_KEY")
	}
	secure := address.Scheme == "https"
	transport, err := minio.DefaultTransport(secure)
	if err != nil {
		return err
	}
	transport.ResponseHeaderTimeout = 15 * time.Second
	client, err := minio.New(address.Host, &minio.Options{
		Creds: credentials.NewStaticV4(accessKey, secretKey, ""), Secure: secure,
		Transport: transport, Region: config.Value("CWATCH_MINIO_REGION", ""),
		MaxRetries: 2, BucketLookup: minio.BucketLookupPath,
	})
	if err != nil {
		return err
	}
	objectStorage = &minioStorage{client: client, bucket: config.Value("CWATCH_MINIO_BUCKET", "cwatch")}
	return nil
}

func DefaultVideoStorage() string {
	if objectStorage != nil {
		return models.StorageMinIO
	}
	return ""
}

func videoObjectPrefix(video *models.Video) string {
	return fmt.Sprintf("videos/%d/%s/", video.ID, video.FileName)
}

func storeMinIOUpload(ctx context.Context, video *models.Video, file string) error {
	if objectStorage == nil {
		return ErrMediaUnavailable
	}
	if video.SourceObject != "" {
		return nil
	}

	object := videoObjectPrefix(video) + "source/" + uuid.NewString()
	uploadCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := objectStorage.upload(uploadCtx, object, file, "application/octet-stream"); err != nil {
		objectStorage.discard(object)
		return err
	}

	var retained bool
	err := transaction(ctx, func(tx *gorm.DB) error {
		retained = false
		current, err := lockVideo(tx, video.ID, "UPDATE")
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && !sameUpload(current, video) {
			return ErrVideoUnavailable
		}
		if err != nil {
			return err
		}
		if current.SourceObject != "" {
			return nil
		}
		if err := tx.Model(&current).Update("source_object", object).Error; err != nil {
			return err
		}
		retained = true
		return nil
	})
	// An ambiguous commit may have saved the reference. Only discard the
	// upload when the database has definitively rejected or superseded it.
	if err == nil && !retained || errors.Is(err, ErrVideoUnavailable) {
		objectStorage.discard(object)
	}
	return err
}

func (storage *minioStorage) upload(ctx context.Context, object, file, contentType string) error {
	_, err := storage.client.FPutObject(ctx, storage.bucket, object, file, minio.PutObjectOptions{ContentType: contentType})
	return objectError(err)
}

func (storage *minioStorage) discard(object string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = storage.client.RemoveObject(ctx, storage.bucket, object, minio.RemoveObjectOptions{})
}

func (storage *minioStorage) deletePrefix(ctx context.Context, prefix string) error {
	listCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	for object := range storage.client.ListObjects(listCtx, storage.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if object.Err != nil {
			return objectError(object.Err)
		}
		if err := storage.client.RemoveObject(ctx, storage.bucket, object.Key, minio.RemoveObjectOptions{}); err != nil {
			return objectError(err)
		}
	}
	return ctx.Err()
}

func sourceSize(ctx context.Context, video *models.Video) (int64, error) {
	if video.Storage == models.StorageMinIO {
		if objectStorage == nil {
			return 0, ErrMediaUnavailable
		}
		if video.SourceObject == "" {
			return 0, os.ErrNotExist
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		info, err := objectStorage.client.StatObject(ctx, objectStorage.bucket, video.SourceObject, minio.StatObjectOptions{})
		return info.Size, objectError(err)
	}
	path, err := MediaPath(video.ID, video.FileName)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if !info.Mode().IsRegular() {
		return 0, os.ErrNotExist
	}
	return info.Size(), nil
}

func objectError(err error) error {
	if err == nil {
		return nil
	}
	switch minio.ToErrorResponse(err).Code {
	case "NoSuchKey", "NoSuchObject", "NotFound":
		return os.ErrNotExist
	default:
		return fmt.Errorf("%w: %v", ErrMediaUnavailable, err)
	}
}

func getMinIOObject(ctx context.Context, video *models.Video, name string) (*minio.Object, time.Time, error) {
	if objectStorage == nil {
		return nil, time.Time{}, ErrMediaUnavailable
	}
	key := videoObjectPrefix(video) + name
	if name == video.FileName {
		key = video.SourceObject
	}
	if key == "" {
		return nil, time.Time{}, os.ErrNotExist
	}
	object, err := objectStorage.client.GetObject(ctx, objectStorage.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, time.Time{}, objectError(err)
	}
	info, err := object.Stat()
	if err != nil {
		_ = object.Close()
		return nil, time.Time{}, objectError(err)
	}
	return object, info.LastModified, nil
}

func downloadMinIOSource(ctx context.Context, video *models.Video, dir string) (string, error) {
	if objectStorage == nil {
		return "", ErrMediaUnavailable
	}
	if video.SourceObject == "" {
		return "", os.ErrNotExist
	}
	input := filepath.Join(dir, "source"+filepath.Ext(video.FileName))
	err := objectStorage.client.FGetObject(ctx, objectStorage.bucket, video.SourceObject, input, minio.GetObjectOptions{})
	return input, objectError(err)
}
