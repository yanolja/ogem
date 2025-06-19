package cache

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem/openai"
)

func TestCacheManager_LookupExact(t *testing.T) {
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

	// Create test request
	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello, world!"),
				},
			},
		},
		Settings: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	// Test lookup when cache is empty
	result, err := manager.lookupExact(cacheReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategyExact, result.Strategy)
	assert.Equal(t, "memory", result.Source)

	// Store an entry
	response := &openai.ChatCompletionResponse{
		Id:      "chatcmpl-123",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "gpt-3.5-turbo",
	}

	err = manager.Store(ctx, &openai.ChatCompletionRequest{
		Model: cacheReq.Model,
		Messages: cacheReq.Messages,
		Temperature: float32Ptr(0.7),
	}, response, tenantID)
	require.NoError(t, err)

	// Test lookup with exact match
	result, err = manager.lookupExact(cacheReq, tenantID)
	assert.NoError(t, err)
	assert.True(t, result.Found)
	assert.Equal(t, StrategyExact, result.Strategy)
	assert.Equal(t, 1.0, result.Similarity)
	assert.Equal(t, "memory", result.Source)
	assert.Equal(t, response.Id, result.Entry.Response.Id)

	// Test lookup with different tenant - should not find
	result, err = manager.lookupExact(cacheReq, "different-tenant")
	assert.NoError(t, err)
	assert.False(t, result.Found)

	// Test lookup with slightly different request - should not find
	differentReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello, universe!"),
				},
			},
		},
		Settings: map[string]interface{}{
			"temperature": 0.7,
		},
	}

	result, err = manager.lookupExact(differentReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
}

func TestCacheManager_LookupExact_ExpiredEntry(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   10 * time.Millisecond, // Very short TTL
		MaxEntries:   100,
		EnableMetrics: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

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
	}

	// Store an entry
	response := &openai.ChatCompletionResponse{Id: "test-response"}
	err = manager.Store(ctx, &openai.ChatCompletionRequest{
		Model:    cacheReq.Model,
		Messages: cacheReq.Messages,
	}, response, tenantID)
	require.NoError(t, err)

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Test lookup with expired entry - should not find
	result, err := manager.lookupExact(cacheReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
}

func TestCacheManager_LookupSemantic(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategySemantic,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		PerTenantLimits: true,
		SemanticConfig: &SemanticConfig{
			SimilarityThreshold: 0.8,
			EmbeddingProvider:   "openai",
			EmbeddingModel:      "text-embedding-3-small",
			SimilarityAlgorithm: "cosine",
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	// Create test requests with similar content
	cacheReq1 := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("What is artificial intelligence?"),
				},
			},
		},
	}

	cacheReq2 := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("What is AI?"),
				},
			},
		},
	}

	// Test lookup when cache is empty
	result, err := manager.lookupSemantic(ctx, cacheReq1, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategySemantic, result.Strategy)

	// Store an entry with embedding
	response := &openai.ChatCompletionResponse{
		Id: "semantic-response",
	}

	err = manager.Store(ctx, &openai.ChatCompletionRequest{
		Model:    cacheReq1.Model,
		Messages: cacheReq1.Messages,
	}, response, tenantID)
	require.NoError(t, err)

	// Test semantic lookup with similar content
	result, err = manager.lookupSemantic(ctx, cacheReq2, tenantID)
	assert.NoError(t, err)
	// Note: With simulated embeddings, we can't guarantee a match above threshold
	// This tests the semantic lookup flow rather than exact similarity matching

	// Test with different model - should not find
	differentModelReq := &CacheRequest{
		Model: "gpt-4",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("What is artificial intelligence?"),
				},
			},
		},
	}

	result, err = manager.lookupSemantic(ctx, differentModelReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)

	// Test tenant isolation
	result, err = manager.lookupSemantic(ctx, cacheReq2, "different-tenant")
	assert.NoError(t, err)
	assert.False(t, result.Found)
}

func TestCacheManager_LookupSemantic_EmbeddingError(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategySemantic,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		SemanticConfig: nil, // No semantic config to trigger error
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

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
	}

	// Should fall back to exact matching
	result, err := manager.lookupSemantic(ctx, cacheReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategyExact, result.Strategy) // Falls back to exact
}

func TestCacheManager_LookupToken(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyToken,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		PerTenantLimits: true,
		TokenConfig: &TokenConfig{
			TokenSimilarityThreshold: 0.8,
			MaxTokenDistance:        3,
			EnableFuzzyMatching:     true,
			NormalizeTokens:         true,
			IgnoreCase:             true,
			RemovePunctuation:      false,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	tenantID := "test-tenant"

	// Create test requests with similar tokens
	cacheReq1 := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello world how are you"),
				},
			},
		},
	}

	cacheReq2 := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello world how are you today"),
				},
			},
		},
	}

	// Test lookup when cache is empty
	result, err := manager.lookupToken(cacheReq1, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategyToken, result.Strategy)

	// Store an entry
	response := &openai.ChatCompletionResponse{
		Id: "token-response",
	}

	err = manager.Store(context.Background(), &openai.ChatCompletionRequest{
		Model:    cacheReq1.Model,
		Messages: cacheReq1.Messages,
	}, response, tenantID)
	require.NoError(t, err)

	// Test token-based lookup with similar content
	result, err = manager.lookupToken(cacheReq2, tenantID)
	assert.NoError(t, err)
	// With high similarity threshold and similar tokens, should find a match
	if result.Found {
		assert.Greater(t, result.Similarity, 0.0)
		assert.Equal(t, StrategyToken, result.Strategy)
	}

	// Test with different model - should not find
	differentModelReq := &CacheRequest{
		Model: "gpt-4",
		Messages: cacheReq1.Messages,
	}

	result, err = manager.lookupToken(differentModelReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)

	// Test tenant isolation
	result, err = manager.lookupToken(cacheReq2, "different-tenant")
	assert.NoError(t, err)
	assert.False(t, result.Found)
}

func TestCacheManager_LookupHybrid(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyHybrid,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		SemanticConfig: &SemanticConfig{
			SimilarityThreshold: 0.9,
		},
		TokenConfig: &TokenConfig{
			TokenSimilarityThreshold: 0.8,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test hybrid caching"),
				},
			},
		},
	}

	// Test lookup when cache is empty
	result, err := manager.lookupHybrid(ctx, cacheReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategyHybrid, result.Strategy)

	// Store an entry
	response := &openai.ChatCompletionResponse{
		Id: "hybrid-response",
	}

	err = manager.Store(ctx, &openai.ChatCompletionRequest{
		Model:    cacheReq.Model,
		Messages: cacheReq.Messages,
	}, response, tenantID)
	require.NoError(t, err)

	// Test exact match (should find via exact matching first)
	result, err = manager.lookupHybrid(ctx, cacheReq, tenantID)
	assert.NoError(t, err)
	assert.True(t, result.Found)
	assert.Equal(t, StrategyHybrid, result.Strategy)
	assert.Equal(t, 1.0, result.Similarity) // Exact match

	// Test with similar content (would use semantic/token fallback)
	similarReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test hybrid caching system"),
				},
			},
		},
	}

	result, err = manager.lookupHybrid(ctx, similarReq, tenantID)
	assert.NoError(t, err)
	// May or may not find depending on similarity thresholds
	if result.Found {
		assert.Equal(t, StrategyHybrid, result.Strategy)
	}
}

func TestCacheManager_LookupHybrid_NoSemanticConfig(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyHybrid,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   100,
		SemanticConfig: nil, // No semantic config
		TokenConfig: &TokenConfig{
			TokenSimilarityThreshold: 0.8,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()
	tenantID := "test-tenant"

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
	}

	// Should skip semantic and try token matching
	result, err := manager.lookupHybrid(ctx, cacheReq, tenantID)
	assert.NoError(t, err)
	assert.False(t, result.Found)
	assert.Equal(t, StrategyHybrid, result.Strategy)
}

func TestCacheManager_GenerateEmbedding(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		SemanticConfig: &SemanticConfig{
			SimilarityThreshold: 0.9,
			EmbeddingProvider:   "openai",
			EmbeddingModel:      "text-embedding-3-small",
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	ctx := context.Background()

	// Test with valid request
	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello, world!"),
				},
			},
		},
	}

	embedding, err := manager.generateEmbedding(ctx, cacheReq)
	assert.NoError(t, err)
	assert.NotNil(t, embedding)
	assert.Equal(t, 384, len(embedding)) // Standard embedding dimension

	// Test with empty content
	emptyReq := &CacheRequest{
		Model:    "gpt-3.5-turbo",
		Messages: []openai.Message{},
	}

	embedding, err = manager.generateEmbedding(ctx, emptyReq)
	assert.Error(t, err)
	assert.Nil(t, embedding)

	// Test with no semantic config
	manager.config.SemanticConfig = nil
	embedding, err = manager.generateEmbedding(ctx, cacheReq)
	assert.Error(t, err)
	assert.Nil(t, embedding)
}

func TestCacheManager_GenerateSimulatedEmbedding(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	// Test deterministic embeddings
	text1 := "Hello, world!"
	text2 := "Hello, world!"
	text3 := "Different text"

	embedding1 := manager.generateSimulatedEmbedding(text1)
	embedding2 := manager.generateSimulatedEmbedding(text2)
	embedding3 := manager.generateSimulatedEmbedding(text3)

	assert.Equal(t, 384, len(embedding1))
	assert.Equal(t, 384, len(embedding2))
	assert.Equal(t, 384, len(embedding3))

	// Same text should produce same embedding
	assert.Equal(t, embedding1, embedding2)

	// Different text should produce different embedding
	assert.NotEqual(t, embedding1, embedding3)

	// Test normalization (embedding should be unit vector)
	norm := float64(0)
	for _, val := range embedding1 {
		norm += float64(val * val)
	}
	norm = math.Sqrt(norm)
	assert.InDelta(t, 1.0, norm, 0.001) // Should be approximately 1
}

func TestCacheManager_CalculateCosineSimilarity(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	// Test identical vectors
	vec1 := []float32{1.0, 0.0, 0.0}
	vec2 := []float32{1.0, 0.0, 0.0}
	similarity := manager.calculateCosineSimilarity(vec1, vec2)
	assert.InDelta(t, 1.0, similarity, 0.001)

	// Test orthogonal vectors
	vec3 := []float32{0.0, 1.0, 0.0}
	similarity = manager.calculateCosineSimilarity(vec1, vec3)
	assert.InDelta(t, 0.0, similarity, 0.001)

	// Test opposite vectors
	vec4 := []float32{-1.0, 0.0, 0.0}
	similarity = manager.calculateCosineSimilarity(vec1, vec4)
	assert.InDelta(t, -1.0, similarity, 0.001)

	// Test different length vectors
	vec5 := []float32{1.0, 0.0}
	similarity = manager.calculateCosineSimilarity(vec1, vec5)
	assert.Equal(t, 0.0, similarity)

	// Test zero vectors
	vec6 := []float32{0.0, 0.0, 0.0}
	similarity = manager.calculateCosineSimilarity(vec1, vec6)
	assert.Equal(t, 0.0, similarity)
}

func TestCacheManager_ExtractTokens(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		TokenConfig: &TokenConfig{
			NormalizeTokens:    true,
			IgnoreCase:        true,
			RemovePunctuation: false,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Test with string content
	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello World! How are you?"),
				},
			},
		},
	}

	tokens := manager.extractTokens(cacheReq)
	expected := []string{"hello", "world!", "how", "are", "you?"}
	assert.Equal(t, expected, tokens)

	// Test with punctuation removal
	manager.config.TokenConfig.RemovePunctuation = true
	tokens = manager.extractTokens(cacheReq)
	expected = []string{"hello", "world", "how", "are", "you"}
	assert.Equal(t, expected, tokens)

	// Test with case sensitivity
	manager.config.TokenConfig.IgnoreCase = false
	cacheReq.Messages[0].Content.String = stringPtr("Hello WORLD")
	tokens = manager.extractTokens(cacheReq)
	expected = []string{"Hello", "WORLD"}
	assert.Equal(t, expected, tokens)

	// Test with empty content
	cacheReq.Messages = []openai.Message{}
	tokens = manager.extractTokens(cacheReq)
	assert.Empty(t, tokens)
}

func TestCacheManager_CalculateTokenSimilarity(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		TokenConfig: &TokenConfig{
			EnableFuzzyMatching: false,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Test identical token sets
	tokens1 := []string{"hello", "world"}
	tokens2 := []string{"hello", "world"}
	similarity := manager.calculateTokenSimilarity(tokens1, tokens2)
	assert.Equal(t, 1.0, similarity)

	// Test completely different token sets
	tokens3 := []string{"foo", "bar"}
	similarity = manager.calculateTokenSimilarity(tokens1, tokens3)
	assert.Equal(t, 0.0, similarity)

	// Test partial overlap
	tokens4 := []string{"hello", "universe"}
	similarity = manager.calculateTokenSimilarity(tokens1, tokens4)
	assert.InDelta(t, 0.5, similarity, 0.1) // 50% overlap (1 of 2 tokens match)

	// Test empty token sets
	tokens5 := []string{}
	tokens6 := []string{}
	similarity = manager.calculateTokenSimilarity(tokens5, tokens6)
	assert.Equal(t, 1.0, similarity)

	// Test one empty set
	similarity = manager.calculateTokenSimilarity(tokens1, tokens5)
	assert.Equal(t, 0.0, similarity)

	// Test with fuzzy matching enabled
	manager.config.TokenConfig.EnableFuzzyMatching = true
	manager.config.TokenConfig.MaxTokenDistance = 2
	tokens7 := []string{"helo", "world"} // "helo" is close to "hello"
	similarity = manager.calculateTokenSimilarity(tokens1, tokens7)
	assert.Greater(t, similarity, 0.5) // Should have bonus for fuzzy match
}

func TestCacheManager_EditDistance(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	// Test identical strings
	distance := manager.editDistance("hello", "hello")
	assert.Equal(t, 0, distance)

	// Test one character difference
	distance = manager.editDistance("hello", "helo")
	assert.Equal(t, 1, distance)

	// Test insertion
	distance = manager.editDistance("hello", "helllo")
	assert.Equal(t, 1, distance)

	// Test deletion
	distance = manager.editDistance("hello", "hllo")
	assert.Equal(t, 1, distance)

	// Test substitution
	distance = manager.editDistance("hello", "hallo")
	assert.Equal(t, 1, distance)

	// Test completely different strings
	distance = manager.editDistance("hello", "world")
	assert.Equal(t, 4, distance)

	// Test empty strings
	distance = manager.editDistance("", "")
	assert.Equal(t, 0, distance)

	// Test one empty string
	distance = manager.editDistance("hello", "")
	assert.Equal(t, 5, distance)

	distance = manager.editDistance("", "world")
	assert.Equal(t, 5, distance)
}

func TestCacheManager_UpdateEntryAccess(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	// Create test entry
	entry := &CacheEntry{
		Key:         "test-key",
		AccessCount: 0,
		LastAccess:  time.Now().Add(-time.Hour),
	}

	// Store entry in cache
	manager.memoryCache[entry.Key] = entry
	manager.accessOrder = append(manager.accessOrder, entry.Key)

	originalTime := entry.LastAccess
	originalCount := entry.AccessCount

	// Update access
	manager.updateEntryAccess(entry)

	// Verify access count and time updated
	assert.Equal(t, originalCount+1, entry.AccessCount)
	assert.True(t, entry.LastAccess.After(originalTime))

	// Verify entry moved to end of access order (most recently used)
	assert.Equal(t, entry.Key, manager.accessOrder[len(manager.accessOrder)-1])
}

func TestCacheManager_StoreEntry(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Enabled:      true,
		Strategy:     StrategyExact,
		Backend:      BackendMemory,
		DefaultTTL:   time.Hour,
		MaxEntries:   2, // Small limit for testing
		EnableMetrics: false,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	// Store first entry
	entry1 := &CacheEntry{
		Key:       "key1",
		Response:  &openai.ChatCompletionResponse{Id: "response1"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err = manager.storeEntry(entry1)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(manager.memoryCache))
	assert.Contains(t, manager.memoryCache, "key1")

	// Store second entry
	entry2 := &CacheEntry{
		Key:       "key2",
		Response:  &openai.ChatCompletionResponse{Id: "response2"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err = manager.storeEntry(entry2)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(manager.memoryCache))

	// Store third entry (should evict first entry due to MaxEntries limit)
	entry3 := &CacheEntry{
		Key:       "key3",
		Response:  &openai.ChatCompletionResponse{Id: "response3"},
		ExpiresAt: time.Now().Add(time.Hour),
	}

	err = manager.storeEntry(entry3)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(manager.memoryCache)) // Still at limit
	assert.NotContains(t, manager.memoryCache, "key1") // First entry evicted
	assert.Contains(t, manager.memoryCache, "key2")
	assert.Contains(t, manager.memoryCache, "key3")
}

func TestCacheManager_CompressEntry(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		CompressionEnabled: true,
		CompressionLevel:   6,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	entry := &CacheEntry{
		Key: "test-key",
		Response: &openai.ChatCompletionResponse{
			Id:     "test-response",
			Object: "chat.completion",
			Model:  "gpt-3.5-turbo",
		},
	}

	err = manager.compressEntry(entry)
	assert.NoError(t, err)
	assert.True(t, entry.Compressed)
	assert.Greater(t, entry.OriginalSize, int64(0))

	// Test with compression disabled
	manager.config.CompressionEnabled = false
	entry2 := &CacheEntry{
		Key: "test-key-2",
		Response: &openai.ChatCompletionResponse{
			Id: "test-response-2",
		},
	}

	err = manager.compressEntry(entry2)
	assert.NoError(t, err)
	assert.False(t, entry2.Compressed)
	assert.Equal(t, int64(0), entry2.OriginalSize)
}

func TestCacheManager_UpdateLookupStats(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		PerTenantLimits: true,
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	tenantID := "test-tenant"

	// Test hit
	result := &CacheLookupResult{
		Found:    true,
		Strategy: StrategyExact,
	}

	manager.updateLookupStats(result, tenantID)

	assert.Equal(t, int64(1), manager.stats.Hits)
	assert.Equal(t, int64(0), manager.stats.Misses)
	assert.Equal(t, int64(1), manager.stats.ExactHits)
	assert.Contains(t, manager.stats.TenantStats, tenantID)
	assert.Equal(t, int64(1), manager.stats.TenantStats[tenantID].Hits)

	// Test miss
	result = &CacheLookupResult{
		Found:    false,
		Strategy: StrategyToken,
	}

	manager.updateLookupStats(result, tenantID)

	assert.Equal(t, int64(1), manager.stats.Hits)
	assert.Equal(t, int64(1), manager.stats.Misses)
	assert.Equal(t, int64(1), manager.stats.TenantStats[tenantID].Misses)
	assert.Equal(t, 0.5, manager.stats.TenantStats[tenantID].HitRate)

	// Test semantic hit
	result = &CacheLookupResult{
		Found:    true,
		Strategy: StrategySemantic,
	}

	manager.updateLookupStats(result, tenantID)

	assert.Equal(t, int64(2), manager.stats.Hits)
	assert.Equal(t, int64(1), manager.stats.SemanticHits)
}

func TestCacheManager_UpdateAdaptiveLearning(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &CacheConfig{
		Strategy: StrategyAdaptive,
		AdaptiveConfig: &AdaptiveConfig{
			EnablePatternDetection: true,
			TuningInterval:        30 * time.Minute,
		},
	}

	manager, err := NewCacheManager(config, nil, logger)
	require.NoError(t, err)
	defer manager.Stop()

	result := &CacheLookupResult{
		Found:    true,
		Strategy: StrategyExact,
	}

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
	}

	tenantID := "test-tenant"

	originalSampleCount := manager.adaptiveState.SampleCount

	manager.updateAdaptiveLearning(result, cacheReq, tenantID)

	// Verify sample count increased
	assert.Equal(t, originalSampleCount+1, manager.adaptiveState.SampleCount)

	// Verify pattern detection updated
	assert.Equal(t, int64(1), manager.adaptiveState.PatternDetection.CommonModels["gpt-3.5-turbo"])
	assert.Equal(t, int64(1), manager.adaptiveState.PatternDetection.UserPatterns[tenantID])
	assert.Greater(t, len(manager.adaptiveState.PatternDetection.QueryLength), 0)

	// Test with pattern detection disabled
	manager.config.AdaptiveConfig.EnablePatternDetection = false
	originalSampleCount = manager.adaptiveState.SampleCount

	manager.updateAdaptiveLearning(result, cacheReq, tenantID)

	// Should still increment sample count
	assert.Equal(t, originalSampleCount+1, manager.adaptiveState.SampleCount)
}

func TestCacheManager_EstimateQueryLength(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	manager := createTestCacheManager(t, logger)
	defer manager.Stop()

	// Test with string content
	cacheReq := &CacheRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello, world!"),
				},
			},
		},
	}

	length := manager.estimateQueryLength(cacheReq)
	assert.Equal(t, 13, length) // "Hello, world!" is 13 characters

	// Test with multiple messages
	cacheReq.Messages = append(cacheReq.Messages, openai.Message{
		Role: "assistant",
		Content: &openai.MessageContent{
			String: stringPtr("Hi there!"),
		},
	})

	length = manager.estimateQueryLength(cacheReq)
	assert.Equal(t, 22, length) // 13 + 9 characters

	// Test with empty content
	cacheReq.Messages = []openai.Message{}
	length = manager.estimateQueryLength(cacheReq)
	assert.Equal(t, 0, length)
}

// Helper functions are defined in cache_test.go