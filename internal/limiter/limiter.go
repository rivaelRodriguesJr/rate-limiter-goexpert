// Package limiter holds the rate limiting rules. It knows nothing about HTTP.
// It only talks to a storage.Store, which is an interface (like a Java interface
// or a TypeScript type): any struct with the right methods fits it automatically,
// with no `implements` keyword.
package limiter

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

// Window is how long one counter lives. 1 second => limits are "requests per second".
const Window = time.Second

// Config is a plain struct: just data, like a Java POJO / JS object.
type Config struct {
	IPLimit       int            // max requests/second per IP
	TokenLimit    int            // default max requests/second per token
	TokenLimits   map[string]int // per-token overrides (like a HashMap / JS object)
	BlockDuration time.Duration  // how long to block after the limit is exceeded
	FailOpen      bool           // if the storage fails: true = allow, false = reject
}

// Decision is what Allow returns. Go functions can return several values,
// but here one small struct is easier to read.
type Decision struct {
	Allowed    bool
	RetryAfter time.Duration // only meaningful when Allowed is false
}

// Limiter has no class; it's a struct with methods. Lowercase field names are
// private to the package, uppercase names are public (that's Go's public/private).
type Limiter struct {
	store  storage.Store
	cfg    Config
	logger *slog.Logger
}

// New is a constructor by convention (Go has no constructors, just functions).
func New(store storage.Store, cfg Config, logger *slog.Logger) *Limiter {
	if logger == nil {
		logger = slog.Default()
	}
	return &Limiter{store: store, cfg: cfg, logger: logger}
}

// Allow answers: "may this request pass?"
// If a token is present we use ONLY the token's limit (token beats IP).
func (l *Limiter) Allow(ctx context.Context, ip, token string) Decision {
	// Pick who we are counting ("subject") and their limit.
	subject := "ip:" + ip
	limit := l.cfg.IPLimit
	if token != "" {
		subject = "token:" + token
		limit = l.tokenLimit(token)
	}

	decision, err := l.check(ctx, subject, limit)
	if err != nil {
		// Storage problem (e.g. Redis down). Log it and apply the FailOpen policy.
		l.logger.Error("storage failure", "subject", shorten(subject), "fail_open", l.cfg.FailOpen, "error", err)
		return Decision{Allowed: l.cfg.FailOpen}
	}
	return decision
}

// check does the real work, in three steps:
//  1. already blocked?  -> deny
//  2. count this request in the current 1-second window
//  3. over the limit?   -> create a block (Redis expires it automatically) and deny
func (l *Limiter) check(ctx context.Context, subject string, limit int) (Decision, error) {
	blockKey := "rl:block:" + subject

	remaining, err := l.store.BlockTTL(ctx, blockKey)
	if err != nil {
		return Decision{}, err
	}
	if remaining > 0 {
		return Decision{Allowed: false, RetryAfter: remaining}, nil
	}

	count, err := l.store.Incr(ctx, "rl:count:"+subject, Window)
	if err != nil {
		return Decision{}, err
	}
	if count <= int64(limit) {
		return Decision{Allowed: true}, nil
	}

	if err := l.store.Block(ctx, blockKey, l.cfg.BlockDuration); err != nil {
		// %w wraps the original error, like `new Exception("...", cause)` in Java.
		return Decision{}, fmt.Errorf("could not block %s: %w", subject, err)
	}
	return Decision{Allowed: false, RetryAfter: l.cfg.BlockDuration}, nil
}

// tokenLimit uses the per-token limit if there is one, otherwise the default.
func (l *Limiter) tokenLimit(token string) int {
	// `v, ok := map[key]` is the Go way to check "does this key exist?".
	if v, ok := l.cfg.TokenLimits[token]; ok {
		return v
	}
	return l.cfg.TokenLimit
}

// shorten keeps secrets out of logs: "token:abc123456789" -> "token:abc1…".
func shorten(subject string) string {
	if len(subject) > 10 && subject[:6] == "token:" {
		return subject[:10] + "…"
	}
	return subject
}
