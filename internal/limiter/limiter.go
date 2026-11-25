package limiter

import (
	"context"
	"log"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

// Window is the time of each counter: 1 second, so limits are requests per second.
const Window = time.Second

type Config struct {
	IPLimit       int            // max requests per second for an IP
	TokenLimit    int            // default max requests per second for a token
	TokenLimits   map[string]int // limits for specific tokens
	BlockDuration time.Duration  // how long we block after the limit is passed
	FailOpen      bool           // if the store fails: true = allow, false = reject
}

type Limiter struct {
	store storage.Store
	cfg   Config
}

func New(store storage.Store, cfg Config) *Limiter {
	return &Limiter{store: store, cfg: cfg}
}

// Allow says if the request can pass. If it can't, it also says how long to wait.
// When there is a token we only use the token limit, the IP limit is ignored.
func (l *Limiter) Allow(ctx context.Context, ip string, token string) (bool, time.Duration) {
	who := "ip:" + ip
	limit := l.cfg.IPLimit

	if token != "" {
		who = "token:" + token
		limit = l.cfg.TokenLimit

		// this token may have its own limit
		specificLimit, found := l.cfg.TokenLimits[token]
		if found {
			limit = specificLimit
		}
	}

	// first: is it already blocked?
	blockKey := "block:" + who
	ttl, err := l.store.BlockTTL(ctx, blockKey)
	if err != nil {
		return l.storeFailed(err)
	}
	if ttl > 0 {
		return false, ttl
	}

	// second: count this request
	count, err := l.store.Incr(ctx, "count:"+who, Window)
	if err != nil {
		return l.storeFailed(err)
	}
	if count <= int64(limit) {
		return true, 0
	}

	// third: it passed the limit, so block it
	err = l.store.Block(ctx, blockKey, l.cfg.BlockDuration)
	if err != nil {
		return l.storeFailed(err)
	}
	return false, l.cfg.BlockDuration
}

// storeFailed is used when Redis (or other store) gives an error.
func (l *Limiter) storeFailed(err error) (bool, time.Duration) {
	log.Printf("store error: %v (fail open: %v)", err, l.cfg.FailOpen)
	return l.cfg.FailOpen, 0
}
