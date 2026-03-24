package controllers

import (
	"backend/services"
	"net/http"

	"github.com/gin-gonic/gin"
)

var randomFeedService = services.RandomFeedService{}

type RandomFeedNextRequest struct {
	Init bool `json:"init"`
	Page int  `json:"page"`
}

// RandomFeedNext 获取随机 Feed 下一批（MVP：每次返回 3 条）
// 认证：必须登录（AuthMiddleware）
// 请求体：
//   { "init": true }                       // 初始化：随机起点
//   { "init": false, "page": 2 }         // 后续：从 page 开始扫描
// 响应：
//   { "videos": [...], "next_page": 3, "has_more": true }
func RandomFeedNext(c *gin.Context) {
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未授权"})
		return
	}
	usernameStr := username.(string)

	var req RandomFeedNextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// 简化：body 解析失败则按 init=false&page=1 处理
		req.Init = false
		req.Page = 1
	}

	resp, err := randomFeedService.NextRandomFeed(usernameStr, req.Page, req.Init, 3)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

