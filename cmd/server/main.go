// This is the entry point, like `public static void main` in Java or index.js in Node.
// Its only job is "wiring": build the pieces and plug them together.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5" // router, similar to Express

	"github.com/rivaelRodriguesJr/rate-limiter/internal/config"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/middleware"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

func main() {
	// Go has no exceptions. Functions return an `error` value and the caller checks it.
	// We keep main() tiny and put the real work in run(), so we can just `return err`.
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// 1) Read config from env vars (and .env). Fails fast if something is invalid.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// 2) A context carries cancellation. Here it is cancelled when the user presses
	// Ctrl+C or Docker sends SIGTERM. Think of it as an AbortController in Node.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop() // `defer` = run this when the function exits (like `finally`)

	// 3) Choose the storage backend. This is the ONLY place that knows about Redis.
	// To use another database, change just these lines (see README).
	store := storage.NewRedis(storage.RedisOptions{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer store.Close()

	// Check Redis is reachable, but don't crash if it isn't: the limiter has
	// a fail-open/fail-closed setting to decide what to do.
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	if err := store.Ping(pingCtx); err != nil {
		slog.Warn("redis not reachable at startup", "addr", cfg.RedisAddr, "error", err)
	}
	cancel()

	// 4) Create the limiter (business logic). It receives `store` through its
	// constructor: plain dependency injection, no framework needed.
	lim := limiter.New(store, limiter.Config{
		IPLimit:       cfg.RateLimitIP,
		TokenLimit:    cfg.RateLimitTok,
		TokenLimits:   cfg.TokenLimits,
		BlockDuration: cfg.BlockDuration,
		FailOpen:      cfg.FailOpen,
	}, slog.Default())

	// 5) Router + middleware, like `app.use(rateLimit)` in Express.
	r := chi.NewRouter()
	r.Use(middleware.RateLimit(lim))
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("root."))
	})

	srv := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	// 6) Start the server in a goroutine (a lightweight thread) so we can also
	// wait for the shutdown signal. A channel (`errCh`) is how goroutines talk;
	// think of it as a tiny blocking queue.
	errCh := make(chan error, 1)
	go func() {
		slog.Info("server listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe() // blocks until the server stops
	}()

	// 7) Wait for whichever happens first: server error or Ctrl+C.
	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil // normal stop
		}
		return err
	case <-ctx.Done():
		slog.Info("shutting down, finishing in-flight requests...")
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx) // graceful: let running requests finish
	}
}
