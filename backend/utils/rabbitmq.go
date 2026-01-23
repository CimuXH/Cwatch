package utils

// RabbitMQ 生产者

import (
	"encoding/json"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQ 连接实例
var rabbitConn *amqp.Connection
var rabbitChannel *amqp.Channel

// RabbitMQ 配置
const (
	RabbitMQURL   = "amqp://admin:rabbitmq111111@101.132.25.34:5672/" // RabbitMQ 连接地址
	QueueName     = "video_processing"                        // 队列名称
)

// VideoTask 视频处理任务结构
type VideoTask struct {
	VideoID  uint   `json:"video_id"`  // 视频ID
	FileName string `json:"file_name"` // 文件名
}

// InitRabbitMQ 初始化 RabbitMQ 连接
func InitRabbitMQ() error {
	// 连接到 RabbitMQ 服务器
	conn, err := amqp.Dial(RabbitMQURL)
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

	// 声明队列（如果队列不存在则创建）
	_, err = ch.QueueDeclare(
		QueueName, // 队列名称
		true,      // durable: 持久化队列
		false,     // autoDelete: 不自动删除
		false,     // exclusive: 非独占队列
		false,     // noWait: 不等待服务器确认
		nil,       // arguments: 额外参数
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
		QueueName, // routing key: 队列名称
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