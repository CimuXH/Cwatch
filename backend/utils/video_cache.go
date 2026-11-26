package utils

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type videoSnapshot struct {
	Items []VideoListItem `json:"videos"`
	Total int64           `json:"total"`
}

const (
	videoLoadLockTTL      = 5 * time.Second
	videoLoadQueryTimeout = 3 * time.Second
	videoLoadWaitTimeout  = 4 * time.Second
	videoLoadPollInterval = 25 * time.Millisecond
)

var (
	videoLoadSlots           = make(chan struct{}, 6)
	ErrVideoListBusy         = errors.New("视频列表暂时繁忙，请稍后重试")
	errVideoCacheUnavailable = errors.New("视频列表缓存不可用")
)

const reserveVideoSnapshot = `
if redis.call('SET', KEYS[1], ARGV[1], 'NX', 'PX', ARGV[2]) then
    return 1
end
return 0
`

const refillVideoSnapshot = `
local revision = redis.call('GET', KEYS[3]) or ''
if #KEYS == 4 then
    revision = revision .. '/' .. (redis.call('GET', KEYS[4]) or '')
end
if redis.call('GET', KEYS[2]) == ARGV[1] and revision == ARGV[2] then
    redis.call('SET', KEYS[1], ARGV[3], 'PX', ARGV[4])
    return 1
end
return 0
`

const releaseVideoSnapshotLock = `
if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end
return 0
`

func InvalidateVideoCache() {
	if sharedVideoCache != nil {
		sharedVideoCache.invalidate()
	}
}

func CachedVideoList(ctx context.Context, page, pageSize int) ([]VideoListItem, int64, error) {
	if page > 3 {
		return GetVideoList(ctx, page, pageSize, 0)
	}
	items, total, fromCache, err := cachedVideos(ctx, fmt.Sprintf("list:%d:%d", page, pageSize), "", func(ctx context.Context, _ string) ([]VideoListItem, int64, error) {
		return GetVideoList(ctx, page, pageSize, 0)
	})
	if fromCache {
		err = fillVideoLikeCounts(ctx, items)
	}
	return items, total, err
}

func CachedHotVideos(ctx context.Context, limit int) ([]VideoListItem, int64, error) {
	items, total, fromCache, err := cachedVideos(ctx, fmt.Sprintf("hot:%d", limit), hotVideoRankingKey, func(ctx context.Context, source string) ([]VideoListItem, int64, error) {
		ranking := parseHotVideoRanking(source)
		if ranking == nil {
			items, err := GetHotVideos(ctx, limit)
			return items, int64(len(items)), err
		}
		items, err := loadRankedVideos(ctx, ranking.IDs, limit)
		return items, int64(len(items)), err
	})
	if fromCache {
		err = fillVideoLikeCounts(ctx, items)
	}
	return items, total, err
}

func cachedVideos(ctx context.Context, key, sourceKey string, load func(context.Context, string) ([]VideoListItem, int64, error)) ([]VideoListItem, int64, bool, error) {
	return sharedVideoCache.cachedVideos(ctx, key, sourceKey, load)
}

func (cache *redisVideoCache) cachedVideos(ctx context.Context, key, sourceKey string, load func(context.Context, string) ([]VideoListItem, int64, error)) ([]VideoListItem, int64, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, false, err
	}
	if cache == nil {
		return loadVideoSnapshot(ctx, "", load)
	}
	waitCtx, cancel := context.WithTimeout(ctx, videoLoadWaitTimeout)
	defer cancel()
	for {
		snapshot, _, source, available := cache.get(waitCtx, key, sourceKey)
		if err := ctx.Err(); err != nil {
			return nil, 0, false, err
		}
		if waitCtx.Err() != nil {
			return nil, 0, false, ErrVideoListBusy
		}
		if !available {
			return loadVideoSnapshot(ctx, source, load)
		}
		if snapshot != nil {
			return snapshot.Items, snapshot.Total, true, nil
		}
		snapshot, fromCache, err := cache.loadLockedVideos(waitCtx, key, sourceKey, load)
		if ctx.Err() != nil {
			return nil, 0, false, ctx.Err()
		}
		if waitCtx.Err() != nil {
			return nil, 0, false, ErrVideoListBusy
		}
		if errors.Is(err, errVideoCacheUnavailable) {
			return loadVideoSnapshot(ctx, source, load)
		}
		if err != nil {
			return nil, 0, false, err
		}
		if snapshot != nil {
			return snapshot.Items, snapshot.Total, fromCache, nil
		}
		timer := time.NewTimer(videoLoadPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, 0, false, ctx.Err()
		case <-waitCtx.Done():
			timer.Stop()
			return nil, 0, false, ErrVideoListBusy
		case <-timer.C:
		}
	}
}

func loadVideoSnapshot(ctx context.Context, source string, load func(context.Context, string) ([]VideoListItem, int64, error)) ([]VideoListItem, int64, bool, error) {
	select {
	case videoLoadSlots <- struct{}{}:
		defer func() { <-videoLoadSlots }()
	default:
		return nil, 0, false, ErrVideoListBusy
	}
	queryCtx, cancel := context.WithTimeout(ctx, videoLoadQueryTimeout)
	defer cancel()
	items, total, err := load(queryCtx, source)
	if err == nil {
		err = queryCtx.Err()
	}
	if err != nil {
		return nil, 0, false, err
	}
	if items == nil {
		items = []VideoListItem{}
	}
	return items, total, false, nil
}

func (cache *redisVideoCache) loadLockedVideos(ctx context.Context, key, sourceKey string, load func(context.Context, string) ([]VideoListItem, int64, error)) (*videoSnapshot, bool, error) {
	lockKey := redisVideoPrefix + key + ":lock"
	token := uuid.NewString()
	lockCtx, cancel := context.WithTimeout(ctx, redisCacheTimeout)
	acquired, err := cache.client.Eval(lockCtx, reserveVideoSnapshot, []string{lockKey}, token, videoLoadLockTTL.Milliseconds()).Int()
	cancel()
	if err != nil {
		cache.releaseVideoLock(lockKey, token)
		cache.backOff()
		return nil, false, errVideoCacheUnavailable
	}
	if acquired == 0 {
		return nil, false, nil
	}
	defer cache.releaseVideoLock(lockKey, token)
	// Another owner may have filled the snapshot between our first read and SET NX.
	snapshot, revision, source, available := cache.get(ctx, key, sourceKey)
	if !available {
		return nil, false, errVideoCacheUnavailable
	}
	if snapshot != nil {
		return snapshot, true, nil
	}
	items, total, _, err := loadVideoSnapshot(ctx, source, load)
	if err != nil {
		return nil, false, err
	}
	snapshot = &videoSnapshot{Items: items, Total: total}
	cache.put(ctx, key, sourceKey, revision, token, *snapshot)
	return snapshot, false, nil
}

func (cache *redisVideoCache) releaseVideoLock(key, token string) {
	// Release even if the original HTTP request was cancelled.
	ctx, cancel := context.WithTimeout(context.Background(), redisCacheTimeout)
	defer cancel()
	if err := cache.client.Eval(ctx, releaseVideoSnapshotLock, []string{key}, token).Err(); err != nil {
		cache.backOff()
	}
}
