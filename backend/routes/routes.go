package routes

import (
	"backend/controllers"
	"backend/middlewares"
	"github.com/gin-gonic/gin"
)

// SetupRoutes 设置所有路由
func SetupRoutes(router *gin.Engine) {
	// API路由组
	api := router.Group("/api")

	// 公开路由（不需要认证）
	{
		api.POST("/register", controllers.Register)      // 注册
		api.POST("/login", controllers.Login)            // 登录
		api.GET("/videos", controllers.GetVideoList)     // 获取视频列表（公开）
		api.GET("/video/:videoid/comments", controllers.GetComments) // 获取视频评论（公开）
	}

	// 需要认证的路由
	// 使用AuthMiddleware中间件保护这些路由
	protected := api.Group("")
	protected.Use(middlewares.AuthMiddleware())
	{
		protected.GET("/user/info", controllers.GetUserInfo) // 获取用户信息
		protected.POST("/logout", controllers.Logout)        // 登出

		// 视频相关路由
		protected.POST("/video/upload-url", controllers.GetUploadURL)      // 获取上传URL
		protected.POST("/video/upload-complete", controllers.ConfirmUpload) // 确认上传完成
		 
		// 点赞相关路由
		protected.POST("/video/like", controllers.AddLike)           // 点赞视频
		protected.DELETE("/video/like", controllers.RemoveLike)      // 取消点赞
		protected.POST("/video/toggle-like", controllers.ToggleLike) // 切换点赞状态（推荐）

		// 评论相关路由
		protected.POST("/video/comment/:videoid", controllers.AddComment) // 添加评论
		protected.DELETE("/video/comment/:commentid", controllers.DeleteComment) // 删除评论
	}
}
