// Package config loads and validates settings from environment variables,
// optionally seeded from a .env file.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// Config is the full application configuration.
type Config struct {
	RateLimitIP   int
	RateLimitTok  int
	TokenLimits   map[string]int
	BlockDuration time.Duration
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	ServerPort    string
	FailOpen      bool
}

// Load reads a .env file from the working directory (if present; real
// environment variables always win) and then parses the environment.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("reading .env: %w", err)
	}
	return FromLookup(os.LookupEnv)
}

// FromLookup takes a function instead of calling os.Getenv directly so tests can
// feed fake values (no need to set real env vars). It collects ALL errors and
// reports them together, instead of stopping at the first one.
// FromLookup parses configuration using the given lookup function.
func FromLookup(lookup func(string) (string, bool)) (Config, error) {
	get := func(key, def string) string {
		if v, ok := lookup(key); ok && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
		return def
	}
	var errs []error
	posInt := func(key, def string) int {
		raw := get(key, def)
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			errs = append(errs, fmt.Errorf("%s must be a positive integer, got %q", key, raw))
			return 0
		}
		return n
	}

	c := Config{
		RateLimitIP:  posInt("RATE_LIMIT_IP", "10"),
		RateLimitTok: posInt("RATE_LIMIT_TOKEN", "100"),
		ServerPort:   get("SERVER_PORT", "8080"),
	}
	c.RedisPassword = get("REDIS_PASSWORD", "")
	c.BlockDuration = time.Duration(posInt("BLOCK_DURATION_SECONDS", "300")) * time.Second

	if p, err := strconv.Atoi(c.ServerPort); err != nil || p < 1 || p > 65535 {
		errs = append(errs, fmt.Errorf("SERVER_PORT must be a port number (1-65535), got %q", c.ServerPort))
	}

	db := get("REDIS_DB", "0")
	if n, err := strconv.Atoi(db); err != nil || n < 0 {
		errs = append(errs, fmt.Errorf("REDIS_DB must be a non-negative integer, got %q", db))
	} else {
		c.RedisDB = n
	}

	if addr := get("REDIS_ADDR", ""); addr != "" {
		c.RedisAddr = addr
	} else {
		c.RedisAddr = get("REDIS_HOST", "localhost") + ":" + get("REDIS_PORT", "6379")
	}

	switch strings.ToLower(get("FAIL_OPEN", "true")) {
	case "true", "1", "yes":
		c.FailOpen = true
	case "false", "0", "no":
		c.FailOpen = false
	default:
		errs = append(errs, fmt.Errorf("FAIL_OPEN must be true or false, got %q", get("FAIL_OPEN", "")))
	}

	tl, err := ParseTokenLimits(get("TOKEN_LIMITS", ""))
	if err != nil {
		errs = append(errs, err)
	}
	c.TokenLimits = tl

	if len(errs) > 0 {
		return Config{}, fmt.Errorf("invalid configuration: %w", errors.Join(errs...))
	}
	return c, nil
}

// ParseTokenLimits parses "token:limit,token2:limit2".
func ParseTokenLimits(s string) (map[string]int, error) {
	out := make(map[string]int)
	if strings.TrimSpace(s) == "" {
		return out, nil
	}
	for _, pair := range strings.Split(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		tok, lim, ok := strings.Cut(pair, ":")
		tok = strings.TrimSpace(tok)
		n, err := strconv.Atoi(strings.TrimSpace(lim))
		if !ok || tok == "" || err != nil || n <= 0 {
			return nil, fmt.Errorf("TOKEN_LIMITS entry %q must look like token:limit with a positive limit", pair)
		}
		out[tok] = n
	}
	return out, nil
}
