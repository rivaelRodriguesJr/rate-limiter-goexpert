package middleware

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
)

const Message = "you have reached the maximum number of requests or actions allowed within a certain time frame"

// RateLimit wraps a handler. If the limit is passed, it answers 429
// and the real handler is not called.
func RateLimit(l *limiter.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := ClientIP(r)
			token := r.Header.Get("API_KEY")

			allowed, wait := l.Allow(r.Context(), ip, token)
			if allowed {
				next.ServeHTTP(w, r)
				return
			}

			// round up to whole seconds, at least 1
			seconds := int(wait.Seconds()) + 1
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(Message))
		})
	}
}

// ClientIP finds the IP of the client.
// It tries X-Forwarded-For (first IP), then X-Real-IP, then RemoteAddr.
func ClientIP(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return strings.TrimSpace(realIP)
	}

	// RemoteAddr looks like "1.2.3.4:5678", we only want the IP
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
