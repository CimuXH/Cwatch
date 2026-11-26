package models

import "time"

const (
	TaskTranscode = "transcode"
	TaskCleanup   = "cleanup"
	TaskPending   = "pending"
	TaskRunning   = "running"
	TaskDone      = "done"
	TaskFailed    = "failed"
	TaskCanceled  = "canceled"
)

type VideoTask struct {
	ID          uint64
	VideoID     uint   `gorm:"not null;uniqueIndex:ux_video_task;index"`
	Source      string `gorm:"size:128;not null;uniqueIndex:ux_video_task"`
	Kind        string `gorm:"size:16;not null;uniqueIndex:ux_video_task"`
	State       string `gorm:"size:16;not null;index:ix_video_task_ready,priority:1;index:ix_video_task_lease,priority:1"`
	Attempts    int
	RunAt       time.Time  `gorm:"not null;index:ix_video_task_ready,priority:2"`
	LeaseUntil  *time.Time `gorm:"index:ix_video_task_lease,priority:2"`
	PublishedAt *time.Time
	Token       string `gorm:"size:36"`
	LastError   string `gorm:"size:1024"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Session struct {
	TokenHash string    `gorm:"size:64;primaryKey"`
	UserID    uint      `gorm:"not null"`
	ExpiresAt time.Time `gorm:"not null;index"`
}
