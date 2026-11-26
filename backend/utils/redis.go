package utils

import (
	"context"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"slices"
	"sync/atomic"
	"time"

	"backend/config"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	redisVideoPrefix      = "cwatch:video-cache:v2:"
	redisVideoRevisionKey = redisVideoPrefix + "revision"
	redisCacheTimeout     = 150 * time.Millisecond
)

type redisCacheCommands interface {
	MGet(context.Context, ...string) *redis.SliceCmd
	Set(context.Context, string, interface{}, time.Duration) *redis.StatusCmd
	Del(context.Context, ...string) *redis.IntCmd
	Eval(context.Context, string, []string, ...interface{}) *redis.Cmd
}

type redisVideoCache struct {
	client     redisCacheCommands
	retryAfter atomic.Int64
}

type redisVideoSnapshot struct {
	Revision string        `json:"revision"`
	Data     videoSnapshot `json:"data"`
}

var sharedVideoCache *redisVideoCache

func InitRedisVideoCache() (func(), error) {
	url := config.Value("CWATCH_REDIS_URL", "")
	if url == "" {
		return func() {}, nil
	}
	options, err := redis.ParseURL(url)
	if err != nil {
		return nil, errors.New("CWATCH_REDIS_URL 无效，请使用 redis:// 或 rediss:// 连接地址")
	}
	options.DialTimeout = redisCacheTimeout
	options.ReadTimeout = redisCacheTimeout
	options.WriteTimeout = redisCacheTimeout
	options.PoolTimeout = redisCacheTimeout
	options.ContextTimeoutEnabled = true
	options.MaxRetries = -1
	client := redis.NewClient(options)
	sharedVideoCache = &redisVideoCache{client: client}
	return func() { _ = client.Close() }, nil
}

func (cache *redisVideoCache) get(parent context.Context, key, sourceKey string) (*videoSnapshot, string, string, bool) {
	if parent.Err() != nil || time.Now().UnixNano() < cache.retryAfter.Load() {
		return nil, "", "", false
	}
	ctx, cancel := context.WithTimeout(parent, redisCacheTimeout)
	defer cancel()
	keys := []string{redisVideoRevisionKey, redisVideoPrefix + key}
	if sourceKey != "" {
		keys = append(keys, sourceKey)
	}
	values, err := cache.client.MGet(ctx, keys...).Result()
	if err != nil {
		if parent.Err() == nil {
			cache.backOff()
		}
		return nil, "", "", false
	}

	if len(values) != len(keys) {
		return nil, "", "", false
	}
	revision, _ := values[0].(string)
	var source string
	if sourceKey != "" {
		source, _ = values[2].(string)
		// Hot details depend on both catalog changes and the published order.
		revision += "/" + source
	}
	payload, ok := values[1].(string)
	if !ok {
		return nil, revision, source, true
	}
	var entry redisVideoSnapshot
	if json.Unmarshal([]byte(payload), &entry) != nil || entry.Revision != revision || entry.Data.Items == nil {
		return nil, revision, source, true
	}
	return &entry.Data, revision, source, true
}

func (cache *redisVideoCache) put(parent context.Context, key, sourceKey, revision, token string, snapshot videoSnapshot) {
	snapshot.Items = slices.Clone(snapshot.Items)
	for i := range snapshot.Items {
		snapshot.Items[i].Likes = 0
	}
	payload, err := json.Marshal(redisVideoSnapshot{Revision: revision, Data: snapshot})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, redisCacheTimeout)
	defer cancel()
	ttl := 5*time.Second + rand.N(3*time.Second)
	keys := []string{redisVideoPrefix + key, redisVideoPrefix + key + ":lock", redisVideoRevisionKey}
	if sourceKey != "" {
		keys = append(keys, sourceKey)
	}
	if err := cache.client.Eval(ctx, refillVideoSnapshot, keys, token, revision, string(payload), ttl.Milliseconds()).Err(); err != nil {
		cache.backOff()
	}
}

func (cache *redisVideoCache) invalidate() {
	ctx, cancel := context.WithTimeout(context.Background(), redisCacheTimeout)
	defer cancel()
	if err := cache.client.Set(ctx, redisVideoRevisionKey, uuid.NewString(), 0).Err(); err != nil {
		cache.backOff()
	}
}

func (cache *redisVideoCache) backOff() {
	cache.retryAfter.Store(time.Now().Add(2 * time.Second).UnixNano())
}
