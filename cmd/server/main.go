package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/config"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/limiter"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/middleware"
	"github.com/rivaelRodriguesJr/rate-limiter/internal/storage"
)

func main() {
	// load the settings, stop the program if something is wrong
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	// Here we choose where the data is saved. To use another database,
	// change only this part (see the README).

	var store storage.Store
	store = storage.NewRedis(storage.RedisOptions{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	
	defer store.Close()

	// if Redis is down we only warn, the limiter uses FAIL_OPEN to decide what to do
	pingCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	err = store.Ping(pingCtx)
	cancel()
	if err != nil {
		log.Printf("warning: cannot connect to redis at %s: %v", cfg.RedisAddr, err)
	}

	rateLimiter := limiter.New(store, limiter.Config{
		IPLimit:       cfg.RateLimitIP,
		TokenLimit:    cfg.RateLimitTok,
		TokenLimits:   cfg.TokenLimits,
		BlockDuration: cfg.BlockDuration,
		FailOpen:      cfg.FailOpen,
	})

	router := chi.NewRouter()
	router.Use(middleware.RateLimit(rateLimiter))
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("root."))
	})

	server := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: router,
	}

	// run the server in the background (goroutine)
	go func() {
		log.Printf("server running on port %s", cfg.ServerPort)
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	// wait here until Ctrl+C or docker stop
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	// graceful shutdown: give the running requests 10 seconds to finish
	log.Println("shutting down...")
	ctx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
