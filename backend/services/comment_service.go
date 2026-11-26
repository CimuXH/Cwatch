package services

import (
	"context"
	"errors"
	"strings"

	"backend/models"
	"backend/utils"
)

type CommentService struct{}

type CommentRequest struct {
	Content string `json:"content" binding:"required,max=500"`
}

type CommentResponse struct {
	Message      string `json:"message"`
	CommentCount int64  `json:"comment_count"`
}

func (*CommentService) AddComment(ctx context.Context, username string, videoID uint, req CommentRequest) (*CommentResponse, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, errors.New("评论内容不能为空")
	}
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	count, err := utils.CreateComment(ctx, &models.Comment{Content: content, UserID: user.ID, VideoID: videoID})
	if err != nil {
		return nil, err
	}
	return &CommentResponse{Message: "评论成功", CommentCount: count}, nil
}

func (*CommentService) GetComments(ctx context.Context, videoID uint) ([]utils.CommentListItem, error) {
	video, err := utils.GetVideoByID(ctx, videoID)
	if err != nil || video.Status != models.VideoStatusUploaded {
		return nil, utils.ErrVideoUnavailable
	}
	return utils.GetVideoComments(ctx, videoID)
}

func (*CommentService) DeleteComment(ctx context.Context, username string, commentID uint) error {
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return err
	}
	return utils.DeleteComment(ctx, user.ID, commentID)
}
