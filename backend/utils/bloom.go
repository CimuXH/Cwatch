package utils

import (
	"context"
	"backend/config"
	"fmt"
)

// BloomFilter 简易布隆过滤器（基于 Redis bitmap）
// MVP 目的：对“用户在某个时间窗口内已看过的视频”做去重，避免 feed 重复。
// 注意：布隆过滤器是概率结构，可能出现误判（false positive）。
//
// key 组织：
//   bf:randomfeed:<userID>:<YYYYMMDD>
func bloomKey(userID uint, dayKey string) string {
	return fmt.Sprintf("%s%d:%s", config.BloomRandomFeedKeyPrefix, userID, dayKey)
}

func fnv1a64(s string) uint64 {
	// FNV-1a 64-bit
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	var hash uint64 = offset64
	for i := 0; i < len(s); i++ {
		hash ^= uint64(s[i])
		hash *= prime64
	}
	return hash
}

func bloomIndexes(key string) []uint32 {
	// 双重哈希：index_i = (h1 + i*h2) % m
	h1 := fnv1a64(key)
	h2 := fnv1a64(key + "|salt")

	m := uint32(config.BloomRandomFeedBitsM)
	if m == 0 {
		m = 1
	}
	idxs := make([]uint32, config.BloomRandomFeedHashK)
	for i := 0; i < config.BloomRandomFeedHashK; i++ {
		// 避免溢出：使用 uint64 再取模
		idx := (uint64(h1) + uint64(i)*uint64(h2)) % uint64(m)
		idxs[i] = uint32(idx)
	}
	return idxs
}

// BloomMightContain 判断某视频是否“可能已存在”
func BloomMightContain(ctx context.Context, userID uint, dayKey string, videoID uint) (bool, error) {
	if rdb == nil {
		return false, fmt.Errorf("redis client not initialized")
	}
	key := fmt.Sprintf("%d", videoID)
	bkey := bloomKey(userID, dayKey)

	for _, idx := range bloomIndexes(key) {
		v, err := rdb.GetBit(ctx, bkey, int64(idx)).Result()
		if err != nil {
			return false, err
		}
		if v == 0 {
			return false, nil
		}
	}
	return true, nil
}

// BloomAdd 向布隆过滤器中添加视频ID
func BloomAdd(ctx context.Context, userID uint, dayKey string, videoID uint) error {
	if rdb == nil {
		return fmt.Errorf("redis client not initialized")
	}
	key := fmt.Sprintf("%d", videoID)
	bkey := bloomKey(userID, dayKey)

	pipe := rdb.Pipeline()
	for _, idx := range bloomIndexes(key) {
		pipe.SetBit(ctx, bkey, int64(idx), 1)
	}
	_, err := pipe.Exec(ctx)
	return err
}

