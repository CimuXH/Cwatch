package services

import (
	"backend/models"
	"backend/utils"
	"errors"
	"fmt"
	"log"
)

// LikeService 点赞服务层
type LikeService struct{}

// LikeRequest 点赞请求参数
type LikeRequest struct {
	VideoID uint `json:"video_id" binding:"required"` // 视频ID
}

// LikeResponse 点赞响应
type LikeResponse struct {
	Message   string `json:"message"`    // 响应消息
	LikeCount int64  `json:"like_count"` // 当前点赞数
	IsLiked   bool   `json:"is_liked"`   // 是否已点赞
}

// AddLike 添加点赞
func (s *LikeService) AddLike(username string, req LikeRequest) (*LikeResponse, error) {
	
	// 1. 获取用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 2. 检查视频是否存在
	_, err = utils.GetVideoByID(req.VideoID)
	if err != nil {
		return nil, errors.New("视频不存在")
	}

	// 3. 使用GetByUseridAndVideoid检查是否已点赞
	isLiked := utils.GetByUseridAndVideoid(user.ID, req.VideoID)
	
	if isLiked {
		return nil, errors.New("已经点赞过了")
	}

	// 4. 创建点赞记录
	like := &models.Like{
		UserID:  user.ID,
		VideoID: req.VideoID,
	}

	err = utils.CreateLike(like)
	if err != nil {
		return nil, fmt.Errorf("点赞失败: %v", err)
	}

	// 5. 获取最新点赞数
	likeCount := utils.GetVideoLikeCount(req.VideoID)

	return &LikeResponse{
		Message:   "点赞成功",
		LikeCount: likeCount,
		IsLiked:   true,
	}, nil
}

// RemoveLike 取消点赞
func (s *LikeService) RemoveLike(username string, req LikeRequest) (*LikeResponse, error) {
	// 1. 获取用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 2. 检查视频是否存在
	_, err = utils.GetVideoByID(req.VideoID)
	if err != nil {
		return nil, errors.New("视频不存在")
	}

	// 3. 使用GetByUseridAndVideoid检查是否已点赞
	if !utils.GetByUseridAndVideoid(user.ID, req.VideoID) {
		return nil, errors.New("还没有点赞")
	}

	// 4. 删除点赞记录
	err = utils.DeleteLike(user.ID, req.VideoID)
	if err != nil {
		return nil, errors.New("取消点赞失败")
	}

	// 5. 获取最新点赞数
	likeCount := utils.GetVideoLikeCount(req.VideoID)

	return &LikeResponse{
		Message:   "取消点赞成功",
		LikeCount: likeCount,
		IsLiked:   false,
	}, nil
}

// ToggleLike 切换点赞状态（点赞/取消点赞）
func (s *LikeService) ToggleLike(username string, req LikeRequest) (*LikeResponse, error) {
	// 1. 获取用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, errors.New("用户不存在")
	}

	// 2. 检查视频是否存在
	_, err = utils.GetVideoByID(req.VideoID)
	if err != nil {
		return nil, errors.New("视频不存在")
	}

	// 3. 使用GetByUseridAndVideoid检查当前点赞状态并切换
	isLiked := utils.GetByUseridAndVideoid(user.ID, req.VideoID)
	
	if isLiked {
		// 已点赞，执行取消点赞
		return s.RemoveLike(username, req)
	} else {
		// 未点赞，执行点赞
		return s.AddLike(username, req)
	}
}