package limiter_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func setup(ipLimit int, tokenLimit int, tokens map[string]int) (*limiter.Limiter, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	l := limiter.New(storage.NewMemory(clk), limiter.Config{
		IPLimit: ipLimit, TokenLimit: tokenLimit, TokenLimits: tokens,
		BlockDuration: 5 * time.Minute, FailOpen: true,
	}, nil)
	return l, clk
}

func allowN(l *limiter.Limiter, ip, tok string, n int) (allowed int) {
	for i := 0; i < n; i++ {
		if l.Allow(context.Background(), ip, tok).Allowed {
			allowed++
		}
	}
	return
}

func TestUnderLimitAllowedThenDenied(t *testing.T) {
	l, _ := setup(3, 10, nil)
	if got := allowN(l, "1.1.1.1", "", 3); got != 3 {
		t.Fatalf("allowed %d, want 3", got)
	}
	d := l.Allow(context.Background(), "1.1.1.1", "")
	if d.Allowed || d.RetryAfter <= 0 {
		t.Fatalf("4th request should be denied with RetryAfter, got %+v", d)
	}
}

func TestBlockPersistsThenExpires(t *testing.T) {
	l, clk := setup(1, 10, nil)
	allowN(l, "1.1.1.1", "", 2)  // exceed -> blocked
	clk.Advance(2 * time.Minute) // counter window long gone, still blocked
	if l.Allow(context.Background(), "1.1.1.1", "").Allowed {
		t.Fatal("should still be blocked")
	}
	clk.Advance(3*time.Minute + time.Second)
	if !l.Allow(context.Background(), "1.1.1.1", "").Allowed {
		t.Fatal("block should have expired")
	}
}

func TestIndependentIPs(t *testing.T) {
	l, _ := setup(2, 10, nil)
	allowN(l, "1.1.1.1", "", 5)
	if got := allowN(l, "2.2.2.2", "", 2); got != 2 {
		t.Fatalf("other IP affected: allowed %d", got)
	}
}

func TestIndependentTokens(t *testing.T) {
	l, _ := setup(1, 2, nil)
	allowN(l, "1.1.1.1", "a", 5)
	if got := allowN(l, "1.1.1.1", "b", 2); got != 2 {
		t.Fatalf("other token affected: allowed %d", got)
	}
}

func TestTokenPrecedenceOverIP(t *testing.T) {
	l, _ := setup(2, 2, map[string]int{"abc123": 5})
	if got := allowN(l, "1.1.1.1", "abc123", 6); got != 5 {
		t.Fatalf("token allowed %d, want 5", got)
	}
	// Without a token the same IP is limited at the IP limit (and not blocked by token usage).
	if got := allowN(l, "1.1.1.1", "", 4); got != 2 {
		t.Fatalf("ip allowed %d, want 2", got)
	}
}

func TestDefaultTokenLimit(t *testing.T) {
	l, _ := setup(1, 4, map[string]int{"vip": 9})
	if got := allowN(l, "1.1.1.1", "unknown", 6); got != 4 {
		t.Fatalf("allowed %d, want 4", got)
	}
}

func TestCounterResetsAfterWindow(t *testing.T) {
	l, clk := setup(2, 10, nil)
	if got := allowN(l, "1.1.1.1", "", 2); got != 2 {
		t.Fatal("setup")
	}
	clk.Advance(limiter.Window + time.Millisecond)
	if got := allowN(l, "1.1.1.1", "", 2); got != 2 {
		t.Fatalf("after window allowed %d, want 2", got)
	}
}

func TestConcurrencyExactLimit(t *testing.T) {
	l, _ := setup(50, 10, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow(context.Background(), "9.9.9.9", "").Allowed {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 50 {
		t.Fatalf("allowed %d, want exactly 50", allowed)
	}
}

type failingStore struct{ storage.Store }

func (failingStore) BlockTTL(context.Context, string) (time.Duration, error) {
	return 0, context.DeadlineExceeded
}

func TestFailOpenAndClosed(t *testing.T) {
	for _, open := range []bool{true, false} {
		l := limiter.New(failingStore{}, limiter.Config{IPLimit: 1, FailOpen: open}, nil)
		if got := l.Allow(context.Background(), "1.1.1.1", "").Allowed; got != open {
			t.Fatalf("failOpen=%v: allowed=%v", open, got)
		}
	}
}
