package storage

import (
	"context"
	"sync"
	"time"
)

type counter struct {
	value   int64
	endTime time.Time
}

// Memory keeps everything in maps. We use it in the tests.
type Memory struct {
	clock    Clock
	mutex    sync.Mutex // protects the maps, because many requests run at the same time
	counters map[string]counter
	blocks   map[string]time.Time
}

func NewMemory(clock Clock) *Memory {
	if clock == nil {
		clock = RealClock{}
	}
	return &Memory{
		clock:    clock,
		counters: make(map[string]counter),
		blocks:   make(map[string]time.Time),
	}
}

func (m *Memory) Incr(ctx context.Context, key string, window time.Duration) (int64, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := m.clock.Now()

	c, found := m.counters[key]
	if !found || !now.Before(c.endTime) {
		// no counter yet, or the old window is over: start again
		c = counter{value: 0, endTime: now.Add(window)}
	}
	c.value++
	m.counters[key] = c

	return c.value, nil
}

func (m *Memory) BlockTTL(ctx context.Context, key string) (time.Duration, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	endTime, found := m.blocks[key]
	if !found {
		return 0, nil
	}

	now := m.clock.Now()
	if !now.Before(endTime) {
		delete(m.blocks, key)
		return 0, nil
	}
	return endTime.Sub(now), nil
}

func (m *Memory) Block(ctx context.Context, key string, duration time.Duration) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.blocks[key] = m.clock.Now().Add(duration)
	return nil
}

func (m *Memory) Ping(ctx context.Context) error {
	return nil
}

func (m *Memory) Close() error {
	return nil
}
