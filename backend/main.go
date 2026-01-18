package main

import (
	"backend/models"
	"backend/routes"
	"backend/utils"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"log"
)

func main() {
	// 初始化 MySQL 连接
	if err := utils.InitMySQL(); err != nil {
		log.Fatal("MySQL 连接失败:", err)
	}

	// 自动迁移数据库表结构
	// 会根据模型自动创建或更新表
	err := utils.AutoMigrate(
		&models.User{},
		&models.Video{},
		&models.Comment{},
		&models.Like{},
	)
	if err != nil {
		log.Fatal("模型迁移失败:", err)
	}
	log.Println("模型迁移完成")

	// 初始化 Redis 连接
	if err := utils.InitRedis(); err != nil {
		log.Fatal("Redis 连接失败:", err)
	}

	// 初始化 MinIO 连接
	if err := utils.InitMinIO(); err != nil {
		log.Fatal("MinIO 连接失败：", err)
	}

	// 创建Gin路由引擎
	router := gin.Default()

	// 配置CORS跨域中间件
	// 允许前端从不同域名访问后端API
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"}, // 允许所有来源（生产环境应指定具体域名）
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// 设置路由
	routes.SetupRoutes(router)

	router.Run(":5000")

}
