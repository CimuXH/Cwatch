package utils

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"backend/models"
	"github.com/google/uuid"
)

func TestConcurrentWritesAndTaskLease(t *testing.T) {
	dsn := os.Getenv("CWATCH_TEST_DSN")
	if dsn == "" {
		t.Skip("set CWATCH_TEST_DSN to an empty MySQL test database")
	}
	t.Setenv("CWATCH_DSN", dsn)
	t.Setenv("CWATCH_MEDIA_DIR", t.TempDir())
	if err := InitMySQL(); err != nil {
		t.Fatal(err)
	}
	tables, err := db.Migrator().GetTables()
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 0 {
		t.Fatal("the test database must be empty")
	}
	if err := Migrate(); err != nil {
		t.Fatal(err)
	}
	if err := InitMedia(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	user := models.User{Username: "tester", Password: "unused"}
	if err := CreateUser(&user); err != nil {
		t.Fatal(err)
	}

	upload := func() models.Video {
		t.Helper()
		video := models.Video{UserID: user.ID, Title: "test", FileName: uuid.NewString() + ".mp4", SourceSize: 4}
		if err := CreateVideo(&video); err != nil {
			t.Fatal(err)
		}
		if err := StoreUpload(ctx, user.ID, video.ID, strings.NewReader("test")); err != nil {
			t.Fatal(err)
		}
		return video
	}

	video := upload()
	for err := range concurrently(8, func() error {
		_, err := ConfirmUpload(ctx, user.ID, video.ID)
		return err
	}) {
		if err != nil {
			t.Fatal(err)
		}
	}
	var tasks int64
	if err := db.Model(&models.VideoTask{}).Where("video_id = ?", video.ID).Count(&tasks).Error; err != nil || tasks != 1 {
		t.Fatalf("expected one transcode task, got %d (%v)", tasks, err)
	}
	pending, err := TasksToPublish(ctx, 32)
	if err != nil || len(pending) != 1 || pending[0].VideoID != video.ID {
		t.Fatalf("expected one committed task ready for RabbitMQ, got %+v (%v)", pending, err)
	}
	if err := MarkTaskPublished(ctx, pending[0]); err != nil {
		t.Fatal(err)
	}
	if awaiting, err := TasksToPublish(ctx, 32); err != nil || len(awaiting) != 0 {
		t.Fatalf("confirmed tasks should not be immediately published again: %+v (%v)", awaiting, err)
	}

	conflict := upload()
	if err := db.Create(&models.VideoTask{VideoID: conflict.ID, Source: conflict.FileName, Kind: models.TaskTranscode, State: models.TaskCanceled, RunAt: time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ConfirmUpload(ctx, user.ID, conflict.ID); !IsDuplicate(err) {
		t.Fatalf("expected task creation to fail, got %v", err)
	}
	reloaded, err := GetVideoByID(ctx, conflict.ID)
	if err != nil || reloaded.Status != models.VideoStatusUploading || reloaded.URL != "" {
		t.Fatalf("upload confirmation did not roll back: %#v, %v", reloaded, err)
	}

	for _, desired := range []bool{true, false, true} {
		for err := range concurrently(8, func() error {
			_, _, err := SetLike(ctx, user.ID, video.ID, &desired)
			return err
		}) {
			if err != nil {
				t.Fatal(err)
			}
		}
		reloaded, err := GetVideoByID(ctx, video.ID)
		expected := uint(0)
		if desired {
			expected = 1
		}
		if err != nil || reloaded.LikeCount != expected {
			t.Fatalf("expected %d likes, got %#v (%v)", expected, reloaded, err)
		}
	}

	var created int
	for err := range concurrently(8, func() error {
		_, err := CreateComment(ctx, &models.Comment{UserID: user.ID, VideoID: video.ID, Content: "same content"})
		return err
	}) {
		if err == nil {
			created++
		} else if !errors.Is(err, ErrDuplicateComment) {
			t.Fatal(err)
		}
	}
	if created != 1 {
		t.Fatalf("expected one comment, got %d", created)
	}

	first, err := ClaimTaskByID(ctx, pending[0].ID)
	if err != nil || first == nil {
		t.Fatalf("claim task: %v", err)
	}
	if other, err := ClaimTaskByID(ctx, first.ID); err != nil || other != nil {
		t.Fatalf("claimed an active lease: %#v, %v", other, err)
	}
	if err := db.Model(first).Update("lease_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	redelivery, err := TasksToPublish(ctx, 32)
	if err != nil || len(redelivery) != 1 || redelivery[0].ID != first.ID {
		t.Fatalf("expired lease should be redispatched: %+v (%v)", redelivery, err)
	}
	second, err := ClaimTaskByID(ctx, first.ID)
	if err != nil || second == nil || second.ID != first.ID || second.Token == first.Token || second.Attempts != 2 {
		t.Fatalf("reclaim task: %#v, %v", second, err)
	}
	if err := CompleteTranscode(ctx, *first, "/stale.jpg", "", ""); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker should fail: %v", err)
	}
	if deleted, err := DeleteUserVideos(ctx, user.ID, []uint{video.ID, video.ID}); err != nil || deleted != 1 {
		t.Fatalf("delete videos: count=%d error=%v", deleted, err)
	}
	if err := CompleteTranscode(ctx, *second, "/deleted.jpg", "", ""); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("deleted video should reject worker completion: %v", err)
	}
	cleanup, err := ClaimTask(ctx)
	if err != nil || cleanup == nil || cleanup.Kind != models.TaskCleanup || cleanup.VideoID != video.ID {
		t.Fatalf("claim cleanup task: %#v, %v", cleanup, err)
	}
}

func concurrently(n int, work func() error) <-chan error {
	result := make(chan error, n)
	var wait sync.WaitGroup
	for range n {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result <- work()
		}()
	}
	wait.Wait()
	close(result)
	return result
}
