package utils

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisCacheAside(t *testing.T) {
	commands := &testRedisCommands{values: map[string]string{}}
	previous := sharedVideoCache
	sharedVideoCache = &redisVideoCache{client: commands}
	t.Cleanup(func() {
		sharedVideoCache = previous
		InvalidateVideoCache()
	})

	const key = "test:home"
	loads := 0
	load := func(context.Context, string) ([]VideoListItem, int64, error) {
		loads++
		return []VideoListItem{{ID: uint(loads), Likes: 15}}, 1, nil
	}
	read := func(wantID uint, wantLoads int) {
		t.Helper()
		items, total, fromCache, err := cachedVideos(context.Background(), key, "", load)
		if err != nil || total != 1 || len(items) != 1 || items[0].ID != wantID || loads != wantLoads {
			t.Fatalf("items=%+v total=%d loads=%d err=%v", items, total, loads, err)
		}
		if items[0].IsLiked {
			t.Fatal("per-user changes leaked into the shared snapshot")
		}
		if fromCache && items[0].Likes != 0 || !fromCache && items[0].Likes != 15 {
			t.Fatalf("metadata snapshots should strip likes without changing database results: %+v", items[0])
		}
		items[0].IsLiked = true
	}

	read(1, 1)
	stale := commands.values[redisVideoPrefix+key]
	if stale == "" || commands.ttl <= 0 {
		t.Fatal("expected a Redis snapshot with a TTL")
	}
	read(1, 1)

	InvalidateVideoCache()
	commands.values[redisVideoPrefix+key] = stale
	read(2, 2)

	commands.err = errors.New("Redis unavailable")
	read(3, 3)
	read(4, 4)
	sharedVideoCache = nil
	read(5, 5)
	read(6, 6)
}

func TestPublishedRankingInvalidatesHotDetails(t *testing.T) {
	ctx := context.Background()
	commands := &testRedisCommands{values: map[string]string{hotVideoRankingKey: "first"}}
	previous := sharedVideoCache
	sharedVideoCache = &redisVideoCache{client: commands}
	t.Cleanup(func() { sharedVideoCache = previous })
	var calls []string
	load := func(_ context.Context, source string) ([]VideoListItem, int64, error) {
		calls = append(calls, source)
		return []VideoListItem{{ID: uint(len(calls))}}, 1, nil
	}
	for _, ranking := range []string{"first", "first", "second"} {
		commands.values[hotVideoRankingKey] = ranking
		if _, _, _, err := cachedVideos(ctx, "hot:20", hotVideoRankingKey, load); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(calls, []string{"first", "second"}) {
		t.Fatalf("unexpected rank sources %v", calls)
	}
}

func TestLikeCountsBatchAndStaleRefill(t *testing.T) {
	ctx := context.Background()
	commands := &testRedisCommands{values: map[string]string{likeCountKey(1): "0"}}
	previous := sharedVideoCache
	cache := &redisVideoCache{client: commands}
	sharedVideoCache = cache
	t.Cleanup(func() { sharedVideoCache = previous })

	load := func(_ context.Context, ids []uint) (map[uint]int64, error) {
		if !slices.Equal(ids, []uint{2, 3}) {
			t.Fatalf("loaded IDs %v, want only the two misses", ids)
		}
		// A write commits after this reader fetched the old count for video 2.
		invalidateLikeCount(2)
		return map[uint]int64{2: 10, 3: 20}, nil
	}
	got, err := cache.likeCounts(ctx, []uint{1, 2, 3}, load)
	if err != nil || got[1] != 0 || got[2] != 10 || got[3] != 20 {
		t.Fatalf("unexpected counts %v (%v)", got, err)
	}
	if _, ok := commands.values[likeCountKey(2)]; ok {
		t.Fatal("an old reader refilled the deleted cache key")
	}
	got, err = cache.likeCounts(ctx, []uint{1, 2, 3}, func(_ context.Context, ids []uint) (map[uint]int64, error) {
		if !slices.Equal(ids, []uint{2}) {
			t.Fatalf("loaded IDs %v, want only the invalidated video", ids)
		}
		return map[uint]int64{2: 11}, nil
	})
	if err != nil || got[2] != 11 || commands.values[likeCountKey(2)] != "11" {
		t.Fatalf("expected refreshed count, got %v (%v), Redis %v", got, err, commands.values)
	}
	commands.err = errors.New("Redis unavailable")
	got, err = cache.likeCounts(ctx, []uint{1, 2, 3}, func(_ context.Context, ids []uint) (map[uint]int64, error) {
		if !slices.Equal(ids, []uint{1, 2, 3}) {
			t.Fatalf("database fallback should load all IDs, got %v", ids)
		}
		return map[uint]int64{1: 0, 2: 12, 3: 21}, nil
	})
	if err != nil || got[2] != 12 || got[3] != 21 {
		t.Fatalf("unexpected fallback counts %v (%v)", got, err)
	}
}

type testRedisCommands struct {
	mu      sync.Mutex
	values  map[string]string
	expires map[string]time.Time
	ttl     time.Duration
	err     error
}

func (c *testRedisCommands) MGet(_ context.Context, keys ...string) *redis.SliceCmd {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireKeys()
	values := make([]any, len(keys))
	for i, key := range keys {
		if value, ok := c.values[key]; ok {
			values[i] = value
		}
	}
	return redis.NewSliceResult(values, c.err)
}

func (c *testRedisCommands) Set(_ context.Context, key string, value any, ttl time.Duration) *redis.StatusCmd {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch value := value.(type) {
	case []byte:
		c.values[key] = string(value)
	case string:
		c.values[key] = value
	}
	c.ttl = ttl
	return redis.NewStatusResult("OK", c.err)
}

func (c *testRedisCommands) Del(_ context.Context, keys ...string) *redis.IntCmd {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, key := range keys {
		delete(c.values, key)
	}
	return redis.NewIntResult(int64(len(keys)), c.err)
}

func (c *testRedisCommands) Eval(_ context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.expireKeys()
	if c.err != nil {
		return redis.NewCmdResult(nil, c.err)
	}
	switch script {
	case reserveVideoSnapshot:
		if _, ok := c.values[keys[0]]; ok {
			return redis.NewCmdResult(int64(0), nil)
		}
		c.values[keys[0]] = args[0].(string)
		c.setExpiry(keys[0], args[1].(int64))
	case refillVideoSnapshot:
		revision := c.values[keys[2]]
		if len(keys) == 4 {
			revision += "/" + c.values[keys[3]]
		}
		if c.values[keys[1]] != args[0] || revision != args[1] {
			return redis.NewCmdResult(int64(0), nil)
		}
		c.values[keys[0]] = args[2].(string)
		c.ttl = time.Duration(args[3].(int64)) * time.Millisecond
		c.setExpiry(keys[0], args[3].(int64))
	case releaseVideoSnapshotLock:
		if c.values[keys[0]] == args[0] {
			delete(c.values, keys[0])
		}
	case reserveLikeCounts:
		owned := []any{}
		for i := 0; i < len(keys); i += 2 {
			key, lock := keys[i], keys[i+1]
			if _, ok := c.values[lock]; ok {
				continue
			}
			if value, err := strconv.ParseInt(c.values[key], 10, 64); err == nil && value >= 0 {
				continue
			}
			c.values[lock] = args[0].(string)
			c.values[key] = args[0].(string)
			if c.expires == nil {
				c.expires = make(map[string]time.Time)
			}
			deadline := time.Now().Add(time.Duration(args[1].(int64)) * time.Millisecond)
			c.expires[key], c.expires[lock] = deadline, deadline
			owned = append(owned, int64(i/2+1))
		}
		return redis.NewCmdResult(owned, nil)
	case refillLikeCounts:
		for i := 0; i < len(keys); i += 2 {
			if c.values[keys[i]] == args[0] && c.values[keys[i+1]] == args[0] {
				c.values[keys[i]] = fmt.Sprint(args[i/2+2])
				c.expires[keys[i]] = time.Now().Add(time.Duration(args[1].(int64)) * time.Millisecond)
			}
		}
	case releaseLikeCountLocks:
		for i := 0; i < len(keys); i += 2 {
			if c.values[keys[i+1]] == args[0] {
				delete(c.values, keys[i+1])
				if c.values[keys[i]] == args[0] {
					delete(c.values, keys[i])
				}
			}
		}
	default:
		return redis.NewCmdResult(nil, errors.New("unexpected Redis script"))
	}
	return redis.NewCmdResult(int64(1), c.err)
}

func (c *testRedisCommands) setExpiry(key string, milliseconds int64) {
	if c.expires == nil {
		c.expires = make(map[string]time.Time)
	}
	c.expires[key] = time.Now().Add(time.Duration(milliseconds) * time.Millisecond)
}

func (c *testRedisCommands) expireKeys() {
	for key, deadline := range c.expires {
		if !time.Now().Before(deadline) {
			delete(c.values, key)
			delete(c.expires, key)
		}
	}
}
