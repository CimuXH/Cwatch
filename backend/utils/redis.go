package utils

import (
	"backend/config"
	"context"
	"fmt"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

// Redis 客户端实例
var rdb *redis.Client

// Redis Key 前缀
const (
	TokenPrefix     = "token:"     // 有效 token 前缀
	BlacklistPrefix = "blacklist:" // 黑名单 token 前缀
)

// Token 过期时间（与 JWT 过期时间一致）
const TokenExpiration = 24 * time.Hour

// InitRedis 初始化 Redis 连接
func InitRedis() error {
	rdb = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", config.RedisHost, config.RedisPort),
		Password: config.RedisPassword,
		DB:       0,
	})

	// 测试连接
	ctx := context.Background()
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return err
	}

	log.Println("Redis 连接成功")
	return nil
}

// SaveToken 保存 token 到 Redis
// 参数：用户名、token
func SaveToken(username, token string) error {
	ctx := context.Background()
	key := TokenPrefix + username
	return rdb.Set(ctx, key, token, TokenExpiration).Err()
}

// AddToBlacklist 将 token 加入黑名单
// 参数：token、剩余过期时间
func AddToBlacklist(token string, expiration time.Duration) error {
	ctx := context.Background()
	key := BlacklistPrefix + token
	// 黑名单中的 token 只需要保存到原 token 过期即可
	return rdb.Set(ctx, key, "1", expiration).Err()
}

// IsBlacklisted 检查 token 是否在黑名单中
// 参数：token
// 返回：是否在黑名单中
func IsBlacklisted(token string) bool {
	ctx := context.Background()
	key := BlacklistPrefix + token
	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		return false
	}
	return exists > 0
}

// DeleteUserToken 删除用户的 token（用于登出时清理）
func DeleteUserToken(username string) error {
	ctx := context.Background()
	key := TokenPrefix + username
	return rdb.Del(ctx, key).Err()
}
