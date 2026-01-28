package services

import (
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

// AddLike 添加点赞（Redis 主 + MQ 异步落库）
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

	// 3. 使用 Redis SET 检查是否已点赞（幂等性）
	isLiked, err := utils.IsUserLikedVideo(req.VideoID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("检查点赞状态失败: %v", err)
	}
	
	if isLiked {
		// 已点赞，直接返回成功（幂等）
		likeCount, _ := utils.GetVideoLikeCount(req.VideoID)
		return &LikeResponse{
			Message:   "已点赞",
			LikeCount: likeCount,
			IsLiked:   true,
		}, nil
	}

	// 4. 添加用户点赞记录到 Redis SET
	added, err := utils.AddUserLikeVideo(req.VideoID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("添加点赞记录失败: %v", err)
	}
	
	if !added {
		// 并发情况下已被添加
		likeCount, _ := utils.GetVideoLikeCount(req.VideoID)
		return &LikeResponse{
			Message:   "已点赞",
			LikeCount: likeCount,
			IsLiked:   true,
		}, nil
	}

	// 5. 增加 Redis ZSET 排行榜点赞数
	newCount, err := utils.IncrVideoLikeRank(req.VideoID, 1)
	if err != nil {
		// 回滚 Redis SET
		utils.RemoveUserLikeVideo(req.VideoID, user.ID)
		return nil, fmt.Errorf("更新排行榜失败: %v", err)
	}

	// 6. 发送 MQ 消息异步更新 MySQL
	err = utils.PublishLikeTask(req.VideoID, user.ID, 1)
	if err != nil {
		// MQ 发送失败只记录日志，不影响用户体验
		// 可以通过定时任务或其他方式补偿
		log.Printf("发送点赞 MQ 消息失败: VideoID=%d, UserID=%d, Error=%v", req.VideoID, user.ID, err)
	}

	return &LikeResponse{
		Message:   "点赞成功",
		LikeCount: newCount,
		IsLiked:   true,
	}, nil
}

// RemoveLike 取消点赞（Redis 主 + MQ 异步落库）
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

	// 3. 使用 Redis SET 检查是否已点赞（幂等性）
	isLiked, err := utils.IsUserLikedVideo(req.VideoID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("检查点赞状态失败: %v", err)
	}
	
	if !isLiked {
		// 未点赞，直接返回成功（幂等）
		likeCount, _ := utils.GetVideoLikeCount(req.VideoID)
		return &LikeResponse{
			Message:   "未点赞",
			LikeCount: likeCount,
			IsLiked:   false,
		}, nil
	}

	// 4. 从 Redis SET 移除用户点赞记录
	removed, err := utils.RemoveUserLikeVideo(req.VideoID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("移除点赞记录失败: %v", err)
	}
	
	if !removed {
		// 并发情况下已被移除
		likeCount, _ := utils.GetVideoLikeCount(req.VideoID)
		return &LikeResponse{
			Message:   "未点赞",
			LikeCount: likeCount,
			IsLiked:   false,
		}, nil
	}

	// 5. 减少 Redis ZSET 排行榜点赞数
	newCount, err := utils.IncrVideoLikeRank(req.VideoID, -1)
	if err != nil {
		// 回滚 Redis SET
		utils.AddUserLikeVideo(req.VideoID, user.ID)
		return nil, fmt.Errorf("更新排行榜失败: %v", err)
	}

	// 6. 发送 MQ 消息异步更新 MySQL
	err = utils.PublishLikeTask(req.VideoID, user.ID, -1)
	if err != nil {
		// MQ 发送失败只记录日志，不影响用户体验
		log.Printf("发送取消点赞 MQ 消息失败: VideoID=%d, UserID=%d, Error=%v", req.VideoID, user.ID, err)
	}

	return &LikeResponse{
		Message:   "取消点赞成功",
		LikeCount: newCount,
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

	// 3. 使用 Redis 检查当前点赞状态并切换
	isLiked, err := utils.IsUserLikedVideo(req.VideoID, user.ID)
	if err != nil {
		return nil, fmt.Errorf("检查点赞状态失败: %v", err)
	}
	
	if isLiked {
		// 已点赞，执行取消点赞
		return s.RemoveLike(username, req)
	} else {
		// 未点赞，执行点赞
		return s.AddLike(username, req)
	}
}