package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	// empty values mean "not set"
	for _, name := range []string{"RATE_LIMIT_IP", "RATE_LIMIT_TOKEN", "BLOCK_DURATION_SECONDS", "SERVER_PORT", "REDIS_ADDR", "REDIS_HOST", "REDIS_PORT", "REDIS_DB", "TOKEN_LIMITS", "FAIL_OPEN"} {
		t.Setenv(name, "")
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RateLimitIP != 10 {
		t.Errorf("RateLimitIP = %d, want 10", cfg.RateLimitIP)
	}
	if cfg.RateLimitTok != 100 {
		t.Errorf("RateLimitTok = %d, want 100", cfg.RateLimitTok)
	}
	if cfg.BlockDuration != 300*time.Second {
		t.Errorf("BlockDuration = %v, want 5m", cfg.BlockDuration)
	}
	if cfg.ServerPort != "8080" {
		t.Errorf("ServerPort = %s, want 8080", cfg.ServerPort)
	}
	if cfg.RedisAddr != "localhost:6379" {
		t.Errorf("RedisAddr = %s", cfg.RedisAddr)
	}
	if !cfg.FailOpen {
		t.Error("FailOpen should be true by default")
	}
}

func TestTokenLimitsAndRedisHost(t *testing.T) {
	t.Setenv("TOKEN_LIMITS", "abc123:100,xyz789:50")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("REDIS_HOST", "redis")
	t.Setenv("REDIS_PORT", "6380")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TokenLimits["abc123"] != 100 || cfg.TokenLimits["xyz789"] != 50 {
		t.Errorf("wrong token limits: %v", cfg.TokenLimits)
	}
	if cfg.RedisAddr != "redis:6380" {
		t.Errorf("RedisAddr = %s", cfg.RedisAddr)
	}
}

func TestInvalidValues(t *testing.T) {
	bad := map[string]string{
		"RATE_LIMIT_IP":    "0",
		"RATE_LIMIT_TOKEN": "abc",
		"TOKEN_LIMITS":     "abc",
		"REDIS_DB":         "-1",
		"FAIL_OPEN":        "maybe",
	}

	for name, value := range bad {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, value)
			_, err := Load()
			if err == nil {
				t.Errorf("expected an error for %s=%s", name, value)
			}
		})
	}
}
