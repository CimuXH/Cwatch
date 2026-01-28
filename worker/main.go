package main

// RabbitMQ 消费者

import (
	"worker/config"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 配置常量
const (
	// RabbitMQ 配置
	QueueVideoName = "video_processing"
	QueueVideoLikeName = "video_like_processing"   // 新增：点赞处理队列

	// MinIO 配置
	MinioBucket = "cwatch"

	// FFmpeg 配置（Windows路径）
	FFmpegPath = "E:/soft/ffmpeg-8.0.1-essentials_build/bin/ffmpeg.exe"
)

// VideoTask 视频处理任务结构
type VideoTask struct {
	VideoID  uint   `json:"video_id"`
	FileName string `json:"file_name"`
}

// LikeTask  点赞处理任务结构
type LikeTask struct {
	VideoID uint  `json:"video_id"`
	UserID  uint  `json:"user_id"`
	Delta   int   `json:"delta"` // +1 点赞；-1 取消
	TS      int64 `json:"ts"`	// TS: time.Now().Unix() 时间戳
}

// 全局变量
var (
	db          *gorm.DB
	minioClient *minio.Client
)

func main() {
	log.Println("视频处理 Worker 启动中...")

	// 初始化数据库连接
	if err := initMySQL(); err != nil {
		log.Fatal("MySQL 连接失败:", err)
	}
	log.Println("MySQL 连接成功")

	// 初始化 MinIO 客户端
	if err := initMinIO(); err != nil {
		log.Fatal("MinIO 连接失败:", err)
	}
	log.Println("MinIO 连接成功")

	// 连接到 RabbitMQ
	rabbitMQURL := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		config.RabbitMQUsername,
		config.RabbitMQPassword,
		config.RabbitMQHost,
		config.RabbitMQPort)
	
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		log.Fatal("RabbitMQ 连接失败:", err)
	}
	defer conn.Close()
	log.Println("RabbitMQ 连接成功")

	// =====================================连接工作准备完成=====================================

	// 持续监听消息，阻塞主程序，持续处理任务
	forever := make(chan bool)
	
	go startVideoConsumer(conn)	 // 处理视频消费者

	go startLikeConsumer(conn)  // 处理点赞消费者

	<-forever
}

// startVideoConsumer	=============视频处理的消费者==============	
func startVideoConsumer(conn *amqp.Connection) {
    log.Println("视频 Consumer 启动中...")

    workerCount := 3

    for i := 0; i < workerCount; i++ {
        go func(workerID int) {
            ch, err := conn.Channel()
            if err != nil {
                log.Printf("Worker %d 创建通道失败: %v", workerID, err)
                return
            }
            defer ch.Close()

            // 声明队列（建议所有 worker 都声明一遍，幂等）
            q, err := ch.QueueDeclare(
                QueueVideoName,
                true,
                false,
                false,
                false,
                nil,
            )
            if err != nil {
                log.Printf("Worker %d 声明队列失败: %v", workerID, err)
                return
            }

            // 每个 worker 一次只拿一条未确认消息
            if err := ch.Qos(1, 0, false); err != nil {
                log.Printf("Worker %d 设置 QoS 失败: %v", workerID, err)
                return
            }

            msgs, err := ch.Consume(
                q.Name,
                "",    // consumer tag
                false, // autoAck
                false,
                false,
                false,
                nil,
            )
            if err != nil {
                log.Printf("Worker %d 开始消费失败: %v", workerID, err)
                return
            }

            log.Printf("Video Worker %d 已启动，等待任务...", workerID)

            for d := range msgs {
                var task VideoTask
                if err := json.Unmarshal(d.Body, &task); err != nil {
                    log.Printf("Worker %d 解析失败: %v", workerID, err)
                    _ = d.Nack(false, false) // 丢弃（也可改成 true 重新入队）
                    continue
                }

                if err := processVideo(task); err != nil {	// 处理视频任务
                    log.Printf("Worker %d 处理失败: %v", workerID, err)
                    _ = d.Nack(false, true) // 重新入队
                    continue
                }

                _ = d.Ack(false)
                log.Printf("Worker %d 完成: VideoID=%d", workerID, task.VideoID)
            }

            log.Printf("Worker %d msgs channel closed, 退出", workerID)
        }(i)
    }

    // 关键：阻塞，防止函数返回； 用channel阻塞也可以
    select {}
}


// startLikeConsumer	============点赞处理的消费者==============
func startLikeConsumer(conn *amqp.Connection) {
	log.Println("点赞 Consumer 启动中...")

	workerCount := 2 // 点赞处理相对简单，2个worker足够

	for i := 0; i < workerCount; i++ {
		go func(workerID int) {
			ch, err := conn.Channel()
			if err != nil {
				log.Printf("Like Worker %d 创建通道失败: %v", workerID, err)
				return
			}
			defer ch.Close()

			// 声明队列（幂等操作）
			q, err := ch.QueueDeclare(
				QueueVideoLikeName,
				true,  // durable: 持久化队列
				false, // autoDelete: 不自动删除
				false, // exclusive: 非独占队列
				false, // noWait: 不等待服务器确认
				nil,   // arguments: 额外参数
			)
			if err != nil {
				log.Printf("Like Worker %d 声明队列失败: %v", workerID, err)
				return
			}

			// 每个 worker 一次只拿一条未确认消息
			if err := ch.Qos(1, 0, false); err != nil {
				log.Printf("Like Worker %d 设置 QoS 失败: %v", workerID, err)
				return
			}

			msgs, err := ch.Consume(
				q.Name,
				"",    // consumer tag
				false, // autoAck: 手动确认
				false,
				false,
				false,
				nil,
			)
			if err != nil {
				log.Printf("Like Worker %d 开始消费失败: %v", workerID, err)
				return
			}

			log.Printf("Like Worker %d 已启动，等待任务...", workerID)

			for d := range msgs {
				var task LikeTask
				if err := json.Unmarshal(d.Body, &task); err != nil {
					log.Printf("Like Worker %d 解析失败: %v", workerID, err)
					_ = d.Nack(false, false) // 丢弃无效消息
					continue
				}

				if err := processLike(task); err != nil { // 处理点赞任务
					log.Printf("Like Worker %d 处理失败: %v", workerID, err)
					_ = d.Nack(false, true) // 重新入队
					continue
				}

				_ = d.Ack(false)
				log.Printf("Like Worker %d 完成: VideoID=%d, Delta=%d", workerID, task.VideoID, task.Delta)
			}

			log.Printf("Like Worker %d msgs channel closed, 退出", workerID)
		}(i)
	}

	// 阻塞，防止函数返回
	select {}
}


func processLike(task LikeTask) error {
	log.Printf("开始处理点赞任务: VideoID=%d, UserID=%d, Delta=%d", task.VideoID, task.UserID, task.Delta)

	// 开启事务，确保 videos 表和 likes 表的操作原子性
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("开启事务失败: %v", tx.Error)
	}
	defer func() {	// 防止程序panic意外终止而没有回滚导致数据库状态不一致
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// appliedDelta：本次真正需要应用到 videos.like_count 的变化（只可能 -1/0/+1）
	appliedDelta := 0

	// 1) 先更新 likes 表（通过查询判断是否需要插入/删除）
	if task.Delta > 0 {
		// 点赞：如果不存在记录才插入，存在则幂等不操作
		exists, err := TxGetByUseridAndVideoid(tx, task.UserID, task.VideoID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("查询点赞记录失败: %v", err)
		}

		// if !exists {
		// 	like := Like{
		// 		UserID:  task.UserID,
		// 		VideoID: task.VideoID,
		// 	}
		// 	if err := tx.Create(&like).Error; err != nil {
		// 		tx.Rollback()
		// 		return fmt.Errorf("插入点赞记录失败: %v", err)
		// 	}
		// 	appliedDelta = 1
		// }
		if !exists {
			res := tx.Exec(
				"INSERT INTO likes(user_id, video_id, created_at, updated_at) VALUES (?, ?, NOW(), NOW())",
				task.UserID,
				task.VideoID,
			)

			if res.Error != nil {
				tx.Rollback()
				return fmt.Errorf("插入点赞记录失败: %v", res.Error)
			}

			if res.RowsAffected > 0 {
				appliedDelta = 1
			}
		}


	} else if task.Delta < 0 {
		// 取消点赞：如果存在记录才删除，不存在则幂等不操作
		exists, err := TxGetByUseridAndVideoid(tx, task.UserID, task.VideoID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("查询点赞记录失败: %v", err)
		}

		if exists {
			// res := tx.Where("user_id = ? AND video_id = ?", task.UserID, task.VideoID).Delete(&Like{})
			res := tx.Exec(
				"UPDATE likes SET deleted_at = NOW() WHERE user_id = ? AND video_id = ? AND deleted_at IS NULL",
				task.UserID,
				task.VideoID,
			)
			if res.Error != nil {
				tx.Rollback()
				return fmt.Errorf("删除点赞记录失败: %v", res.Error)
			}

			// - 正常情况下应当 RowsAffected > 0
			// - 如果 RowsAffected == 0（并发导致刚被删），就当幂等不改计数
			if res.RowsAffected > 0 {
				appliedDelta = -1
			}
		}
	}

	// 2) 再更新 videos 表的 like_count（仅当 likes 实际发生变化时）
	if appliedDelta != 0 {
		result := tx.Exec(
			"UPDATE videos SET like_count = GREATEST(CAST(like_count AS SIGNED) + ?, 0) WHERE id = ?",
			appliedDelta,
			task.VideoID,
		)
		if result.Error != nil {
			tx.Rollback()
			return fmt.Errorf("更新 MySQL 失败: %v", result.Error)
		}
		if result.RowsAffected == 0 {
			tx.Rollback()
			log.Printf("警告: 视频不存在或未更新 VideoID=%d", task.VideoID)
			return nil
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("提交事务失败: %v", err)
	}

	log.Printf("点赞任务处理完成: VideoID=%d, Delta=%d, AppliedDelta=%d", task.VideoID, task.Delta, appliedDelta)
	return nil
}



// initMySQL 初始化 MySQL 连接
func initMySQL() error {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		config.MySQLUsername,
		config.MySQLPassword,
		config.MySQLHost,
		config.MySQLPort,
		config.MySQLDatabase)
	
	conn, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}
	db = conn
	return nil
}

// initMinIO 初始化 MinIO 客户端
func initMinIO() error {
	minioEndpoint := fmt.Sprintf("%s:%s", config.MinIOHost, config.MinIOPort)
	
	client, err := minio.New(minioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinIOAccessKey, config.MinIOSecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return err
	}
	minioClient = client
	return nil
}

// processVideo 处理视频（生成封面 + 转码）
func processVideo(task VideoTask) error {
	log.Printf("开始处理视频: VideoID=%d, FileName=%s", task.VideoID, task.FileName)

	// 1. 从 MinIO 下载视频到本地临时目录
	tempDir := "E:/temp/video_processing"
	
	// 确保临时目录存在
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	
	videoPath := filepath.Join(tempDir, task.FileName)

	err := downloadFromMinIO(task.FileName, videoPath)
	if err != nil {
		return fmt.Errorf("下载视频失败: %v", err)
	}
	defer os.Remove(videoPath) // 处理完成后删除临时文件

	// 2. 使用 WaitGroup 并行处理封面和转码
	var wg sync.WaitGroup
	var coverErr, transcode720Err, transcode1080Err error
	var coverURL, url720p, url1080p string

	// 生成文件名
	baseFilename := strings.TrimSuffix(task.FileName, filepath.Ext(task.FileName))
	coverFilename := baseFilename + "_cover.jpg"
	filename720p := baseFilename + "_720p.mp4"
	filename1080p := baseFilename + "_1080p.mp4"

	// 生成文件路径
	coverPath := filepath.Join(tempDir, coverFilename)
	videoPath720p := filepath.Join(tempDir, filename720p)
	videoPath1080p := filepath.Join(tempDir, filename1080p)

	// 并行任务1：生成封面
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("开始生成封面: VideoID=%d", task.VideoID)
		
		// 生成封面
		if err := generateCover(videoPath, coverPath); err != nil {
			coverErr = fmt.Errorf("生成封面失败: %v", err)
			return
		}
		defer os.Remove(coverPath) // 上传后删除临时文件
		
		// 上传封面到 MinIO
		url, err := uploadToMinIO(coverPath, coverFilename, "image/jpeg")
		if err != nil {
			coverErr = fmt.Errorf("上传封面失败: %v", err)
			return
		}
		coverURL = url
		log.Printf("封面生成完成: VideoID=%d", task.VideoID)
	}()

	// 并行任务2：转码720p
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("开始转码720p: VideoID=%d", task.VideoID)
		
		// 转码720p
		if err := transcodeVideo720p(videoPath, videoPath720p); err != nil {
			transcode720Err = fmt.Errorf("转码720p失败: %v", err)
			return
		}
		defer os.Remove(videoPath720p) // 上传后删除临时文件
		
		// 上传720p视频到 MinIO
		url, err := uploadToMinIO(videoPath720p, filename720p, "video/mp4")
		if err != nil {
			transcode720Err = fmt.Errorf("上传720p视频失败: %v", err)
			return
		}
		url720p = url
		log.Printf("720p转码完成: VideoID=%d", task.VideoID)
	}()

	// 并行任务3：转码1080p
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Printf("开始转码1080p: VideoID=%d", task.VideoID)
		
		// 转码1080p
		if err := transcodeVideo1080p(videoPath, videoPath1080p); err != nil {
			transcode1080Err = fmt.Errorf("转码1080p失败: %v", err)
			return
		}
		defer os.Remove(videoPath1080p) // 上传后删除临时文件
		
		// 上传1080p视频到 MinIO
		url, err := uploadToMinIO(videoPath1080p, filename1080p, "video/mp4")
		if err != nil {
			transcode1080Err = fmt.Errorf("上传1080p视频失败: %v", err)
			return
		}
		url1080p = url
		log.Printf("1080p转码完成: VideoID=%d", task.VideoID)
	}()

	// 等待所有任务完成
	wg.Wait()

	// 检查是否有错误
	if coverErr != nil {
		return coverErr
	}
	if transcode720Err != nil {
		return transcode720Err
	}
	if transcode1080Err != nil {
		return transcode1080Err
	}

	// 3. 更新数据库中的封面URL和转码后的视频URL
	err = updateVideoURLs(task.VideoID, coverURL, url720p, url1080p)
	if err != nil {
		return fmt.Errorf("更新数据库失败: %v", err)
	}

	log.Printf("视频处理完成: VideoID=%d", task.VideoID)
	return nil
}

// downloadFromMinIO 从 MinIO 下载文件
func downloadFromMinIO(filename, localPath string) error {
	ctx := context.Background()
	err := minioClient.FGetObject(ctx, MinioBucket, filename, localPath, minio.GetObjectOptions{})
	return err
}

// generateCover 使用 FFmpeg 生成视频封面
// 从视频的第1秒截取一帧作为封面
func generateCover(videoPath, coverPath string) error {
	// FFmpeg 命令：从视频第1秒截取一帧
	// -i: 输入文件
	// -ss: 指定时间点（秒）
	// -vframes 1: 只截取1帧
	// -q:v 2: 设置图片质量（1-31，数字越小质量越高）
	cmd := exec.Command(
		FFmpegPath,
		"-i", videoPath,
		"-ss", "00:00:01",
		"-vframes", "1",
		"-q:v", "2",
		coverPath,
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg 执行失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// transcodeVideo720p 转码视频为720p
func transcodeVideo720p(inputPath, outputPath string) error {
	// FFmpeg 转码命令
	// -i: 输入文件
	// -vf "scale=-2:720": 缩放到720p，宽度自动计算（保持宽高比）
	// -c:v libx264: 使用H.264编码器
	// -preset medium: 编码速度预设（medium平衡速度和质量）
	// -crf 23: 恒定质量模式（18-28，数字越小质量越高）
	// -b:v 3000k: 视频比特率3Mbps
	// -c:a aac: 音频编码器AAC
	// -b:a 128k: 音频比特率128kbps
	cmd := exec.Command(   // 创建命令不执行
		FFmpegPath,
		"-i", inputPath,
		"-vf", "scale=-2:720",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-b:v", "3000k",
		"-c:a", "aac",
		"-b:a", "128k",
		outputPath,
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg 720p转码失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// transcodeVideo1080p 转码视频为1080p
func transcodeVideo1080p(inputPath, outputPath string) error {
	// FFmpeg 转码命令
	// -i: 输入文件
	// -vf "scale=-2:1080": 缩放到1080p，宽度自动计算（保持宽高比）
	// -c:v libx264: 使用H.264编码器
	// -preset medium: 编码速度预设
	// -crf 23: 恒定质量模式
	// -b:v 5000k: 视频比特率5Mbps
	// -c:a aac: 音频编码器AAC
	// -b:a 192k: 音频比特率192kbps
	cmd := exec.Command(
		FFmpegPath,
		"-i", inputPath,
		"-vf", "scale=-2:1080",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-b:v", "5000k",
		"-c:a", "aac",
		"-b:a", "192k",
		outputPath,
	)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("FFmpeg 1080p转码失败: %v, 输出: %s", err, string(output))
	}

	return nil
}

// uploadToMinIO 上传文件到 MinIO，返回访问url
func uploadToMinIO(localPath, filename, contentType string) (string, error) {
	ctx := context.Background()

	// 上传文件
	_, err := minioClient.FPutObject(ctx, MinioBucket, filename, localPath, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return "", err
	}

	// 生成永久访问URL
	minioEndpoint := fmt.Sprintf("%s:%s", config.MinIOHost, config.MinIOPort)
	fileURL := fmt.Sprintf("http://%s/%s/%s", minioEndpoint, MinioBucket, filename)
	return fileURL, nil
}

// updateVideoURLs 更新视频的封面URL和转码后的视频URL
func updateVideoURLs(videoID uint, coverURL, url720p, url1080p string) error {
	updates := map[string]interface{}{
		"cover_url":  coverURL,
		"url720p":   url720p,
		"url1080p":  url1080p,
	}
	
	result := db.Table("videos").Where("id = ?", videoID).Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	return nil
}

// 保证事务一致性的mysql数据库查询结果
func TxGetByUseridAndVideoid(dbConn *gorm.DB, userid uint, videoid uint) (bool, error) {
	var exist int

	err := dbConn.Raw(
		"SELECT 1 FROM likes WHERE user_id = ? AND video_id = ? AND deleted_at IS NULL LIMIT 1",
		userid,
		videoid,
	).Scan(&exist).Error

	if err != nil {
		log.Printf("查询点赞记录失败: %v", err)
		return false, err
	}

	return exist == 1, nil
}
