package services

import (
	"context"

	"backend/utils"
)

type LikeService struct{}

type LikeRequest struct {
	VideoID uint  `json:"video_id" binding:"required"`
	Liked   *bool `json:"liked"`
}

type LikeResponse struct {
	Message   string `json:"message"`
	LikeCount int64  `json:"like_count"`
	IsLiked   bool   `json:"is_liked"`
}

func (s *LikeService) AddLike(ctx context.Context, username string, req LikeRequest) (*LikeResponse, error) {
	desired := true
	return s.set(ctx, username, req.VideoID, &desired)
}

func (s *LikeService) RemoveLike(ctx context.Context, username string, req LikeRequest) (*LikeResponse, error) {
	desired := false
	return s.set(ctx, username, req.VideoID, &desired)
}

func (s *LikeService) ToggleLike(ctx context.Context, username string, req LikeRequest) (*LikeResponse, error) {
	return s.set(ctx, username, req.VideoID, req.Liked)
}

func (*LikeService) set(ctx context.Context, username string, videoID uint, desired *bool) (*LikeResponse, error) {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	count, liked, err := utils.SetLike(ctx, user.ID, videoID, desired)
	if err != nil {
		return nil, err
	}
	message := "取消点赞成功"
	if liked {
		message = "点赞成功"
	}
	return &LikeResponse{Message: message, LikeCount: count, IsLiked: liked}, nil
}
