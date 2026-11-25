package limiter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

// fakeClock lets the tests move the time, so we don't need time.Sleep.
type fakeClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	return f.now
}

func (f *fakeClock) Add(d time.Duration) {
	f.mutex.Lock()
	defer f.mutex.Unlock()
	f.now = f.now.Add(d)
}

// newTestLimiter makes a limiter that uses the memory store.
func newTestLimiter(ipLimit int, tokenLimit int, tokens map[string]int) (*Limiter, *fakeClock) {
	clock := &fakeClock{now: time.Unix(1000, 0)}
	store := storage.NewMemory(clock)
	l := New(store, Config{
		IPLimit:       ipLimit,
		TokenLimit:    tokenLimit,
		TokenLimits:   tokens,
		BlockDuration: 5 * time.Minute,
		FailOpen:      true,
	})
	return l, clock
}

// countAllowed sends n requests and tells how many were allowed.
func countAllowed(l *Limiter, ip string, token string, n int) int {
	allowed := 0
	for i := 0; i < n; i++ {
		ok, _ := l.Allow(context.Background(), ip, token)
		if ok {
			allowed++
		}
	}
	return allowed
}

func TestBlocksAfterLimit(t *testing.T) {
	l, _ := newTestLimiter(3, 10, nil)

	if got := countAllowed(l, "1.1.1.1", "", 3); got != 3 {
		t.Fatalf("allowed %d, want 3", got)
	}

	ok, wait := l.Allow(context.Background(), "1.1.1.1", "")
	if ok {
		t.Fatal("4th request should be blocked")
	}
	if wait <= 0 {
		t.Fatal("wait time should be more than 0")
	}
}

func TestBlockLastsUntilDurationEnds(t *testing.T) {
	l, clock := newTestLimiter(1, 10, nil)

	countAllowed(l, "1.1.1.1", "", 2) // the second one gets the IP blocked

	clock.Add(2 * time.Minute)
	ok, _ := l.Allow(context.Background(), "1.1.1.1", "")
	if ok {
		t.Fatal("should still be blocked after 2 minutes")
	}

	clock.Add(4 * time.Minute)
	ok, _ = l.Allow(context.Background(), "1.1.1.1", "")
	if !ok {
		t.Fatal("block should be over after 6 minutes")
	}
}

func TestEachIPHasItsOwnLimit(t *testing.T) {
	l, _ := newTestLimiter(2, 10, nil)

	countAllowed(l, "1.1.1.1", "", 5)

	if got := countAllowed(l, "2.2.2.2", "", 2); got != 2 {
		t.Fatalf("other IP allowed %d, want 2", got)
	}
}

func TestEachTokenHasItsOwnLimit(t *testing.T) {
	l, _ := newTestLimiter(1, 2, nil)

	countAllowed(l, "1.1.1.1", "token-a", 5)

	if got := countAllowed(l, "1.1.1.1", "token-b", 2); got != 2 {
		t.Fatalf("other token allowed %d, want 2", got)
	}
}

func TestTokenLimitBeatsIPLimit(t *testing.T) {
	// IP limit is 2, but the token abc123 can do 5
	l, _ := newTestLimiter(2, 2, map[string]int{"abc123": 5})

	if got := countAllowed(l, "1.1.1.1", "abc123", 6); got != 5 {
		t.Fatalf("with token allowed %d, want 5", got)
	}

	// same IP without a token uses the IP limit
	if got := countAllowed(l, "1.1.1.1", "", 4); got != 2 {
		t.Fatalf("without token allowed %d, want 2", got)
	}
}

func TestDefaultTokenLimit(t *testing.T) {
	l, _ := newTestLimiter(1, 4, map[string]int{"vip": 9})

	if got := countAllowed(l, "1.1.1.1", "other", 6); got != 4 {
		t.Fatalf("allowed %d, want 4", got)
	}
}

func TestCounterResetsAfterOneSecond(t *testing.T) {
	l, clock := newTestLimiter(2, 10, nil)

	if got := countAllowed(l, "1.1.1.1", "", 2); got != 2 {
		t.Fatalf("allowed %d, want 2", got)
	}

	clock.Add(1100 * time.Millisecond)

	if got := countAllowed(l, "1.1.1.1", "", 2); got != 2 {
		t.Fatalf("after 1 second allowed %d, want 2", got)
	}
}

func TestManyRequestsAtTheSameTime(t *testing.T) {
	l, _ := newTestLimiter(50, 10, nil)

	var wg sync.WaitGroup
	var mutex sync.Mutex
	allowed := 0

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := l.Allow(context.Background(), "9.9.9.9", "")
			if ok {
				mutex.Lock()
				allowed++
				mutex.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != 50 {
		t.Fatalf("allowed %d, want exactly 50", allowed)
	}
}

// brokenStore always fails, to test FailOpen.
type brokenStore struct{}

func (brokenStore) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	return 0, errors.New("broken")
}
func (brokenStore) BlockTTL(ctx context.Context, key string) (time.Duration, error) {
	return 0, errors.New("broken")
}
func (brokenStore) Block(ctx context.Context, key string, d time.Duration) error {
	return errors.New("broken")
}
func (brokenStore) Ping(ctx context.Context) error { return errors.New("broken") }
func (brokenStore) Close() error                   { return nil }

func TestFailOpen(t *testing.T) {
	l := New(brokenStore{}, Config{IPLimit: 1, FailOpen: true})
	ok, _ := l.Allow(context.Background(), "1.1.1.1", "")
	if !ok {
		t.Fatal("should allow when the store is broken and FailOpen is true")
	}
}

func TestFailClosed(t *testing.T) {
	l := New(brokenStore{}, Config{IPLimit: 1, FailOpen: false})
	ok, _ := l.Allow(context.Background(), "1.1.1.1", "")
	if ok {
		t.Fatal("should reject when the store is broken and FailOpen is false")
	}
}
