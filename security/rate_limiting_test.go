package security

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkOgem "github.com/yanolja/ogem/sdk/go"
	"go.uber.org/zap/zaptest"
)

func TestAdvancedRateLimiter_NewAdvancedRateLimiter(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		GlobalLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  1000,
				Window: time.Hour,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	assert.NotNil(t, limiter)
	assert.Equal(t, config, limiter.config)
}

func TestAdvancedRateLimiter_CheckRateLimit_Success(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  10,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
			{
				Type:   RateLimitTypeTokens,
				Limit:  1000,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// First request should succeed
	result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 100, 0.01)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
	assert.Equal(t, int64(9), result.Remaining) // 10 - 1 = 9
	assert.True(t, result.ResetTime.After(time.Now()))
}

func TestAdvancedRateLimiter_CheckRateLimit_ExceedRequestLimit(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  2,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// Third request should be blocked
	result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.ErrorMessage, "Request rate limit exceeded")
}

func TestAdvancedRateLimiter_CheckRateLimit_ExceedTokenLimit(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeTokens,
				Limit:  100,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// Request with 50 tokens should succeed
	result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 50, 0.001)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Request with 60 more tokens should exceed limit
	result, err = limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 60, 0.001)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.Contains(t, result.ErrorMessage, "Token rate limit exceeded")
}

func TestAdvancedRateLimiter_CheckRateLimit_ModelSpecificLimits(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		ModelLimits: map[string][]RateLimit{
			sdkOgem.ModelGPT4: {
				{
					Type:   RateLimitTypeRequests,
					Limit:  5,
					Window: time.Minute,
					Action: RateLimitActionBlock,
				},
			},
			sdkOgem.ModelGPT35Turbo: {
				{
					Type:   RateLimitTypeRequests,
					Limit:  20,
					Window: time.Minute,
					Action: RateLimitActionBlock,
				},
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// Test GPT-4 limit (5 requests)
	for i := 0; i < 5; i++ {
		result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// 6th GPT-4 request should fail
	result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// But GPT-3.5-turbo should still work
	result, err = limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT35Turbo, 10, 0.001)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestAdvancedRateLimiter_CheckRateLimit_Disabled(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: false,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  1,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// Should allow unlimited requests when disabled
	for i := 0; i < 10; i++ {
		result, err := limiter.CheckRateLimit(ctx, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 100, 0.01)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}
}

func TestAdvancedRateLimiter_ConcurrentAccess(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  100,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// Test concurrent access
	var wg sync.WaitGroup
	successCount := 0
	failCount := 0
	var mu sync.Mutex

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			result, err := limiter.CheckRateLimit(ctx, "concurrent-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			if result.Allowed {
				successCount++
			} else {
				failCount++
			}
		}(i)
	}

	wg.Wait()
	assert.Equal(t, 50, successCount+failCount)
	assert.Greater(t, successCount, 0)
}

func TestAdvancedRateLimiter_SlidingWindow(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  10,
				Window: 2 * time.Second,
				Action: RateLimitActionBlock,
			},
		},
		SlidingWindow: &SlidingWindowConfig{
			Enabled:    true,
			WindowSize: 2 * time.Second,
			SubWindows: 4,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// Use up the limit
	for i := 0; i < 10; i++ {
		result, err := limiter.CheckRateLimit(ctx, "sliding-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// Should be blocked now
	result, err := limiter.CheckRateLimit(ctx, "sliding-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Wait for half the window
	time.Sleep(1 * time.Second)

	// Should still be blocked (sliding window)
	result, err = limiter.CheckRateLimit(ctx, "sliding-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.False(t, result.Allowed)

	// Wait for the full window to pass
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	result, err = limiter.CheckRateLimit(ctx, "sliding-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestAdvancedRateLimiter_GetRateLimitStats(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  10,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// Make some requests
	for i := 0; i < 5; i++ {
		_, err := limiter.CheckRateLimit(ctx, "stats-user", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
		require.NoError(t, err)
	}

	// Get stats
	stats := limiter.GetRateLimitStats()
	assert.NotNil(t, stats)
	assert.Equal(t, true, stats["enabled"])
	assert.GreaterOrEqual(t, stats["active_states"], 1)
}

func TestAdvancedRateLimiter_ConcurrentLimits(t *testing.T) {
	config := &RateLimitConfig{
		Enabled: true,
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeConcurrent,
				Limit:  2,
				Window: time.Minute,
				Action: RateLimitActionBlock,
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	limiter := NewAdvancedRateLimiter(config, logger)
	ctx := context.Background()

	// First two concurrent requests should succeed
	result1, err := limiter.CheckRateLimit(ctx, "concurrent-test", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.True(t, result1.Allowed)

	result2, err := limiter.CheckRateLimit(ctx, "concurrent-test", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.True(t, result2.Allowed)

	// Third concurrent request should fail
	result3, err := limiter.CheckRateLimit(ctx, "concurrent-test", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.False(t, result3.Allowed)

	// Release one resource
	limiter.ReleaseResource("concurrent-test", "/v1/chat/completions", sdkOgem.ModelGPT4)

	// Now a new request should succeed
	result4, err := limiter.CheckRateLimit(ctx, "concurrent-test", "/v1/chat/completions", sdkOgem.ModelGPT4, 10, 0.001)
	require.NoError(t, err)
	assert.True(t, result4.Allowed)
}
