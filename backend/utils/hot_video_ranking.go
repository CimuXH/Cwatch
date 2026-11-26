package utils

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

const (
	hotVideoRankingKey      = redisVideoPrefix + "hot-ranking"
	hotVideoRankingSize     = 100
	hotVideoRankingInterval = time.Hour
	hotVideoRankingRetry    = time.Minute
)

const publishHotVideoRanking = `
if (redis.call('GET', KEYS[1]) or '') == ARGV[1] then
    redis.call('SET', KEYS[1], ARGV[2])
    return 1
end
return 0
`

type hotVideoRanking struct {
	GeneratedAt time.Time `json:"generated_at"`
	IDs         []uint    `json:"ids"`
}

func RunHotVideoRanking(ctx context.Context) {
	cache := sharedVideoCache
	if cache == nil {
		return
	}
	for ctx.Err() == nil {
		delay, err := cache.refreshHotVideoRanking(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("刷新热门榜失败: %v", err)
		}
		timer := time.NewTimer(min(delay, hotVideoRankingRetry))
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}

func (cache *redisVideoCache) readHotVideoRanking(ctx context.Context) (*hotVideoRanking, string, error) {
	readCtx, cancel := context.WithTimeout(ctx, redisCacheTimeout)
	defer cancel()
	values, err := cache.client.MGet(readCtx, hotVideoRankingKey).Result()
	if err != nil {
		if ctx.Err() == nil {
			cache.backOff()
		}
		return nil, "", err
	}
	raw, _ := values[0].(string)
	return parseHotVideoRanking(raw), raw, nil
}

func parseHotVideoRanking(raw string) *hotVideoRanking {
	var ranking hotVideoRanking
	if raw == "" || json.Unmarshal([]byte(raw), &ranking) != nil || ranking.GeneratedAt.IsZero() || ranking.IDs == nil {
		return nil
	}
	return &ranking
}

func (cache *redisVideoCache) refreshHotVideoRanking(ctx context.Context) (time.Duration, error) {
	current, raw, err := cache.readHotVideoRanking(ctx)
	if err != nil {
		return hotVideoRankingRetry, err
	}
	if current != nil {
		if delay := time.Until(current.GeneratedAt.Add(hotVideoRankingInterval)); delay > 0 {
			return delay, nil
		}
	}
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	ranking := hotVideoRanking{GeneratedAt: time.Now().UTC(), IDs: make([]uint, 0, hotVideoRankingSize)}
	if err := hotVideosQuery(queryCtx, hotVideoRankingSize).Pluck("id", &ranking.IDs).Error; err != nil {
		return hotVideoRankingRetry, err
	}
	payload, err := json.Marshal(ranking)
	if err != nil {
		return hotVideoRankingRetry, err
	}
	writeCtx, cancelWrite := context.WithTimeout(ctx, redisCacheTimeout)
	defer cancelWrite()
	// Preserve the last published ranking on failure and let only one concurrent refresh win.
	published, err := cache.client.Eval(writeCtx, publishHotVideoRanking, []string{hotVideoRankingKey}, raw, string(payload)).Int()
	if err != nil || published != 1 {
		return hotVideoRankingRetry, err
	}
	return time.Until(ranking.GeneratedAt.Add(hotVideoRankingInterval)), nil
}

func loadRankedVideos(ctx context.Context, ids []uint, limit int) ([]VideoListItem, error) {
	ordered := make([]VideoListItem, 0, min(len(ids), limit))
	if len(ids) == 0 {
		return ordered, nil
	}
	items, err := loadVideoItems(visibleVideos(ctx).Where("id IN ?", ids))
	if err != nil {
		return nil, err
	}
	byID := make(map[uint]VideoListItem, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	for _, id := range ids {
		if item, ok := byID[id]; ok {
			ordered = append(ordered, item)
			if len(ordered) == limit {
				break
			}
		}
	}
	return ordered, nil
}
