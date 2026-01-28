package controllers

import (
	"backend/services"
	"net/http"
	"github.com/gin-gonic/gin"
)

// 创建点赞服务实例
var likeService = services.LikeService{}

// AddLike 点赞视频（Redis 主 + MQ 异步落库）
// 请求：POST /api/video/like
// Header: Authorization: Bearer <token>
// Body: { "video_id": 1 }
// 返回：{ "message": "点赞成功", "like_count": 10, "is_liked": true }
func AddLike(c *gin.Context) {	
	// 1. 获取当前用户（JWT中间件已验证）
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 2. 绑定请求参数
	var req services.LikeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 3. 调用服务层添加点赞
	resp, err := likeService.AddLike(username.(string), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RemoveLike 取消点赞（Redis 主 + MQ 异步落库）
// 请求：DELETE /api/video/like
// Header: Authorization: Bearer <token>
// Body: { "video_id": 1 }
// 返回：{ "message": "取消点赞成功", "like_count": 9, "is_liked": false }
func RemoveLike(c *gin.Context) {
	// 1. 获取当前用户
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 2. 绑定请求参数
	var req services.LikeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 3. 调用服务层取消点赞
	resp, err := likeService.RemoveLike(username.(string), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ToggleLike 切换点赞状态（推荐使用）
// 请求：POST /api/video/toggle-like
// Header: Authorization: Bearer <token>
// Body: { "video_id": 1 }
// 返回：{ "message": "点赞成功", "like_count": 10, "is_liked": true }
func ToggleLike(c *gin.Context) {
	// 1. 获取当前用户
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 2. 绑定请求参数
	var req services.LikeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 3. 调用服务层切换点赞状态
	resp, err := likeService.ToggleLike(username.(string), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}