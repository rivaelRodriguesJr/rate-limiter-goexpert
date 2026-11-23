package storage

import (
	"context"
	"sync"
	"time"
)

type counter struct {
	n       int64
	expires time.Time
}

// Memory is an in-memory Store, intended for tests and local development.
type Memory struct {
	clock    Clock
	mu       sync.Mutex
	counters map[string]counter
	blocks   map[string]time.Time
}

// NewMemory creates a Memory store. A nil clock means the system clock.
func NewMemory(clock Clock) *Memory {
	if clock == nil {
		clock = SystemClock{}
	}
	return &Memory{
		clock:    clock,
		counters: make(map[string]counter),
		blocks:   make(map[string]time.Time),
	}
}

// Incr implements Store.
func (m *Memory) Incr(_ context.Context, key string, window time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	c, ok := m.counters[key]
	if !ok || !now.Before(c.expires) {
		c = counter{expires: now.Add(window)}
	}
	c.n++
	m.counters[key] = c
	return c.n, nil
}

// BlockTTL implements Store.
func (m *Memory) BlockTTL(_ context.Context, key string) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	until, ok := m.blocks[key]
	if !ok {
		return 0, nil
	}
	now := m.clock.Now()
	if !now.Before(until) {
		delete(m.blocks, key)
		return 0, nil
	}
	return until.Sub(now), nil
}

// Block implements Store.
func (m *Memory) Block(_ context.Context, key string, d time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blocks[key] = m.clock.Now().Add(d)
	return nil
}

// Ping implements Store.
func (m *Memory) Ping(context.Context) error { return nil }

// Close implements Store.
func (m *Memory) Close() error { return nil }
