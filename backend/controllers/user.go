package controllers

import (
	"backend/services"
	"backend/utils"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// 创建用户服务实例
var userService = services.UserService{}

// Register 注册API
// 请求：POST /api/register
// Body: {"username": "test", "password": "123456"}
// 返回：注册成功的消息和用户信息
func Register(c *gin.Context) {
	var req services.RegisterRequest

	// 绑定并验证JSON数据
	// ShouldBindJSON会自动验证binding标签中的规则
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的输入: " + err.Error(),
		})
		return
	}

	// 调用服务层注册用户
	user, err := userService.Register(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "注册成功",
		"user":    user,
	})
}

// Login 登录API
// 请求：POST /api/login
// Body: {"username": "test", "password": "123456"}
// 返回：登录成功的消息、用户信息和JWT令牌
func Login(c *gin.Context) {
	var req services.LoginRequest

	// 绑定并验证JSON数据
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "无效的输入: " + err.Error(),
		})
		return
	}

	// 调用服务层登录
	user, err := userService.Login(req)
	if err != nil {
		// 401 Unauthorized 表示认证失败
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 返回成功响应
	c.JSON(http.StatusOK, gin.H{
		"message": "登录成功",
		"user":    user,
		"token":   user.Token,
	})

	// 将 token 保存到 Redis
	_ = utils.SaveToken(user.Username, user.Token)
}

// GetUserInfo 获取用户信息API（需要JWT认证）
// 请求：GET /api/user/info
// Header: Authorization: Bearer <token>
// 返回：用户信息
func GetUserInfo(c *gin.Context) {
	// 从上下文中获取用户名
	// 这个值是由JWT中间件在验证令牌后设置的
	username, exists := c.Get("username")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 获取用户信息
	user, err := userService.GetUserByUsername(username.(string))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 返回用户信息
	c.JSON(http.StatusOK, gin.H{
		"user": user,
	})
}

// Logout 登出API（需要JWT认证）
// 请求：POST /api/logout
// Header: Authorization: Bearer <token>
// 返回：登出成功的消息
func Logout(c *gin.Context) {
	// 从上下文中获取 token
	token, exists := c.Get("token")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "未授权",
		})
		return
	}

	// 从上下文中获取用户名
	username, _ := c.Get("username")

	// 将 token 加入黑名单（设置过期时间为 24 小时）
	err := utils.AddToBlacklist(token.(string), 24*time.Hour)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "登出失败",
		})
		return
	}

	// 删除 Redis 中保存的用户 token
	_ = utils.DeleteUserToken(username.(string))

	c.JSON(http.StatusOK, gin.H{
		"message": "登出成功",
	})
}
