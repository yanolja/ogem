package state

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
)

func TestMemoryManager(t *testing.T) {
	t.Run("New memory manager", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		assert.NotNil(t, manager)
		assert.NotNil(t, manager.state)
		assert.NotNil(t, manager.cache)
		assert.NotNil(t, manager.cacheHeap)
		assert.Equal(t, int64(1024), manager.cacheMaxBytes)
		assert.Equal(t, int64(0), manager.cacheUsage)
	})

	t.Run("Allow and Disable", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()
		interval := 100 * time.Millisecond

		// Initial request should be allowed
		allowed, wait, err := manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, time.Duration(0), wait)

		// Request within interval should not be allowed
		allowed, wait, err = manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.False(t, allowed)
		assert.True(t, wait > 0)

		// Advance clock by interval
		mockClock.Add(interval)

		// Request after interval should be allowed
		allowed, wait, err = manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, time.Duration(0), wait)

		// Disable the model
		disableDuration := 200 * time.Millisecond
		err = manager.Disable(ctx, "openai", "us-east-1", "gpt-4o", disableDuration)
		assert.NoError(t, err)

		// Request while disabled should not be allowed
		allowed, wait, err = manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.False(t, allowed)
		assert.True(t, wait > 0)

		// Advance clock by disable duration
		mockClock.Add(disableDuration)

		// Request after disable period should be allowed
		allowed, wait, err = manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, time.Duration(0), wait)
	})

	t.Run("Cache operations", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()
		key := "test-key"
		value := []byte("test-value")
		duration := 100 * time.Millisecond

		// Save to cache
		err := manager.SaveCache(ctx, key, value, duration)
		assert.NoError(t, err)

		// Load from cache
		loadedValue, err := manager.LoadCache(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, loadedValue)

		// Advance clock past expiration
		mockClock.Add(duration)

		// Load expired cache
		loadedValue, err = manager.LoadCache(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, loadedValue) // Should still return value even if expired

		// Load non-existent key
		loadedValue, err = manager.LoadCache(ctx, "non-existent")
		assert.NoError(t, err)
		assert.Nil(t, loadedValue)
	})

	t.Run("Cache eviction", func(t *testing.T) {
		mockClock := clock.NewMock()
		maxBytes := int64(256)
		manager, cleanup := newMemoryManagerWithClock(maxBytes, mockClock)
		defer cleanup()

		ctx := context.Background()
		duration := 1 * time.Hour

		// Fill cache beyond capacity
		for i := 0; i < 10; i++ {
			key := fmt.Sprintf("key-%d", i)
			value := []byte(fmt.Sprintf("value-%d", i))
			err := manager.SaveCache(ctx, key, value, duration)
			assert.NoError(t, err)
			assert.True(t, manager.cacheUsage <= maxBytes)
		}

		// Verify least frequently used entries were evicted
		totalEntries := len(manager.cache)
		assert.True(t, totalEntries < 10)
		assert.True(t, manager.cacheUsage <= maxBytes)
	})

	t.Run("Cache cleanup", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()
		shortDuration := 100 * time.Millisecond

		// Add entries with short expiration
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("key-%d", i)
			value := []byte(fmt.Sprintf("value-%d", i))
			err := manager.SaveCache(ctx, key, value, shortDuration)
			assert.NoError(t, err)
		}

		// Initial count
		initialCount := len(manager.cache)
		assert.Equal(t, 5, initialCount)

		// Advance clock past expiration
		mockClock.Add(shortDuration)

		// Force cleanup
		manager.cleanup()

		// Verify expired entries were removed
		assert.Equal(t, 0, len(manager.cache))
		assert.Equal(t, int64(0), manager.cacheUsage)
	})

	t.Run("Cache read count updates", func(t *testing.T) {
		mockClock := clock.NewMock()
		mockClock.Set(time.Unix(0, 0))
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()
		key := "test-key"
		value := []byte("test-value")
		duration := time.Hour

		// Save to cache
		err := manager.SaveCache(ctx, key, value, duration)
		assert.NoError(t, err)

		// Initial read count should be 1
		entry := manager.cache[key]
		assert.Equal(t, int64(1), entry.readCount)

		loadedValue, err := manager.LoadCache(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, loadedValue)

		mockClock.Add(time.Millisecond)

		loadedValue, err = manager.LoadCache(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, loadedValue)

		mockClock.Add(time.Millisecond)

		loadedValue, err = manager.LoadCache(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value, loadedValue)

		// Initial read count is 1 and plus 3 more reads.
		assert.Equal(t, int64(4), entry.readCount)
		assert.Equal(t, int64(2000000), entry.lastReadAt)
	})

	t.Run("Precise waiting durations", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()
		interval := 100 * time.Millisecond

		// Initial request
		allowed, _, err := manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.True(t, allowed)

		// Request exactly 50ms after
		mockClock.Add(50 * time.Millisecond)
		allowed, wait, err := manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.False(t, allowed)
		assert.Equal(t, 50*time.Millisecond, wait)

		// Request exactly at interval boundary
		mockClock.Add(50 * time.Millisecond)
		allowed, wait, err = manager.Allow(ctx, "openai", "us-east-1", "gpt-4o", interval)
		assert.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, time.Duration(0), wait)
	})

	t.Run("Cleanup timer behavior", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()

		// Add some entries that will expire at different times
		for i := 0; i < 5; i++ {
			// State entries
			err := manager.Disable(ctx, "provider", "region", fmt.Sprintf("model-%d", i), time.Duration(i+1)*time.Minute)
			assert.NoError(t, err)

			// Cache entries
			err = manager.SaveCache(ctx, fmt.Sprintf("key-%d", i), []byte("value"), time.Duration(i+1)*time.Minute)
			assert.NoError(t, err)
		}

		// Verify initial counts
		assert.Equal(t, 5, len(manager.state))
		assert.Equal(t, 5, len(manager.cache))

		// Advance clock just past first minute
		mockClock.Add(61 * time.Second)
		manager.cleanup()

		// Verify first entries expired
		assert.Equal(t, 4, len(manager.state))
		assert.Equal(t, 4, len(manager.cache))

		// Advance clock past all expiration times
		mockClock.Add(5 * time.Minute)
		manager.cleanup()

		// Verify all entries expired
		assert.Equal(t, 0, len(manager.state))
		assert.Equal(t, 0, len(manager.cache))
	})

	t.Run("Cache overwrite behavior", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(1024, mockClock)
		defer cleanup()

		ctx := context.Background()
		key := "test-key"
		value1 := []byte("value1")
		value2 := []byte("value2")

		// Save initial value
		err := manager.SaveCache(ctx, key, value1, time.Hour)
		assert.NoError(t, err)
		initialUsage := manager.cacheUsage

		// Override with new value
		err = manager.SaveCache(ctx, key, value2, time.Hour)
		assert.NoError(t, err)

		// Verify new value is present
		loaded, err := manager.LoadCache(ctx, key)
		assert.NoError(t, err)
		assert.Equal(t, value2, loaded)

		// Verify cache size is correct after overwrite
		expectedSize := cacheEntryOverhead + int64(len(key)+len(value2))
		assert.Equal(t, expectedSize, manager.cacheUsage)
		assert.Less(t, manager.cacheUsage, initialUsage+expectedSize)
	})

	t.Run("Cache eviction order", func(t *testing.T) {
		mockClock := clock.NewMock()
		// Enough to hold 3 entries (4 bytes key + 5 bytes value + overhead) but
		// not enough to hold 4 entries.
		maxBytes := int64((cacheEntryOverhead+4+5)*4 - 1)
		manager, cleanup := newMemoryManagerWithClock(maxBytes, mockClock)
		defer cleanup()

		ctx := context.Background()

		// Add entries and access them different numbers of times
		keys := []string{"key1", "key2", "key3"}
		accessCounts := map[string]int{
			"key1": 1,  // Least frequently accessed
			"key2": 5,  // More frequently accessed
			"key3": 10, // Most frequently accessed
		}

		// Save all entries
		for _, key := range keys {
			err := manager.SaveCache(ctx, key, []byte("value"), time.Hour)
			assert.NoError(t, err)

			// Access each entry according to its count
			for i := 0; i < accessCounts[key]; i++ {
				_, err := manager.LoadCache(ctx, key)
				assert.NoError(t, err)
				mockClock.Add(time.Millisecond) // Space out accesses
			}
		}

		// Add a large entry to force eviction
		err := manager.SaveCache(ctx, "key4", []byte("value"), time.Hour)
		assert.NoError(t, err)

		// Verify least frequently accessed entries were evicted first
		_, err = manager.LoadCache(ctx, "key1")
		assert.NoError(t, err)
		assert.Nil(t, manager.cache["key1"]) // Should be evicted

		_, err = manager.LoadCache(ctx, "key3")
		assert.NoError(t, err)
		assert.NotNil(t, manager.cache["key3"]) // Should still exist
	})

	t.Run("Edge cases", func(t *testing.T) {
		mockClock := clock.NewMock()
		manager, cleanup := newMemoryManagerWithClock(5*1024*1024, mockClock)
		defer cleanup()

		ctx := context.Background()

		t.Run("Zero duration", func(t *testing.T) {
			err := manager.SaveCache(ctx, "key", []byte("value"), 0)
			assert.NoError(t, err)

			// Should be immediately expired
			mockClock.Add(1 * time.Nanosecond)
			manager.cleanup()
			assert.Empty(t, manager.cache)
		})

		t.Run("Negative duration", func(t *testing.T) {
			err := manager.SaveCache(ctx, "key", []byte("value"), -time.Hour)
			assert.NoError(t, err)

			// Should be immediately expired
			mockClock.Add(1 * time.Nanosecond)
			manager.cleanup()
			assert.Empty(t, manager.cache)
		})

		t.Run("Empty key", func(t *testing.T) {
			err := manager.SaveCache(ctx, "", []byte("value"), time.Hour)
			assert.NoError(t, err)

			loaded, err := manager.LoadCache(ctx, "")
			assert.NoError(t, err)
			assert.NotNil(t, loaded)
		})

		t.Run("Nil value", func(t *testing.T) {
			err := manager.SaveCache(ctx, "key", nil, time.Hour)
			assert.NoError(t, err)

			loaded, err := manager.LoadCache(ctx, "key")
			assert.NoError(t, err)
			assert.Empty(t, loaded)
		})

		t.Run("Very large value", func(t *testing.T) {
			largeValue := bytes.Repeat([]byte("x"), 1024*1024) // 1MB
			err := manager.SaveCache(ctx, "key", largeValue, time.Hour)
			assert.NoError(t, err)

			loaded, err := manager.LoadCache(ctx, "key")
			assert.NoError(t, err)
			assert.Equal(t, largeValue, loaded)
		})
	})
}
