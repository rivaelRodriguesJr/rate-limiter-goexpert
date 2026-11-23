// Package storage defines the persistence strategy used by the limiter and
// ships a Redis and an in-memory implementation.
package storage

import (
	"context"
	"time"
)

// Store is the "strategy" interface: the limiter only depends on this, never on Redis.
// Redis and Memory both have these methods, so both count as a Store with no
// `implements` keyword (Go interfaces are satisfied implicitly).
// Store is the persistence strategy required by the limiter. Implementations
// must be safe for concurrent use.
type Store interface {
	// Incr atomically increments the counter at key and returns the new value.
	// The first increment of a key starts a window of the given length; once it
	// elapses the counter disappears and starts again from 1.
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)

	// BlockTTL returns the remaining block time for key, or 0 if key is not blocked.
	BlockTTL(ctx context.Context, key string) (time.Duration, error)

	// Block marks key as blocked for d. The block expires automatically.
	Block(ctx context.Context, key string, d time.Duration) error

	// Ping checks that the backend is reachable.
	Ping(ctx context.Context) error

	// Close releases resources held by the store.
	Close() error
}

// Clock is an injectable time source.
type Clock interface {
	Now() time.Time
}

// SystemClock is the real wall clock.
type SystemClock struct{}

// Now returns time.Now().
func (SystemClock) Now() time.Time { return time.Now() }
