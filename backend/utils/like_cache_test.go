package utils

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestLikeCountsHotVideoConcurrent(t *testing.T) {
	commands := &testRedisCommands{values: map[string]string{likeCountKey(2): "0"}}
	// Separate cache instances model independent application processes.
	caches := []*redisVideoCache{{client: commands}, {client: commands}}
	var calls atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	load := func(ctx context.Context, ids []uint) (map[uint]int64, error) {
		if !slices.Equal(ids, []uint{1, 3}) {
			return nil, fmt.Errorf("expected one batch of misses, got %v", ids)
		}
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-finish:
			return map[uint]int64{1: 42, 3: 7}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var wg sync.WaitGroup
	results := make(chan error, 100)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			counts, err := caches[i%2].likeCounts(context.Background(), []uint{1, 2, 3}, load)
			if err == nil && (counts[1] != 42 || counts[2] != 0 || counts[3] != 7) {
				err = fmt.Errorf("unexpected counts %v", counts)
			}
			results <- err
		}()
	}
	<-started
	time.Sleep(75 * time.Millisecond)
	close(finish)
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("got %d database loads, want 1", calls.Load())
	}
}

func TestLikeCountLockReleasedOnLoadFailure(t *testing.T) {
	for _, mode := range []string{"database error", "cancel", "query timeout"} {
		t.Run(mode, func(t *testing.T) {
			commands := &testRedisCommands{values: map[string]string{}}
			cache := &redisVideoCache{client: commands}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("database failed")
			if mode == "cancel" {
				failure = context.Canceled
			}
			if mode == "query timeout" {
				failure = context.DeadlineExceeded
			}
			_, err := cache.likeCounts(ctx, []uint{1}, func(ctx context.Context, _ []uint) (map[uint]int64, error) {
				if mode == "cancel" {
					cancel()
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if mode == "query timeout" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return nil, failure
			})
			if !errors.Is(err, failure) {
				t.Fatalf("got %v, want %v", err, failure)
			}
			if len(commands.values) != 0 {
				t.Fatalf("failed loader left keys behind: %v", commands.values)
			}
			_, err = cache.likeCounts(context.Background(), []uint{1}, func(context.Context, []uint) (map[uint]int64, error) {
				return map[uint]int64{1: 5}, nil
			})
			if err != nil || commands.values[likeCountKey(1)] != "5" {
				t.Fatalf("retry failed: %v", err)
			}
		})
	}
}

func TestLikeCountWaitTimeoutAndCancel(t *testing.T) {
	for _, mode := range []string{"timeout", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			key := likeCountKey(1)
			commands := &testRedisCommands{values: map[string]string{key + ":lock": "other"}}
			cache := &redisVideoCache{client: commands}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				timer := time.AfterFunc(60*time.Millisecond, cancel)
				defer timer.Stop()
			}
			start := time.Now()
			_, err := cache.likeCounts(ctx, []uint{1}, func(context.Context, []uint) (map[uint]int64, error) {
				t.Error("waiter must not query the database")
				return nil, nil
			})
			want := ErrLikeCountsBusy
			if mode == "cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if time.Since(start) > likeCountWaitTimeout+500*time.Millisecond {
				t.Fatal("wait exceeded deadline")
			}
			if commands.values[key+":lock"] != "other" {
				t.Fatal("waiter deleted another owner's lock")
			}
		})
	}
}

func TestLikeCountInvalidationPreservesLock(t *testing.T) {
	commands := &testRedisCommands{values: map[string]string{}}
	cache := &redisVideoCache{client: commands}
	key := likeCountKey(1)
	var calls atomic.Int32
	started, finish := make(chan struct{}), make(chan struct{})
	load := func(ctx context.Context, _ []uint) (map[uint]int64, error) {
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-finish:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return map[uint]int64{1: 10}, nil
		}
		return map[uint]int64{1: 11}, nil
	}
	first := make(chan error, 1)
	go func() { _, err := cache.likeCounts(context.Background(), []uint{1}, load); first <- err }()
	<-started
	// The writer removes only the count/refill token after its commit.
	commands.Del(context.Background(), key)
	commands.mu.Lock()
	locked := commands.values[key+":lock"] != ""
	commands.mu.Unlock()
	if !locked {
		t.Fatal("invalidation removed the independent lock")
	}
	second := make(chan error, 1)
	go func() {
		counts, err := cache.likeCounts(context.Background(), []uint{1}, load)
		if err == nil && counts[1] != 11 {
			err = fmt.Errorf("waiter read stale count %v", counts)
		}
		second <- err
	}()
	time.Sleep(60 * time.Millisecond)
	if calls.Load() != 1 {
		t.Error("waiter queried before the old owner released its lock")
	}
	close(finish)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	if err := <-second; err != nil {
		t.Fatal(err)
	}
	if commands.values[key] != "11" {
		t.Fatal("stale refill survived invalidation")
	}
}

type fillBeforeLockCommands struct{ *testRedisCommands }

func (c fillBeforeLockCommands) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	if script == reserveLikeCounts {
		c.Set(ctx, keys[0], "9", time.Second)
	}
	return c.testRedisCommands.Eval(ctx, script, keys, args...)
}

func TestLikeCountDoubleCheckAndExpiredOwner(t *testing.T) {
	ctx := context.Background()
	commands := &testRedisCommands{values: map[string]string{}}
	cache := &redisVideoCache{client: fillBeforeLockCommands{commands}}
	counts, err := cache.likeCounts(ctx, []uint{1}, func(context.Context, []uint) (map[uint]int64, error) {
		t.Error("cache was filled before lock acquisition, database must be skipped")
		return nil, nil
	})
	if err != nil || counts[1] != 9 {
		t.Fatalf("double check failed: %v %v", counts, err)
	}
	keys := likeCountKeysWithLocks([]uint{1})
	commands.Del(ctx, keys...)
	commands.Eval(ctx, reserveLikeCounts, keys, "old", int64(1))
	time.Sleep(5 * time.Millisecond)
	commands.Eval(ctx, reserveLikeCounts, keys, "new", int64(3000))
	cache.refillCounts(ctx, "old", []uint{1}, map[uint]int64{1: 10})
	cache.releaseCountLocks(keys, "old")
	if commands.values[keys[0]] != "new" || commands.values[keys[1]] != "new" {
		t.Fatal("expired owner changed the new owner's keys")
	}
}

func TestLikeCountsLoadsOnlyOwnedVideos(t *testing.T) {
	ctx := context.Background()
	key := likeCountKey(1)
	commands := &testRedisCommands{values: map[string]string{key + ":lock": "other"}}
	cache := &redisVideoCache{client: commands}
	var calls atomic.Int32
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		time.Sleep(75 * time.Millisecond)
		commands.Set(ctx, key, "7", time.Second)
		commands.Del(ctx, key+":lock")
	}()
	counts, err := cache.likeCounts(ctx, []uint{1, 2, 3}, func(_ context.Context, ids []uint) (map[uint]int64, error) {
		calls.Add(1)
		if !slices.Equal(ids, []uint{2, 3}) {
			return nil, fmt.Errorf("loaded unlocked IDs %v", ids)
		}
		// Video 3 has disappeared from MySQL, its count remains zero.
		return map[uint]int64{2: 9}, nil
	})
	<-finished
	if err != nil || counts[1] != 7 || counts[2] != 9 || counts[3] != 0 || calls.Load() != 1 {
		t.Fatalf("partial ownership failed: %v, calls=%d, err=%v", counts, calls.Load(), err)
	}
	if commands.values[likeCountKey(3)] != "0" {
		t.Fatal("missing database row was not cached as zero")
	}
}

type failReserveCommands struct{ *testRedisCommands }

func (c failReserveCommands) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	result := c.testRedisCommands.Eval(ctx, script, keys, args...)
	if script == reserveLikeCounts {
		return redis.NewCmdResult(nil, errors.New("lost Redis reply"))
	}
	return result
}

func TestLikeCountsLostLockReplyFallsBack(t *testing.T) {
	commands := &testRedisCommands{values: map[string]string{}}
	cache := &redisVideoCache{client: failReserveCommands{commands}}
	var calls int
	counts, err := cache.likeCounts(context.Background(), []uint{1}, func(_ context.Context, ids []uint) (map[uint]int64, error) {
		calls++
		return map[uint]int64{1: 8}, nil
	})
	if err != nil || counts[1] != 8 || calls != 1 {
		t.Fatalf("fallback failed: %v %v", counts, err)
	}
	if len(commands.values) != 0 {
		t.Fatalf("lost reply leaked acquired locks: %v", commands.values)
	}
}
