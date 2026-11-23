// Package middleware connects the limiter to HTTP.
// A Go middleware is a function that takes a handler and returns a new handler
// wrapping it, the same idea as Express `(req, res, next)`.
package middleware

import (
	"context"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
)

// TokenHeader is the request header carrying the access token.
const TokenHeader = "API_KEY"

// TooManyRequestsMessage is the exact body returned on 429.
const TooManyRequestsMessage = "you have reached the maximum number of requests or actions allowed within a certain time frame"

// Allower is a tiny interface declared by the *consumer* (a common Go habit).
// *limiter.Limiter satisfies it automatically, and tests can pass a fake.
type Allower interface {
	Allow(ctx context.Context, ip, token string) limiter.Decision
}

// RateLimit returns a middleware that rejects over-limit requests with 429.
func RateLimit(l Allower) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ask the limiter. r.Header.Get("API_KEY") returns "" when absent.
			d := l.Allow(r.Context(), ClientIP(r), r.Header.Get(TokenHeader))
			if d.Allowed {
				next.ServeHTTP(w, r) // same as calling next() in Express
				return
			}
			// Denied: set headers BEFORE WriteHeader/Write, because they can't change after.
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Retry-After", retryAfterSeconds(d.RetryAfter))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(TooManyRequestsMessage))
		})
	}
}

func retryAfterSeconds(d time.Duration) string {
	return strconv.Itoa(max(1, int(math.Ceil(d.Seconds()))))
}

// ClientIP returns the first X-Forwarded-For entry, else X-Real-IP, else the
// host part of RemoteAddr.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if first = strings.TrimSpace(first); first != "" {
			return first
		}
	}
	if xri := strings.TrimSpace(r.Header.Get("X-Real-IP")); xri != "" {
		return xri
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
