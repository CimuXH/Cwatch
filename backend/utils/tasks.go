package utils

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"time"

	"backend/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TaskLease          = time.Minute
	MaxTaskAttempts    = 4
	taskRepublishDelay = 15 * time.Minute
)

var ErrLeaseLost = errors.New("任务租约已失效")

func ClaimTask(ctx context.Context) (*models.VideoTask, error) {
	return claimTask(ctx, 0)
}

func ClaimTaskByID(ctx context.Context, id uint64) (*models.VideoTask, error) {
	if id == 0 {
		return nil, errors.New("无效的任务 ID")
	}
	return claimTask(ctx, id)
}

func readyTasks(query *gorm.DB, now time.Time) *gorm.DB {
	return query.Where("((state = ? AND run_at <= ?) OR (state = ? AND lease_until <= ?))",
		models.TaskPending, now, models.TaskRunning, now)
}

func claimTask(ctx context.Context, id uint64) (*models.VideoTask, error) {
	var claimed *models.VideoTask
	err := transaction(ctx, func(tx *gorm.DB) error {
		claimed = nil
		now := time.Now().UTC()
		var task models.VideoTask
		query := readyTasks(tx, now)
		if id != 0 {
			query = query.Where("id = ?", id).Clauses(clause.Locking{Strength: "UPDATE"})
		} else {
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"})
		}
		err := query.Order("run_at, id").Take(&task).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if task.Attempts >= MaxTaskAttempts {
			return tx.Model(&task).Updates(map[string]any{
				"state": models.TaskFailed, "token": "", "lease_until": nil, "last_error": "执行租约过期，已达到重试上限",
			}).Error
		}
		until := now.Add(TaskLease)
		task.State, task.Token, task.LeaseUntil = models.TaskRunning, uuid.NewString(), &until
		task.Attempts++
		task.PublishedAt = nil
		if err := tx.Model(&task).Select("State", "Token", "LeaseUntil", "Attempts", "PublishedAt").Updates(&task).Error; err != nil {
			return err
		}
		claimed = &task
		return nil
	})
	return claimed, err
}

func TasksToPublish(ctx context.Context, limit int) ([]models.VideoTask, error) {
	now := time.Now().UTC()
	var tasks []models.VideoTask
	err := readyTasks(db.WithContext(ctx), now).
		Where("(published_at IS NULL OR published_at <= ?)", now.Add(-taskRepublishDelay)).
		Order("run_at, id").Limit(limit).Find(&tasks).Error
	return tasks, err
}

func MarkTaskPublished(ctx context.Context, task models.VideoTask) error {
	return db.WithContext(ctx).Model(&models.VideoTask{}).
		Where("id = ? AND state = ? AND attempts = ?", task.ID, task.State, task.Attempts).
		Update("published_at", time.Now().UTC()).Error
}

func leasedTask(db *gorm.DB, task models.VideoTask) *gorm.DB {
	return db.Model(&models.VideoTask{}).Where("id = ? AND state = ? AND token = ? AND lease_until > ?",
		task.ID, models.TaskRunning, task.Token, time.Now().UTC())
}

func verifyTaskUpdate(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrLeaseLost
	}
	return nil
}

func RenewTask(ctx context.Context, task models.VideoTask) error {
	return verifyTaskUpdate(leasedTask(db.WithContext(ctx), task).Update("lease_until", time.Now().UTC().Add(TaskLease)))
}

func CompleteTranscode(ctx context.Context, task models.VideoTask, cover, hd, fullHD string) error {
	err := transaction(ctx, func(tx *gorm.DB) error {
		video, err := lockVideo(tx, task.VideoID, "UPDATE")
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (video.FileName != task.Source || video.Status != models.VideoStatusUploaded) {
			return ErrLeaseLost
		}
		if err != nil {
			return err
		}
		if err := finishTask(tx, task); err != nil {
			return err
		}
		return tx.Model(&video).Updates(map[string]any{"cover_url": cover, "url720p": hd, "url1080p": fullHD}).Error
	})
	if err == nil {
		InvalidateVideoCache()
	}
	return err
}

func CompleteCleanup(ctx context.Context, task models.VideoTask) error {
	return finishTask(db.WithContext(ctx), task)
}

func finishTask(db *gorm.DB, task models.VideoTask) error {
	return verifyTaskUpdate(leasedTask(db, task).Updates(map[string]any{
		"state": models.TaskDone, "token": "", "lease_until": nil, "last_error": "",
	}))
}

func FailTask(ctx context.Context, task models.VideoTask, reason error, permanent bool) error {
	message := []rune(reason.Error())
	if len(message) > 1024 {
		message = message[:1024]
	}
	state := models.TaskFailed
	delay := time.Duration(0)
	if !permanent && task.Attempts < MaxTaskAttempts {
		state = models.TaskPending
		delay = 5 * time.Second << (task.Attempts - 1)
		delay += rand.N(delay / 4)
	}
	return verifyTaskUpdate(leasedTask(db.WithContext(ctx), task).Updates(map[string]any{
		"state": state, "run_at": time.Now().UTC().Add(delay), "lease_until": nil, "token": "",
		"published_at": nil, "last_error": string(message),
	}))
}

func RetryTranscode(ctx context.Context, userID, videoID uint) error {
	return transaction(ctx, func(tx *gorm.DB) error {
		video, err := lockVideo(tx, videoID, "UPDATE")
		if err != nil || video.UserID != userID || video.Status != models.VideoStatusUploaded {
			return ErrVideoUnavailable
		}
		var task models.VideoTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("video_id = ? AND source = ? AND kind = ?", videoID, video.FileName, models.TaskTranscode).Take(&task).Error; err != nil {
			return err
		}
		if task.State == models.TaskPending || task.State == models.TaskRunning {
			return nil
		}
		if task.State != models.TaskFailed {
			return errors.New("该任务不需要重试")
		}
		return tx.Model(&task).Updates(map[string]any{
			"state": models.TaskPending, "run_at": time.Now().UTC(), "attempts": 0, "published_at": nil, "last_error": "",
		}).Error
	})
}

func DeleteUserVideos(ctx context.Context, userID uint, ids []uint) (int, error) {
	ids = slices.Clone(ids)
	slices.Sort(ids)
	ids = slices.Compact(ids)
	deleted := 0
	err := transaction(ctx, func(tx *gorm.DB) error {
		deleted = 0
		for _, id := range ids {
			video, err := lockVideo(tx, id, "UPDATE")
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if video.UserID != userID {
				continue
			}
			if err := tx.Delete(&video).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.VideoTask{}).
				Where("video_id = ? AND kind = ? AND state IN ?", id, models.TaskTranscode, []string{models.TaskPending, models.TaskRunning, models.TaskFailed}).
				Updates(map[string]any{"state": models.TaskCanceled, "token": "", "lease_until": nil}).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.VideoTask{VideoID: id, Source: video.FileName, Kind: models.TaskCleanup,
				State: models.TaskPending, RunAt: time.Now().UTC()}).Error; err != nil {
				return err
			}
			deleted++
		}
		if deleted == 0 {
			return ErrVideoUnavailable
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	InvalidateVideoCache()
	return deleted, nil
}
