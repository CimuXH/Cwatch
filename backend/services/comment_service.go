package services

import (
	"backend/models"
	"backend/utils"
	"errors"
	"log"
	"strings"
)

// CommentService 视频评论服务层
type CommentService struct{}

// CommentRequest 视频评论请求参数（用于添加评论）
type CommentRequest struct {
	Content string `json:"content" binding:"required"` // 评论内容，1-500字符
}

// CommentResponse 视频评论返回请求
type CommentResponse struct {
	Message 		string	`json:"message"`		// 响应信息
	CommentCount 	int64 	`json:"comment_count"`	// 当前评论数
}


// AddComment 添加视频评论
func (s *CommentService) AddComment(username string, videoID uint, req CommentRequest) (*CommentResponse, error) {
	// 1. 获取用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}

	// 2. 检查视频是否存在
	_, err = utils.GetVideoByID(videoID)
	if err != nil {
		return nil, err
	}

	// 3. 验证评论内容
	content := strings.TrimSpace(req.Content)
	if len(content) == 0 {
		return nil, errors.New("评论内容不能为空")
	}

	// 4. 创建评论对象
	comment := &models.Comment{
		Content: content,
		UserID:  user.ID,
		VideoID: videoID,
	}

	// 5. 检查是否重复评论
	isRepetition, err := utils.CheckCommentIsRepetition(comment)
	if err != nil {
		return nil, err
	}

	if isRepetition {
		return nil, errors.New("请勿重复发表相同内容的评论")
	}

	// 6. 将评论添加到数据库
	if err := utils.CreateComment(comment); err != nil {
		return nil, err
	}

	// 7. 获取该视频的评论总数
	commentCount := utils.GetVideoCommentCount(videoID)

	// 8. 返回响应
	return &CommentResponse{
		Message:      "评论成功",
		CommentCount: commentCount,
	}, nil
}

// GetComments 获取视频所有评论内容
func (s *CommentService)GetComments(videoid uint)([]utils.CommentListItem, error){
	// 检查视频是否存在
	if _, err := utils.GetVideoByID(videoid); err != nil {
		return nil, errors.New("视频不存在")
	}

	// 获取视频评论
	commentList, err := utils.GetVideoComments(videoid)
	if err != nil {
		return nil, errors.New("获取评论失败：" + err.Error())
	}
	
	// 返回信息
	return commentList, nil
}

// DeleteComment 删除评论
func (s *CommentService) DeleteComment(username string, commentid uint) error {
	// 1. 检查评论是否存在
	comment, err := utils.GetCommentByID(commentid)
	if err != nil {
		return errors.New("评论不存在")
	}

	// 2. 获取当前用户信息
	user, err := utils.GetUserByUsername(username)
	if err != nil {
		return errors.New("用户不存在")
	}

	// 3. 验证权限：只能删除自己的评论
	if comment.UserID != user.ID {
		return errors.New("无权删除他人的评论")
	}

	// 4. 删除评论
	if err := utils.DeleteComment(commentid); err != nil {
		log.Printf("删除评论失败: %v", err)
		return errors.New("删除评论失败")
	}

	// 5. 返回成功
	return nil
}
