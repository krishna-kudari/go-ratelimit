# go-ratelimit

[![CI](https://github.com/krishna-kudari/ratelimit/actions/workflows/ci.yml/badge.svg)](https://github.com/krishna-kudari/ratelimit/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/krishna-kudari/ratelimit.svg)](https://pkg.go.dev/github.com/krishna-kudari/ratelimit)
[![Go Report Card](https://goreportcard.com/badge/github.com/krishna-kudari/ratelimit)](https://goreportcard.com/report/github.com/krishna-kudari/ratelimit)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Rate limiting for Go that stays out of your way.**
One import. Seven algorithms. Any backend. Drop into any framework in three lines.

```go
limiter, _ := goratelimit.New("redis://localhost:6379", goratelimit.PerMinute(100))
r.Use(middleware.RateLimit(limiter, middleware.KeyByIP))
```

That's it. You're production-ready.

---

## Why this one

Most Go rate limiters make you choose between simple-but-wrong and powerful-but-painful.
This library gives you both — sensible defaults that work instantly, and the knobs to
tune every detail when you need them.

- **Seven algorithms** — from Fixed Window to GCRA to Count-Min Sketch, each the right
  tool for a specific problem. [Pick one](#algorithms) or chain them.
- **Any backend** — in-memory for tests, Redis standalone, Cluster, Sentinel, or Ring
  for production. Same API, zero code changes.
- **Every major framework** — net/http, Gin, Echo, Fiber, gRPC. Copy one line.
- **Built for DDoS** — Count-Min Sketch absorbs billion-key attack traffic in 30KB of
  fixed memory. PreFilter chains it with GCRA so attackers never reach Redis.
- **Honest benchmarks** — 86k req/sec under 1000 concurrent VUs, p99 under 11ms,
  correctness verified 10,000+ times. [See the numbers.](#benchmarks)

---

## Install

```bash
go get github.com/krishna-kudari/ratelimit
```

Requires Go 1.21+. No CGO. No system dependencies.

---

## Start in 2 minutes

### In-memory (perfect for development and tests)

```go
import goratelimit "github.com/krishna-kudari/ratelimit"

limiter, _ := goratelimit.NewInMemory(goratelimit.PerMinute(100))

result, _ := limiter.Allow(ctx, "user:123")
if !result.Allowed {
    // result.RetryAfter tells the client exactly when to try again
}
```

### With Redis (production)

```go
limiter, _ := goratelimit.New("redis://localhost:6379", goratelimit.PerMinute(100))
```

Switch from in-memory to Redis by changing one string. Your application code doesn't change.

### As HTTP middleware

```go
import "github.com/krishna-kudari/ratelimit/middleware"

mux.Handle("/api/", middleware.RateLimit(limiter, middleware.KeyByIP)(handler))
```

Every response automatically gets the headers your clients expect:

```
X-RateLimit-Limit:     100
X-RateLimit-Remaining: 73
X-RateLimit-Reset:     1709391845
Retry-After:           47          (only on 429)
```

---

## Algorithms

Not sure which to pick? Start with GCRA. It's what Stripe and GitHub use.

| Algorithm | Best for | Memory | Burst |
|---|---|---|---|
| [Fixed Window](#fixed-window) | Simple quotas, billing tiers | O(1) | Hard cliff |
| [Sliding Window Log](#sliding-window-log) | Strict per-user limits, low traffic | O(n) | None |
| [Sliding Window Counter](#sliding-window-counter) | High-scale APIs, ~1% error acceptable | O(1) | None |
| [Token Bucket](#token-bucket) | Network throttling, SDKs | O(1) | ✓ Smooth |
| [Leaky Bucket](#leaky-bucket) | Traffic shaping, steady output | O(1) | Queued |
| [GCRA](#gcra) | API rate limiting, SaaS | O(1) | ✓ Configurable |
| [Count-Min Sketch](#count-min-sketch) | DDoS mitigation, billion-key traffic | **Fixed** | None |

### Fixed Window

Counts requests in a fixed time window. Resets at the boundary.

```go
limiter, _ := goratelimit.NewFixedWindow(
    100,  // max requests
    60,   // per 60 seconds
)
```

Simple and fast. Watch out for burst at window boundaries — a client can fire
100 requests at 11:59 and 100 more at 12:00. Use Sliding Window Counter if
that matters to you.

### Sliding Window Log

Stores every request timestamp. Perfectly accurate, no boundary bursts.

```go
limiter, _ := goratelimit.NewSlidingWindow(100, 60)
```

The right choice when you're billing per request and need exact counts. Memory
grows with traffic — not suitable for high-cardinality keys at scale.

### Sliding Window Counter

Approximates a sliding window using two fixed windows. ~1% worst-case error.
O(1) memory. This is what Cloudflare uses.

```go
limiter, _ := goratelimit.NewSlidingWindowCounter(100, 60)
```

### Token Bucket

Tokens refill at a steady rate. Each request costs one token. Leftover tokens
accumulate as burst capacity.

```go
limiter, _ := goratelimit.NewTokenBucket(
    100,  // capacity (max burst)
    10,   // refill rate per second
)
```

The right choice when bursts are intentional — mobile clients syncing after a
gap, batch jobs, anything that idles and then fires.

### Leaky Bucket

Requests fill a bucket that leaks at a constant rate. Two modes:

```go
// Policing — excess requests are dropped immediately (fast 429)
limiter, _ := goratelimit.NewLeakyBucket(100, 10, goratelimit.Policing)

// Shaping — excess requests are queued (smooth output, adds delay)
limiter, _ := goratelimit.NewLeakyBucket(100, 10, goratelimit.Shaping)
```

Use Policing for APIs. Use Shaping when you control both sides and want
to smooth traffic rather than reject it.

### GCRA

Generic Cell Rate Algorithm. Single timestamp per key, exact accounting,
configurable burst. What Stripe, GitHub, and Shopify use.

```go
limiter, _ := goratelimit.NewGCRA(
    16,  // sustained rate (requests per second)
    32,  // burst allowance
)
```

If you only learn one algorithm, learn this one. O(1) memory, exact counts,
handles burst as a first-class concept.

### Count-Min Sketch

A probabilistic data structure that tracks request counts in **fixed memory**,
regardless of how many unique keys exist.

```go
limiter, _ := goratelimit.NewCMS(
    100,   // requests per window
    60,    // window seconds
    0.01,  // 1% error rate
    0.001, // 0.1% failure probability
)

fmt.Println(goratelimit.CMSMemoryBytes(0.01, 0.001)) // 30,464 bytes — fixed forever
```

The math: a 30KB array replaces a map that would grow to 50GB under a billion-key
DDoS attack. Counts are approximate — always slightly over, never under. At DDoS
scale, a 1% overcount is an acceptable tradeoff for a 1,000,000x memory reduction.

The right choice when you can't bound the number of unique keys.

---

## Chaining Algorithms — PreFilter

Chain a fast local CMS with a precise distributed limiter. Attack traffic gets
absorbed in nanoseconds at the CMS layer. Legitimate traffic — the small fraction
that looks normal — passes through to GCRA on Redis.

```go
// Fast local sketch — no network calls, no Redis load during attacks
cms, _ := goratelimit.NewCMS(100, 60, 0.01, 0.001)

// Precise distributed limiter for traffic that passes the sketch
gcra, _ := goratelimit.NewGCRA(16, 32, goratelimit.WithRedis(client))

// Chain them — CMS runs first, GCRA only sees what CMS lets through
limiter := goratelimit.NewPreFilter(cms, gcra)
```

Under a billion-IP DDoS, Redis sees almost nothing. Your API stays up.

---

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

### Key extractors — built-in

```go
middleware.KeyByIP          // client IP
middleware.KeyByRealIP      // X-Forwarded-For aware, for proxied traffic
middleware.KeyByAPIKey      // Authorization header
```

### Key extractors — custom

```go
// Per-tenant + per-route limiting — most SaaS APIs need exactly this
middleware.RateLimit(limiter, func(r *http.Request) string {
    return r.Header.Get("X-Tenant-ID") + ":" + r.URL.Path
})
```

---

## Advanced

### Different limits per plan

The most common real-world need — free, pro, and enterprise tiers with different limits.

```go
limiter, _ := goratelimit.NewFixedWindow(60, 60,
    goratelimit.WithLimitFunc(func(ctx context.Context, key string) int64 {
        switch getPlan(ctx, key) {
        case "pro":         return 1_000
        case "enterprise":  return 100_000
        default:            return 60    // free tier
        }
    }),
)
```

### L1 + L2 cache — skip Redis on the hot path

```go
import "github.com/krishna-kudari/ratelimit/cache"

// Checks in-process cache first. Only hits Redis on a miss.
// L1 hit: ~100ns. L2 Redis hit: ~1ms.
cached := cache.New(limiter, cache.WithTTL(100*time.Millisecond))
```

### Prometheus metrics

```go
import "github.com/krishna-kudari/ratelimit/metrics"

collector := metrics.NewCollector()
limiter = metrics.Wrap(limiter, metrics.GCRA, collector)

// Automatically exposes:
// ratelimit_requests_total{algorithm="gcra", result="allowed|denied"}
// ratelimit_request_duration_seconds{quantile="0.5|0.95|0.99"}
```

### Redis Cluster

```go
limiter, _ := goratelimit.NewGCRA(100, 20,
    goratelimit.WithRedis(clusterClient),
    goratelimit.WithHashTag(), // keys become {user:123} for correct slot routing
)
```

### Fail-open vs fail-closed

```go
goratelimit.WithFailOpen(true)  // allow requests if Redis is down (default)
goratelimit.WithFailOpen(false) // deny requests if Redis is down
```

Pick based on your threat model. Public APIs usually fail open — a Redis blip
shouldn't take down your service. Internal or security-critical APIs fail closed.

### Builder API — when you want everything explicit

```go
limiter, _ := goratelimit.NewBuilder().
    SlidingWindowCounter(100, 60*time.Second).
    Redis(client).
    HashTag().
    Build()
```

---

## Benchmarks

### Microbenchmarks — algorithm cost in isolation

Single goroutine, in-memory, no network, 10 runs each.

```bash
go test -bench=. -benchmem -count=10 ./...
```

| Algorithm        | ops/sec    | ns/op | allocs/op |
|------------------|------------|-------|-----------|
| GCRA             | 17,200,000 | 57.7  | 1         |
| Fixed Window     | 17,000,000 | 59.3  | 1         |
| Token Bucket     | 14,500,000 | 69.0  | 1         |
| Count-Min Sketch | 10,600,000 | 94.5  | 1         |

_Apple M4 · Go 1.23 · in-process memory store_

> The `1 allocs/op` is the `*Result` struct per call. Tracked as a known
> improvement — eliminating it would push GCRA to ~40 ns/op.

### Load tests — real concurrent pressure

1000 goroutines hammering a real HTTP server simultaneously for 60 seconds.
This is what your users actually experience.

```bash
./bench/run_load.sh              # all 5 algorithms
python3 bench/parse_summaries.py # summary table
```

| Algorithm           | req/sec | p50    | p95    | p99     | rate limited |
|---------------------|---------|--------|--------|---------|--------------|
| **GCRA**            | 86,559  | 1.19ms | 6.83ms | 10.24ms | 80.94%       |
| **Token Bucket**    | 86,024  | 1.14ms | 7.03ms | 10.83ms | 80.82%       |
| **PreFilter**       | 80,408  | 1.13ms | 6.95ms | 10.79ms | 97.01%†      |
| **Fixed Window**    | 78,926  | 1.23ms | 7.11ms | 11.19ms | 78.88%       |
| **Count-Min Sketch**| 78,910  | 1.22ms | 7.17ms | 11.50ms | 89.60%       |

_Apple M4 · k6 · 1000 VUs · in-memory store · 1000 unique API keys_

† PreFilter stacks CMS and GCRA limits by design. It's intended for DDoS
scenarios where blocking aggressively is the goal.

### How latency scales with concurrency (GCRA)

```
VUs     p50       p99
────────────────────────
50      0.21ms    1.2ms
100     0.31ms    2.1ms
250     0.58ms    4.8ms
500     0.89ms    7.4ms
1000    1.19ms   10.24ms
```

p99 grows sub-linearly. At 20x more concurrent users, p99 grows roughly 8x —
not 20x. The algorithm doesn't degrade sharply under pressure.

### The gap between ns/op and p99 is real and expected

The microbenchmark (57 ns/op) measures one goroutine with no contention.
The load test p99 (10ms) measures 1000 goroutines contending on the same
mutex. Both numbers are true. The difference is the cost of correctness
under real concurrent pressure — and 10ms p99 at 86k req/sec on a laptop
is a number worth putting in production.

### Correctness under concurrency

Speed without correctness is useless for a rate limiter.

500 goroutines fire simultaneously against a limit of 100. Exactly 100 must
be allowed — not 99, not 101.

```bash
go test -run TestCorrectness -v -count=100 ./...
```

This test has passed 10,000+ consecutive runs in CI. If your atomicity is
broken — a missing lock, a race in your Lua script, a TOCTOU — this test
will catch it.

---

## Examples

| Example | What it shows |
|---|---|
| [`basic`](examples/basic) | All 7 algorithms, AllowN, Reset, Builder |
| [`httpserver`](examples/httpserver) | net/http middleware |
| [`ginserver`](examples/ginserver) | Gin middleware |
| [`echoserver`](examples/echoserver) | Echo middleware |
| [`fiberserver`](examples/fiberserver) | Fiber middleware |
| [`grpcserver`](examples/grpcserver) | gRPC unary + stream interceptors |
| [`redis`](examples/redis) | Redis backend, Cluster, hash tags |
| [`advanced`](examples/advanced) | Dynamic limits, L1 cache, Prometheus, PreFilter |
| [`demo`](examples/demo) | Interactive browser visualizer for all algorithms |

### Interactive demo

See every algorithm in action — configurable parameters, burst testing,
real-time visualization. No Redis required.

```bash
cd examples/demo && go run .
# open http://localhost:8080
```

---

## Full API reference

### Constructors

```go
// Auto-selects in-memory or Redis based on URL
New(redisURL string, rate Rate, opts ...Option) (Limiter, error)
NewInMemory(rate Rate, opts ...Option) (Limiter, error)

// Algorithm-specific
NewFixedWindow(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error)
NewSlidingWindow(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error)
NewSlidingWindowCounter(maxRequests, windowSeconds int64, opts ...Option) (Limiter, error)
NewTokenBucket(capacity, refillRate int64, opts ...Option) (Limiter, error)
NewLeakyBucket(capacity, leakRate int64, mode LeakyBucketMode, opts ...Option) (Limiter, error)
NewGCRA(rate, burst int64, opts ...Option) (Limiter, error)
NewCMS(limit, windowSeconds int64, epsilon, delta float64, opts ...Option) (Limiter, error)
NewPreFilter(local, precise Limiter) Limiter

// Builder
NewBuilder() *Builder
CMSMemoryBytes(epsilon, delta float64) int
```

### Rate helpers

```go
PerSecond(n int64) Rate
PerMinute(n int64) Rate
PerHour(n int64) Rate
```

### Limiter interface

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
    RetryAfter time.Duration  // how long to wait before retrying (only meaningful when !Allowed)
}
```

### Options

| Option | Description | Default |
|---|---|---|
| `WithRedis(client)` | Redis backing store | in-memory |
| `WithStore(store)` | Custom `store.Store` implementation | — |
| `WithKeyPrefix(s)` | Redis key prefix | `"ratelimit"` |
| `WithFailOpen(bool)` | Allow requests on backend error | `true` |
| `WithHashTag()` | Wrap keys for Redis Cluster slot routing | off |
| `WithLimitFunc(fn)` | Dynamic per-key limit resolver | — |

---

## License

MIT — do whatever you want with it.

---

<p align="center">
  Built with care. Benchmarked honestly. Correctness verified.
  <br/>
  If it saves you time, consider starring the repo or opening a PR.
</p>
