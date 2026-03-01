// Package store defines the backend storage contract for rate limiters.
//
// The Store interface abstracts the storage operations that rate limiting
// algorithms need. The primary implementation is RedisStore (in store/redis),
// which supports standalone Redis, Redis Cluster, and Redis Sentinel via
// redis.UniversalClient.
//
// A MemoryStore (in store/memory) is provided for testing and single-process
// deployments that don't need distributed state.
package store

import (
	"context"
	"time"
)

// Store abstracts the backend for rate limit state persistence.
// Implementations must be safe for concurrent use.
type Store interface {
	// Eval executes a Lua script atomically with the given keys and args.
	// Implementations that don't support scripting (e.g. MemoryStore)
	// should return ErrScriptNotSupported.
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)

	// EvalSha executes a pre-cached script by its SHA1 hash.
	// Falls back to Eval if the script is not cached.
	EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error)

	// ScriptLoad loads a script into the backend's script cache, returning its SHA1.
	ScriptLoad(ctx context.Context, script string) (string, error)

	// Get returns the string value for key, or ("", ErrKeyNotFound) if not found.
	Get(ctx context.Context, key string) (string, error)

	// Set stores a value with optional TTL (0 = no expiry).
	Set(ctx context.Context, key string, value string, ttl time.Duration) error

	// Del deletes one or more keys.
	Del(ctx context.Context, keys ...string) error

	// IncrBy atomically increments key by n, returning the new value.
	// Creates the key with value n if it doesn't exist.
	IncrBy(ctx context.Context, key string, n int64) (int64, error)

	// Expire sets a TTL on an existing key.
	Expire(ctx context.Context, key string, ttl time.Duration) error

	// TTL returns the remaining TTL for a key.
	// Returns -1 if the key has no TTL, -2 if the key doesn't exist.
	TTL(ctx context.Context, key string) (time.Duration, error)

	// HGetAll returns all fields and values of a hash stored at key.
	HGetAll(ctx context.Context, key string) (map[string]string, error)

	// HSet sets fields in a hash stored at key. Values are key-value pairs.
	HSet(ctx context.Context, key string, values ...interface{}) error

	// ZAdd adds a member with score to the sorted set at key.
	ZAdd(ctx context.Context, key string, score float64, member string) error

	// ZCard returns the number of members in the sorted set at key.
	ZCard(ctx context.Context, key string) (int64, error)

	// ZRemRangeByScore removes sorted set members with scores in [min, max].
	ZRemRangeByScore(ctx context.Context, key, min, max string) error

	// ZRangeWithScores returns members with scores in the range [start, stop].
	ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]ZEntry, error)

	// Pipeline returns a Pipeline for batching multiple commands.
	Pipeline() Pipeline

	// Close releases any resources held by the store.
	Close() error
}

// ZEntry represents a sorted set member with its score.
type ZEntry struct {
	Score  float64
	Member string
}

// Pipeline batches multiple commands for a single round-trip.
type Pipeline interface {
	ZAdd(ctx context.Context, key string, score float64, member string)
	Expire(ctx context.Context, key string, ttl time.Duration)
	Exec(ctx context.Context) error
}

// ErrKeyNotFound is returned by Get when the key doesn't exist.
type ErrKeyNotFound struct {
	Key string
}

func (e *ErrKeyNotFound) Error() string {
	return "store: key not found: " + e.Key
}

// ErrScriptNotSupported is returned by Eval/EvalSha when the store
// doesn't support server-side scripting.
type ErrScriptNotSupported struct{}

func (e *ErrScriptNotSupported) Error() string {
	return "store: scripting not supported by this backend"
}
