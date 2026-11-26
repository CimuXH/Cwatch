package utils

import (
	"context"
	"errors"

	"backend/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrVideoUnavailable   = errors.New("视频不存在或不可操作")
	ErrDuplicateComment   = errors.New("请勿重复发表相同内容的评论")
	ErrCommentUnavailable = errors.New("评论不存在或无权删除")
)

func SetLike(ctx context.Context, userID, videoID uint, desired *bool) (int64, bool, error) {
	var count int64
	var liked bool
	var changed bool
	err := transaction(ctx, func(tx *gorm.DB) error {
		changed = false
		video, err := lockVideo(tx, videoID, "UPDATE")
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && video.Status != models.VideoStatusUploaded {
			return ErrVideoUnavailable
		}
		if err != nil {
			return err
		}
		var like models.Like
		err = tx.Unscoped().Where("user_id = ? AND video_id = ?", userID, videoID).Take(&like).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		active := like.ID != 0 && !like.DeletedAt.Valid
		liked = !active
		if desired != nil {
			liked = *desired
		}
		count = int64(video.LikeCount)
		if liked == active {
			return nil
		}
		changed = true
		switch {
		case like.ID == 0:
			if err := tx.Create(&models.Like{UserID: userID, VideoID: videoID}).Error; err != nil {
				return err
			}
		case liked:
			if err := tx.Unscoped().Model(&like).Update("deleted_at", nil).Error; err != nil {
				return err
			}
		default:
			if err := tx.Delete(&like).Error; err != nil {
				return err
			}
		}
		if liked {
			count++
		} else if count > 0 {
			count--
		}
		return tx.Model(&video).Update("like_count", count).Error
	})
	if err == nil && changed {
		invalidateLikeCount(videoID)
	}
	return count, liked, err
}

func CreateComment(ctx context.Context, comment *models.Comment) (int64, error) {
	var count int64
	err := transaction(ctx, func(tx *gorm.DB) error {
		video, err := lockVideo(tx, comment.VideoID, "SHARE")
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && video.Status != models.VideoStatusUploaded {
			return ErrVideoUnavailable
		}
		if err != nil {
			return err
		}
		// Serialize duplicate checks for this author using the existing user row.
		var author models.User
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&author, comment.UserID).Error; err != nil {
			return err
		}
		var match models.Comment
		err = tx.Select("id").Where("user_id = ? AND video_id = ? AND content = ?", comment.UserID, comment.VideoID, comment.Content).Take(&match).Error
		if err == nil {
			return ErrDuplicateComment
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		record := models.Comment{Content: comment.Content, UserID: comment.UserID, VideoID: comment.VideoID}
		if err := tx.Create(&record).Error; err != nil {
			return err
		}
		return tx.Model(&models.Comment{}).Where("video_id = ?", comment.VideoID).Count(&count).Error
	})
	return count, err
}

func DeleteComment(ctx context.Context, userID, commentID uint) error {
	result := db.WithContext(ctx).Where("id = ? AND user_id = ?", commentID, userID).Delete(&models.Comment{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrCommentUnavailable
	}
	return nil
}
