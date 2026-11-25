package storage

import (
	"context"
	"os"
	"testing"
	"time"
)

type fakeClock struct {
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	return f.now
}

func TestMemoryCounter(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	store := NewMemory(clock)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		n, _ := store.Incr(ctx, "a", time.Second)
		if n != int64(i) {
			t.Fatalf("got %d, want %d", n, i)
		}
	}

	// after the window the counter starts again
	clock.now = clock.now.Add(1100 * time.Millisecond)
	n, _ := store.Incr(ctx, "a", time.Second)
	if n != 1 {
		t.Fatalf("got %d, want 1", n)
	}
}

func TestMemoryBlock(t *testing.T) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	store := NewMemory(clock)
	ctx := context.Background()

	ttl, _ := store.BlockTTL(ctx, "a")
	if ttl != 0 {
		t.Fatalf("should not be blocked, ttl = %v", ttl)
	}

	store.Block(ctx, "a", 10*time.Second)
	ttl, _ = store.BlockTTL(ctx, "a")
	if ttl <= 0 {
		t.Fatalf("should be blocked, ttl = %v", ttl)
	}

	clock.now = clock.now.Add(11 * time.Second)
	ttl, _ = store.BlockTTL(ctx, "a")
	if ttl != 0 {
		t.Fatalf("block should be over, ttl = %v", ttl)
	}
}

// This test needs a real Redis. If we can't connect, we skip it.
func TestRedis(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	store := NewRedis(RedisOptions{Addr: addr, DB: 15})
	defer store.Close()

	ctx := context.Background()
	if err := store.Ping(ctx); err != nil {
		t.Skip("redis is not running at", addr)
	}

	// the key has the time in it, so old runs don't affect this one
	key := "test:" + time.Now().Format("150405.000000")

	for i := 1; i <= 3; i++ {
		n, err := store.Incr(ctx, key, 300*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		if n != int64(i) {
			t.Fatalf("got %d, want %d", n, i)
		}
	}

	time.Sleep(400 * time.Millisecond)
	n, _ := store.Incr(ctx, key, 300*time.Millisecond)
	if n != 1 {
		t.Fatalf("counter should restart, got %d", n)
	}

	blockKey := key + ":block"
	ttl, _ := store.BlockTTL(ctx, blockKey)
	if ttl != 0 {
		t.Fatalf("should not be blocked, ttl = %v", ttl)
	}

	if err := store.Block(ctx, blockKey, 300*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	ttl, _ = store.BlockTTL(ctx, blockKey)
	if ttl <= 0 {
		t.Fatalf("should be blocked, ttl = %v", ttl)
	}

	time.Sleep(400 * time.Millisecond)
	ttl, _ = store.BlockTTL(ctx, blockKey)
	if ttl != 0 {
		t.Fatalf("block should have expired, ttl = %v", ttl)
	}
}
