package storage

import (
	"context"
	"time"
)

// Store is where the limiter saves its data.
// Redis and Memory both have these methods, so both can be used as a Store.
type Store interface {
	// Incr adds 1 to the counter of the key and returns the new value.
	// The counter is deleted after the window time.
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)

	// BlockTTL returns how long the key is still blocked (0 if it is not blocked).
	BlockTTL(ctx context.Context, key string) (time.Duration, error)

	// Block blocks the key for some time. The block goes away by itself.
	Block(ctx context.Context, key string, duration time.Duration) error

	Ping(ctx context.Context) error
	Close() error
}

// Clock gives the current time. In the tests we use a fake one.
type Clock interface {
	Now() time.Time
}

// RealClock uses the real time.
type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now()
}
