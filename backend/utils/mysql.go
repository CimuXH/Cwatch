package utils

import (
	"context"
	"log"
	"os"
	"time"

	"backend/config"
	"backend/models"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

func InitMySQL() error {
	conn, err := gorm.Open(mysql.Open(config.MySQLDSN()), &gorm.Config{
		Logger: logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
			SlowThreshold:             500 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			ParameterizedQueries:      true,
		}),
	})
	if err != nil {
		return err
	}
	pool, err := conn.DB()
	if err != nil {
		return err
	}
	pool.SetMaxOpenConns(24)
	pool.SetMaxIdleConns(8)
	pool.SetConnMaxLifetime(5 * time.Minute)
	db = conn
	return nil
}

func GetUserByUsername(username string) (*models.User, error) {
	var user models.User
	err := db.Where("username = ?", username).First(&user).Error
	return &user, err
}

func CreateUser(user *models.User) error {
	return db.Create(user).Error
}

func CreateVideo(video *models.Video) error {
	return db.Create(video).Error
}

func GetVideoByID(ctx context.Context, id uint) (*models.Video, error) {
	var video models.Video
	err := db.WithContext(ctx).First(&video, id).Error
	return &video, err
}

type VideoListItem struct {
	ID          uint   `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	URL720p     string `json:"url_720p"`
	URL1080p    string `json:"url_1080p"`
	CoverURL    string `json:"cover_url"`
	UserID      uint   `json:"user_id"`
	Username    string `json:"username"`
	AvatarURL   string `json:"avatar_url"`
	CreatedAt   string `json:"created_at"`
	Likes       int64  `json:"likes"`
	Comments    int64  `json:"comments"`
	IsLiked     bool   `json:"is_liked"`
}

func visibleVideos(ctx context.Context) *gorm.DB {
	return db.WithContext(ctx).Model(&models.Video{}).Where("status = ?", models.VideoStatusUploaded)
}

func GetVideoList(ctx context.Context, page, pageSize int, userID uint) ([]VideoListItem, int64, error) {
	query := visibleVideos(ctx)
	if userID != 0 {
		query = query.Where("user_id = ?", userID)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 || int64(page-1) > (total-1)/int64(pageSize) {
		return []VideoListItem{}, total, nil
	}
	items, err := loadVideoItems(query.Order("created_at DESC, id DESC").Offset((page - 1) * pageSize).Limit(pageSize))
	return items, total, err
}

func GetHotVideos(ctx context.Context, limit int) ([]VideoListItem, error) {
	return loadVideoItems(hotVideosQuery(ctx, limit))
}

func hotVideosQuery(ctx context.Context, limit int) *gorm.DB {
	return visibleVideos(ctx).Where("like_count > 0").Order("like_count DESC, id DESC").Limit(limit)
}

func getVideoLikeCounts(ctx context.Context, ids []uint) (map[uint]int64, error) {
	var rows []struct {
		ID        uint
		LikeCount int64
	}
	err := visibleVideos(ctx).Select("id", "like_count").Where("id IN ?", ids).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	counts := make(map[uint]int64, len(rows))
	for _, row := range rows {
		counts[row.ID] = row.LikeCount
	}
	return counts, nil
}

func loadVideoItems(query *gorm.DB) ([]VideoListItem, error) {
	var videos []models.Video
	if err := query.Preload("User").Find(&videos).Error; err != nil {
		return nil, err
	}
	items := make([]VideoListItem, 0, len(videos))
	if len(videos) == 0 {
		return items, nil
	}
	ids := make([]uint, 0, len(videos))
	for _, video := range videos {
		ids = append(ids, video.ID)
	}
	var counts []struct {
		VideoID uint
		Total   int64
	}
	if err := db.WithContext(query.Statement.Context).Model(&models.Comment{}).
		Select("video_id, COUNT(*) AS total").Where("video_id IN ?", ids).Group("video_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	commentCounts := make(map[uint]int64, len(counts))
	for _, count := range counts {
		commentCounts[count.VideoID] = count.Total
	}
	for _, v := range videos {
		items = append(items, VideoListItem{
			ID: v.ID, Title: v.Title, Description: v.Description, URL: v.URL,
			URL720p: v.URL720p, URL1080p: v.URL1080p, CoverURL: v.CoverURL,
			UserID: v.UserID, Username: v.User.Username, AvatarURL: v.User.AvatarURL,
			CreatedAt: v.CreatedAt.Format("2006-01-02 15:04"),
			Likes:     int64(v.LikeCount), Comments: commentCounts[v.ID],
		})
	}
	return items, nil
}

func FillLiked(ctx context.Context, username string, items []VideoListItem) error {
	if username == "" || len(items) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	var liked []uint
	if err := db.WithContext(ctx).Model(&models.Like{}).
		Where("user_id = (SELECT id FROM users WHERE username = ? AND deleted_at IS NULL) AND video_id IN ?", username, ids).
		Pluck("video_id", &liked).Error; err != nil {
		return err
	}
	set := make(map[uint]bool, len(liked))
	for _, id := range liked {
		set[id] = true
	}
	for i := range items {
		items[i].IsLiked = set[items[i].ID]
	}
	return nil
}

type CommentListItem struct {
	ID        uint   `json:"id"`
	Content   string `json:"content"`
	UserID    uint   `json:"user_id"`
	Username  string `json:"username"`
	AvatarURL string `json:"avatar_url"`
	VideoID   uint   `json:"video_id"`
	CreatedAt string `json:"created_at"`
}

func GetVideoComments(ctx context.Context, videoID uint) ([]CommentListItem, error) {
	var comments []models.Comment
	if err := db.WithContext(ctx).Preload("User").Where("video_id = ?", videoID).Order("id").Find(&comments).Error; err != nil {
		return nil, err
	}
	items := make([]CommentListItem, 0, len(comments))
	for _, c := range comments {
		items = append(items, CommentListItem{
			ID: c.ID, Content: c.Content, UserID: c.UserID, Username: c.User.Username,
			AvatarURL: c.User.AvatarURL, VideoID: c.VideoID, CreatedAt: c.CreatedAt.Format("2006-01-02 15:04"),
		})
	}
	return items, nil
}
