package utils

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const (
	likeCountLockTTL      = 3 * time.Second
	likeCountQueryTimeout = time.Second
	likeCountWaitTimeout  = 2 * time.Second
	likeCountPollInterval = 25 * time.Millisecond
)

var (
	ErrLikeCountsBusy       = errors.New("点赞计数暂时繁忙，请稍后重试")
	errLikeCacheUnavailable = errors.New("点赞计数缓存不可用")
)

// Each pair is a count key and its independent lock key. Recheck the count
// after acquiring the lock, then reserve a refill token before querying MySQL.
const reserveLikeCounts = `
local owned = {}
for i = 1, #KEYS, 2 do
    if redis.call('SET', KEYS[i + 1], ARGV[1], 'NX', 'PX', ARGV[2]) then
        local raw = redis.call('GET', KEYS[i])
        local value = tonumber(raw)
        if raw and string.match(raw, '^%d+$') and value and value <= 9223372036854775807 then
            redis.call('DEL', KEYS[i + 1])
        else
            redis.call('SET', KEYS[i], ARGV[1], 'PX', ARGV[2])
            table.insert(owned, (i + 1) / 2)
        end
    end
end
return owned
`

const refillLikeCounts = `
for i = 1, #KEYS, 2 do
    if redis.call('GET', KEYS[i]) == ARGV[1] and redis.call('GET', KEYS[i + 1]) == ARGV[1] then
        redis.call('SET', KEYS[i], ARGV[(i + 1) / 2 + 2], 'PX', ARGV[2])
    end
end
return 1
`

const releaseLikeCountLocks = `
for i = 1, #KEYS, 2 do
    if redis.call('GET', KEYS[i + 1]) == ARGV[1] then
        redis.call('DEL', KEYS[i + 1])
        if redis.call('GET', KEYS[i]) == ARGV[1] then
            redis.call('DEL', KEYS[i])
        end
    end
end
return 1
`

func likeCountKey(videoID uint) string {
	return redisVideoPrefix + "likes:" + strconv.FormatUint(uint64(videoID), 10)
}

func invalidateLikeCount(videoID uint) {
	cache := sharedVideoCache
	if cache == nil {
		return
	}
	// The database commit may succeed even if the HTTP request has ended.
	ctx, cancel := context.WithTimeout(context.Background(), redisCacheTimeout)
	defer cancel()
	if err := cache.client.Del(ctx, likeCountKey(videoID)).Err(); err != nil {
		cache.backOff()
		log.Printf("删除视频 %d 点赞缓存失败: %v", videoID, err)
	}
}

func fillVideoLikeCounts(ctx context.Context, items []VideoListItem) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]uint, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	counts, err := sharedVideoCache.likeCounts(ctx, ids, getVideoLikeCounts)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Likes = counts[items[i].ID]
	}
	return nil
}

func (cache *redisVideoCache) likeCounts(ctx context.Context, ids []uint, load func(context.Context, []uint) (map[uint]int64, error)) (map[uint]int64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[uint]int64{}, nil
	}
	if cache == nil || time.Now().UnixNano() < cache.retryAfter.Load() {
		return load(ctx, ids)
	}
	waitCtx, cancel := context.WithTimeout(ctx, likeCountWaitTimeout)
	defer cancel()
	counts := make(map[uint]int64, len(ids))
	pending := ids
	for len(pending) > 0 {
		remaining, err := cache.readLikeCounts(waitCtx, pending, counts)
		if err == nil && len(remaining) > 0 {
			var loaded map[uint]int64
			loaded, err = cache.loadLockedLikeCounts(waitCtx, remaining, load)
			for id, count := range loaded {
				counts[id] = count
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if waitCtx.Err() != nil {
				return nil, ErrLikeCountsBusy
			}
			if !errors.Is(err, errLikeCacheUnavailable) {
				return nil, err
			}
			// A Redis outage keeps the existing database fallback behavior.
			loaded, err := load(ctx, pending)
			if err != nil {
				return nil, err
			}
			for id, count := range loaded {
				counts[id] = count
			}
			return counts, nil
		}
		pending = nil
		for _, id := range remaining {
			if _, ok := counts[id]; !ok {
				pending = append(pending, id)
			}
		}
		if len(pending) == 0 {
			return counts, nil
		}
		timer := time.NewTimer(likeCountPollInterval)
		select {
		case <-waitCtx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrLikeCountsBusy
		case <-timer.C:
		}
	}
	return counts, nil
}

func (cache *redisVideoCache) readLikeCounts(ctx context.Context, ids []uint, counts map[uint]int64) ([]uint, error) {
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = likeCountKey(id)
	}
	readCtx, cancel := context.WithTimeout(ctx, redisCacheTimeout)
	defer cancel()
	values, err := cache.client.MGet(readCtx, keys...).Result()
	if err != nil {
		return nil, cache.likeCacheError(err)
	}
	if len(values) != len(ids) {
		return nil, cache.likeCacheError(errors.New("unexpected Redis count response"))
	}
	missing := make([]uint, 0, len(ids))
	for i, value := range values {
		raw, _ := value.(string)
		count, err := strconv.ParseInt(raw, 10, 64)
		if err == nil && count >= 0 {
			counts[ids[i]] = count
		} else {
			missing = append(missing, ids[i])
		}
	}
	return missing, nil
}

func (cache *redisVideoCache) likeCacheError(err error) error {
	cache.backOff()
	return fmt.Errorf("%w: %v", errLikeCacheUnavailable, err)
}

func (cache *redisVideoCache) loadLockedLikeCounts(ctx context.Context, ids []uint, load func(context.Context, []uint) (map[uint]int64, error)) (map[uint]int64, error) {
	token := "loading:" + uuid.NewString()
	keys := likeCountKeysWithLocks(ids)
	reserveCtx, cancel := context.WithTimeout(ctx, redisCacheTimeout)
	owned, err := cache.client.Eval(reserveCtx, reserveLikeCounts, keys, token, likeCountLockTTL.Milliseconds()).Int64Slice()
	cancel()
	if err != nil {
		// A command can execute even when its response is lost or cancelled.
		cache.releaseCountLocks(keys, token)
		return nil, cache.likeCacheError(err)
	}
	if len(owned) == 0 {
		return nil, nil
	}
	ownedIDs := make([]uint, 0, len(owned))
	for _, index := range owned {
		ownedIDs = append(ownedIDs, ids[index-1])
	}
	defer cache.releaseCountLocks(likeCountKeysWithLocks(ownedIDs), token)
	queryCtx, cancel := context.WithTimeout(ctx, likeCountQueryTimeout)
	loaded, err := load(queryCtx, ownedIDs)
	queryErr := queryCtx.Err()
	cancel()
	if err != nil {
		return nil, err
	}
	if queryErr != nil {
		return nil, queryErr
	}
	// Missing/deleted videos retain the existing zero-count presentation.
	counts := make(map[uint]int64, len(ownedIDs))
	for _, id := range ownedIDs {
		counts[id] = loaded[id]
	}
	cache.refillCounts(ctx, token, ownedIDs, counts)
	return counts, nil
}

func likeCountKeysWithLocks(ids []uint) []string {
	keys := make([]string, 0, 2*len(ids))
	for _, id := range ids {
		key := likeCountKey(id)
		keys = append(keys, key, key+":lock")
	}
	return keys
}

func (cache *redisVideoCache) releaseCountLocks(keys []string, token string) {
	ctx, cancel := context.WithTimeout(context.Background(), redisCacheTimeout)
	defer cancel()
	if err := cache.client.Eval(ctx, releaseLikeCountLocks, keys, token).Err(); err != nil {
		cache.backOff()
		log.Printf("释放点赞计数回源锁失败: %v", err)
	}
}

func (cache *redisVideoCache) refillCounts(ctx context.Context, token string, ids []uint, counts map[uint]int64) {
	ttl := 5*time.Second + rand.N(3*time.Second)
	args := []any{token, ttl.Milliseconds()}
	for _, id := range ids {
		args = append(args, counts[id])
	}
	refillCtx, cancel := context.WithTimeout(ctx, redisCacheTimeout)
	defer cancel()
	if err := cache.client.Eval(refillCtx, refillLikeCounts, likeCountKeysWithLocks(ids), args...).Err(); err != nil && ctx.Err() == nil {
		cache.backOff()
	}
}
