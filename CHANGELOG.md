# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] — 2025-03-02

### Added

- **Six algorithms:** Fixed Window, Sliding Window Log, Sliding Window Counter, Token Bucket, Leaky Bucket, GCRA.
- **In-memory store** for single-process use and testing.
- **Redis store** via `redis.UniversalClient` — standalone, Cluster, Ring, Sentinel.
- **Redis Cluster hash-tag routing** (`WithHashTag`) for multi-key algorithms.
- **Middleware** for net/http, Gin, Echo, Fiber, and gRPC (unary + streaming).
- **Key extractors:** by IP, header, path+IP, method, metadata, and custom functions.
- **Configurable response:** custom status codes, messages, denied/error handlers.
- **Path exclusion** in all middleware.
- **Dynamic per-key limits** via `WithLimitFunc`.
- **Fail-open/fail-closed** behavior on backend errors.
- **Local cache (L1)** to reduce backend round-trips under load.
- **Prometheus metrics** — request counts, latency histograms, error counters by algorithm.
- **Builder API** for fluent limiter construction.
- **Leaky Bucket modes:** Policing (reject) and Shaping (queue with delay).
- **AllowN** for batch token consumption.
- **Interactive demo** — browser-based algorithm visualizer (`examples/demo`).
- **Comprehensive examples** — basic, HTTP, Gin, Echo, Fiber, gRPC, Redis, advanced patterns.

[1.0.0]: https://github.com/krishna-kudari/ratelimit/releases/tag/v1.0.0
