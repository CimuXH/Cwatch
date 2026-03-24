package config

const (
    // MySQL 配置
    MySQLUsername = "root"
    MySQLPassword = "mysql030303"
    MySQLHost     = "101.132.25.34"
    MySQLPort     = "3306"
    MySQLDatabase = "cwatch"
    
    // Redis 配置
    RedisPassword = "redis030303"
    RedisHost     = "101.132.25.34"
    RedisPort     = "6379"
    
    // RabbitMQ 配置
    RabbitMQUsername = "admin"
    RabbitMQPassword = "rabbitmq030303"
    RabbitMQHost     = "101.132.25.34"
    RabbitMQPort     = "5672"
    
    // MinIO 配置
    MinIOAccessKey = "minioadmin"
    MinIOSecretKey = "minio030303"
    MinIOHost      = "101.132.25.34"
    MinIOPort      = "9000"

    // =========================
    // BloomFilter（随机 Feed 去重）
    // =========================
    // 每个用户每天的 bitmap 大小（bits）
    // 位越多，误判率越低，但 Redis 内存占用更大。
    BloomRandomFeedBitsM = 1_048_576 // 约 128KB/用户/天
    // hash 次数 k（一般 3~8）
    BloomRandomFeedHashK = 5
    // Redis key 前缀
    BloomRandomFeedKeyPrefix = "bf:randomfeed:"
)