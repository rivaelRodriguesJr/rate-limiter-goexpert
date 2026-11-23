package storage_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }

// contract runs the behavior every Store must satisfy.
func contract(t *testing.T, s storage.Store, advance func(time.Duration)) {
	ctx := context.Background()
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	for want := int64(1); want <= 3; want++ {
		if got, err := s.Incr(ctx, "c", time.Second); err != nil || got != want {
			t.Fatalf("incr got %d err %v, want %d", got, err, want)
		}
	}
	advance(1100 * time.Millisecond)
	if got, _ := s.Incr(ctx, "c", time.Second); got != 1 {
		t.Fatalf("counter should reset, got %d", got)
	}

	if ttl, err := s.BlockTTL(ctx, "b"); err != nil || ttl != 0 {
		t.Fatalf("unblocked ttl %v err %v", ttl, err)
	}
	if err := s.Block(ctx, "b", 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if ttl, _ := s.BlockTTL(ctx, "b"); ttl <= 0 || ttl > 2*time.Second {
		t.Fatalf("blocked ttl %v", ttl)
	}
	advance(2100 * time.Millisecond)
	if ttl, _ := s.BlockTTL(ctx, "b"); ttl != 0 {
		t.Fatalf("block should expire, ttl %v", ttl)
	}
}

func TestMemoryContract(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	contract(t, storage.NewMemory(clk), func(d time.Duration) { clk.t = clk.t.Add(d) })
}

func TestRedisContract(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	s := storage.NewRedis(storage.RedisOptions{Addr: addr, DB: 15})
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Ping(ctx); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	// Real time: keys use unique names per run and short TTLs.
	contract(t, s, time.Sleep)
}
