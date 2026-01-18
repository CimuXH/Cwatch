package services

import (
	"backend/models"
	"backend/utils"
	"errors"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// VideoService 视频服务层
type VideoService struct{}

// 允许的视频格式
var allowedVideoFormats = map[string]bool{
	".mp4":  true,
	".webm": true,
	".mov":  true,
	".avi":  true,
	".mkv":  true,
}

// 文件大小限制（500MB）
const MaxFileSize = 500 * 1024 * 1024

// UploadURLRequest 获取上传URL请求
type UploadURLRequest struct {
	Filename string `json:"filename" binding:"required"` // 原始文件名
	Filesize int64  `json:"filesize" binding:"required"` // 文件大小（字节）
	Title    string `json:"title"`                       // 视频标题（可选）
}

// UploadURLResponse 获取上传URL响应
type UploadURLResponse struct {
	UploadURL string `json:"upload_url"` // 预签名上传URL
	VideoID   uint   `json:"video_id"`   // 视频ID
}

// ConfirmUploadRequest 确认上传完成请求
type ConfirmUploadRequest struct {
	VideoID uint `json:"video_id" binding:"required"` // 视频ID
}

// ConfirmUploadResponse 确认上传完成响应
type ConfirmUploadResponse struct {
	Success  bool   `json:"success"`   // 是否成功
	VideoURL string `json:"video_url"` // 视频访问URL
}

// VideoListResponse 视频列表响应
type VideoListResponse struct {
	Videos   []utils.VideoListItem `json:"videos"`    // 视频列表
	Total    int64                 `json:"total"`     // 总数
	Page     int                   `json:"page"`      // 当前页
	PageSize int                   `json:"page_size"` // 每页数量
}

// GetUploadURL 获取上传凭证
// 参数：用户名、请求参数
// 返回：上传URL和视频ID
func (s *VideoService) GetUploadURL(username string, req UploadURLRequest) (*UploadURLResponse, error) {
	// 获取用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 1. 验证文件格式
	ext := strings.ToLower(filepath.Ext(req.Filename))
	if !allowedVideoFormats[ext] {
		return nil, errors.New("不支持的视频格式，仅支持 mp4, webm, mov, avi, mkv")
	}

	// 2. 验证文件大小
	if req.Filesize <= 0 {
		return nil, errors.New("文件大小无效")
	}
	if req.Filesize > MaxFileSize {
		return nil, errors.New("文件大小超过限制（最大500MB）")
	}

	// 3. 生成唯一文件名：UUID + 原始扩展名
	uniqueFilename := uuid.New().String() + ext

	// 4. 生成预签名上传URL
	uploadURL, err := utils.GenerateUploadURL(uniqueFilename)
	if err != nil {
		return nil, errors.New("生成上传URL失败")
	}

	// 5. 设置视频标题（如果未提供，使用原始文件名）
	title := req.Title
	if title == "" {
		title = strings.TrimSuffix(req.Filename, ext)
	}

	// 6. 创建视频记录（状态：上传中）
	video := &models.Video{
		Title:    title,
		UserID:   user.ID,
		Status:   models.VideoStatusUploading,
		FileName: uniqueFilename,
	}

	if err := utils.CreateVideo(video); err != nil {
		return nil, errors.New("创建视频记录失败")
	}

	return &UploadURLResponse{
		UploadURL: uploadURL,
		VideoID:   video.ID,
	}, nil
}

// ConfirmUpload 确认上传完成
// 参数：用户名、视频ID
// 返回：视频访问URL
func (s *VideoService) ConfirmUpload(username string, req ConfirmUploadRequest) (*ConfirmUploadResponse, error) {
	// 获取用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 1. 查询视频记录
	video, err := utils.GetVideoByID(req.VideoID)
	if err != nil {
		return nil, errors.New("视频不存在")
	}

	// 2. 验证视频所有权
	if video.UserID != user.ID {
		return nil, errors.New("无权操作此视频")
	}

	// 3. 验证视频状态（只有"上传中"状态才能确认）
	if video.Status != models.VideoStatusUploading {
		return nil, errors.New("视频状态异常")
	}

	// 4. 检查文件是否真的上传到了MinIO
	exists, err := utils.CheckFileExists(video.FileName)
	if err != nil {
		return nil, errors.New("检查文件失败")
	}
	if !exists {
		return nil, errors.New("文件未上传成功，请重新上传")
	}

	// 5. 生成永久视频访问URL
	videoURL, err := utils.GenerateDownloadURL(video.FileName)
	if err != nil {
		return nil, errors.New("生成视频URL失败")
	}

	// 6. 更新视频状态和URL
	err = utils.UpdateVideo(video.ID, map[string]interface{}{
		"status": models.VideoStatusUploaded,
		"url":    videoURL,
	})
	if err != nil {
		return nil, errors.New("更新视频状态失败")
	}

	// TODO: 发送消息到 RabbitMQ 触发转码任务

	return &ConfirmUploadResponse{
		Success:  true,
		VideoURL: videoURL,
	}, nil
}

// GetVideoList 获取视频列表
// 参数：页码、每页数量
// 返回：视频列表响应
func (s *VideoService) GetVideoList(page, pageSize int) (*VideoListResponse, error) {
	// 限制每页数量
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 12
	}

	// 调用工具层获取视频列表
	videos, total, err := utils.GetVideoList(page, pageSize)
	if err != nil {
		return nil, errors.New("获取视频列表失败")
	}

	return &VideoListResponse{
		Videos:   videos,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}