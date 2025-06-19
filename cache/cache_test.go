package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem/openai"
)

func TestNewCacheManager(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name    string
		config  *CacheConfig
		wantErr bool
	}{
		{
			name:    "default config",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config",
			config: &CacheConfig{
				Enabled:          true,
				Strategy:         StrategySemantic,
				Backend:          BackendMemory,
				DefaultTTL:       30 * time.Minute,
				MaxEntries:       1000,
				EnableMetrics:    true,
				MetricsInterval:  5 * time.Minute,
			},
			wantErr: false,
		},
		{
			name: "adaptive strategy",
			config: &CacheConfig{
				Enabled:         true,
				Strategy:        StrategyAdaptive,
				Backend:         BackendMemory,
				EnableMetrics:   false, // Disable metrics to avoid ticker issues
				AdaptiveConfig: &AdaptiveConfig{
					LearningWindow:         time.Hour,
					MinSamples:            50,
					Sensitivity:           0.1,
					HighHitThreshold:      0.8,
					LowHitThreshold:       0.3,
					EnablePatternDetection: true,
					EnableAutoTuning:       true,
					TuningInterval:        30 * time.Minute,
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewCacheManager(tt.config, nil, logger)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, manager)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, manager)

				if tt.config == nil {
					assert.NotNil(t, manager.config)
					assert.True(t, manager.config.Enabled)
				} else {
					assert.Equal(t, tt.config.Enabled, manager.config.Enabled)
					assert.Equal(t, tt.config.Strategy, manager.config.Strategy)
				}

				if tt.config != nil && tt.config.Strategy == StrategyAdaptive {
					assert.NotNil(t, manager.adaptiveState)
					assert.Equal(t, StrategyExact, manager.adaptiveState.CurrentStrategy)
					assert.NotNil(t, manager.adaptiveState.PatternDetection)
				}

				manager.Stop()
			}
		})
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	config := DefaultCacheConfig()

	assert.NotNil(t, config)
	assert.True(t, config.Enabled)
	assert.Equal(t, StrategyExact, config.Strategy)
	assert.Equal(t, BackendMemory, config.Backend)
	assert.Equal(t, time.Hour, config.DefaultTTL)
	assert.Equal(t, 24*time.Hour, config.MaxTTL)
	assert.Equal(t, 30*time.Minute, config.SemanticTTL)
	assert.Equal(t, int64(10000), config.MaxEntries)
	assert.Equal(t, int64(500), config.MaxMemoryMB)
	assert.True(t, config.CompressionEnabled)
	assert.Equal(t, 6, config.CompressionLevel)
	assert.Equal(t, InvalidationLRU, config.InvalidationPolicy)
	assert.True(t, config.PerTenantLimits)
	assert.True(t, config.EnableMetrics)
	assert.Equal(t, 5*time.Minute, config.MetricsInterval)

	// Check semantic config
	assert.NotNil(t, config.SemanticConfig)
	assert.Equal(t, 0.95, config.SemanticConfig.SimilarityThreshold)
	assert.Equal(t, "openai", config.SemanticConfig.EmbeddingProvider)
	assert.Equal(t, "text-embedding-3-small", config.SemanticConfig.EmbeddingModel)
	assert.Equal(t, "cosine", config.SemanticConfig.SimilarityAlgorithm)
	assert.Equal(t, int64(1000), config.SemanticConfig.MaxEmbeddingsPerHour)
	assert.True(t, config.SemanticConfig.CacheEmbeddings)

	// Check token config
	assert.NotNil(t, config.TokenConfig)
	assert.Equal(t, 0.9, config.TokenConfig.TokenSimilarityThreshold)
	assert.Equal(t, 5, config.TokenConfig.MaxTokenDistance)
	assert.True(t, config.TokenConfig.EnableFuzzyMatching)
	assert.True(t, config.TokenConfig.NormalizeTokens)
	assert.True(t, config.TokenConfig.IgnoreCase)
	assert.False(t, config.TokenConfig.RemovePunctuation)

	// Check adaptive config
	assert.NotNil(t, config.AdaptiveConfig)
	assert.Equal(t, 24*time.Hour, config.AdaptiveConfig.LearningWindow)
	assert.Equal(t, 100, config.AdaptiveConfig.MinSamples)
	assert.Equal(t, 0.1, config.AdaptiveConfig.Sensitivity)
	assert.Equal(t, 0.8, config.AdaptiveConfig.HighHitThreshold)
	assert.Equal(t, 0.3, config.AdaptiveConfig.LowHitThreshold)
	assert.True(t, config.AdaptiveConfig.EnablePatternDetection)
	assert.True(t, config.AdaptiveConfig.EnableAutoTuning)
	assert.Equal(t, time.Hour, config.AdaptiveConfig.TuningInterval)
}

func TestCacheManager_StoreAndLookup(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		EnableMetrics: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Create test request and response
	request := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello, world!"),
				},
			},
		},
		Temperature: float32Ptr(0.7),
		MaxTokens:   int32Ptr(100),
	}

	response := &openai.ChatCompletionResponse{
		Id:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-3.5-turbo",
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role: "assistant",
					Content: &openai.MessageContent{
						String: stringPtr("Hello! How can I help you today?"),
					},
				},
				FinishReason: "stop",
			},
		},
	}

	// Test storing
	err = manager.Store(ctx, request, response, tenantID)
	assert.NoError(t, err)

	// Test lookup - should find exact match
	result, err := manager.Lookup(ctx, request, tenantID)
	assert.NoError(t, err)
	assert.True(t, result.Found)
	assert.Equal(t, StrategyExact, result.Strategy)
	assert.Equal(t, 1.0, result.Similarity)
	assert.NotNil(t, result.Entry)
	assert.Equal(t, response.Id, result.Entry.Response.Id)

	// Test lookup with different request - should not find
	differentRequest := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Different message"),
				},
			},
		},
	}

	result, err = manager.Lookup(ctx, differentRequest, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)

	// Test tenant isolation
	result, err = manager.Lookup(ctx, request, "different-tenant")
	assert.NoError(t, err)
	assert.False(t, result.Found)
}

func TestCacheManager_LookupDisabled(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test"),
				},
			},
		},
	}

	result, err := manager.Lookup(ctx, request, "tenant")
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategyNone, result.Strategy)
}

func TestCacheManager_TTLExpiration(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   10 * time.Millisecond, // Very short TTL for testing
		MaxEntries:   100,
		EnableMetrics: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello"),
				},
			},
		},
	}

	response := &openai.ChatCompletionResponse{
		Id: "test-response",
	}

	// Store entry
	err = manager.Store(ctx, request, response, tenantID)
	assert.NoError(t, err)

	// Immediately lookup - should find
	result, err := manager.Lookup(ctx, request, tenantID)
	assert.NoError(t, err)
	assert.True(t, result.Found)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Lookup after expiration - should not find
	result, err = manager.Lookup(ctx, request, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
}

func TestCacheManager_TenantSpecificTTL(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		TenantTTLOverrides: map[string]time.Duration{
			"premium-tenant": 2 * time.Hour,
			"trial-tenant":   30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Test TTL calculation for different tenants
	cacheReq := &CacheRequest{Model: "gpt-3.5-turbo"}

	defaultTTL := manager.calculateTTL(cacheReq, "regular-tenant")
	assert.Equal(t, time.Hour, defaultTTL)

	premiumTTL := manager.calculateTTL(cacheReq, "premium-tenant")
	assert.Equal(t, 2*time.Hour, premiumTTL)

	trialTTL := manager.calculateTTL(cacheReq, "trial-tenant")
	assert.Equal(t, 30*time.Minute, trialTTL)
}

func TestCacheManager_MaxEntriesEviction(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   3, // Small limit for testing
		EnableMetrics: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Store entries up to the limit
	for i := 0; i < 4; i++ {
		request := &openai.ChatCompletionRequest{
			Model: "gpt-4o",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr(fmt.Sprintf("Message %d", i)),
					},
				},
			},
		}

		response := &openai.ChatCompletionResponse{
			Id: fmt.Sprintf("response-%d", i),
		}

		err = manager.Store(ctx, request, response, tenantID)
		assert.NoError(t, err)
	}

	// Check that cache size is limited
	assert.LessOrEqual(t, len(manager.memoryCache), int(config.MaxEntries))

	// First entry should have been evicted
	firstRequest := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Message 0"),
				},
			},
		},
	}

	result, err := manager.Lookup(ctx, firstRequest, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)

	// Last entry should still be there
	lastRequest := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Message 3"),
				},
			},
		},
	}

	result, err = manager.Lookup(ctx, lastRequest, tenantID)
	assert.NoError(t, err)
	assert.True(t, result.Found)
}

func TestCacheManager_Clear(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Store some entries
	for i := 0; i < 3; i++ {
		request := &openai.ChatCompletionRequest{
			Model: "gpt-4o",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr(fmt.Sprintf("Message %d", i)),
					},
				},
			},
		}

		response := &openai.ChatCompletionResponse{
			Id: fmt.Sprintf("response-%d", i),
		}

		err := manager.Store(ctx, request, response, tenantID)
		assert.NoError(t, err)
	}

	// Verify entries exist
	assert.Equal(t, 3, len(manager.memoryCache))

	// Clear cache
	err := manager.Clear()
	assert.NoError(t, err)

	// Verify cache is empty
	assert.Equal(t, 0, len(manager.memoryCache))
	assert.Equal(t, 0, len(manager.accessOrder))

	// Verify stats are reset
	stats := manager.GetStats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, int64(0), stats.TotalEntries)
}

func TestCacheManager_ClearTenant(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	ctx := context.Background()

	// Store entries for different tenants
	tenants := []string{"tenant-1", "tenant-2", "tenant-3"}
	for _, tenantID := range tenants {
		request := &openai.ChatCompletionRequest{
			Model: "gpt-4o",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr(fmt.Sprintf("Message for %s", tenantID)),
					},
				},
			},
		}

		response := &openai.ChatCompletionResponse{
			Id: fmt.Sprintf("response-%s", tenantID),
		}

		err := manager.Store(ctx, request, response, tenantID)
		assert.NoError(t, err)
	}

	// Verify all entries exist
	assert.Equal(t, 3, len(manager.memoryCache))

	// Clear one tenant
	err := manager.ClearTenant("tenant-1")
	assert.NoError(t, err)

	// Verify only entries for other tenants remain
	assert.Equal(t, 2, len(manager.memoryCache))

	// Verify specific tenant entries are gone
	for key, entry := range manager.memoryCache {
		assert.NotEqual(t, "tenant-1", entry.TenantID)
		t.Logf("Remaining entry: %s for tenant %s", key, entry.TenantID)
	}
}

func TestCacheManager_GetStats(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:         true,
		Strategy:        StrategyExact,
		Backend:         BackendMemory,
		DefaultTTL:      time.Hour,
		MaxEntries:      100,
		PerTenantLimits: true,
		EnableMetrics:   false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()

	// Perform some cache operations
	request := &openai.ChatCompletionRequest{
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
		Id: "test-response",
	}

	// Miss (lookup before store)
	result, err := manager.Lookup(ctx, request, "tenant-1")
	assert.NoError(t, err)
	assert.False(t, result.Found)

	// Store
	err = manager.Store(ctx, request, response, "tenant-1")
	assert.NoError(t, err)

	// Hit (lookup after store)
	result, err = manager.Lookup(ctx, request, "tenant-1")
	assert.NoError(t, err)
	assert.True(t, result.Found)

	// Get stats
	stats := manager.GetStats()
	assert.NotNil(t, stats)
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(1), stats.Stores)
	assert.Equal(t, int64(1), stats.TotalEntries)
	assert.Equal(t, 0.5, stats.HitRate) // 1 hit / 2 total lookups

	// Check tenant-specific stats
	assert.Contains(t, stats.TenantStats, "tenant-1")
	tenantStats := stats.TenantStats["tenant-1"]
	assert.Equal(t, int64(1), tenantStats.Hits)
	assert.Equal(t, int64(1), tenantStats.Misses)
	assert.Equal(t, 0.5, tenantStats.HitRate)
	assert.Equal(t, int64(1), tenantStats.Entries)
}

func TestCacheManager_ConvertToCacheRequest(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test message"),
				},
			},
		},
		Temperature:      float32Ptr(0.8),
		MaxTokens:        int32Ptr(150),
		TopP:             float32Ptr(0.9),
		FrequencyPenalty: float32Ptr(0.1),
		PresencePenalty:  float32Ptr(0.2),
	}

	cacheReq := manager.convertToCacheRequest(request)

	assert.Equal(t, "gpt-4", cacheReq.Model)
	assert.Len(t, cacheReq.Messages, 1)
	assert.Equal(t, "user", cacheReq.Messages[0].Role)

	// Check settings extraction
	assert.Equal(t, float32(0.8), cacheReq.Settings["temperature"])
	assert.Equal(t, int32(150), cacheReq.Settings["max_tokens"])
	assert.Equal(t, float32(0.9), cacheReq.Settings["top_p"])
	assert.Equal(t, float32(0.1), cacheReq.Settings["frequency_penalty"])
	assert.Equal(t, float32(0.2), cacheReq.Settings["presence_penalty"])
}

func TestCacheManager_GenerateCacheKey(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test"),
				},
			},
		},
		Settings: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	// Same request should produce same key
	key1 := manager.generateCacheKey(cacheReq, "tenant-1")
	key2 := manager.generateCacheKey(cacheReq, "tenant-1")
	assert.Equal(t, key1, key2)

	// Different tenant should produce different key
	key3 := manager.generateCacheKey(cacheReq, "tenant-2")
	assert.NotEqual(t, key1, key3)

	// Different request should produce different key
	cacheReq2 := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Different"),
				},
			},
		},
	}
	key4 := manager.generateCacheKey(cacheReq2, "tenant-1")
	assert.NotEqual(t, key1, key4)

	// All keys should be valid hex strings
	assert.Regexp(t, "^[a-f0-9]+$", key1)
	assert.Regexp(t, "^[a-f0-9]+$", key3)
	assert.Regexp(t, "^[a-f0-9]+$", key4)
}

func TestCacheManager_GenerateHash(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test message"),
				},
			},
		},
		Settings: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	hash1 := manager.generateHash(cacheReq)
	hash2 := manager.generateHash(cacheReq)

	// Same request should produce same hash
	assert.Equal(t, hash1, hash2)

	// Hash should be shortened to 16 characters
	assert.Len(t, hash1, 16)

	// Hash should be valid hex
	assert.Regexp(t, "^[a-f0-9]{16}$", hash1)

	// Different request should produce different hash
	cacheReq.Model = "gpt-4"
	hash3 := manager.generateHash(cacheReq)
	assert.NotEqual(t, hash1, hash3)
}

func TestCacheManager_GetActiveStrategy(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name     string
		strategy CacheStrategy
		expected CacheStrategy
	}{
		{
			name:     "exact strategy",
			strategy: StrategyExact,
			expected: StrategyExact,
		},
		{
			name:     "semantic strategy",
			strategy: StrategySemantic,
			expected: StrategySemantic,
		},
		{
			name:     "token strategy",
			strategy: StrategyToken,
			expected: StrategyToken,
		},
		{
			name:     "hybrid strategy",
			strategy: StrategyHybrid,
			expected: StrategyHybrid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &CacheConfig{
				Enabled:  true,
				Strategy: tt.strategy,
				Backend:  BackendMemory,
			}

			manager, err := NewCacheManager(config, nil, logger)
			require.NoError(t, err)
			defer manager.Stop()

			active := manager.getActiveStrategy()
			assert.Equal(t, tt.expected, active)
		})
	}
}

func TestCacheManager_GetActiveStrategyAdaptive(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:  true,
		Strategy: StrategyAdaptive,
		Backend:  BackendMemory,
		AdaptiveConfig: &AdaptiveConfig{
			LearningWindow:   time.Hour,
			MinSamples:      50,
			Sensitivity:     0.1,
			HighHitThreshold: 0.8,
			LowHitThreshold: 0.3,
			TuningInterval:  30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Adaptive strategy should start with exact matching
	active := manager.getActiveStrategy()
	assert.Equal(t, StrategyExact, active)

	// Change adaptive state
	manager.adaptiveState.mutex.Lock()
	manager.adaptiveState.CurrentStrategy = StrategySemantic
	manager.adaptiveState.mutex.Unlock()

	// Should now return semantic strategy
	active = manager.getActiveStrategy()
	assert.Equal(t, StrategySemantic, active)
}

func TestCacheEntry_Structure(t *testing.T) {
	now := time.Now()
	entry := &CacheEntry{
		Key:        "test-key",
		Hash:       "test-hash",
		Request:    &CacheRequest{Model: "gpt-3.5-turbo"},
		Response:   &openai.ChatCompletionResponse{Id: "test-response"},
		CreatedAt:  now,
		ExpiresAt:  now.Add(time.Hour),
		AccessCount: 5,
		LastAccess: now,
		TenantID:   "test-tenant",
		Metadata: map[string]interface{}{
			"source": "test",
		},
		Embedding:    []float32{0.1, 0.2, 0.3},
		Similarity:   0.95,
		Compressed:   true,
		OriginalSize: 1024,
	}

	assert.Equal(t, "test-key", entry.Key)
	assert.Equal(t, "test-hash", entry.Hash)
	assert.Equal(t, "gpt-3.5-turbo", entry.Request.Model)
	assert.Equal(t, "test-response", entry.Response.Id)
	assert.Equal(t, now, entry.CreatedAt)
	assert.Equal(t, now.Add(time.Hour), entry.ExpiresAt)
	assert.Equal(t, int64(5), entry.AccessCount)
	assert.Equal(t, now, entry.LastAccess)
	assert.Equal(t, "test-tenant", entry.TenantID)
	assert.Equal(t, "test", entry.Metadata["source"])
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, entry.Embedding)
	assert.Equal(t, 0.95, entry.Similarity)
	assert.True(t, entry.Compressed)
	assert.Equal(t, int64(1024), entry.OriginalSize)
}

func TestCacheRequest_Structure(t *testing.T) {
	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test message"),
				},
			},
		},
		Settings: map[string]interface{}{
			"temperature": 0.8,
			"max_tokens":  150,
		},
	}

	assert.Equal(t, "gpt-4", cacheReq.Model)
	assert.Len(t, cacheReq.Messages, 1)
	assert.Equal(t, "user", cacheReq.Messages[0].Role)
	assert.Equal(t, 0.8, cacheReq.Settings["temperature"])
	assert.Equal(t, 150, cacheReq.Settings["max_tokens"])
}

func TestCacheLookupResult_Structure(t *testing.T) {
	result := &CacheLookupResult{
		Found:      true,
		Entry:      &CacheEntry{Key: "test-key"},
		Strategy:   StrategySemantic,
		Similarity: 0.92,
		Latency:    50 * time.Millisecond,
		Source:     "memory",
	}

	assert.True(t, result.Found)
	assert.NotNil(t, result.Entry)
	assert.Equal(t, "test-key", result.Entry.Key)
	assert.Equal(t, StrategySemantic, result.Strategy)
	assert.Equal(t, 0.92, result.Similarity)
	assert.Equal(t, 50*time.Millisecond, result.Latency)
	assert.Equal(t, "memory", result.Source)
}

func TestCacheStats_Structure(t *testing.T) {
	stats := &CacheStats{
		Hits:              100,
		Misses:            25,
		Stores:            110,
		Evictions:         5,
		TotalEntries:      105,
		MemoryUsageMB:     50.5,
		HitRate:           0.8,
		AverageLatency:    10 * time.Millisecond,
		ExactHits:         60,
		SemanticHits:      25,
		TokenHits:         15,
		TenantStats: map[string]*TenantCacheStats{
			"tenant-1": {
				Hits:     50,
				Misses:   10,
				HitRate:  0.83,
				Entries:  55,
				MemoryMB: 25.2,
			},
		},
		LastUpdated: time.Now(),
	}

	assert.Equal(t, int64(100), stats.Hits)
	assert.Equal(t, int64(25), stats.Misses)
	assert.Equal(t, int64(110), stats.Stores)
	assert.Equal(t, int64(5), stats.Evictions)
	assert.Equal(t, int64(105), stats.TotalEntries)
	assert.Equal(t, 50.5, stats.MemoryUsageMB)
	assert.Equal(t, 0.8, stats.HitRate)
	assert.Equal(t, 10*time.Millisecond, stats.AverageLatency)
	assert.Equal(t, int64(60), stats.ExactHits)
	assert.Equal(t, int64(25), stats.SemanticHits)
	assert.Equal(t, int64(15), stats.TokenHits)
	assert.Contains(t, stats.TenantStats, "tenant-1")
	assert.Equal(t, int64(50), stats.TenantStats["tenant-1"].Hits)
}

func TestAdaptiveState_Structure(t *testing.T) {
	state := &AdaptiveState{
		CurrentStrategy: StrategySemantic,
		StrategyHistory: []StrategyChange{
			{
				Timestamp:    time.Now(),
				FromStrategy: StrategyExact,
				ToStrategy:   StrategySemantic,
				Reason:       "low hit rate",
				HitRate:      0.25,
				Metrics: map[string]interface{}{
					"total_requests": 1000,
				},
			},
		},
		LearningData: map[string]interface{}{
			"avg_query_length": 150,
		},
		LastEvaluation: time.Now(),
		SampleCount:    150,
		PatternDetection: &PatternData{
			CommonModels: map[string]int64{
				"gpt-3.5-turbo": 800,
				"gpt-4":         200,
			},
			TimePatterns: map[int]int64{
				9:  150,
				10: 200,
				14: 180,
			},
			UserPatterns: map[string]int64{
				"tenant-1": 500,
				"tenant-2": 300,
			},
			QueryLength:  []int{100, 150, 200, 80, 120},
			ResponseSize: []int{500, 800, 600, 400, 700},
			LastAnalysis: time.Now(),
		},
	}

	assert.Equal(t, StrategySemantic, state.CurrentStrategy)
	assert.Len(t, state.StrategyHistory, 1)
	assert.Equal(t, StrategyExact, state.StrategyHistory[0].FromStrategy)
	assert.Equal(t, StrategySemantic, state.StrategyHistory[0].ToStrategy)
	assert.Equal(t, "low hit rate", state.StrategyHistory[0].Reason)
	assert.Equal(t, 0.25, state.StrategyHistory[0].HitRate)
	assert.Equal(t, 1000, state.StrategyHistory[0].Metrics["total_requests"])
	assert.Equal(t, 150, state.LearningData["avg_query_length"])
	assert.Equal(t, 150, state.SampleCount)
	assert.NotNil(t, state.PatternDetection)
	assert.Equal(t, int64(800), state.PatternDetection.CommonModels["gpt-3.5-turbo"])
	assert.Equal(t, int64(200), state.PatternDetection.TimePatterns[10])
	assert.Equal(t, int64(500), state.PatternDetection.UserPatterns["tenant-1"])
	assert.Len(t, state.PatternDetection.QueryLength, 5)
}

func TestCacheStrategy_Constants(t *testing.T) {
	assert.Equal(t, CacheStrategy("none"), StrategyNone)
	assert.Equal(t, CacheStrategy("exact"), StrategyExact)
	assert.Equal(t, CacheStrategy("semantic"), StrategySemantic)
	assert.Equal(t, CacheStrategy("token"), StrategyToken)
	assert.Equal(t, CacheStrategy("hybrid"), StrategyHybrid)
	assert.Equal(t, CacheStrategy("adaptive"), StrategyAdaptive)
}

func TestCacheBackend_Constants(t *testing.T) {
	assert.Equal(t, CacheBackend("memory"), BackendMemory)
	assert.Equal(t, CacheBackend("redis"), BackendRedis)
	assert.Equal(t, CacheBackend("multi_tier"), BackendMultiTier)
}

func TestInvalidationPolicy_Constants(t *testing.T) {
	assert.Equal(t, InvalidationPolicy("lru"), InvalidationLRU)
	assert.Equal(t, InvalidationPolicy("lfu"), InvalidationLFU)
	assert.Equal(t, InvalidationPolicy("ttl"), InvalidationTTL)
	assert.Equal(t, InvalidationPolicy("random"), InvalidationRandom)
}

// Helper functions

func createTestCacheManager(t *testing.T, logger *zap.SugaredLogger) *CacheManager {
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		EnableMetrics: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	return manager
}

func stringPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}

func float32Ptr(f float32) *float32 {
	return &f
}

func intPtr(i int) *int {
	return &i
}

func int32Ptr(i int32) *int32 {
	return &i
}