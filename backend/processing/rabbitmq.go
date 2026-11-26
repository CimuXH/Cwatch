package processing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"backend/utils"
	amqp "github.com/rabbitmq/amqp091-go"
)

func runRabbitMQ(ctx context.Context, url, queue string, parallelism int) {
	if _, err := amqp.ParseURI(url); err != nil {
		log.Print("CWATCH_RABBITMQ_URL 无效，异步任务将留在 MySQL")
		return
	}
	for ctx.Err() == nil {
		conn, err := amqp.DialConfig(url, amqp.Config{
			Dial: amqp.DefaultDial(3 * time.Second), Heartbeat: 10 * time.Second,
		})
		if err != nil {
			log.Print("RabbitMQ 连接失败，待投递任务保留在 MySQL，稍后重连")
		} else if err := serveRabbitMQ(ctx, conn, queue, parallelism); err != nil && ctx.Err() == nil {
			log.Printf("RabbitMQ 任务通道中断，稍后重连: %v", err)
		}

		timer := time.NewTimer(3 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func serveRabbitMQ(parent context.Context, conn *amqp.Connection, queue string, parallelism int) error {
	ctx, cancel := context.WithCancel(parent)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		_ = conn.CloseDeadline(time.Now().Add(2 * time.Second))
		workers.Wait()
	}()

	failed := make(chan error, parallelism+1)
	start := func(run func() error) {
		workers.Add(1)
		go func() {
			defer workers.Done()
			failed <- run()
		}()
	}

	publisher, err := conn.Channel()
	if err != nil {
		return err
	}
	if _, err := publisher.QueueDeclare(queue, true, false, false, false, nil); err != nil {
		return err
	}
	if err := publisher.Confirm(false); err != nil {
		return err
	}
	returns := publisher.NotifyReturn(make(chan amqp.Return, 1))
	start(func() error { return publishTasks(ctx, publisher, queue, returns) })
	for range parallelism {
		channel, err := conn.Channel()
		if err != nil {
			return err
		}
		if err := channel.Qos(1, 0, false); err != nil {
			return err
		}
		deliveries, err := channel.ConsumeWithContext(ctx, queue, "", false, false, false, false, nil)
		if err != nil {
			return err
		}
		start(func() error { return consumeTasks(ctx, deliveries) })
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-conn.NotifyClose(make(chan *amqp.Error, 1)):
		if err == nil {
			return errors.New("RabbitMQ 连接已关闭")
		}
		return err
	case err := <-failed:
		return err
	}
}

func publishTasks(ctx context.Context, ch *amqp.Channel, queue string, returns <-chan amqp.Return) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		tasks, err := utils.TasksToPublish(ctx, 32)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			id := strconv.FormatUint(task.ID, 10)
			if err := publishTask(ctx, ch, queue, returns, id); err != nil {
				return fmt.Errorf("投递任务 %s: %w", id, err)
			}
			if err := utils.MarkTaskPublished(ctx, task); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	return ctx.Err()
}

func publishTask(parent context.Context, ch *amqp.Channel, queue string, returns <-chan amqp.Return, id string) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	confirm, err := ch.PublishWithDeferredConfirmWithContext(ctx, "", queue, true, false, amqp.Publishing{
		ContentType: "text/plain", DeliveryMode: amqp.Persistent, MessageId: id, Body: []byte(id),
	})
	if err != nil {
		return err
	}
	if confirm == nil {
		return errors.New("发布确认未启用")
	}
	accepted, err := confirm.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !accepted {
		return errors.New("RabbitMQ 拒绝消息")
	}
	// The broker returns an unroutable mandatory message before its confirm.
	select {
	case returned, open := <-returns:
		if !open {
			return errors.New("RabbitMQ 发布通道已关闭")
		}
		return fmt.Errorf("RabbitMQ 未路由消息: %d %s", returned.ReplyCode, returned.ReplyText)
	default:
		return nil
	}
}

func consumeTasks(ctx context.Context, deliveries <-chan amqp.Delivery) error {
	for delivery := range deliveries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		id, err := strconv.ParseUint(string(delivery.Body), 10, 64)
		if err != nil || id == 0 {
			log.Print("RabbitMQ 拒绝无效的视频任务消息")
			if err := delivery.Reject(false); err != nil {
				return err
			}
			continue
		}
		claimCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		task, err := utils.ClaimTaskByID(claimCtx, id)
		cancel()
		if err != nil {
			return err
		}
		if task != nil {
			if err := runTask(ctx, *task); err != nil {
				return err
			}
		}
		// A duplicate may find a completed task or an active lease. Expired
		// leases and scheduled retries are delivered again from MySQL.
		if err := delivery.Ack(false); err != nil {
			return err
		}
	}
	return errors.New("RabbitMQ 消费通道已关闭")
}
