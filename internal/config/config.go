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

// Config has all the settings of the app.
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

// Load reads the .env file (if it exists) and then the environment variables.
func Load() (*Config, error) {
	// the .env file is optional, so we ignore the error here
	godotenv.Load()

	cfg := &Config{}
	var err error

	cfg.RateLimitIP, err = getInt("RATE_LIMIT_IP", 10, 1)
	if err != nil {
		return nil, err
	}

	cfg.RateLimitTok, err = getInt("RATE_LIMIT_TOKEN", 100, 1)
	if err != nil {
		return nil, err
	}

	seconds, err := getInt("BLOCK_DURATION_SECONDS", 300, 1)
	if err != nil {
		return nil, err
	}
	cfg.BlockDuration = time.Duration(seconds) * time.Second

	cfg.RedisDB, err = getInt("REDIS_DB", 0, 0)
	if err != nil {
		return nil, err
	}

	port, err := getInt("SERVER_PORT", 8080, 1)
	if err != nil {
		return nil, err
	}
	cfg.ServerPort = strconv.Itoa(port)

	cfg.TokenLimits, err = parseTokenLimits(os.Getenv("TOKEN_LIMITS"))
	if err != nil {
		return nil, err
	}

	// REDIS_ADDR wins, if it is empty we use REDIS_HOST and REDIS_PORT
	cfg.RedisAddr = os.Getenv("REDIS_ADDR")
	if cfg.RedisAddr == "" {
		host := os.Getenv("REDIS_HOST")
		if host == "" {
			host = "localhost"
		}
		redisPort := os.Getenv("REDIS_PORT")
		if redisPort == "" {
			redisPort = "6379"
		}
		cfg.RedisAddr = host + ":" + redisPort
	}

	cfg.RedisPassword = os.Getenv("REDIS_PASSWORD")

	cfg.FailOpen = true
	failOpen := os.Getenv("FAIL_OPEN")
	if failOpen != "" {
		cfg.FailOpen, err = strconv.ParseBool(failOpen)
		if err != nil {
			return nil, fmt.Errorf("FAIL_OPEN must be true or false, got %q", failOpen)
		}
	}

	return cfg, nil
}

// getInt reads a number from the environment. If it is not set we use the default.
func getInt(name string, defaultValue int, min int) (int, error) {
	text := os.Getenv(name)
	if text == "" {
		return defaultValue, nil
	}

	number, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", name, text)
	}
	if number < min {
		return 0, fmt.Errorf("%s must be at least %d, got %d", name, min, number)
	}
	return number, nil
}

// parseTokenLimits turns "abc:100,xyz:50" into a map.
func parseTokenLimits(text string) (map[string]int, error) {
	limits := make(map[string]int)
	if strings.TrimSpace(text) == "" {
		return limits, nil
	}

	for _, pair := range strings.Split(text, ",") {
		parts := strings.Split(strings.TrimSpace(pair), ":")
		if len(parts) != 2 || parts[0] == "" {
			return nil, errors.New("TOKEN_LIMITS must look like token:limit,token:limit")
		}

		limit, err := strconv.Atoi(parts[1])
		if err != nil || limit < 1 {
			return nil, fmt.Errorf("TOKEN_LIMITS has a bad limit in %q", pair)
		}
		limits[parts[0]] = limit
	}
	return limits, nil
}
