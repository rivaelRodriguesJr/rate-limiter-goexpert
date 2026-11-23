package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/middleware"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

func handler(ipLimit, tokLimit int, tokens map[string]int) http.Handler {
	l := limiter.New(storage.NewMemory(nil), limiter.Config{
		IPLimit: ipLimit, TokenLimit: tokLimit, TokenLimits: tokens,
		BlockDuration: time.Minute, FailOpen: true,
	}, nil)
	return middleware.RateLimit(l)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
}

func do(h http.Handler, remote, token string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remote
	if token != "" {
		r.Header.Set("API_KEY", token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAllowedThenExactly429(t *testing.T) {
	h := handler(2, 10, nil)
	for i := 0; i < 2; i++ {
		if w := do(h, "1.2.3.4:5555", ""); w.Code != http.StatusOK {
			t.Fatalf("req %d: status %d", i, w.Code)
		}
	}
	w := do(h, "1.2.3.4:5556", "")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", w.Code)
	}
	const want = "you have reached the maximum number of requests or actions allowed within a certain time frame"
	if w.Body.String() != want {
		t.Fatalf("body %q", w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
	// Still blocked afterwards, same response.
	if w := do(h, "1.2.3.4:5557", ""); w.Code != 429 || w.Body.String() != want {
		t.Fatalf("blocked request: %d %q", w.Code, w.Body.String())
	}
}

func TestPrecedence(t *testing.T) {
	h := handler(2, 2, map[string]int{"abc123": 5})
	for i := 0; i < 5; i++ {
		if w := do(h, "1.2.3.4:1", "abc123"); w.Code != 200 {
			t.Fatalf("token req %d: %d", i, w.Code)
		}
	}
	if w := do(h, "1.2.3.4:1", "abc123"); w.Code != 429 {
		t.Fatalf("6th token req: %d", w.Code)
	}
	for i := 0; i < 2; i++ {
		if w := do(h, "1.2.3.4:1", ""); w.Code != 200 {
			t.Fatalf("ip req %d: %d", i, w.Code)
		}
	}
	if w := do(h, "1.2.3.4:1", ""); w.Code != 429 {
		t.Fatalf("3rd ip req: %d", w.Code)
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name, remote, xff, xri, want string
	}{
		{"remote", "10.0.0.1:1234", "", "", "10.0.0.1"},
		{"xff first", "10.0.0.1:1234", "203.0.113.5, 70.1.1.1", "9.9.9.9", "203.0.113.5"},
		{"x-real-ip", "10.0.0.1:1234", "", "198.51.100.7", "198.51.100.7"},
		{"no port", "10.0.0.1", "", "", "10.0.0.1"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if c.xri != "" {
			r.Header.Set("X-Real-IP", c.xri)
		}
		if got := middleware.ClientIP(r); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}
