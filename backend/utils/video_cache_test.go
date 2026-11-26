package utils

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestVideoSnapshotConcurrentReaders(t *testing.T) {
	commands := &testRedisCommands{values: map[string]string{}}
	caches := []*redisVideoCache{{client: commands}, {client: commands}}
	var calls atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	load := func(ctx context.Context, _ string) ([]VideoListItem, int64, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-finish:
			return []VideoListItem{{ID: 1, Likes: 9}}, 1, nil
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		}
	}
	var wg sync.WaitGroup
	errors := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, total, _, err := caches[i%2].cachedVideos(context.Background(), "list:1:12", "", load)
			if err == nil && (len(items) != 1 || items[0].ID != 1 || total != 1 || items[0].IsLiked) {
				err = fmt.Errorf("invalid snapshot %v", items)
			}
			if err == nil {
				items[0].IsLiked = true
			}
			errors <- err
		}()
	}
	<-started
	time.Sleep(75 * time.Millisecond)
	close(finish)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("got %d database loads, want 1", calls.Load())
	}
}

func TestVideoSnapshotFailureReleasesLock(t *testing.T) {
	for _, mode := range []string{"database error", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			commands := &testRedisCommands{values: map[string]string{}}
			cache := &redisVideoCache{client: commands}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("database failed")
			if mode == "cancel" {
				failure = context.Canceled
			}
			_, _, _, err := cache.cachedVideos(ctx, "list:1:12", "", func(queryCtx context.Context, _ string) ([]VideoListItem, int64, error) {
				deadline, ok := queryCtx.Deadline()
				if !ok || time.Until(deadline) > videoLoadQueryTimeout || videoLoadQueryTimeout >= videoLoadLockTTL {
					t.Error("query must time out before its lock")
				}
				if mode == "cancel" {
					cancel()
					<-queryCtx.Done()
				}
				return nil, 0, failure
			})
			if !errors.Is(err, failure) {
				t.Fatalf("got %v, want %v", err, failure)
			}
			if len(commands.values) != 0 {
				t.Fatalf("lock leaked: %v", commands.values)
			}
			_, _, _, err = cache.cachedVideos(context.Background(), "list:1:12", "", func(context.Context, string) ([]VideoListItem, int64, error) { return nil, 0, nil })
			if err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			_, _, hit, err := cache.cachedVideos(context.Background(), "list:1:12", "", func(context.Context, string) ([]VideoListItem, int64, error) {
				t.Error("empty lists should be cached")
				return nil, 0, nil
			})
			if err != nil || !hit {
				t.Fatalf("empty snapshot not cached: %v", err)
			}
		})
	}
}

func TestVideoSnapshotWaitTimeoutAndCancel(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			key := redisVideoPrefix + "list:1:12:lock"
			commands := &testRedisCommands{values: map[string]string{key: "other"}}
			cache := &redisVideoCache{client: commands}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				timer := time.AfterFunc(50*time.Millisecond, cancel)
				defer timer.Stop()
			}
			_, _, _, err := cache.cachedVideos(ctx, "list:1:12", "", func(context.Context, string) ([]VideoListItem, int64, error) {
				t.Error("waiting reader must not query MySQL")
				return nil, 0, nil
			})
			want := ErrVideoListBusy
			if mode == "cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if commands.values[key] != "other" {
				t.Fatal("waiter removed the owner's lock")
			}
		})
	}
}

func TestVideoSnapshotInvalidationDuringLoad(t *testing.T) {
	for _, sourceKey := range []string{"", hotVideoRankingKey} {
		t.Run(sourceKey, func(t *testing.T) {
			commands := &testRedisCommands{values: map[string]string{}}
			cache := &redisVideoCache{client: commands}
			var calls int
			load := func(ctx context.Context, source string) ([]VideoListItem, int64, error) {
				calls++
				if calls == 1 {
					if sourceKey == "" {
						cache.invalidate()
					} else {
						commands.Set(ctx, sourceKey, "new-ranking", 0)
					}
				} else if sourceKey != "" && source != "new-ranking" {
					t.Errorf("loaded old rank %q", source)
				}
				return []VideoListItem{{ID: uint(calls)}}, 1, nil
			}
			for i := 0; i < 2; i++ {
				if _, _, _, err := cache.cachedVideos(context.Background(), "list:1:12", sourceKey, load); err != nil {
					t.Fatal(err)
				}
				if i == 0 && commands.values[redisVideoPrefix+"list:1:12"] != "" {
					t.Fatal("old data refilled after invalidation")
				}
			}
			if calls != 2 {
				t.Fatalf("got %d loads, want fresh data after invalidation", calls)
			}
		})
	}
}

type videoLockCommands struct {
	*testRedisCommands
	beforeLock func()
	lostReply  bool
}

func (c videoLockCommands) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	if script == reserveVideoSnapshot && c.beforeLock != nil {
		c.beforeLock()
	}
	result := c.testRedisCommands.Eval(ctx, script, keys, args...)
	if script == reserveVideoSnapshot && c.lostReply {
		return redis.NewCmdResult(nil, errors.New("lost lock reply"))
	}
	return result
}

func TestVideoSnapshotDoubleCheck(t *testing.T) {
	ctx := context.Background()
	commands := &testRedisCommands{values: map[string]string{}}
	key := "list:1:12"
	commandsWithHook := videoLockCommands{testRedisCommands: commands, beforeLock: func() {
		commands.Set(ctx, redisVideoPrefix+key, `{"revision":"","data":{"videos":[{"id":9}],"total":1}}`, time.Second)
	}}
	cache := &redisVideoCache{client: commandsWithHook}
	items, _, hit, err := cache.cachedVideos(ctx, key, "", func(context.Context, string) ([]VideoListItem, int64, error) {
		t.Error("double check should skip MySQL")
		return nil, 0, nil
	})
	if err != nil || !hit || len(items) != 1 || items[0].ID != 9 {
		t.Fatalf("double check failed: %v %v", items, err)
	}
}

func TestVideoSnapshotExpiredOwnerAndRedisFailure(t *testing.T) {
	ctx := context.Background()
	commands := &testRedisCommands{values: map[string]string{}}
	cache := &redisVideoCache{client: commands}
	key := "list:1:12"
	lock := redisVideoPrefix + key + ":lock"
	commands.Eval(ctx, reserveVideoSnapshot, []string{lock}, "old", int64(1))
	time.Sleep(5 * time.Millisecond)
	commands.Eval(ctx, reserveVideoSnapshot, []string{lock}, "new", int64(5000))
	cache.put(ctx, key, "", "", "old", videoSnapshot{Items: []VideoListItem{{ID: 1}}, Total: 1})
	cache.releaseVideoLock(lock, "old")
	if commands.values[lock] != "new" || commands.values[redisVideoPrefix+key] != "" {
		t.Fatal("expired owner modified the new owner's data")
	}
	commands.Del(ctx, lock)
	cache.client = videoLockCommands{testRedisCommands: commands, lostReply: true}
	var calls int
	_, _, _, err := cache.cachedVideos(ctx, key, "", func(context.Context, string) ([]VideoListItem, int64, error) {
		calls++
		return []VideoListItem{{ID: 2}}, 1, nil
	})
	if err != nil || calls != 1 || len(commands.values) != 0 {
		t.Fatalf("Redis fallback or lock cleanup failed: %v, calls=%d, keys=%v", err, calls, commands.values)
	}
}

func TestVideoSnapshotDifferentPagesLoadIndependently(t *testing.T) {
	commands := &testRedisCommands{values: map[string]string{}}
	cache := &redisVideoCache{client: commands}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	started := make(chan struct{}, 3)
	finish := make(chan struct{})
	results := make(chan error, 3)
	for _, key := range []string{"list:1:12", "list:2:12", "list:1:24"} {
		go func() {
			_, _, _, err := cache.cachedVideos(ctx, key, "", func(ctx context.Context, _ string) ([]VideoListItem, int64, error) {
				started <- struct{}{}
				select {
				case <-finish:
					return []VideoListItem{}, 0, nil
				case <-ctx.Done():
					return nil, 0, ctx.Err()
				}
			})
			results <- err
		}()
	}
	var failure error
	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			failure = fmt.Errorf("page loads blocked one another: %w", ctx.Err())
		}
	}
	close(finish)
	for i := 0; i < 3; i++ {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
	if failure != nil {
		t.Fatal(failure)
	}
}
