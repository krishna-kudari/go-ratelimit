# go-ratelimit

Production-grade rate limiting for [Go](https://go.dev/). One import, six algorithms, any backend.

- In-memory or [Redis](https://redis.io/) (standalone, Cluster, Ring, Sentinel)
- Drop-in middleware for [net/http](https://pkg.go.dev/net/http), [Gin](https://gin-gonic.com/), [Echo](https://echo.labstack.com/), [Fiber](https://gofiber.io/), and [gRPC](https://grpc.io/)
- [Prometheus](https://prometheus.io/) metrics in one line

> **NOTE:** Run the [interactive demo](#interactive-demo) to visualize each algorithm in your browser.

## Rate Limiting Algorithms

| Algorithm                  | Redis Data Structure                       | Description                                                                                              |
| -------------------------- | ------------------------------------------ | -------------------------------------------------------------------------------------------------------- |
| **Fixed Window Counter**   | `STRING` (`INCR` + `EXPIRE`)               | Counts requests in fixed time windows. Simple but susceptible to boundary bursts.                        |
| **Sliding Window Log**     | `SORTED SET` (`ZADD` + `ZREMRANGEBYSCORE`) | Logs each request timestamp. Precise sliding window, but stores every request.                           |
| **Sliding Window Counter** | `STRING` x2 (weighted)                     | Weighted average of current and previous window counts. Smooths the fixed-window boundary problem.       |
| **Token Bucket**           | `HASH` + Lua script                        | Tokens refill at a steady rate; each request consumes one. Allows short bursts.                          |
| **Leaky Bucket**           | `HASH` + Lua script                        | Requests fill a bucket that leaks at a constant rate. Policing drops excess; shaping queues with a delay.|
| **GCRA**                   | `HASH` + Lua script                        | Generic Cell Rate Algorithm. Sustained rate + burst allowance via virtual scheduling.                    |

## Install

```bash
go get github.com/krishna-kudari/ratelimit
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    goratelimit "github.com/krishna-kudari/ratelimit"
)

func main() {
    // 10 requests per 60-second window
    limiter, _ := goratelimit.NewFixedWindow(10, 60)

    result, _ := limiter.Allow(context.Background(), "user:123")
    fmt.Printf("allowed=%v remaining=%d\n", result.Allowed, result.Remaining)
}
```

### With Redis

```go
import "github.com/redis/go-redis/v9"

client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

limiter, _ := goratelimit.NewTokenBucket(100, 10,
    goratelimit.WithRedis(client),
)
```

### Builder API

```go
limiter, _ := goratelimit.NewBuilder().
    SlidingWindowCounter(100, 60*time.Second).
    Redis(client).
    HashTag().
    Build()
```

## Middleware

### net/http

```go
import "github.com/krishna-kudari/ratelimit/middleware"

mux.Handle("/api/", middleware.RateLimit(limiter, middleware.KeyByIP)(handler))
```

### Gin

```go
import "github.com/krishna-kudari/ratelimit/middleware/ginmw"

r.Use(ginmw.RateLimit(limiter, ginmw.KeyByClientIP))
```

### Echo

```go
import "github.com/krishna-kudari/ratelimit/middleware/echomw"

e.Use(echomw.RateLimit(limiter, echomw.KeyByRealIP))
```

### Fiber

```go
import "github.com/krishna-kudari/ratelimit/middleware/fibermw"

app.Use(fibermw.RateLimit(limiter, fibermw.KeyByIP))
```

### gRPC

```go
import "github.com/krishna-kudari/ratelimit/middleware/grpcmw"

grpc.ChainUnaryInterceptor(grpcmw.UnaryServerInterceptor(limiter, grpcmw.KeyByPeer))
grpc.ChainStreamInterceptor(grpcmw.StreamServerInterceptor(limiter, grpcmw.StreamKeyByPeer))
```

## Advanced Features

### Dynamic Per-Key Limits

```go
limiter, _ := goratelimit.NewFixedWindow(10, 60,
    goratelimit.WithLimitFunc(func(key string) int64 {
        if key == "premium" { return 1000 }
        return 0 // fallback to default
    }),
)
```

### Fail-Open / Fail-Closed

```go
// Default: fail-open (allow on backend errors)
goratelimit.WithFailOpen(true)

// Strict: deny on backend errors
goratelimit.WithFailOpen(false)
```

### Local Cache (L1 + L2)

```go
import "github.com/krishna-kudari/ratelimit/cache"

cached := cache.New(limiter, cache.WithTTL(100*time.Millisecond))
```

### Prometheus Metrics

```go
import "github.com/krishna-kudari/ratelimit/metrics"

collector := metrics.NewCollector()
limiter = metrics.Wrap(limiter, metrics.TokenBucket, collector)
```

### Redis Cluster

```go
goratelimit.WithHashTag() // keys become prefix:{key} for slot routing
```

## Interactive Demo

Run the interactive algorithm visualizer locally — no Redis required:

```bash
cd examples/demo
go run .
```

Open `http://localhost:8080` to explore all six algorithms with real-time visualizations, configurable parameters, and burst testing.

## API

### Constructors

```go
NewFixedWindow(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error)
NewSlidingWindow(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error)
NewSlidingWindowCounter(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error)
NewTokenBucket(capacity, refillRate int64, opts ...Option) (Limiter, error)
NewLeakyBucket(capacity, leakRate int64, mode LeakyBucketMode, opts ...Option) (Limiter, error)
NewGCRA(rate, burst int64, opts ...Option) (Limiter, error)
```

### Limiter Interface

```go
type Limiter interface {
    Allow(ctx context.Context, key string) (*Result, error)
    AllowN(ctx context.Context, key string, n int) (*Result, error)
    Reset(ctx context.Context, key string) error
}
```

### Result

```go
type Result struct {
    Allowed    bool
    Remaining  int64
    Limit      int64
    ResetAt    time.Time
    RetryAfter time.Duration
}
```

### Options

| Option | Description |
|--------|-------------|
| `WithRedis(client)` | Use Redis as backing store |
| `WithStore(store)` | Use a custom `store.Store` backend |
| `WithKeyPrefix(prefix)` | Key prefix (default: `"ratelimit"`) |
| `WithFailOpen(bool)` | Allow on backend errors (default: `true`) |
| `WithHashTag()` | Enable Redis Cluster hash-tag wrapping |
| `WithLimitFunc(fn)` | Dynamic per-key limit resolver |

## License

MIT
