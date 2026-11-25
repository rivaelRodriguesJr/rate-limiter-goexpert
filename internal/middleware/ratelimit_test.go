package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

const expectedMessage = "you have reached the maximum number of requests or actions allowed within a certain time frame"

// newTestServer makes a handler with the rate limiter and the memory store.
func newTestServer(ipLimit int, tokenLimit int, tokens map[string]int) http.Handler {
	l := limiter.New(storage.NewMemory(nil), limiter.Config{
		IPLimit:       ipLimit,
		TokenLimit:    tokenLimit,
		TokenLimits:   tokens,
		BlockDuration: time.Minute,
		FailOpen:      true,
	})

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	return RateLimit(l)(okHandler)
}

// send makes one fake request and returns the response.
func send(h http.Handler, remoteAddr string, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remoteAddr
	if token != "" {
		req.Header.Set("API_KEY", token)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAllowedThen429(t *testing.T) {
	h := newTestServer(2, 10, nil)

	for i := 1; i <= 2; i++ {
		rec := send(h, "1.2.3.4:1000", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: status %d, want 200", i, rec.Code)
		}
	}

	rec := send(h, "1.2.3.4:1001", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status %d, want 429", rec.Code)
	}
	if rec.Body.String() != expectedMessage {
		t.Fatalf("wrong body: %q", rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After header is missing")
	}
}

func TestStaysBlocked(t *testing.T) {
	h := newTestServer(1, 10, nil)

	send(h, "1.2.3.4:1000", "")
	send(h, "1.2.3.4:1000", "") // this one gets blocked

	rec := send(h, "1.2.3.4:1000", "")
	if rec.Code != http.StatusTooManyRequests || rec.Body.String() != expectedMessage {
		t.Fatalf("should still be blocked, got %d %q", rec.Code, rec.Body.String())
	}
}

func TestTokenBeatsIP(t *testing.T) {
	h := newTestServer(2, 2, map[string]int{"abc123": 5})

	for i := 1; i <= 5; i++ {
		rec := send(h, "1.2.3.4:1000", "abc123")
		if rec.Code != http.StatusOK {
			t.Fatalf("token request %d: status %d, want 200", i, rec.Code)
		}
	}
	rec := send(h, "1.2.3.4:1000", "abc123")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("6th token request: status %d, want 429", rec.Code)
	}

	// no token: the IP limit (2) is used
	for i := 1; i <= 2; i++ {
		rec := send(h, "1.2.3.4:1000", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("ip request %d: status %d, want 200", i, rec.Code)
		}
	}
	rec = send(h, "1.2.3.4:1000", "")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd ip request: status %d, want 429", rec.Code)
	}
}

func TestClientIPFromRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	if ip := ClientIP(req); ip != "10.0.0.1" {
		t.Fatalf("got %q", ip)
	}
}

func TestClientIPFromForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 70.1.1.1")
	req.Header.Set("X-Real-IP", "9.9.9.9")

	if ip := ClientIP(req); ip != "203.0.113.5" {
		t.Fatalf("got %q", ip)
	}
}

func TestClientIPFromRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Real-IP", "198.51.100.7")

	if ip := ClientIP(req); ip != "198.51.100.7" {
		t.Fatalf("got %q", ip)
	}
}
