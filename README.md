# go-ratelimit

Production-grade rate limiting for Go. One import, six algorithms, any backend.

## Why this one?

- **6 algorithms** — Fixed Window, Sliding Window, Sliding Window Counter, Token Bucket, Leaky Bucket, GCRA. Pick the right tool, not the only tool.
- **Redis Cluster native** — `redis.UniversalClient` throughout: standalone, Cluster, Ring, Sentinel. No adapter layer, no surprises.
- **Local cache (L1 + L2)** — Hot keys are counted in-process first, cutting Redis round-trips by orders of magnitude under load.
- **Every major framework** — Drop-in middleware for `net/http`, Gin, Echo, Fiber, and gRPC. Headers, key extractors, path exclusion — all included.
- **Fail-open by default** — Redis goes down, your app stays up. Flip one flag to fail-closed if you need hard enforcement.
- **Dynamic per-key limits** — Pass a `LimitFunc` to resolve limits at request time. Premium users get higher caps without rebuilding the limiter.
- **Prometheus metrics** — Wrap any limiter in one line. Request counts, latency histograms, error counters — all partitioned by algorithm.
- **Zero global state** — No `init()`, no singletons. Construct, configure, inject.

## Install

```bash
go get github.com/krishna-kudari/ratelimit
```

## Quick look

```go
limiter, _ := goratelimit.NewTokenBucket(100, 10)

// net/http
mux.Handle("/api/", middleware.RateLimit(limiter, middleware.KeyByIP)(handler))

// Gin
r.Use(ginmw.RateLimit(limiter, ginmw.KeyByIP))

// With Prometheus
collector := metrics.NewCollector()
limiter = metrics.Wrap(limiter, metrics.TokenBucket, collector)
```

## License

MIT
