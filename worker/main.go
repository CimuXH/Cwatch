package main

// RabbitMQ 消费者

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	amqp "github.com/rabbitmq/amqp091-go"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 配置常量
const (
	// RabbitMQ 配置
	RabbitMQURL = "amqp://admin:rabbitmq030303@101.132.25.34:5672/"
	QueueName   = "video_processing"

	// MinIO 配置
	MinioEndpoint   = "101.132.25.34:9000"
	MinioBucket     = "cwatch"
	MinioAccessKey  = "minioadmin"
	MinioSecretKey  = "minio030303"

	// MySQL 配置
	MySQLDSN = "root:mysql030303@tcp(101.132.25.34:3306)/cwatch?charset=utf8mb4&parseTime=True&loc=Local"

	// FFmpeg 配置（Windows路径）
	FFmpegPath = "E:/soft/ffmpeg-8.0.1-essentials_build/bin/ffmpeg.exe"
)

// VideoTask 视频处理任务结构
type VideoTask struct {
	VideoID  uint   `json:"video_id"`
	FileName string `json:"file_name"`
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
	conn, err := amqp.Dial(RabbitMQURL)
	if err != nil {
		log.Fatal("RabbitMQ 连接失败:", err)
	}
	defer conn.Close()
	log.Println("RabbitMQ 连接成功")

	// =====================================连接工作准备完成=====================================

	// 创建通道
	ch, err := conn.Channel()
	if err != nil {
		log.Fatal("创建通道失败:", err)
	}
	defer ch.Close()

	// 声明队列
	q, err := ch.QueueDeclare(
		QueueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,   // arguments
	)
	if err != nil {
		log.Fatal("声明队列失败:", err)
	}

	// 设置预取数量（一次只处理一个任务）
	err = ch.Qos(
		1,     // prefetchCount
		0,     // prefetchSize
		false, // global
	)
	if err != nil {
		log.Fatal("设置 QoS 失败:", err)
	}

	// 开始消费消息
	msgs, err := ch.Consume(
		q.Name,
		"",    // consumer
		false, // autoAck: 手动确认
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		log.Fatal("开始消费失败:", err)
	}

	log.Println("Worker 已启动，等待任务...")

	// 持续监听消息
	forever := make(chan bool)


	// 启动多个消费者 goroutine，（如果只想启动一个消费者把for去掉就行）
	for i := 0; i < 5; i++ {
		go func(workerID int) {
			log.Printf("Worker %d 启动", workerID)
			
			for d := range msgs {
				log.Printf("Worker %d 收到任务: %s", workerID, d.Body)

				// 解析任务
				var task VideoTask
				err := json.Unmarshal(d.Body, &task)
				if err != nil {
					log.Printf("Worker %d 解析任务失败: %v", workerID, err)
					d.Nack(false, false) // 拒绝消息，不重新入队
					continue
				}

				err = processVideo(task)
				if err != nil {
					log.Printf("Worker %d 处理视频失败: %v", workerID, err)
					d.Nack(false, true) // 拒绝消息，重新入队
					continue
				}

				// 确认消息
				d.Ack(false)
				log.Printf("Worker %d 任务完成: VideoID=%d", workerID, task.VideoID)
			}
		}(i)
	}


	<-forever
}

// initMySQL 初始化 MySQL 连接
func initMySQL() error {
	conn, err := gorm.Open(mysql.Open(MySQLDSN), &gorm.Config{})
	if err != nil {
		return err
	}
	db = conn
	return nil
}

// initMinIO 初始化 MinIO 客户端
func initMinIO() error {
	client, err := minio.New(MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(MinioAccessKey, MinioSecretKey, ""),
		Secure: false,
	})
	if err != nil {
		return err
	}
	minioClient = client
	return nil
}

// processVideo 处理视频（生成封面）
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

	// 2. 使用 FFmpeg 生成封面图
	coverFilename := strings.TrimSuffix(task.FileName, filepath.Ext(task.FileName)) + "_cover.jpg"
	coverPath := filepath.Join(tempDir, coverFilename)

	err = generateCover(videoPath, coverPath)
	if err != nil {
		return fmt.Errorf("生成封面失败: %v", err)
	}
	defer os.Remove(coverPath) // 处理完成后删除临时文件

	// 3. 上传封面图到 MinIO
	coverURL, err := uploadToMinIO(coverPath, coverFilename)
	if err != nil {
		return fmt.Errorf("上传封面失败: %v", err)
	}

	// 4. 更新数据库中的封面URL
	err = updateVideoCover(task.VideoID, coverURL)
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

// uploadToMinIO 上传文件到 MinIO，返回访问url
func uploadToMinIO(localPath, filename string) (string, error) {
	ctx := context.Background()

	// 上传文件
	_, err := minioClient.FPutObject(ctx, MinioBucket, filename, localPath, minio.PutObjectOptions{
		ContentType: "image/jpeg",
	})
	if err != nil {
		return "", err
	}

	// 生成永久访问URL
	fileURL := fmt.Sprintf("http://%s/%s/%s", MinioEndpoint, MinioBucket, filename)
	return fileURL, nil
}

// updateVideoCover 更新视频封面URL
func updateVideoCover(videoID uint, coverURL string) error {
	result := db.Table("videos").Where("id = ?", videoID).Update("cover_url", coverURL)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
