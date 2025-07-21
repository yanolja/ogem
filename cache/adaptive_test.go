package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem/openai"
)

func TestCacheManager_AdaptiveStrategy(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyAdaptive,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		AdaptiveConfig: &AdaptiveConfig{
			LearningWindow:         time.Hour,
			MinSamples:            10,
			Sensitivity:           0.1,
			HighHitThreshold:      0.8,
			LowHitThreshold:       0.3,
			EnablePatternDetection: true,
			EnableAutoTuning:       true,
			TuningInterval:        30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Verify adaptive state is initialized
	assert.NotNil(t, manager.adaptiveState)
	assert.Equal(t, StrategyExact, manager.adaptiveState.CurrentStrategy) // Starts with exact
	assert.NotNil(t, manager.adaptiveState.PatternDetection)
	assert.Equal(t, 0, manager.adaptiveState.SampleCount)

	// Test getActiveStrategy returns adaptive strategy's current strategy
	activeStrategy := manager.getActiveStrategy()
	assert.Equal(t, StrategyExact, activeStrategy)

	// Change adaptive strategy
	manager.adaptiveState.mutex.Lock()
	manager.adaptiveState.CurrentStrategy = StrategySemantic
	manager.adaptiveState.mutex.Unlock()

	activeStrategy = manager.getActiveStrategy()
	assert.Equal(t, StrategySemantic, activeStrategy)
}

func TestCacheManager_PerformAdaptiveTuning(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyAdaptive,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		AdaptiveConfig: &AdaptiveConfig{
			LearningWindow:    time.Minute, // Short window for testing
			MinSamples:       5,
			Sensitivity:      0.1,
			HighHitThreshold: 0.8,
			LowHitThreshold:  0.3,
			EnableAutoTuning: true,
			TuningInterval:   30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Test tuning with insufficient samples
	manager.adaptiveState.SampleCount = 3
	manager.adaptiveState.LastEvaluation = time.Now().Add(-2 * time.Minute)

	originalStrategy := manager.adaptiveState.CurrentStrategy
	manager.performAdaptiveTuning()

	// Should not change strategy with insufficient samples
	assert.Equal(t, originalStrategy, manager.adaptiveState.CurrentStrategy)

	// Test tuning with low hit rate
	manager.adaptiveState.SampleCount = 10
	manager.adaptiveState.LastEvaluation = time.Now().Add(-2 * time.Minute)
	
	// Simulate low hit rate
	manager.stats.Hits = 2
	manager.stats.Misses = 8

	originalStrategy = manager.adaptiveState.CurrentStrategy
	manager.performAdaptiveTuning()

	// Should change strategy due to low hit rate
	assert.NotEqual(t, originalStrategy, manager.adaptiveState.CurrentStrategy)
	assert.Greater(t, len(manager.adaptiveState.StrategyHistory), 0)

	// Verify strategy history recorded
	lastChange := manager.adaptiveState.StrategyHistory[len(manager.adaptiveState.StrategyHistory)-1]
	assert.Equal(t, originalStrategy, lastChange.FromStrategy)
	assert.Equal(t, manager.adaptiveState.CurrentStrategy, lastChange.ToStrategy)
	assert.Contains(t, lastChange.Reason, "low hit rate")

	// Test tuning with high hit rate
	manager.adaptiveState.CurrentStrategy = StrategyExact
	manager.adaptiveState.SampleCount = 10
	manager.adaptiveState.LastEvaluation = time.Now().Add(-2 * time.Minute)
	
	// Simulate high hit rate
	manager.stats.Hits = 9
	manager.stats.Misses = 1

	originalStrategy = manager.adaptiveState.CurrentStrategy
	manager.performAdaptiveTuning()

	// Should change to hybrid for high hit rate
	if manager.adaptiveState.CurrentStrategy != originalStrategy {
		assert.Equal(t, StrategyHybrid, manager.adaptiveState.CurrentStrategy)
	}

	// Test recent evaluation (should not tune)
	manager.adaptiveState.SampleCount = 10
	manager.adaptiveState.LastEvaluation = time.Now() // Recent evaluation

	originalStrategy = manager.adaptiveState.CurrentStrategy
	manager.performAdaptiveTuning()

	// Should not change strategy due to recent evaluation
	assert.Equal(t, originalStrategy, manager.adaptiveState.CurrentStrategy)
}

func TestCacheManager_AdaptiveTuning_StrategyProgression(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyAdaptive,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		AdaptiveConfig: &AdaptiveConfig{
			LearningWindow:    time.Minute,
			MinSamples:       5,
			Sensitivity:      0.1,
			HighHitThreshold: 0.8,
			LowHitThreshold:  0.3,
			EnableAutoTuning: true,
			TuningInterval:   30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Test strategy progression for low hit rates
	strategies := []CacheStrategy{StrategyExact, StrategySemantic, StrategyToken, StrategyHybrid}

	for i, currentStrategy := range strategies {
		manager.adaptiveState.CurrentStrategy = currentStrategy
		manager.adaptiveState.SampleCount = 10
		manager.adaptiveState.LastEvaluation = time.Now().Add(-2 * time.Minute)
		
		// Simulate low hit rate
		manager.stats.Hits = 2
		manager.stats.Misses = 8

		manager.performAdaptiveTuning()

		// Should progress to next strategy or reset to exact
		if i < len(strategies)-1 {
			assert.Equal(t, strategies[i+1], manager.adaptiveState.CurrentStrategy)
		} else {
			// Last strategy should reset to exact
			assert.Equal(t, StrategyExact, manager.adaptiveState.CurrentStrategy)
		}
	}
}

func TestCacheManager_AdaptivePatternDetection(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyAdaptive,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		AdaptiveConfig: &AdaptiveConfig{
			EnablePatternDetection: true,
			TuningInterval:        30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Create test requests with different models and patterns
	requests := []*openai.ChatCompletionRequest{
		{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Short query"),
					},
				},
			},
		},
		{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("This is a much longer query with more detailed content"),
					},
				},
			},
		},
		{
			Model: "gpt-4",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Medium length query"),
					},
				},
			},
		},
	}

	responses := []*openai.ChatCompletionResponse{
		{Id: "response-1"},
		{Id: "response-2"},
		{Id: "response-3"},
	}

	// Perform lookups and stores to build patterns
	for i, req := range requests {
		// Lookup (will be miss initially)
		_, err := manager.Lookup(ctx, req, tenantID)
		assert.NoError(t, err)

		// Store response
		err = manager.Store(ctx, req, responses[i], tenantID)
		assert.NoError(t, err)
	}

	// Verify pattern detection
	patterns := manager.adaptiveState.PatternDetection

	// Check model patterns
	assert.Equal(t, int64(3), patterns.CommonModels["gpt-4o"])

	// Check user patterns
	assert.Equal(t, int64(3), patterns.UserPatterns[tenantID])

	// Check time patterns (should have entries for current hour)
	currentHour := time.Now().Hour()
	assert.Greater(t, patterns.TimePatterns[currentHour], int64(0))

	// Check query length tracking
	assert.Equal(t, 3, len(patterns.QueryLength))
	assert.Contains(t, patterns.QueryLength, 11) // "Short query" length
	assert.Contains(t, patterns.QueryLength, 54) // Long query length
	assert.Contains(t, patterns.QueryLength, 19) // "Medium length query" length
}

func TestCacheManager_AdaptivePatternDetection_MemoryLimit(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyAdaptive,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		AdaptiveConfig: &AdaptiveConfig{
			EnablePatternDetection: true,
			TuningInterval:        30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Create many requests to test memory limit
	for i := 0; i < 1100; i++ { // More than the 1000 limit
		req := &openai.ChatCompletionRequest{
			Model: "gpt-4o",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Test message"),
					},
				},
			},
		}

		response := &openai.ChatCompletionResponse{
			Id: "response",
		}

		// Lookup and store to trigger pattern detection
		_, err := manager.Lookup(ctx, req, tenantID)
		assert.NoError(t, err)

		err = manager.Store(ctx, req, response, tenantID)
		assert.NoError(t, err)
	}

	// Verify query length array is limited to prevent memory issues
	patterns := manager.adaptiveState.PatternDetection
	assert.LessOrEqual(t, len(patterns.QueryLength), 1000)
}

func TestCacheManager_AdaptiveState_ThreadSafety(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyAdaptive,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		AdaptiveConfig: &AdaptiveConfig{
			EnablePatternDetection: true,
			EnableAutoTuning:       true,
			TuningInterval:        30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Test concurrent access to adaptive state
	done := make(chan bool, 10)

	// Start multiple goroutines performing operations
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()

			req := &openai.ChatCompletionRequest{
				Model: "gpt-4o",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("Concurrent test"),
						},
					},
				},
			}

			response := &openai.ChatCompletionResponse{
				Id: "concurrent-response",
			}

			// Perform operations that modify adaptive state
			for j := 0; j < 100; j++ {
				manager.Lookup(ctx, req, tenantID)
				manager.Store(ctx, req, response, tenantID)
			}
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify adaptive state is still consistent
	assert.NotNil(t, manager.adaptiveState)
	assert.NotNil(t, manager.adaptiveState.PatternDetection)
	assert.GreaterOrEqual(t, manager.adaptiveState.SampleCount, 0)
}

func TestStrategyChange_Structure(t *testing.T) {
	now := time.Now()
	change := StrategyChange{
		Timestamp:    now,
		FromStrategy: StrategyExact,
		ToStrategy:   StrategySemantic,
		Reason:       "low hit rate",
		HitRate:      0.25,
		Metrics: map[string]interface{}{
			"total_requests": 1000,
			"cache_size":     500,
		},
	}

	assert.Equal(t, now, change.Timestamp)
	assert.Equal(t, StrategyExact, change.FromStrategy)
	assert.Equal(t, StrategySemantic, change.ToStrategy)
	assert.Equal(t, "low hit rate", change.Reason)
	assert.Equal(t, 0.25, change.HitRate)
	assert.Equal(t, 1000, change.Metrics["total_requests"])
	assert.Equal(t, 500, change.Metrics["cache_size"])
}

func TestPatternData_Structure(t *testing.T) {
	now := time.Now()
	patterns := PatternData{
		CommonModels: map[string]int64{
			"gpt-3.5-turbo": 1000,
			"gpt-4":         500,
			"claude-3":      200,
		},
		TimePatterns: map[int]int64{
			9:  100, // 9 AM
			10: 200, // 10 AM
			14: 150, // 2 PM
			15: 180, // 3 PM
		},
		UserPatterns: map[string]int64{
			"tenant-1": 500,
			"tenant-2": 300,
			"tenant-3": 200,
		},
		QueryLength:  []int{50, 100, 150, 200, 75, 125},
		ResponseSize: []int{500, 800, 1200, 600, 900},
		LastAnalysis: now,
	}

	// Test common models
	assert.Equal(t, int64(1000), patterns.CommonModels["gpt-3.5-turbo"])
	assert.Equal(t, int64(500), patterns.CommonModels["gpt-4"])
	assert.Equal(t, int64(200), patterns.CommonModels["claude-3"])

	// Test time patterns
	assert.Equal(t, int64(100), patterns.TimePatterns[9])
	assert.Equal(t, int64(200), patterns.TimePatterns[10])
	assert.Equal(t, int64(150), patterns.TimePatterns[14])

	// Test user patterns
	assert.Equal(t, int64(500), patterns.UserPatterns["tenant-1"])
	assert.Equal(t, int64(300), patterns.UserPatterns["tenant-2"])

	// Test query and response tracking
	assert.Equal(t, 6, len(patterns.QueryLength))
	assert.Equal(t, 5, len(patterns.ResponseSize))
	assert.Contains(t, patterns.QueryLength, 150)
	assert.Contains(t, patterns.ResponseSize, 1200)

	// Test last analysis timestamp
	assert.Equal(t, now, patterns.LastAnalysis)
}

func TestAdaptiveConfig_Validation(t *testing.T) {
	// Test default adaptive config values
	config := DefaultCacheConfig()
	adaptiveConfig := config.AdaptiveConfig

	assert.NotNil(t, adaptiveConfig)
	assert.Equal(t, 24*time.Hour, adaptiveConfig.LearningWindow)
	assert.Equal(t, 100, adaptiveConfig.MinSamples)
	assert.Equal(t, 0.1, adaptiveConfig.Sensitivity)
	assert.Equal(t, 0.8, adaptiveConfig.HighHitThreshold)
	assert.Equal(t, 0.3, adaptiveConfig.LowHitThreshold)
	assert.True(t, adaptiveConfig.EnablePatternDetection)
	assert.True(t, adaptiveConfig.EnableAutoTuning)
	assert.Equal(t, time.Hour, adaptiveConfig.TuningInterval)

	// Test custom adaptive config
	customConfig := &AdaptiveConfig{
		LearningWindow:         12 * time.Hour,
		MinSamples:            50,
		Sensitivity:           0.2,
		HighHitThreshold:      0.9,
		LowHitThreshold:       0.2,
		EnablePatternDetection: false,
		EnableAutoTuning:       false,
		TuningInterval:        2 * time.Hour,
	}

	assert.Equal(t, 12*time.Hour, customConfig.LearningWindow)
	assert.Equal(t, 50, customConfig.MinSamples)
	assert.Equal(t, 0.2, customConfig.Sensitivity)
	assert.Equal(t, 0.9, customConfig.HighHitThreshold)
	assert.Equal(t, 0.2, customConfig.LowHitThreshold)
	assert.False(t, customConfig.EnablePatternDetection)
	assert.False(t, customConfig.EnableAutoTuning)
	assert.Equal(t, 2*time.Hour, customConfig.TuningInterval)
}

func TestCacheManager_NoAdaptiveState(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact, // Not adaptive
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Verify no adaptive state for non-adaptive strategies
	assert.Nil(t, manager.adaptiveState)

	// Test operations that would use adaptive state
	result := &CacheLookupResult{Found: true, Strategy: StrategyExact}
	cacheReq := &CacheRequest{Model: "gpt-3.5-turbo"}

	// Should not panic
	manager.updateAdaptiveLearning(result, cacheReq, "tenant")
	manager.performAdaptiveTuning()

	// getActiveStrategy should return the configured strategy
	assert.Equal(t, StrategyExact, manager.getActiveStrategy())
}