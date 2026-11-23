# Rate Limiter

An HTTP middleware (Go, `net/http` + chi) that limits requests per second by **client IP** or by **access token** (`API_KEY` header), persisting state in **Redis**. A token's limit takes precedence over the IP limit.

## Architecture

```
request ─▶ middleware.RateLimit ─▶ limiter.Limiter ─▶ storage.Store (interface)
           (IP + API_KEY, 429)     (business logic)      ├─ Redis   (production)
                                                         └─ Memory  (unit tests)
```

- `internal/middleware` – extracts IP/token, calls the limiter, writes the 429.
- `internal/limiter` – HTTP-independent logic: block check → counter (1s window) → block on overflow.
- `internal/storage` – `Store` interface (strategy) with Redis (Lua script, atomic `INCR`+`PEXPIRE`; blocks are keys with TTL) and in-memory implementations (injectable `Clock`).
- `internal/config` – env vars + optional `.env`; fails fast on invalid values.

When the limit is exceeded the client gets `429` with the body
`you have reached the maximum number of requests or actions allowed within a certain time frame` and is blocked for `BLOCK_DURATION_SECONDS`; every request in that period gets the same 429. A `Retry-After` header is also set.

**Redis failures:** errors are logged; with `FAIL_OPEN=true` (default) requests are allowed, with `false` they are rejected with 429.

**Client IP:** first `X-Forwarded-For` entry, else `X-Real-IP`, else `RemoteAddr` without port. These headers are client-controlled; only trust them behind a proxy that overwrites them.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `RATE_LIMIT_IP` | `10` | Max requests/second per IP |
| `RATE_LIMIT_TOKEN` | `100` | Default max requests/second per token |
| `TOKEN_LIMITS` | _(empty)_ | Per-token overrides, `token:limit,token:limit` |
| `BLOCK_DURATION_SECONDS` | `300` | Block time after exceeding the limit |
| `REDIS_ADDR` | _(unset)_ | `host:port`; if unset, `REDIS_HOST`:`REDIS_PORT` is used |
| `REDIS_HOST` / `REDIS_PORT` | `localhost` / `6379` | Used when `REDIS_ADDR` is unset |
| `REDIS_PASSWORD` | _(empty)_ | Redis password |
| `REDIS_DB` | `0` | Redis database |
| `SERVER_PORT` | `8080` | HTTP port |
| `FAIL_OPEN` | `true` | Allow requests when Redis fails |

Copy `.env.example` to `.env` to customize (real environment variables win over `.env`).

## Run

```sh
cp .env.example .env   # optional
docker compose up --build
```

## Tests (Docker only)

```sh
docker compose run --rm test     # or: make test
docker compose --profile test down -v
```

This runs `go test -race ./...` against the Redis container. Without Redis, the Redis integration test is skipped when run locally (`go test -race ./...`).

## Examples

With `RATE_LIMIT_IP=2` and `TOKEN_LIMITS=abc123:5`:

```sh
for i in 1 2 3; do curl -s -o /dev/null -w "%{http_code}\n" localhost:8080/; done
# 200 200 429
curl -i localhost:8080/            # 429, body: you have reached the maximum ...
# Token precedence: the same IP, but token abc123 allows 5 req/s
for i in 1 2 3 4 5 6; do curl -s -o /dev/null -w "%{http_code}\n" -H "API_KEY: abc123" localhost:8080/; done
# 200 200 200 200 200 429
```

## Swapping the persistence strategy

1. Create `internal/storage/<backend>.go` with a type implementing `storage.Store`:
   `Incr(ctx, key, window) (int64, error)` (atomic; first increment starts the window, the counter expires after it), `BlockTTL(ctx, key) (time.Duration, error)` (0 when not blocked), `Block(ctx, key, d) error` (auto-expiring), `Ping`, `Close`.
2. Add `var _ storage.Store = (*YourStore)(nil)` and run the shared contract in `internal/storage/storage_test.go` against it.
3. In `cmd/server/main.go`, replace the `storage.NewRedis(...)` block with your constructor. Nothing else changes: the limiter and middleware only know the interface.

## Go cheat sheet for Java / Node.js developers

| In Go | Think of it as |
|---|---|
| `package` / `internal/` | Java package / Node module. `internal/` can't be imported from outside this project |
| Uppercase name (`Limiter`) vs lowercase (`store`) | `public` vs `private` |
| `struct` + methods | A class without inheritance |
| `interface` (e.g. `storage.Store`) | Java interface / TS type, but implemented implicitly |
| `value, err := f()` + `if err != nil` | Instead of `try/catch`. Errors are normal return values |
| `defer x.Close()` | `finally` / `try-with-resources` |
| `context.Context` | Cancellation + timeout token passed down calls (like `AbortController`) |
| `go func() {...}()` | Starts a lightweight thread (goroutine) |
| `chan` | A thread-safe queue for goroutines to talk to each other |
| `sync.Mutex` | `synchronized` / a lock |
| `*Limiter` | A reference/pointer to the object (avoids copying it) |
| `_test.go` files, `go test` | Test files sit next to the code, no extra framework needed |

Suggested reading order: `cmd/server/main.go` → `internal/limiter/limiter.go` → `internal/middleware/ratelimit.go` → `internal/storage/memory.go` → the tests.
