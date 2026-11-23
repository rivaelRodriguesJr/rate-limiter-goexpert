package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// incrScript increments a counter and guarantees it has a TTL, atomically.
var incrScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
if c == 1 or redis.call('PTTL', KEYS[1]) < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
end
return c
`)

// Redis is a Store backed by Redis.
type Redis struct {
	client *redis.Client
}

// RedisOptions configures the Redis connection.
type RedisOptions struct {
	Addr     string
	Password string
	DB       int
}

// NewRedis creates a Redis store. It does not connect eagerly; use Ping.
func NewRedis(opts RedisOptions) *Redis {
	return &Redis{client: redis.NewClient(&redis.Options{
		Addr:     opts.Addr,
		Password: opts.Password,
		DB:       opts.DB,
	})}
}

// Incr implements Store using a Lua script (INCR + PEXPIRE are atomic).
func (r *Redis) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	n, err := incrScript.Run(ctx, r.client, []string{key}, window.Milliseconds()).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis incr %q: %w", key, err)
	}
	return n, nil
}

// BlockTTL implements Store.
func (r *Redis) BlockTTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := r.client.PTTL(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, fmt.Errorf("redis pttl %q: %w", key, err)
	}
	if ttl < 0 { // -2: missing, -1: no expiry (never written by us)
		return 0, nil
	}
	if ttl == 0 {
		ttl = time.Millisecond
	}
	return ttl, nil
}

// Block implements Store. The key expires through Redis TTL.
func (r *Redis) Block(ctx context.Context, key string, d time.Duration) error {
	if err := r.client.Set(ctx, key, 1, d).Err(); err != nil {
		return fmt.Errorf("redis set %q: %w", key, err)
	}
	return nil
}

// Ping implements Store.
func (r *Redis) Ping(ctx context.Context) error {
	if err := r.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	return nil
}

// Close implements Store.
func (r *Redis) Close() error { return r.client.Close() }
