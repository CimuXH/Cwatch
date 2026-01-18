package utils

import (
	"backend/models"
	"log"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 数据库实例（私有）
var db *gorm.DB

// InitMySQL 初始化 MySQL 连接
func InitMySQL() error {
	// 配置MySQL连接字符串
	// 格式：用户名:密码@tcp(主机:端口)/数据库名?参数
	dsn := "root:mysql030303@tcp(101.132.25.34:3306)/cwatch?charset=utf8mb4&parseTime=True&loc=Local"

	// 连接数据库
	conn, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	db = conn
	log.Println("MySQL 数据库连接成功")
	return nil
}

// AutoMigrate 自动迁移数据库表结构
func AutoMigrate(m ...interface{}) error {
	return db.AutoMigrate(m...)
}

// ==================== 用户相关数据库操作 ====================

// GetUserByUsername 根据用户名查询用户
func GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// CreateUser 创建用户
func CreateUser(user *models.User) error {
	return db.Create(user).Error
}

// UserExists 检查用户名是否已存在
func UserExists(username string) bool {
	var user models.User
	err := db.Where("username = ?", username).First(&user).Error
	return err == nil
}

// ==================== 视频相关数据库操作 ====================================

// CreateVideo 创建视频记录
func CreateVideo(video *models.Video) error {
	return db.Create(video).Error
}

// GetVideoByID 根据ID查询视频
func GetVideoByID(id uint) (*models.Video, error) {
	var video models.Video
	err := db.First(&video, id).Error
	if err != nil {
		return nil, err
	}
	return &video, nil
}

// UpdateVideoStatus 更新视频状态
func UpdateVideoStatus(id uint, status int) error {
	return db.Model(&models.Video{}).Where("id = ?", id).Update("status", status).Error
}

// UpdateVideoURL 更新视频URL
func UpdateVideoURL(id uint, url string) error {
	return db.Model(&models.Video{}).Where("id = ?", id).Update("url", url).Error
}

// UpdateVideo 更新视频多个字段
func UpdateVideo(id uint, updates map[string]interface{}) error {
	return db.Model(&models.Video{}).Where("id = ?", id).Updates(updates).Error
}

// VideoListItem 视频列表项（包含作者信息）
type VideoListItem struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	CoverURL    string `json:"cover_url"`
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	CreatedAt   string `json:"created_at"`
	Likes       int64  `json:"likes"`
	Comments    int64  `json:"comments"`
}

// GetVideoList 获取视频列表（分页）
// page: 页码（从1开始）
// pageSize: 每页数量
func GetVideoList(page, pageSize int) ([]VideoListItem, int64, error) {
	var total int64
	var videos []models.Video

	// 只查询已上传完成的视频
	query := db.Model(&models.Video{}).Where("status = ?", models.VideoStatusUploaded)

	// 获取总数
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询，按创建时间倒序
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&videos).Error; err != nil {
		return nil, 0, err
	}

	// 组装返回数据
	result := make([]VideoListItem, 0, len(videos))
	for _, v := range videos {
		// 获取作者信息
		var user models.User
		db.First(&user, v.UserID)

		// 获取点赞数
		var likeCount int64
		db.Model(&models.Like{}).Where("video_id = ?", v.ID).Count(&likeCount)

		// 获取评论数
		var commentCount int64
		db.Model(&models.Comment{}).Where("video_id = ?", v.ID).Count(&commentCount)

		result = append(result, VideoListItem{
			ID:          v.ID,
			Title:       v.Title,
			Description: v.Description,
			URL:         v.URL, // 现在使用永久URL，无需重新生成
			CoverURL:    v.CoverURL,
			UserID:      v.UserID,
			Username:    user.Username,
			AvatarURL:   user.AvatarURL,
			CreatedAt:   v.CreatedAt.Format("2006-01-02 15:04"),
			Likes:       likeCount,
			Comments:    commentCount,
		})
	}

	return result, total, nil
}

// ====================================== 点赞相关数据库操作 ================================

// CreateLike 创建点赞记录
func CreateLike(like *models.Like) error {
	
	result := db.Create(like)
	if result.Error != nil {
		log.Printf("插入点赞记录失败: %v", result.Error)
		return result.Error
	}
	
	return nil
}

// DeleteLike 删除点赞记录（取消点赞）
func DeleteLike(userID, videoID uint) error {
	
	result := db.Where("user_id = ? AND video_id = ?", userID, videoID).Delete(&models.Like{})
	if result.Error != nil {
		log.Printf("删除点赞记录失败: %v", result.Error)
		return result.Error
	}
	
	return nil
}

// GetByUseridAndVideoid 通过userid和videoid查看该信息是否存在
// func GetByUseridAndVideoid(userid uint, videoid uint) bool {
	
// 	var like models.Like
// 	err := db.Model(&models.Like{}).Where("user_id = ? AND video_id = ?", userid, videoid).First(&like).Error
// 	if err != nil {
// 		log.Println(err)
// 	}

// 	exists := err == nil
	
// 	if exists {
// 		log.Printf("找到的点赞记录 - ID: %d, UserID: %d, VideoID: %d", like.ID, like.UserID, like.VideoID)
// 	}
	
// 	return exists
// }
func GetByUseridAndVideoid(userid uint, videoid uint) bool {
    var count int64
    err := db.Model(&models.Like{}).
        Where("user_id = ? AND video_id = ?", userid, videoid).
        Count(&count).Error
    
    if err != nil {
        log.Printf("查询点赞记录计数失败: %v", err)
        return false
    }
    
    return count > 0
}

// GetVideoLikeCount 获取视频的点赞数
func GetVideoLikeCount(videoID uint) int64 {	
	var count int64
	err := db.Model(&models.Like{}).Where("video_id = ?", videoID).Count(&count).Error
	
	if err != nil {
		log.Printf("统计点赞数失败: %v", err)
		return 0
	}
	
	return count
}

