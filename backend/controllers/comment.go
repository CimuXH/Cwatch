package controllers

import (
	"backend/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 创建评论服务实例
var commentService = services.CommentService{}

// AddComment 添加评论
// 请求：POST /api/video/comment/:videoid
// Header: Authorization: Bearer <token>
// Body: {"content": "评论内容"}
func AddComment(c *gin.Context) {
	// 1. 获取当前用户（JWT已经验证过了）
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 2. 从路径参数获取视频ID
	videoIDStr := c.Param("videoid")
	videoID, err := strconv.ParseUint(videoIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的视频ID",
		})
		return
	}

	// 3. 绑定请求体（只包含评论内容）
	var req services.CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数：" + err.Error(),
		})
		return
	}

	// 4. 调用服务层添加评论（videoID 作为独立参数传入）
	response, err := commentService.AddComment(username.(string), uint(videoID), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 5. 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message":       response.Message,
		"comment_count": response.CommentCount,
	})
}

// GetComments 获取视频评论
// 请求：GET /video/:videoid/comments
// 不需要JWT验证
func GetComments(c *gin.Context){
	// 从路径参数获取视频ID
	videoidStr := c.Param("videoid")
	videoid, err := strconv.ParseUint(videoidStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的视频ID",
		})
		return
	}


	// 获取评论
	comments, err := commentService.GetComments(uint(videoid))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 返回响应
	c.JSON(http.StatusOK, gin.H{
		"message":	"获取评论成功",
		"comments":	comments,
	})
}

// DeleteComment 删除评论
// 请求：DELETE /api/video/comment/:commentid
// Header: Authorization: Bearer <token>
func DeleteComment(c *gin.Context) {
	// 1. 获取当前用户（JWT已经验证过了）
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 2. 从路径参数获取评论ID
	commentIDStr := c.Param("commentid")
	commentID, err := strconv.ParseUint(commentIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的评论ID",
		})
		return
	}

	// 3. 调用服务层删除评论
	err = commentService.DeleteComment(username.(string), uint(commentID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 4. 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "删除评论成功",
	})
}
