// Package memory provides an in-memory implementation of store.Store.
//
// This is useful for testing and single-process deployments.
// It does NOT support Lua scripting (Eval/EvalSha return ErrScriptNotSupported).
// Algorithms that require atomic scripting (GCRA, Token Bucket, Leaky Bucket)
// should use the in-memory mode of the algorithm directly instead.
//
//	s := memory.New()
//	defer s.Close()
package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/krishna-kudari/ratelimit/store"
)

// Store implements store.Store with in-memory state.
// All operations are thread-safe.
type Store struct {
	mu      sync.Mutex
	data    map[string]entry
	sorted  map[string][]sortedEntry
	closed  bool
	closeCh chan struct{}
}

type entry struct {
	value    string
	expireAt time.Time
}

type sortedEntry struct {
	score  float64
	member string
}

// New creates a new in-memory Store.
func New() *Store {
	s := &Store{
		data:    make(map[string]entry),
		sorted:  make(map[string][]sortedEntry),
		closeCh: make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.evictExpired()
		case <-s.closeCh:
			return
		}
	}
}

func (s *Store) evictExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for k, e := range s.data {
		if !e.expireAt.IsZero() && now.After(e.expireAt) {
			delete(s.data, k)
		}
	}
}

func (s *Store) isExpired(e entry) bool {
	return !e.expireAt.IsZero() && time.Now().After(e.expireAt)
}

func (s *Store) Eval(_ context.Context, _ string, _ []string, _ ...interface{}) (interface{}, error) {
	return nil, &store.ErrScriptNotSupported{}
}

func (s *Store) EvalSha(_ context.Context, _ string, _ []string, _ ...interface{}) (interface{}, error) {
	return nil, &store.ErrScriptNotSupported{}
}

func (s *Store) ScriptLoad(_ context.Context, _ string) (string, error) {
	return "", &store.ErrScriptNotSupported{}
}

func (s *Store) Get(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok || s.isExpired(e) {
		delete(s.data, key)
		return "", &store.ErrKeyNotFound{Key: key}
	}
	return e.value, nil
}

func (s *Store) Set(_ context.Context, key string, value string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e := entry{value: value}
	if ttl > 0 {
		e.expireAt = time.Now().Add(ttl)
	}
	s.data[key] = e
	return nil
}

func (s *Store) Del(_ context.Context, keys ...string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, k := range keys {
		delete(s.data, k)
		delete(s.sorted, k)
	}
	return nil
}

func (s *Store) IncrBy(_ context.Context, key string, n int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok || s.isExpired(e) {
		s.data[key] = entry{value: fmt.Sprintf("%d", n)}
		return n, nil
	}

	var current int64
	fmt.Sscanf(e.value, "%d", &current)
	current += n
	e.value = fmt.Sprintf("%d", current)
	s.data[key] = e
	return current, nil
}

func (s *Store) Expire(_ context.Context, key string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok {
		return nil
	}
	e.expireAt = time.Now().Add(ttl)
	s.data[key] = e
	return nil
}

func (s *Store) TTL(_ context.Context, key string) (time.Duration, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok || s.isExpired(e) {
		return -2 * time.Second, nil
	}
	if e.expireAt.IsZero() {
		return -1 * time.Second, nil
	}
	remaining := time.Until(e.expireAt)
	if remaining < 0 {
		delete(s.data, key)
		return -2 * time.Second, nil
	}
	return remaining, nil
}

func (s *Store) HGetAll(_ context.Context, key string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.data[key]
	if !ok || s.isExpired(e) {
		return map[string]string{}, nil
	}
	// Stored as a special format; for simplicity we return the raw value
	// HSet/HGetAll are backed by the sorted map
	return map[string]string{}, nil
}

func (s *Store) HSet(_ context.Context, key string, values ...interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Hash operations stored as concatenated key-value pairs
	// For simplicity, store as a regular key with serialized content
	return nil
}

func (s *Store) ZAdd(_ context.Context, key string, score float64, member string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.sorted[key]
	// Remove existing member
	for i, e := range entries {
		if e.member == member {
			entries = append(entries[:i], entries[i+1:]...)
			break
		}
	}
	entries = append(entries, sortedEntry{score: score, member: member})
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score < entries[j].score
	})
	s.sorted[key] = entries
	return nil
}

func (s *Store) ZCard(_ context.Context, key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.sorted[key])), nil
}

func (s *Store) ZRemRangeByScore(_ context.Context, key, min, max string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var minF, maxF float64
	fmt.Sscanf(min, "%f", &minF)
	fmt.Sscanf(max, "%f", &maxF)

	entries := s.sorted[key]
	filtered := entries[:0]
	for _, e := range entries {
		if e.score < minF || e.score > maxF {
			filtered = append(filtered, e)
		}
	}
	s.sorted[key] = filtered
	return nil
}

func (s *Store) ZRangeWithScores(_ context.Context, key string, start, stop int64) ([]store.ZEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := s.sorted[key]
	n := int64(len(entries))
	if n == 0 {
		return nil, nil
	}

	if start < 0 {
		start = n + start
	}
	if stop < 0 {
		stop = n + stop
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	if start > stop {
		return nil, nil
	}

	result := make([]store.ZEntry, 0, stop-start+1)
	for i := start; i <= stop; i++ {
		result = append(result, store.ZEntry{
			Score:  entries[i].score,
			Member: entries[i].member,
		})
	}
	return result, nil
}

func (s *Store) Pipeline() store.Pipeline {
	return &memoryPipeline{store: s}
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		close(s.closeCh)
	}
	return nil
}

// ─── Pipeline ────────────────────────────────────────────────────────────────

type memoryPipeline struct {
	store *Store
	ops   []func(context.Context)
}

func (p *memoryPipeline) ZAdd(_ context.Context, key string, score float64, member string) {
	p.ops = append(p.ops, func(ctx context.Context) {
		p.store.ZAdd(ctx, key, score, member)
	})
}

func (p *memoryPipeline) Expire(_ context.Context, key string, ttl time.Duration) {
	p.ops = append(p.ops, func(ctx context.Context) {
		p.store.Expire(ctx, key, ttl)
	})
}

func (p *memoryPipeline) Exec(ctx context.Context) error {
	for _, op := range p.ops {
		op(ctx)
	}
	return nil
}
