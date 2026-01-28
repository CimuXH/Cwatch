package utils

import (
	"backend/models"
	"backend/config"
	"fmt"
	"log"
	"errors"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 数据库实例（私有）
var db *gorm.DB

// InitMySQL 初始化 MySQL 连接
func InitMySQL() error {
	// 配置MySQL连接字符串
	// 格式：用户名:密码@tcp(主机:端口)/数据库名?参数
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.MySQLUsername,
		config.MySQLPassword,
		config.MySQLHost,
		config.MySQLPort,
		config.MySQLDatabase)

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

// ============================================ 用户相关数据库操作 =====================================================

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

// ============================================= 视频相关数据库操作 ============================================
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
	URL         string `json:"url"`          // 原视频URL
	URL720p     string `json:"url_720p"`     // 720p视频URL
	URL1080p    string `json:"url_1080p"`    // 1080p视频URL
	CoverURL    string `json:"cover_url"`
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	CreatedAt   string `json:"created_at"`
	Likes       int64  `json:"likes"`
	Comments    int64  `json:"comments"`
	IsLiked     bool   `json:"is_liked"` // 当前用户是否点赞（需要登录）
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
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&videos).Error; err != nil {
		return nil, 0, err
	}

	// 组装返回数据
	result := make([]VideoListItem, 0, len(videos))
	for _, v := range videos {

		// 直接使用 Video 模型中的 LikeCount 字段
		likeCount := int64(v.LikeCount)

		// 获取评论数
		var commentCount int64
		db.Model(&models.Comment{}).Where("video_id = ?", v.ID).Count(&commentCount)

		result = append(result, VideoListItem{
			ID:          v.ID,
			Title:       v.Title,
			Description: v.Description,
			URL:         v.URL,      // 原视频URL
			URL720p:     v.URL720p,  // 720p视频URL
			URL1080p:    v.URL1080p, // 1080p视频URL
			CoverURL:    v.CoverURL,
			UserID:      v.UserID,
			Username:    v.User.Username,
			AvatarURL:   v.User.AvatarURL,
			CreatedAt:   v.CreatedAt.Format("2006-01-02 15:04"),
			Likes:       likeCount,
			Comments:    commentCount,
		})
	}

	return result, total, nil
}


// GetUserVideoList 获取 某个用户 的视频列表（分页）
// page: 页码（从1开始）
// pageSize: 每页数量
func GetUserVideoList(user_id uint, page, pageSize int) ([]VideoListItem, int64, error){
	var total int64
	var videos []models.Video
	
	// 只查询该用户已上传完成的视频
	query := db.Model(&models.Video{}).Where("user_id = ? and status = ?",user_id, models.VideoStatusUploaded)

	// 获取该用户的视频总是
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询，按照时间倒叙
	offset := (page - 1)*pageSize
	if err := query.Preload("User").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&videos).Error; err != nil {
		return nil, 0, err
	}

	// 组装返回数据
	result := make([]VideoListItem, 0, len(videos))
	for _, v := range videos {

		// 直接使用 Video 模型中的 LikeCount 字段
		likeCount := int64(v.LikeCount)

		// 获取评论数
		var commentCount int64
		db.Model(&models.Comment{}).Where("video_id = ?", v.ID).Count(&commentCount)

		result = append(result, VideoListItem{
			ID:          v.ID,
			Title:       v.Title,
			Description: v.Description,
			URL:         v.URL,      // 原视频URL
			URL720p:     v.URL720p,  // 720p视频URL
			URL1080p:    v.URL1080p, // 1080p视频URL
			CoverURL:    v.CoverURL,
			UserID:      v.UserID,
			Username:    v.User.Username,
			AvatarURL:   v.User.AvatarURL,
			CreatedAt:   v.CreatedAt.Format("2006-01-02 15:04"),
			Likes:       likeCount,
			Comments:    commentCount,
		})
	}

	return result, total, nil
	
}

// DeleteUserVideos 删除 某个用户 的 视频列表
// user_id, videoids : 用户id，视频id列表
func DeleteUserVideos(user_id uint, videoids []uint) error {
    result := db.Where("user_id = ? AND id IN ?", user_id, videoids).Delete(&models.Video{})
    
    // 1. 检查系统错误
    if result.Error != nil {
        log.Printf("删除视频失败: %v", result.Error)
        return errors.New("数据库操作失败")
    }
    
    // 2. 检查是否真的删除了记录
    if result.RowsAffected == 0 {
        return errors.New("视频不存在或无权删除")
    }
    
    return nil
}


// ====================================== 点赞相关数据库操作 ===============================================

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



// MGetVideoLikeCount 获取视频的点赞数（M的意思是操作MySQL数据库，其实可以重构utils中的服务操作函数，为每个服务封装一个操作结构体）
func MGetVideoLikeCount(videoID uint) int64 {	
	var count int64
	err := db.Model(&models.Like{}).Where("video_id = ?", videoID).Count(&count).Error
	
	if err != nil {
		log.Printf("统计点赞数失败: %v", err)
		return 0
	}
	
	return count
}

// GetUserLikedVideoIDs 获取用户点赞的所有视频ID列表
// 参数：用户ID
// 返回：视频ID列表
func GetUserLikedVideoIDs(userID uint) ([]uint, error) {
	var likes []models.Like
	err := db.Where("user_id = ?", userID).Find(&likes).Error
	if err != nil {
		log.Printf("查询用户点赞列表失败: %v", err)
		return nil, err
	}
	
	// 提取视频ID
	videoIDs := make([]uint, 0, len(likes))
	for _, like := range likes {
		videoIDs = append(videoIDs, like.VideoID)
	}
	
	return videoIDs, nil
}

// ===================================     ======= 评论相关数据库操作 ==================================================
// CreateComment 添加评论信息
func CreateComment(comment *models.Comment) error {
	return db.Create(comment).Error
}


// 检查是否重新评论内容
func CheckCommentIsRepetition(comment *models.Comment) (bool, error) {
	// 获取评论内容，用户id，视频id
	content := comment.Content
	userid := comment.UserID
	videoid := comment.VideoID

	// 检查该用户的该视频评论是否重复
	// 可以使用Count进行优化
	var c models.Comment
	err := db.Where("user_id = ? and video_id = ? and content = ?", userid, videoid, content).First(&c).Error

	
	// return err == nil
	// 逻辑上优化一下，（存在评论存在但是数据库的连接出现错误的情况）
	if err != nil {
		// 如果错误是未找记录，返回
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	
	return true, nil
}

// 获取某个视频的评论数
func GetVideoCommentCount(videoid uint) int64 {
	var count int64
	err := db.Model(&models.Comment{}).Where("video_id = ?", videoid).Count(&count).Error

	if err != nil {
		log.Printf("统计评论数失败：%v", err)
		return 0
	}
	
	return count
}

// 某个视频的评论列表
type CommentListItem struct {
	ID          uint   `json:"id"`
	Content		string `json:"content"`
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	VideoID		uint   `json:"video_id"`
	CreatedAt   string `json:"created_at"`
}

// 获取某个视频的所有评论信息
func GetVideoComments(videoid uint) ([]CommentListItem , error){
	var comments []models.Comment

	// 查找所有该视频的评论信息
	if err := db.Preload("User").Where("video_id = ?", videoid).Find(&comments).Error; err != nil {
		log.Printf("查找评论信息错误：%v", err)
		return nil, err
	}

	result := make([]CommentListItem, 0, len(comments))
	for _, v := range comments{
		result = append(result, CommentListItem{
			ID: 		v.ID,
			Content:	v.Content,
			UserID:		v.UserID,
			Username:	v.User.Username,
			AvatarURL:	v.User.AvatarURL,
			VideoID:	v.VideoID,
			CreatedAt:	v.CreatedAt.Format("2006-01-02 15:04"),
		})
	}

	return result, nil
}

// GetCommentByID 根据评论ID获取评论信息
func GetCommentByID(commentid uint) (*models.Comment, error) {
	var comment models.Comment
	err := db.Where("id = ?", commentid).First(&comment).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// 删除某个视频某用户的评论
func DeleteComment(commentid uint) error {
	result := db.Where("id = ?", commentid).Delete(&models.Comment{})
	if result.Error != nil {
		log.Printf("删除评论错误：%v", result.Error)
		return result.Error
	}
	
	return nil
}

// GetVideosByIDs 根据视频ID列表批量查询视频（保持顺序）
// 参数：视频ID列表
// 返回：视频列表
func GetVideosByIDs(videoIDs []uint) ([]VideoListItem, error) {
	if len(videoIDs) == 0 {
		return []VideoListItem{}, nil
	}

	var videos []models.Video
	
	// 批量查询视频
	err := db.Where("id IN ? AND status = ?", videoIDs, models.VideoStatusUploaded).
		Preload("User").
		Find(&videos).Error
	if err != nil {
		return nil, err
	}

	// 创建ID到视频的映射
	videoMap := make(map[uint]models.Video)
	for _, v := range videos {
		videoMap[v.ID] = v
	}

	// 按照传入的ID顺序组装结果
	result := make([]VideoListItem, 0, len(videoIDs))
	for _, id := range videoIDs {
		v, exists := videoMap[id]
		if !exists {
			continue // 视频不存在或未上传完成，跳过
		}

		// 直接使用 Video 模型中的 LikeCount 字段
		likeCount := int64(v.LikeCount)

		// 获取评论数
		var commentCount int64
		db.Model(&models.Comment{}).Where("video_id = ?", v.ID).Count(&commentCount)

		result = append(result, VideoListItem{
			ID:          v.ID,
			Title:       v.Title,
			Description: v.Description,
			URL:         v.URL,
			URL720p:     v.URL720p,
			URL1080p:    v.URL1080p,
			CoverURL:    v.CoverURL,
			UserID:      v.UserID,
			Username:    v.User.Username,
			AvatarURL:   v.User.AvatarURL,
			CreatedAt:   v.CreatedAt.Format("2006-01-02 15:04"),
			Likes:       likeCount,
			Comments:    commentCount,
		})
	}

	return result, nil
}
