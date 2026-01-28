package utils

// RabbitMQ 生产者

import (
	"backend/config"
	"encoding/json"
	"fmt"
	"log"
	"time"
	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ 连接实例
var rabbitConn *amqp.Connection
var rabbitChannel *amqp.Channel

// RabbitMQ 配置
const (
	QueueVideoName = "video_processing" // 视频封面，转码处理 队列名称
	QueueVideoLikeName = "video_like_processing"   // 新增：点赞处理队列
)

// VideoTask 视频处理任务结构
type VideoTask struct {
	VideoID  uint   `json:"video_id"`  // 视频ID
	FileName string `json:"file_name"` // 文件名
}

// LikeTask  点赞处理任务结构
type LikeTask struct {
	VideoID uint  `json:"video_id"`
	UserID  uint  `json:"user_id"`
	Delta   int   `json:"delta"` // +1 点赞；-1 取消
	TS      int64 `json:"ts"`    // TS: time.Now().Unix() 时间戳
}

// InitRabbitMQ 初始化 RabbitMQ 连接
func InitRabbitMQ() error {
	// 构建 RabbitMQ 连接地址
	rabbitMQURL := fmt.Sprintf("amqp://%s:%s@%s:%s/",
		config.RabbitMQUsername,
		config.RabbitMQPassword,
		config.RabbitMQHost,
		config.RabbitMQPort)

	// 连接到 RabbitMQ 服务器
	conn, err := amqp.Dial(rabbitMQURL)
	if err != nil {
		return err
	}
	rabbitConn = conn

	// 创建一个通道
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	rabbitChannel = ch

	// 声明视频处理队列（如果队列不存在则创建）
	_, err = ch.QueueDeclare(
		QueueVideoName, // 队列名称
		true,           // durable: 持久化队列
		false,          // autoDelete: 不自动删除
		false,          // exclusive: 非独占队列
		false,          // noWait: 不等待服务器确认
		nil,            // arguments: 额外参数
	)
	if err != nil {
		return err
	}

	// 声明点赞处理队列（如果队列不存在则创建）
	_, err = ch.QueueDeclare(
		QueueVideoLikeName, // 队列名称
		true,               // durable: 持久化队列
		false,              // autoDelete: 不自动删除
		false,              // exclusive: 非独占队列
		false,              // noWait: 不等待服务器确认
		nil,                // arguments: 额外参数
	)
	if err != nil {
		return err
	}

	log.Println("RabbitMQ 连接成功")
	return nil
}

// PublishVideoTask 发送视频处理任务到队列
// 参数：视频ID、文件名
// 返回：错误
func PublishVideoTask(videoID uint, filename string) error {
	// 构造任务消息
	task := VideoTask{
		VideoID:  videoID,
		FileName: filename,
	}

	// 序列化为JSON，JSON字符串在内存中就是[]byte类型的
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}

	// 发送消息到队列
	err = rabbitChannel.Publish(
		"",        // exchange: 使用默认交换机
		QueueVideoName, // routing key: 队列名称
		false,     // mandatory: 不强制
		false,     // immediate: 不立即
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, // 持久化消息
			ContentType:  "application/json",
			Body:         body,
		},
	)
	if err != nil {
		return err
	}

	log.Printf("视频处理任务已发送到队列: VideoID=%d, FileName=%s", videoID, filename)
	return nil
}

// PublishLikeTask 发送点赞处理任务到队列
func PublishLikeTask(videoID uint, userID uint, delta int) error {
	// 构造任务消息
	task := LikeTask{
		VideoID: videoID,
		UserID:  userID,
		Delta:   delta,
		TS:      time.Now().Unix(),
	}

	// 序列化为JSON
	body, err := json.Marshal(task)
	if err != nil {
		return err
	}

	// 发送消息到队列
	err = rabbitChannel.Publish(
		"",                // exchange: 使用默认交换机
		QueueVideoLikeName, // routing key: 队列名称
		false,             // mandatory: 不强制
		false,             // immediate: 不立即
		amqp.Publishing{
			DeliveryMode: amqp.Persistent, // 持久化消息
			ContentType:  "application/json",
			Body:         body,
		},
	)
	if err != nil {
		return err
	}

	log.Printf("点赞处理任务已发送到队列: VideoID=%d, UserID=%d, Delta=%d", videoID, userID, delta)
	return nil
}

// CloseRabbitMQ 关闭 RabbitMQ 连接
func CloseRabbitMQ() {
	if rabbitChannel != nil {
		rabbitChannel.Close()
	}
	if rabbitConn != nil {
		rabbitConn.Close()
	}
	log.Println("RabbitMQ 连接已关闭")
}