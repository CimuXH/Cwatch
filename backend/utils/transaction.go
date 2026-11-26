package utils

import (
	"context"
	"errors"
	"time"

	"backend/models"
	"database/sql"
	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func IsDuplicate(err error) bool {
	var sqlErr *mysql.MySQLError
	return errors.As(err, &sqlErr) && sqlErr.Number == 1062
}

func transaction(ctx context.Context, operation func(*gorm.DB) error) error {
	for attempt := 0; ; attempt++ {
		err := db.WithContext(ctx).Transaction(operation, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
		var sqlErr *mysql.MySQLError
		if attempt == 2 || !errors.As(err, &sqlErr) || (sqlErr.Number != 1205 && sqlErr.Number != 1213) {
			return err
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func lockVideo(tx *gorm.DB, id uint, strength string) (models.Video, error) {
	var video models.Video
	err := tx.Clauses(clause.Locking{Strength: strength}).First(&video, id).Error
	return video, err
}

func Migrate() error {
	if err := db.AutoMigrate(&models.User{}, &models.Video{}, &models.Comment{}, &models.VideoTask{}, &models.Session{}); err != nil {
		return err
	}
	if db.Migrator().HasTable(&models.Like{}) && !db.Migrator().HasIndex(&models.Like{}, "ux_likes_user_video") {
		if err := transaction(context.Background(), func(tx *gorm.DB) error {
			// Preserve an active relation ahead of any soft-deleted copies.
			if err := tx.Exec(`DELETE obsolete FROM likes AS obsolete JOIN likes AS retained
				ON retained.user_id = obsolete.user_id AND retained.video_id = obsolete.video_id
				AND ((retained.deleted_at IS NULL AND obsolete.deleted_at IS NOT NULL)
					OR ((retained.deleted_at IS NULL) = (obsolete.deleted_at IS NULL) AND retained.id < obsolete.id))`).Error; err != nil {
				return err
			}
			return tx.Exec(`UPDATE videos LEFT JOIN (
				SELECT video_id, COUNT(*) AS total FROM likes WHERE deleted_at IS NULL GROUP BY video_id
			) counts ON counts.video_id = videos.id SET videos.like_count = COALESCE(counts.total, 0)`).Error
		}); err != nil {
			return err
		}
	}
	return db.AutoMigrate(&models.Like{})
}
