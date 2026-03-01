// Package redis provides a Redis-backed implementation of store.Store.
//
// It wraps redis.UniversalClient, which supports Redis standalone,
// Redis Cluster, and Redis Sentinel out of the box.
//
//	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	s := redisstore.New(client)
//
//	// Or with Redis Cluster:
//	client := redis.NewClusterClient(&redis.ClusterOptions{
//	    Addrs: []string{"node1:6379", "node2:6379", "node3:6379"},
//	})
//	s := redisstore.New(client)
package redis

import (
	"context"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/krishna-kudari/ratelimit/store"
)

// Store implements store.Store backed by Redis.
type Store struct {
	client goredis.UniversalClient
}

// New creates a Redis-backed Store from any UniversalClient
// (standalone *redis.Client, *redis.ClusterClient, or *redis.Ring).
func New(client goredis.UniversalClient) *Store {
	return &Store{client: client}
}

// Client returns the underlying Redis client.
func (s *Store) Client() goredis.UniversalClient {
	return s.client
}

func (s *Store) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return s.client.Eval(ctx, script, keys, args...).Result()
}

func (s *Store) EvalSha(ctx context.Context, sha1 string, keys []string, args ...interface{}) (interface{}, error) {
	return s.client.EvalSha(ctx, sha1, keys, args...).Result()
}

func (s *Store) ScriptLoad(ctx context.Context, script string) (string, error) {
	return s.client.ScriptLoad(ctx, script).Result()
}

func (s *Store) Get(ctx context.Context, key string) (string, error) {
	val, err := s.client.Get(ctx, key).Result()
	if err == goredis.Nil {
		return "", &store.ErrKeyNotFound{Key: key}
	}
	return val, err
}

func (s *Store) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *Store) Del(ctx context.Context, keys ...string) error {
	return s.client.Del(ctx, keys...).Err()
}

func (s *Store) IncrBy(ctx context.Context, key string, n int64) (int64, error) {
	return s.client.IncrBy(ctx, key, n).Result()
}

func (s *Store) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return s.client.Expire(ctx, key, ttl).Err()
}

func (s *Store) TTL(ctx context.Context, key string) (time.Duration, error) {
	return s.client.TTL(ctx, key).Result()
}

func (s *Store) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return s.client.HGetAll(ctx, key).Result()
}

func (s *Store) HSet(ctx context.Context, key string, values ...interface{}) error {
	return s.client.HSet(ctx, key, values...).Err()
}

func (s *Store) ZAdd(ctx context.Context, key string, score float64, member string) error {
	return s.client.ZAdd(ctx, key, goredis.Z{Score: score, Member: member}).Err()
}

func (s *Store) ZCard(ctx context.Context, key string) (int64, error) {
	return s.client.ZCard(ctx, key).Result()
}

func (s *Store) ZRemRangeByScore(ctx context.Context, key, min, max string) error {
	return s.client.ZRemRangeByScore(ctx, key, min, max).Err()
}

func (s *Store) ZRangeWithScores(ctx context.Context, key string, start, stop int64) ([]store.ZEntry, error) {
	results, err := s.client.ZRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}
	entries := make([]store.ZEntry, len(results))
	for i, z := range results {
		member, _ := z.Member.(string)
		entries[i] = store.ZEntry{Score: z.Score, Member: member}
	}
	return entries, nil
}

func (s *Store) Pipeline() store.Pipeline {
	return &redisPipeline{pipe: s.client.Pipeline()}
}

func (s *Store) Close() error {
	return s.client.Close()
}

// ─── Pipeline ────────────────────────────────────────────────────────────────

type redisPipeline struct {
	pipe goredis.Pipeliner
}

func (p *redisPipeline) ZAdd(ctx context.Context, key string, score float64, member string) {
	p.pipe.ZAdd(ctx, key, goredis.Z{Score: score, Member: member})
}

func (p *redisPipeline) Expire(ctx context.Context, key string, ttl time.Duration) {
	p.pipe.Expire(ctx, key, ttl)
}

func (p *redisPipeline) Exec(ctx context.Context) error {
	_, err := p.pipe.Exec(ctx)
	return err
}
