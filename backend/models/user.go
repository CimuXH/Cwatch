package models

import (
	"backend/config"
	"fmt"
	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	gorm.Model                                                                                                           // 包含ID、CreatedAt、UpdatedAt、DeletedAt字段
	Username  string `gorm:"unique;not null" json:"username"`                                                            // 用户名，唯一且不能为空
	Password  string `gorm:"not null" json:"-"`                                                                          // 密码，不能为空，json序列化时忽略
	AvatarURL string `gorm:"default:'http://101.132.25.34:9000/cwatch/c.png'" json:"avatar_url"` // 头像URL
	Email     string `json:"email"`
	Phone     string `json:"phone"`
}

// GetDefaultAvatarURL 获取默认头像URL
func GetDefaultAvatarURL() string {
	return fmt.Sprintf("http://%s:%s/cwatch/c.png", config.MinIOHost, config.MinIOPort)
}

// 视频状态常量
const (
	VideoStatusUploading  = 0 // 上传中
	VideoStatusUploaded   = 1 // 上传完成
	VideoStatusProcessing = 2 // 转码中
	VideoStatusPublished  = 3 // 已发布
	VideoStatusFailed     = 4 // 审核失败
)

// Video 视频模型
type Video struct {
	gorm.Model
	Title       string `gorm:"not null" json:"title"`              // 视频标题
	Description string `json:"description"`                        // 视频描述
	URL         string `json:"url"`                                // 视频文件URL（原视频）
	URL720p     string `json:"url_720p"`                           // 720p视频URL
	URL1080p    string `json:"url_1080p"`                          // 1080p视频URL
	CoverURL    string `json:"cover_url"`                          // 视频封面图URL
	UserID      uint   `json:"user_id"`                            // 视频所属用户ID
	User        User   `gorm:"foreignKey:UserID" json:"-"`         // 与User模型建立关联
	Status      int    `gorm:"default:0" json:"status"`            // 视频状态
	FileName    string `json:"file_name"`                          // 存储的文件名（UUID生成）
	LikeCount 	uint   `json:"like_count" gorm:"index"`			   // 视频的点赞量，添加普通索引
}		

// Comment 评论模型
type Comment struct {
	gorm.Model
	Content string `gorm:"not null"` // 评论内容
	UserID  uint   // 评论用户ID
	User    User   `gorm:"foreignKey:UserID"` // 与User模型建立关联
	VideoID uint   // 评论的视频ID
	Video   Video  `gorm:"foreignKey:VideoID"` // 与Video模型建立关联
}

// Like 点赞模型
type Like struct {
	gorm.Model
	UserID  uint  // 点赞用户ID
	User    User  `gorm:"foreignKey:UserID"` // 与User模型建立关联
	VideoID uint  // 被点赞的视频ID
	Video   Video `gorm:"foreignKey:VideoID"` // 与Video模型建立关联
}