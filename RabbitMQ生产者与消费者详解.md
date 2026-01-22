# RabbitMQ 生产者与消费者

## 一、核心概念（3个角色）

```
生产者（Producer） → RabbitMQ 队列 → 消费者（Consumer）
```

| 角色 | 位置 | 职责 |
|------|------|------|
| **生产者** | `backend/utils/rabbitmq.go` | 发送消息到队列 |
| **RabbitMQ** | 服务器 `101.132.25.34:5672` | 存储和分发消息 |
| **消费者** | `worker/main.go` | 接收并处理消息 |

---

## 二、生产者（Producer）

### 2.1 核心代码

```go
// 1. 初始化连接（启动时调用一次）
func InitRabbitMQ() error {
    conn, _ := amqp.Dial("amqp://guest:guest@101.132.25.34:5672/")
    ch, _ := conn.Channel()
    ch.QueueDeclare("video_processing", true, false, false, false, nil)
}

// 2. 发送消息（每次上传视频后调用）
func PublishVideoTask(videoID uint, filename string) error {
    task := VideoTask{VideoID: videoID, FileName: filename}
    body, _ := json.Marshal(task)
    
    ch.Publish("", "video_processing", false, false, amqp.Publishing{
        DeliveryMode: amqp.Persistent,  // 持久化
        Body:         body,
    })
}
```

### 2.2 关键参数

| 参数 | 值 | 作用 |
|------|-----|------|
| `durable` | `true` | 队列持久化（重启不丢失） |
| `DeliveryMode` | `Persistent` | 消息持久化（重启不丢失） |
| `exchange` | `""` | 使用默认交换机 |

---

## 三、消费者（Consumer）

### 3.1 核心代码

```go
func main() {
    // 1. 连接 RabbitMQ
    conn, _ := amqp.Dial("amqp://guest:guest@101.132.25.34:5672/")
    ch, _ := conn.Channel()
    
    // 2. 声明队列
    ch.QueueDeclare("video_processing", true, false, false, false, nil)
    
    // 3. 设置 QoS（重要！）
    ch.Qos(1, 0, false)  // 一次只处理1个任务
    
    // 4. 开始消费
    msgs, _ := ch.Consume(q.Name, "", false, false, false, false, nil)
    
    // 5. 处理消息
    for d := range msgs {
        var task VideoTask
        json.Unmarshal(d.Body, &task)
        
        err := processVideo(task)
        if err != nil {
            d.Nack(false, true)   // 失败，重新入队
        } else {
            d.Ack(false)          // 成功，确认
        }
    }
}
```

### 3.2 关键参数

| 参数 | 值 | 作用 |
|------|-----|------|
| `autoAck` | `false` | 手动确认（防止消息丢失） |
| `prefetchCount` | `1` | 一次只处理1个任务（公平分发） |

---

## 四、手动确认机制（重要！）

### 4.1 为什么需要手动确认？

| 自动确认 | 手动确认 |
|---------|---------|
| 消息一接收就删除 | 处理完才删除 |
| 处理失败消息丢失 ❌ | 处理失败可重试 ✅ |

### 4.2 三种确认方式

```go
// 1. 成功
d.Ack(false)  // 消息从队列删除

// 2. 失败，重试
d.Nack(false, true)  // 消息重新入队

// 3. 失败，不重试
d.Nack(false, false)  // 消息删除（无效消息）
```

---

## 五、多消费者并行处理

### 5.1 如何启动多个消费者？

```bash
# 终端1
go run worker/main.go

# 终端2
go run worker/main.go

# 终端3
go run worker/main.go
```

### 5.2 消息如何分发？

```
队列: [任务1] [任务2] [任务3] [任务4] [任务5] [任务6]

Worker 1: 任务1 → 任务4
Worker 2: 任务2 → 任务5
Worker 3: 任务3 → 任务6
```

**关键：** QoS 设置 `prefetchCount=1` 实现公平分发

---

## 六、工作流程对比

### 生产者流程

```
1. 后端服务启动 → InitRabbitMQ()
2. 视频上传完成 → PublishVideoTask()
3. 构造消息 → 序列化 JSON
4. 发送到队列
5. 完成
```

### 消费者流程

```
1. Worker 启动 → 连接 RabbitMQ
2. 声明队列 → 设置 QoS
3. 开始监听队列
4. 收到消息 → 解析 JSON
5. 处理任务 → 确认消息
6. 继续监听...
```

---

## 七、常见问题

### Q1: 为什么不使用交换机？

**答：** 场景简单，一对一通信，默认交换机足够

### Q2: 多个消费者会重复处理吗？

**答：** 不会！每个消息只会被一个消费者处理

### Q3: Worker 崩溃了怎么办？

**答：** 未确认的消息自动重新入队，其他 Worker 继续处理

### Q4: 如何防止消息丢失？

**答：** 
- 队列持久化 `durable: true`
- 消息持久化 `DeliveryMode: Persistent`
- 手动确认 `autoAck: false`

---

## 八、核心知识点总结

### 必须记住的5点

1. **生产者**：发送消息到队列
2. **消费者**：接收并处理消息
3. **手动确认**：防止消息丢失
4. **QoS 设置**：实现公平分发
5. **多消费者**：提高处理速度

### 关键代码位置

| 功能 | 文件 | 函数 |
|------|------|------|
| 初始化连接 | `backend/utils/rabbitmq.go` | `InitRabbitMQ()` |
| 发送消息 | `backend/utils/rabbitmq.go` | `PublishVideoTask()` |
| 接收消息 | `worker/main.go` | `main()` |
| 处理消息 | `worker/main.go` | `processVideo()` |
