package security

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRateLimiter_NewRateLimiter(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 1000,
		DefaultTokensPerHour:   100000,
		SlidingWindowSize:      time.Hour,
		BurstMultiplier:        2.0,
	}

	limiter := NewRateLimiter(config)
	assert.NotNil(t, limiter)
	assert.Equal(t, config, limiter.config)
}

func TestRateLimiter_CheckRateLimit_Success(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Minute,
		BurstMultiplier:        2.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// First request should succeed
	allowed, remaining, resetTime, err := limiter.CheckRateLimit(ctx, "test-key", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 9, remaining) // 10 - 1 = 9
	assert.True(t, resetTime.After(time.Now()))
}

func TestRateLimiter_CheckRateLimit_ExceedRequestLimit(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 2,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Minute,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Use up the request limit
	for i := 0; i < 2; i++ {
		allowed, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 100)
		require.NoError(t, err)
		assert.True(t, allowed)
	}

	// Third request should be denied
	allowed, remaining, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 100)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, 0, remaining)
}

func TestRateLimiter_CheckRateLimit_ExceedTokenLimit(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   500,
		SlidingWindowSize:      time.Minute,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Use up the token limit with large token requests
	allowed, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 300)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, _, _, err = limiter.CheckRateLimit(ctx, "test-key", 1, 300)
	require.NoError(t, err)
	assert.False(t, allowed) // Should exceed token limit (300 + 300 > 500)
}

func TestRateLimiter_CustomLimits(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Minute,
		BurstMultiplier:        1.0,
		CustomLimits: map[string]*RateLimit{
			"premium-user": {
				RequestsPerHour: 100,
				TokensPerHour:   10000,
			},
		},
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Test with custom limit key
	allowed, remaining, _, err := limiter.CheckRateLimit(ctx, "premium-user", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 99, remaining) // 100 - 1 = 99

	// Test with default limit key
	allowed, remaining, _, err = limiter.CheckRateLimit(ctx, "regular-user", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 9, remaining) // 10 - 1 = 9
}

func TestRateLimiter_SlidingWindow(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 2,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      100 * time.Millisecond, // Very short window for testing
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Use up the limit
	allowed, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)

	allowed, _, _, err = limiter.CheckRateLimit(ctx, "test-key", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)

	// Should be rate limited
	allowed, _, _, err = limiter.CheckRateLimit(ctx, "test-key", 1, 100)
	require.NoError(t, err)
	assert.False(t, allowed)

	// Wait for window to slide
	time.Sleep(150 * time.Millisecond)

	// Should be allowed again
	allowed, _, _, err = limiter.CheckRateLimit(ctx, "test-key", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestRateLimiter_BurstMultiplier(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Hour,
		BurstMultiplier:        2.0, // Allow burst up to 20 requests
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Should allow burst up to 20 requests (10 * 2.0)
	for i := 0; i < 20; i++ {
		allowed, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 50)
		require.NoError(t, err)
		assert.True(t, allowed, "Request %d should be allowed", i+1)
	}

	// 21st request should be denied
	allowed, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 50)
	require.NoError(t, err)
	assert.False(t, allowed)
}

func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 100,
		DefaultTokensPerHour:   10000,
		SlidingWindowSize:      time.Hour,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	const numGoroutines = 10
	const requestsPerGoroutine = 5

	var wg sync.WaitGroup
	var allowedCount int64
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(routineID int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				allowed, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 100)
				require.NoError(t, err)
				
				if allowed {
					mu.Lock()
					allowedCount++
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	// Should allow exactly 100 requests (the limit)
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, int64(50), allowedCount) // 50 total requests, all should be allowed
}

func TestRateLimiter_Disabled(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled: false,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// When disabled, should always allow
	for i := 0; i < 1000; i++ {
		allowed, remaining, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 1000)
		require.NoError(t, err)
		assert.True(t, allowed)
		assert.Equal(t, -1, remaining) // -1 indicates unlimited
	}
}

func TestRateLimiter_GetUsage(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Hour,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Make some requests
	limiter.CheckRateLimit(ctx, "test-key", 3, 300)
	limiter.CheckRateLimit(ctx, "test-key", 2, 200)

	usage := limiter.GetUsage("test-key")
	assert.Equal(t, 5, usage.RequestCount)
	assert.Equal(t, 500, usage.TokenCount)
	assert.True(t, usage.WindowStart.Before(time.Now()))
}

func TestRateLimiter_ResetUsage(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Hour,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Make some requests
	limiter.CheckRateLimit(ctx, "test-key", 5, 500)

	// Verify usage
	usage := limiter.GetUsage("test-key")
	assert.Equal(t, 5, usage.RequestCount)
	assert.Equal(t, 500, usage.TokenCount)

	// Reset usage
	limiter.ResetUsage("test-key")

	// Verify usage is reset
	usage = limiter.GetUsage("test-key")
	assert.Equal(t, 0, usage.RequestCount)
	assert.Equal(t, 0, usage.TokenCount)
}

func TestRateLimiter_CleanupExpiredEntries(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      10 * time.Millisecond, // Very short window
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Make requests with different keys
	limiter.CheckRateLimit(ctx, "key1", 1, 100)
	limiter.CheckRateLimit(ctx, "key2", 1, 100)
	limiter.CheckRateLimit(ctx, "key3", 1, 100)

	// Wait for entries to expire
	time.Sleep(20 * time.Millisecond)

	// Trigger cleanup by making a new request
	limiter.CheckRateLimit(ctx, "key4", 1, 100)

	// The internal cleanup should have removed expired entries
	// We can't directly test this without exposing internal state,
	// but we can verify the limiter still works correctly
	allowed, _, _, err := limiter.CheckRateLimit(ctx, "key1", 1, 100)
	require.NoError(t, err)
	assert.True(t, allowed)
}

func TestUsageTracker_WindowBoundaries(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      100 * time.Millisecond,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	// Add requests at specific times to test window sliding
	now := time.Now()
	
	// Add request at start of window
	limiter.CheckRateLimit(ctx, "test-key", 5, 500)
	
	// Verify usage
	usage := limiter.GetUsage("test-key")
	assert.Equal(t, 5, usage.RequestCount)
	assert.Equal(t, 500, usage.TokenCount)
	
	// Wait for partial window slide
	time.Sleep(50 * time.Millisecond)
	
	// Add more requests
	limiter.CheckRateLimit(ctx, "test-key", 3, 300)
	
	usage = limiter.GetUsage("test-key")
	assert.Equal(t, 8, usage.RequestCount) // 5 + 3
	assert.Equal(t, 800, usage.TokenCount) // 500 + 300
	
	// Wait for first requests to fall out of window
	time.Sleep(60 * time.Millisecond)
	
	// First requests should have expired, only recent ones should count
	usage = limiter.GetUsage("test-key")
	assert.Equal(t, 3, usage.RequestCount) // Only the recent 3
	assert.Equal(t, 300, usage.TokenCount) // Only the recent 300
}

func TestRateLimiter_EdgeCases(t *testing.T) {
	config := &RateLimitingConfig{
		Enabled:                true,
		DefaultRequestsPerHour: 10,
		DefaultTokensPerHour:   1000,
		SlidingWindowSize:      time.Hour,
		BurstMultiplier:        1.0,
	}

	limiter := NewRateLimiter(config)
	ctx := context.Background()

	tests := []struct {
		name         string
		key          string
		requests     int
		tokens       int
		expectError  bool
		expectAllowed bool
	}{
		{
			name:          "empty key",
			key:           "",
			requests:      1,
			tokens:        100,
			expectError:   false,
			expectAllowed: true,
		},
		{
			name:          "zero requests",
			key:           "test-key",
			requests:      0,
			tokens:        100,
			expectError:   false,
			expectAllowed: true,
		},
		{
			name:          "zero tokens",
			key:           "test-key",
			requests:      1,
			tokens:        0,
			expectError:   false,
			expectAllowed: true,
		},
		{
			name:          "negative requests",
			key:           "test-key",
			requests:      -1,
			tokens:        100,
			expectError:   false,
			expectAllowed: true, // Should handle gracefully
		},
		{
			name:          "negative tokens",
			key:           "test-key",
			requests:      1,
			tokens:        -100,
			expectError:   false,
			expectAllowed: true, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, _, _, err := limiter.CheckRateLimit(ctx, tt.key, tt.requests, tt.tokens)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectAllowed, allowed)
			}
		})
	}
}

func TestRateLimiter_Configuration_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  *RateLimitingConfig
		isValid bool
	}{
		{
			name: "valid config",
			config: &RateLimitingConfig{
				Enabled:                true,
				DefaultRequestsPerHour: 100,
				DefaultTokensPerHour:   10000,
				SlidingWindowSize:      time.Hour,
				BurstMultiplier:        2.0,
			},
			isValid: true,
		},
		{
			name: "zero burst multiplier",
			config: &RateLimitingConfig{
				Enabled:                true,
				DefaultRequestsPerHour: 100,
				DefaultTokensPerHour:   10000,
				SlidingWindowSize:      time.Hour,
				BurstMultiplier:        0.0,
			},
			isValid: true, // Should handle gracefully
		},
		{
			name: "negative limits",
			config: &RateLimitingConfig{
				Enabled:                true,
				DefaultRequestsPerHour: -100,
				DefaultTokensPerHour:   -10000,
				SlidingWindowSize:      time.Hour,
				BurstMultiplier:        1.0,
			},
			isValid: true, // Should handle gracefully
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiter := NewRateLimiter(tt.config)
			assert.NotNil(t, limiter)
			
			// Test that it doesn't panic with edge case configurations
			ctx := context.Background()
			_, _, _, err := limiter.CheckRateLimit(ctx, "test-key", 1, 100)
			assert.NoError(t, err)
		})
	}
}