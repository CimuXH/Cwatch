package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"backend/models"
	"backend/utils"
	"github.com/google/uuid"
)

type VideoService struct{}

var allowedVideoFormats = map[string]bool{
	".mp4": true, ".webm": true, ".mov": true, ".avi": true, ".mkv": true,
}

type UploadURLRequest struct {
	Filename string `json:"filename" binding:"required"`
	Filesize int64  `json:"filesize" binding:"required"`
	Title    string `json:"title"`
}

type UploadURLResponse struct {
	UploadURL string `json:"upload_url"`
	VideoID   uint   `json:"video_id"`
}

type ConfirmUploadRequest struct {
	VideoID uint `json:"video_id" binding:"required"`
}

type ConfirmUploadResponse struct {
	Success  bool   `json:"success"`
	VideoURL string `json:"video_url"`
}

type VideoListResponse struct {
	Videos   []utils.VideoListItem `json:"videos"`
	Total    int64                 `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

func (*VideoService) GetUploadURL(username string, req UploadURLRequest) (*UploadURLResponse, error) {
	ext := strings.ToLower(filepath.Ext(req.Filename))
	if !allowedVideoFormats[ext] {
		return nil, errors.New("不支持的视频格式，仅支持 mp4, webm, mov, avi, mkv")
	}
	if req.Filesize <= 0 || req.Filesize > utils.MaxVideoSize {
		return nil, errors.New("文件大小无效（最大500MB）")
	}
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = strings.TrimSuffix(req.Filename, filepath.Ext(req.Filename))
	}
	video := &models.Video{Title: title, UserID: user.ID, Status: models.VideoStatusUploading,
		FileName: uuid.NewString() + ext, SourceSize: req.Filesize, Storage: utils.DefaultVideoStorage()}
	if err := utils.CreateVideo(video); err != nil {
		return nil, err
	}
	return &UploadURLResponse{VideoID: video.ID, UploadURL: fmt.Sprintf("/api/video/upload/%d", video.ID)}, nil
}

func (*VideoService) StoreUpload(ctx context.Context, username string, videoID uint, body io.Reader) error {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return err
	}
	return utils.StoreUpload(ctx, user.ID, videoID, body)
}

func (*VideoService) ConfirmUpload(ctx context.Context, username string, req ConfirmUploadRequest) (*ConfirmUploadResponse, error) {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	url, err := utils.ConfirmUpload(ctx, user.ID, req.VideoID)
	if err != nil {
		return nil, err
	}
	return &ConfirmUploadResponse{Success: true, VideoURL: url}, nil
}

func (*VideoService) RetryProcessing(ctx context.Context, username string, videoID uint) error {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return err
	}
	return utils.RetryTranscode(ctx, user.ID, videoID)
}

func (*VideoService) GetVideoList(ctx context.Context, page, pageSize int, username string) (*VideoListResponse, error) {
	page, pageSize = videoPage(page, pageSize)
	videos, total, err := utils.CachedVideoList(ctx, page, pageSize)
	if err != nil {
		return nil, err
	}
	if err := utils.FillLiked(ctx, username, videos); err != nil {
		return nil, err
	}
	return &VideoListResponse{Videos: videos, Total: total, Page: page, PageSize: pageSize}, nil
}

func (*VideoService) GetUserVideoList(ctx context.Context, page, pageSize int, userID uint) (*VideoListResponse, error) {
	if userID == 0 {
		return nil, errors.New("无效的用户 ID")
	}
	page, pageSize = videoPage(page, pageSize)
	videos, total, err := utils.GetVideoList(ctx, page, pageSize, userID)
	if err != nil {
		return nil, err
	}
	return &VideoListResponse{Videos: videos, Total: total, Page: page, PageSize: pageSize}, nil
}

func videoPage(page, size int) (int, int) {
	if size < 1 || size > 50 {
		size = 12
	}
	return max(1, page), size
}

func (*VideoService) DeleteUserVideos(ctx context.Context, username string, videoIDs []uint) (int, error) {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return 0, err
	}
	return utils.DeleteUserVideos(ctx, user.ID, videoIDs)
}

func (*VideoService) GetHotVideos(ctx context.Context, limit int, username string) (*VideoListResponse, error) {
	videos, total, err := utils.CachedHotVideos(ctx, limit)
	if err != nil {
		return nil, err
	}
	if err := utils.FillLiked(ctx, username, videos); err != nil {
		return nil, err
	}
	return &VideoListResponse{Videos: videos, Total: total, Page: 1, PageSize: limit}, nil
}
