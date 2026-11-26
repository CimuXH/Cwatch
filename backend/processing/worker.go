package processing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"backend/config"
	"backend/models"
	"backend/utils"
	"gorm.io/gorm"
)

func Run(ctx context.Context, parallelism int) {
	if parallelism == 0 {
		return
	}
	if url := config.Value("CWATCH_RABBITMQ_URL", ""); url != "" {
		runRabbitMQ(ctx, url, config.Value("CWATCH_RABBITMQ_QUEUE", "cwatch.video-tasks.v1"), parallelism)
		return
	}
	var workers sync.WaitGroup
	for range parallelism {
		workers.Add(1)
		go func() {
			defer workers.Done()
			runWorker(ctx)
		}()
	}
	workers.Wait()
}

func runWorker(ctx context.Context) {
	for ctx.Err() == nil {
		task, err := utils.ClaimTask(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("领取视频任务失败: %v", err)
		}
		if err == nil && task != nil {
			_ = runTask(ctx, *task)
			continue
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func runTask(parent context.Context, task models.VideoTask) error {
	ctx, cancel := context.WithTimeout(parent, 45*time.Minute)
	defer cancel()
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(utils.TaskLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				renewCtx, stop := context.WithTimeout(ctx, 5*time.Second)
				err := utils.RenewTask(renewCtx, task)
				stop()
				if err != nil {
					if !errors.Is(err, utils.ErrLeaseLost) && ctx.Err() == nil {
						log.Printf("任务 %d 续租失败: %v", task.ID, err)
					}
					cancel()
					return
				}
			}
		}
	}()

	var err error
	var permanent bool
	switch task.Kind {
	case models.TaskCleanup:
		if err = utils.CleanupVideoMedia(ctx, task); err == nil {
			err = utils.CompleteCleanup(ctx, task)
		}
	case models.TaskTranscode:
		err, permanent = transcode(ctx, task)
	default:
		err, permanent = fmt.Errorf("未知任务类型 %q", task.Kind), true
	}
	cancel()
	<-heartbeatDone
	if parent.Err() != nil {
		return parent.Err()
	}
	if err == nil || errors.Is(err, utils.ErrLeaseLost) {
		return nil
	}
	log.Printf("视频任务 %d 第 %d 次执行失败: %v", task.ID, task.Attempts, err)
	saveCtx, stop := context.WithTimeout(parent, 5*time.Second)
	defer stop()
	if saveErr := utils.FailTask(saveCtx, task, err, permanent); saveErr != nil && !errors.Is(saveErr, utils.ErrLeaseLost) {
		log.Printf("保存视频任务 %d 失败状态出错: %v", task.ID, saveErr)
		return saveErr
	}
	return nil
}

func transcode(ctx context.Context, task models.VideoTask) (error, bool) {
	video, err := utils.GetVideoByID(ctx, task.VideoID)
	if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && (video.FileName != task.Source || video.Status != models.VideoStatusUploaded) {
		return utils.ErrLeaseLost, false
	}
	if err != nil {
		return err, false
	}
	dir, input, err := utils.PrepareTranscodeFiles(ctx, video, task.Token)
	if err != nil {
		return err, errors.Is(err, os.ErrNotExist)
	}
	if video.Storage == models.StorageMinIO {
		defer os.RemoveAll(dir)
	}

	ffmpeg := config.Value("CWATCH_FFMPEG", "ffmpeg")
	artifacts := []struct {
		name        string
		contentType string
		args        []string
	}{
		{"cover.jpg", "image/jpeg", []string{"-map", "0:v:0", "-frames:v", "1", "-q:v", "2", "-update", "1"}},
		{"720p.mp4", "video/mp4", encodeArgs("720")},
		{"1080p.mp4", "video/mp4", encodeArgs("1080")},
	}
	for _, artifact := range artifacts {
		output := filepath.Join(dir, artifact.name)
		args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-n",
			"-protocol_whitelist", "file", "-format_whitelist", "mov,matroska,webm,avi",
			"-threads", "2", "-i", input}
		args = append(append(args, artifact.args...), output)
		var diagnostics limitedOutput
		cmd := exec.CommandContext(ctx, ffmpeg, args...)
		cmd.Stderr = &diagnostics
		if err := cmd.Run(); err != nil {
			_ = os.RemoveAll(dir)
			message := string(diagnostics)
			invalid := strings.Contains(message, "Invalid data found when processing input") ||
				strings.Contains(message, "matches no streams") || strings.Contains(message, "not on whitelist")
			return fmt.Errorf("%s: %w %s", artifact.name, err, message), invalid
		}
		info, err := os.Stat(output)
		if err != nil || info.Size() == 0 {
			_ = os.RemoveAll(dir)
			return fmt.Errorf("未生成 %s", artifact.name), true
		}
	}
	for _, artifact := range artifacts {
		if err := utils.StoreTranscodeArtifact(ctx, video, task.Token, artifact.name, dir, artifact.contentType); err != nil {
			utils.DiscardTranscodeFiles(video, task.Token, dir)
			return err, false
		}
	}

	err = utils.CompleteTranscode(ctx, task,
		utils.MediaURL(task.VideoID, task.Token+"/cover.jpg"),
		utils.MediaURL(task.VideoID, task.Token+"/720p.mp4"),
		utils.MediaURL(task.VideoID, task.Token+"/1080p.mp4"))
	if errors.Is(err, utils.ErrLeaseLost) {
		utils.DiscardTranscodeFiles(video, task.Token, dir)
	}
	// A connection error during commit can leave a committed result. In that
	// case keep the outputs available until the video is deleted.
	return err, false
}

func encodeArgs(height string) []string {
	return []string{"-map", "0:v:0", "-map", "0:a:0?", "-vf", "scale=-2:" + height,
		"-c:v", "libx264", "-threads", "2", "-preset", "medium", "-crf", "23",
		"-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart"}
}

type limitedOutput []byte

func (output *limitedOutput) Write(p []byte) (int, error) {
	const limit = 2048
	if remaining := limit - len(*output); remaining > 0 {
		*output = append(*output, p[:min(len(p), remaining)]...)
	}
	return len(p), nil
}
