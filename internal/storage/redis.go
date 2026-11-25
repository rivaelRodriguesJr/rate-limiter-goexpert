package storage

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Redis saves the data in a Redis server.
type Redis struct {
	client *redis.Client
}

type RedisOptions struct {
	Addr     string
	Password string
	DB       int
}

func NewRedis(options RedisOptions) *Redis {
	client := redis.NewClient(&redis.Options{
		Addr:     options.Addr,
		Password: options.Password,
		DB:       options.DB,
	})
	return &Redis{client: client}
}

func (r *Redis) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	// A transaction runs INCR and PEXPIRE together, so another request
	// can't come in the middle. PEXPIRE NX (milliseconds) only sets the time when the key
	// doesn't have one yet, so the window starts at the first request.
	pipe := r.client.TxPipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Do(ctx, "PEXPIRE", key, window.Milliseconds(), "NX")

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (r *Redis) BlockTTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := r.client.PTTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// negative means the key does not exist (or has no expiration)
	if ttl < 0 {
		return 0, nil
	}
	if ttl == 0 {
		ttl = time.Millisecond
	}
	return ttl, nil
}

func (r *Redis) Block(ctx context.Context, key string, duration time.Duration) error {
	// the key is deleted by Redis when the time ends
	return r.client.Set(ctx, key, 1, duration).Err()
}

func (r *Redis) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

func (r *Redis) Close() error {
	return r.client.Close()
}
