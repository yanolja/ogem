package state

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/yanolja/ogem/utils/heap"
)

// New field costs: bool=1 intX=X/8 (e.g., int16=2) string=16 []byte=24 ptr=8
// key (16) + value (24) + expiry (8) + lastReadAt (8) + readCount (8) +
// Map/GC overhead (64) = 128
const cacheEntryOverhead = 128

// If any fields are changed, update cacheEntryOverhead.
type cacheEntry struct {
	// Unique identifier for the cache entry. E.g., "openai:us-east-1:gpt-4o"
	key string

	// Byte representation of the cached value.
	value []byte

	// Expiry time in unix nanoseconds.
	expiry int64

	// Last read time in unix nanoseconds.
	lastReadAt int64

	// Number of times the cache has been read. Starts from 1.
	readCount int64
}

type MemoryManager struct {
	// Key (provider:region:model) -> disabled_until (unix nanoseconds)
	state    map[string]int64
	stateMu  sync.RWMutex

	// Any string key -> cache entry
	cache    map[string]*cacheEntry

	// Priority queue for cache entries, ordered by a combination of read count
	// and last read time
	cacheHeap *heap.MinHeap[*cacheEntry]
	cacheMu   sync.RWMutex

	// Maximum size of the total cache in bytes. If exceeding, the least
	// frequently used and oldest cache will be removed.
	cacheMaxBytes int64

	// Current size of the cache in bytes
	cacheUsage int64

	// Clock interface for time-related operations. Must use this to avoid
	// flakiness in tests.
	clock clock.Clock
}

func NewMemoryManager(cacheMaxBytes int64) (*MemoryManager, func()) {
	return newMemoryManagerWithClock(cacheMaxBytes, clock.New())
}

func newMemoryManagerWithClock(
	cacheMaxBytes int64,
	clk clock.Clock,
) (*MemoryManager, func()) {
	m := &MemoryManager{
		state:         make(map[string]int64),
		cache:         make(map[string]*cacheEntry),
		cacheMaxBytes: cacheMaxBytes,
		cacheUsage:    0,
		clock:         clk,
	}

	// Less frequently used entries, and older entries are at the top.
	m.cacheHeap = heap.NewMinHeap(func(a *cacheEntry, b *cacheEntry) bool {
		if a.readCount != b.readCount {
			return a.readCount < b.readCount
		}
		if a.lastReadAt != b.lastReadAt {
			return a.lastReadAt < b.lastReadAt
		}
		return a.key < b.key
	})

	stop := m.startCleanup(5 * time.Minute)
	return m, stop
}

func (m *MemoryManager) Allow(
	ctx context.Context, provider string, region string, model string,
	// Interval between model uses.
	interval time.Duration,
) (bool, time.Duration, error) {
	key := getKey(provider, region, model)
	now := m.clock.Now().UnixNano()

	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	if disabledUntil, exists := m.state[key]; exists && disabledUntil > now {
		waitDuration := time.Duration(disabledUntil - now)
		return false, waitDuration, nil
	}

	m.state[key] = now + interval.Nanoseconds()
	return true, 0, nil
}

func (m *MemoryManager) Disable(
	ctx context.Context, provider string, region string, model string,
	duration time.Duration,
) error {
	key := getKey(provider, region, model)
	disabledUntil := m.clock.Now().Add(duration).UnixNano()

	m.stateMu.Lock()
	defer m.stateMu.Unlock()

	m.state[key] = disabledUntil
	return nil
}

func (m *MemoryManager) SaveCache(
	ctx context.Context, key string, value []byte, duration time.Duration,
) error {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	sizeToAdd := cacheSize(key, value)
	exceeding := m.cacheUsage + sizeToAdd - m.cacheMaxBytes
	if exceeding > 0 {
		if err := m.evictCache(exceeding); err != nil {
			return fmt.Errorf("failed to evict cache: %v", err)
		}
	}

	now := m.clock.Now().UnixNano()
	entry := &cacheEntry{
		key:        key,
		value:      value,
		expiry:     now + duration.Nanoseconds(),
		lastReadAt: now,
		readCount:  1,
	}

	if existing, exists := m.cache[key]; exists {
		m.cacheHeap.Remove(existing)
		m.cacheUsage -= cacheSize(existing.key, existing.value)
	}

	m.cache[key] = entry
	m.cacheHeap.Push(entry)
	m.cacheUsage += sizeToAdd
	return nil
}

func (m *MemoryManager) LoadCache(
	ctx context.Context, key string) ([]byte, error) {
	m.cacheMu.Lock()
	defer m.cacheMu.Unlock()

	entry, exists := m.cache[key]
	if !exists {
		return nil, nil
	}

	now := m.clock.Now().UnixNano()

	entry.lastReadAt = now
	entry.readCount++

	if entry.expiry <= now {
		m.deleteCache(entry)
		m.cacheHeap.Remove(entry)
		// Still returns the value because there is no point in not returning
		// the value.
	} else {
		m.cacheHeap.Update(entry)
	}

	return entry.value, nil
}

func getKey(provider string, region string, model string) string {
	return fmt.Sprintf("%s:%s:%s", provider, region, model)
}

func (m *MemoryManager) deleteCache(entry *cacheEntry) {
	delete(m.cache, entry.key)
	m.cacheHeap.Remove(entry)
	m.cacheUsage -= cacheSize(entry.key, entry.value)
}

func (m *MemoryManager) evictCache(sizeInBytes int64) error {
	bytesFreed := int64(0)
	for bytesFreed < sizeInBytes {
		entry, ok := m.cacheHeap.Pop()
		if !ok {
			return fmt.Errorf("failed to free enough cache space")
		}
		bytesFreed += cacheSize(entry.key, entry.value)
		delete(m.cache, entry.key)
	}
	m.cacheUsage -= bytesFreed
	return nil
}

func cacheSize(key string, value []byte) int64 {
	return cacheEntryOverhead + int64(len([]byte(key)) + len(value))
}

func (m *MemoryManager) cleanup() {
	now := m.clock.Now().UnixNano()

	m.stateMu.Lock()
	for key, disabledUntil := range m.state {
		if disabledUntil <= now {
			delete(m.state, key)
		}
	}
	m.stateMu.Unlock()

	m.cacheMu.Lock()
	var expiredEntries []*cacheEntry
	for _, entry := range m.cache {
		if entry.expiry <= now {
			expiredEntries = append(expiredEntries, entry)
		}
	}
	for _, entry := range expiredEntries {
		m.deleteCache(entry)
	}
	m.cacheMu.Unlock()
}

func (m *MemoryManager) startCleanup(interval time.Duration) func() {
	ticker := m.clock.Ticker(interval)
	done := make(chan bool)

	go func() {
		for {
			select {
			case <-ticker.C:
				m.cleanup()
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}
