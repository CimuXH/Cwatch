package controllers

import (
	"backend/services"
	"net/http"
	"strconv"
	"github.com/gin-gonic/gin"
)

// 创建视频服务实例
var videoService = services.VideoService{}

// GetVideoList 获取视频列表
// 请求：GET /api/videos?page=1&page_size=10
// Header: Authorization: Bearer <token> (可选，如果提供则返回 is_liked 字段)
// 返回：{ "videos": [...], "total": 100, "page": 1, "page_size": 10 }
func GetVideoList(c *gin.Context) {
	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "12"))

	// 尝试获取当前用户（可选，未登录时为空字符串）
	username, _ := c.Get("username")
	usernameStr := ""
	if username != nil {
		usernameStr = username.(string)
	}

	// 调用服务层获取视频列表
	resp, err := videoService.GetVideoList(page, pageSize, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetUploadURL 获取视频上传URL
// 请求：POST /api/video/upload-url
// Header: Authorization: Bearer <token>
// Body: { "filename": "test.mp4", "filesize": 102400, "title": "我的视频" }
// 返回：{ "upload_url": "...", "video_id": 1 }
func GetUploadURL(c *gin.Context) {
	// 从上下文获取用户名（JWT中间件已验证）
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 绑定请求参数
	var req services.UploadURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 调用服务层获取上传URL
	resp, err := videoService.GetUploadURL(username.(string), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// ConfirmUpload 确认视频上传完成
// 请求：POST /api/video/upload-complete
// Header: Authorization: Bearer <token>
// Body: { "video_id": 1 }
// 返回：{ "success": true, "video_url": "..." }
func ConfirmUpload(c *gin.Context) {
	// 从上下文获取用户名
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 绑定请求参数
	var req services.ConfirmUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 调用服务层确认上传
	resp, err := videoService.ConfirmUpload(username.(string), req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// GetUserVideoList 获取某个用户的视频列表
// 请求：GET /api/user/:userid/videos?page=1&page_size=10
// Header: Authorization: Bearer <token>
// 返回：{ "videos": [...], "total": 100, "page": 1, "page_size": 10 }
func GetUserVideoList(c *gin.Context) {
	// 获取用户ID参数
	userIDStr := c.Param("userid")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的用户ID",
		})
		return
	}

	// 获取分页参数
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "12"))

	// 调用服务层获取用户视频列表
	resp, err := videoService.GetUserVideoList(page, pageSize, uint(userID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// DeleteUserVideos 删除某个用户的视频列表
// 请求：POST /api/videos/delete
// Header: Authorization: Bearer <token>
// Body: { "video_ids": [1, 2, 3] }
// 返回：{ "success": true, "deleted_count": 3 }
func DeleteUserVideos(c *gin.Context){
	// 从上下文获取用户名
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 绑定请求参数
	var req struct {
		VideoIDs []uint `json:"video_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的请求参数: " + err.Error(),
		})
		return
	}

	// 调用服务层删除视频
	err := videoService.DeleteUserVideos(username.(string), req.VideoIDs)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"deleted_count": len(req.VideoIDs),
	})
}

// GetHotVideos 获取热门视频列表（按点赞数排序）
// 请求：POST /api/videos/hot
// Header: Authorization: Bearer <token> (可选)
// Body: { "limit": 20 }
// 返回：{ "videos": [...] }
func GetHotVideos(c *gin.Context) {
	// 绑定请求参数
	var req struct {
		Limit int `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Limit = 20 // 默认20个
	}

	// 限制最大数量
	if req.Limit <= 0 || req.Limit > 100 {
		req.Limit = 20
	}

	// 尝试获取当前用户（可选）
	username, _ := c.Get("username")
	usernameStr := ""
	if username != nil {
		usernameStr = username.(string)
	}

	// 调用服务层获取热门视频
	resp, err := videoService.GetHotVideos(req.Limit, usernameStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, resp)
}
