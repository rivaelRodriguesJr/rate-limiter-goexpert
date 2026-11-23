package config_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rivaelRodriguesJr/rate-limiter/internal/config"
)

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestDefaults(t *testing.T) {
	c, err := config.FromLookup(lookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.RateLimitIP != 10 || c.RateLimitTok != 100 || c.BlockDuration != 300*time.Second ||
		c.ServerPort != "8080" || c.RedisAddr != "localhost:6379" || !c.FailOpen {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestValues(t *testing.T) {
	c, err := config.FromLookup(lookup(map[string]string{
		"TOKEN_LIMITS": "abc123:100, xyz789:50", "REDIS_HOST": "redis", "REDIS_PORT": "6380", "FAIL_OPEN": "false",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if c.TokenLimits["abc123"] != 100 || c.TokenLimits["xyz789"] != 50 || c.RedisAddr != "redis:6380" || c.FailOpen {
		t.Fatalf("%+v", c)
	}
}

func TestInvalid(t *testing.T) {
	for _, env := range []map[string]string{
		{"RATE_LIMIT_IP": "0"}, {"RATE_LIMIT_TOKEN": "x"}, {"TOKEN_LIMITS": "abc"},
		{"TOKEN_LIMITS": "a:-1"}, {"SERVER_PORT": "99999"}, {"REDIS_DB": "-1"}, {"FAIL_OPEN": "maybe"},
	} {
		if _, err := config.FromLookup(lookup(env)); err == nil || !strings.Contains(err.Error(), "invalid configuration") {
			t.Errorf("%v: expected error, got %v", env, err)
		}
	}
}
