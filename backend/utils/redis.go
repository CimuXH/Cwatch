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

// ============================= 点赞排行榜相关 =====================================

// Redis Key 常量
const (
	VideoLikeRankKey = "rank:video:like"      // 视频点赞排行榜 ZSET
	VideoLikeSetKey  = "like:video:"          // 视频点赞用户集合 SET 前缀
)

// ======================================排行榜zset操作

// IncrVideoLikeRank 增加视频点赞数（排行榜）
// 参数：视频ID、增量（+1点赞，-1取消）
// 返回：新的点赞数、错误
func IncrVideoLikeRank(videoID uint, delta int) (int64, error) {
	ctx := context.Background()
	
	// ZINCRBY rank:video:like delta videoID
	newScore, err := rdb.ZIncrBy(ctx, VideoLikeRankKey, float64(delta), fmt.Sprintf("%d", videoID)).Result()
	if err != nil {
		return 0, err
	}
	
	return int64(newScore), nil
}


// GetTopLikedVideos 获取点赞排行榜前N个视频
// 参数：数量限制
// 返回：视频ID列表（按点赞数降序）、错误
func GetTopLikedVideos(limit int) ([]uint, error) {
	ctx := context.Background()
	
	// ZREVRANGE rank:video:like 0 limit-1
	results, err := rdb.ZRevRange(ctx, VideoLikeRankKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	
	// 转换字符串为 uint
	videoIDs := make([]uint, 0, len(results))
	for _, idStr := range results {
		var id uint
		fmt.Sscanf(idStr, "%d", &id)
		videoIDs = append(videoIDs, id)
	}
	
	return videoIDs, nil
}

// GetTopLikedVideosWithScores 获取点赞排行榜前N个视频（带点赞数）
// 参数：数量限制
// 返回：视频ID和点赞数的映射、错误
func GetTopLikedVideosWithScores(limit int) (map[uint]int64, error) {
	ctx := context.Background()
	
	// ZREVRANGE rank:video:like 0 limit-1 WITHSCORES
	results, err := rdb.ZRevRangeWithScores(ctx, VideoLikeRankKey, 0, int64(limit-1)).Result()
	if err != nil {
		return nil, err
	}
	
	// 转换为 map
	videoScores := make(map[uint]int64)
	for _, z := range results {
		var id uint
		fmt.Sscanf(z.Member.(string), "%d", &id)
		videoScores[id] = int64(z.Score)
	}
	
	return videoScores, nil
}

// GetVideoLikeCount 获取视频的点赞数（从排行榜）
// 参数：视频ID
// 返回：点赞数、错误
func GetVideoLikeCount(videoID uint) (int64, error) {
	ctx := context.Background()
	
	// ZSCORE rank:video:like videoID
	score, err := rdb.ZScore(ctx, VideoLikeRankKey, fmt.Sprintf("%d", videoID)).Result()
	if err != nil {
		if err == redis.Nil {
			// 视频不在排行榜中，返回0
			return 0, nil
		}
		return 0, err
	}
	
	return int64(score), nil
}

// InitVideoLikeRank 初始化视频点赞排行榜（从MySQL加载）
// 参数：视频ID和点赞数的映射
// 返回：错误
func InitVideoLikeRank(videoLikes map[uint]int64) error {
	ctx := context.Background()
	
	// 批量添加到 ZSET
	members := make([]redis.Z, 0, len(videoLikes))
	for videoID, likes := range videoLikes {
		members = append(members, redis.Z{
			Score:  float64(likes),
			Member: fmt.Sprintf("%d", videoID),
		})
	}
	
	if len(members) == 0 {
		return nil
	}
	
	// ZADD rank:video:like score1 member1 score2 member2 ...
	_, err := rdb.ZAdd(ctx, VideoLikeRankKey, members...).Result()
	return err
}

// GetAllVideoLikeRanks 获取所有视频的点赞数（用于同步到MySQL）
// 返回：视频ID和点赞数的映射、错误
func GetAllVideoLikeRanks() (map[uint]int64, error) {
	ctx := context.Background()
	
	// ZRANGE rank:video:like 0 -1 WITHSCORES
	results, err := rdb.ZRangeWithScores(ctx, VideoLikeRankKey, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	
	// 转换为 map
	videoScores := make(map[uint]int64)
	for _, z := range results {
		var id uint
		fmt.Sscanf(z.Member.(string), "%d", &id)
		videoScores[id] = int64(z.Score)
	}
	
	return videoScores, nil
}

// =====================================防重复点赞set操作

// IsUserLikedVideo 检查用户是否已点赞该视频
// 参数：视频ID、用户ID
// 返回：是否已点赞、错误
func IsUserLikedVideo(videoID uint, userID uint) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("%s%d", VideoLikeSetKey, videoID)
	
	// SISMEMBER like:video:<videoID> userID
	exists, err := rdb.SIsMember(ctx, key, fmt.Sprintf("%d", userID)).Result()
	if err != nil {
		return false, err
	}
	
	return exists, nil
}

// AddUserLikeVideo 添加用户点赞记录
// 参数：视频ID、用户ID
// 返回：是否成功添加（false表示已存在）、错误
func AddUserLikeVideo(videoID uint, userID uint) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("%s%d", VideoLikeSetKey, videoID)
	
	// SADD like:video:<videoID> userID
	added, err := rdb.SAdd(ctx, key, fmt.Sprintf("%d", userID)).Result()
	if err != nil {
		return false, err
	}
	
	// added > 0 表示成功添加，= 0 表示已存在
	return added > 0, nil
}

// RemoveUserLikeVideo 移除用户点赞记录
// 参数：视频ID、用户ID
// 返回：是否成功移除（false表示不存在）、错误
func RemoveUserLikeVideo(videoID uint, userID uint) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("%s%d", VideoLikeSetKey, videoID)
	
	// SREM like:video:<videoID> userID
	removed, err := rdb.SRem(ctx, key, fmt.Sprintf("%d", userID)).Result()
	if err != nil {
		return false, err
	}
	
	// removed > 0 表示成功移除，= 0 表示不存在
	return removed > 0, nil
}
