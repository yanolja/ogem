package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

// lookupExact performs exact cache matching
func (cm *CacheManager) lookupExact(req *CacheRequest, tenantID string) (*CacheLookupResult, error) {
	key := cm.generateCacheKey(req, tenantID)

	cm.mutex.RLock()
	entry, exists := cm.memoryCache[key]
	cm.mutex.RUnlock()

	if !exists || time.Now().After(entry.ExpiresAt) {
		return &CacheLookupResult{
			Found:    false,
			Strategy: StrategyExact,
			Source:   "memory",
		}, nil
	}

	// Update access tracking
	cm.updateEntryAccess(entry)

	return &CacheLookupResult{
		Found:      true,
		Entry:      entry,
		Strategy:   StrategyExact,
		Similarity: 1.0,
		Source:     "memory",
	}, nil
}

// lookupSemantic performs semantic similarity-based cache matching
func (cm *CacheManager) lookupSemantic(ctx context.Context, req *CacheRequest, tenantID string) (*CacheLookupResult, error) {
	// Generate embedding for the request
	reqEmbedding, err := cm.generateEmbedding(ctx, req)
	if err != nil {
		cm.logger.Warnw("Failed to generate embedding for semantic lookup", "error", err)
		// Fall back to exact matching
		return cm.lookupExact(req, tenantID)
	}

	cm.mutex.RLock()

	var bestMatch *CacheEntry
	var bestSimilarity float64
	threshold := cm.config.SemanticConfig.SimilarityThreshold

	// Search through cached entries for semantic matches
	for _, entry := range cm.memoryCache {
		// Skip entries from different tenants if tenant isolation is enabled
		if cm.config.PerTenantLimits && entry.TenantID != tenantID {
			continue
		}

		// Skip expired entries
		if time.Now().After(entry.ExpiresAt) {
			continue
		}

		// Skip entries without embeddings
		if len(entry.Embedding) == 0 {
			continue
		}

		// Skip entries with different models (semantic matching should be model-specific)
		if entry.Request.Model != req.Model {
			continue
		}

		// Calculate semantic similarity
		similarity := cm.calculateCosineSimilarity(reqEmbedding, entry.Embedding)

		if similarity >= threshold && similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = entry
		}
	}

	if bestMatch == nil {
		cm.mutex.RUnlock()
		return &CacheLookupResult{
			Found:    false,
			Strategy: StrategySemantic,
			Source:   "memory",
		}, nil
	}

	// Update access tracking
	cm.mutex.RUnlock()
	cm.updateEntryAccess(bestMatch)

	return &CacheLookupResult{
		Found:      true,
		Entry:      bestMatch,
		Strategy:   StrategySemantic,
		Similarity: bestSimilarity,
		Source:     "memory",
	}, nil
}

// lookupToken performs token-based fuzzy cache matching
func (cm *CacheManager) lookupToken(req *CacheRequest, tenantID string) (*CacheLookupResult, error) {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	var bestMatch *CacheEntry
	var bestSimilarity float64
	threshold := cm.config.TokenConfig.TokenSimilarityThreshold

	for _, entry := range cm.memoryCache {
		// Skip entries from different tenants if tenant isolation is enabled
		if cm.config.PerTenantLimits && entry.TenantID != tenantID {
			continue
		}

		// Skip expired entries
		if time.Now().After(entry.ExpiresAt) {
			continue
		}

		// Skip entries with different models
		if entry.Request.Model != req.Model {
			continue
		}

		// Extract tokens from request and cached entry
		reqTokens := cm.extractTokens(req)
		entryTokens := cm.extractTokens(entry.Request)

		// Calculate token similarity
		similarity := cm.calculateTokenSimilarity(reqTokens, entryTokens)

		if similarity >= threshold && similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = entry
		}
	}

	if bestMatch == nil {
		return &CacheLookupResult{
			Found:    false,
			Strategy: StrategyToken,
			Source:   "memory",
		}, nil
	}

	// Update access tracking
	cm.mutex.RUnlock()
	cm.updateEntryAccess(bestMatch)
	cm.mutex.RLock()

	return &CacheLookupResult{
		Found:      true,
		Entry:      bestMatch,
		Strategy:   StrategyToken,
		Similarity: bestSimilarity,
		Source:     "memory",
	}, nil
}

// lookupHybrid combines multiple caching strategies
func (cm *CacheManager) lookupHybrid(ctx context.Context, req *CacheRequest, tenantID string) (*CacheLookupResult, error) {
	// Try exact match first (fastest)
	if result, err := cm.lookupExact(req, tenantID); err == nil && result.Found {
		result.Strategy = StrategyHybrid
		return result, nil
	}

	// Try semantic matching if available
	if cm.config.SemanticConfig != nil {
		if result, err := cm.lookupSemantic(ctx, req, tenantID); err == nil && result.Found {
			result.Strategy = StrategyHybrid
			return result, nil
		}
	}

	// Try token-based matching as fallback
	if cm.config.TokenConfig != nil {
		if result, err := cm.lookupToken(req, tenantID); err == nil && result.Found {
			result.Strategy = StrategyHybrid
			return result, nil
		}
	}

	return &CacheLookupResult{
		Found:    false,
		Strategy: StrategyHybrid,
		Source:   "memory",
	}, nil
}

// generateEmbedding creates an embedding for semantic caching
func (cm *CacheManager) generateEmbedding(ctx context.Context, req *CacheRequest) ([]float32, error) {
	if cm.config.SemanticConfig == nil {
		return nil, fmt.Errorf("semantic config not available")
	}

	// Extract text content from messages
	var textContent strings.Builder
	for _, message := range req.Messages {
		if message.Content != nil {
			if message.Content.String != nil {
				textContent.WriteString(*message.Content.String)
				textContent.WriteString(" ")
			} else if message.Content.Parts != nil {
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						textContent.WriteString(part.Content.TextContent.Text)
						textContent.WriteString(" ")
					}
				}
			}
		}
	}

	text := strings.TrimSpace(textContent.String())
	if text == "" {
		return nil, fmt.Errorf("no text content found for embedding")
	}

	// This is a simplified implementation
	// In production, you would call the actual embedding service
	embedding := cm.generateSimulatedEmbedding(text)

	return embedding, nil
}

// generateSimulatedEmbedding creates a simulated embedding for testing
func (cm *CacheManager) generateSimulatedEmbedding(text string) []float32 {
	// This is a simplified simulation
	// In production, this would call an actual embedding API
	embedding := make([]float32, 384) // Common embedding dimension

	// Generate a deterministic but varied embedding based on text content
	hash := 0
	for _, char := range text {
		hash = hash*31 + int(char)
	}

	for i := range embedding {
		// Create a pseudo-random but deterministic value
		seed := hash + i*7
		embedding[i] = float32(math.Sin(float64(seed))) * 0.5
	}

	// Normalize the embedding
	norm := float32(0)
	for _, val := range embedding {
		norm += val * val
	}
	norm = float32(math.Sqrt(float64(norm)))

	if norm > 0 {
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding
}

// calculateCosineSimilarity computes cosine similarity between two embeddings
func (cm *CacheManager) calculateCosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0.0
	}

	var dotProduct, normA, normB float64

	for i := range a {
		dotProduct += float64(a[i] * b[i])
		normA += float64(a[i] * a[i])
		normB += float64(b[i] * b[i])
	}

	if normA == 0 || normB == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// extractTokens extracts and normalizes tokens from a cache request
func (cm *CacheManager) extractTokens(req *CacheRequest) []string {
	var tokens []string

	// Extract tokens from all messages
	for _, message := range req.Messages {
		if message.Content != nil {
			var text string
			if message.Content.String != nil {
				text = *message.Content.String
			} else if message.Content.Parts != nil {
				var parts []string
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						parts = append(parts, part.Content.TextContent.Text)
					}
				}
				text = strings.Join(parts, " ")
			}

			if text != "" {
				messageTokens := cm.tokenizeText(text)
				tokens = append(tokens, messageTokens...)
			}
		}
	}

	return tokens
}

// tokenizeText performs basic tokenization and normalization
func (cm *CacheManager) tokenizeText(text string) []string {
	// Basic tokenization (split on whitespace and common punctuation)
	tokens := strings.FieldsFunc(text, func(c rune) bool {
		return c == ' ' || c == '\t' || c == '\n' || c == '\r'
	})

	var normalizedTokens []string
	for _, token := range tokens {
		normalized := token

		// Apply normalization based on config
		if cm.config.TokenConfig.IgnoreCase {
			normalized = strings.ToLower(normalized)
		}

		if cm.config.TokenConfig.RemovePunctuation {
			// Remove common punctuation
			normalized = strings.Trim(normalized, ".,!?;:\"'()[]{}+-=")
		}

		if normalized != "" {
			normalizedTokens = append(normalizedTokens, normalized)
		}
	}

	return normalizedTokens
}

// calculateTokenSimilarity computes similarity between token sets
func (cm *CacheManager) calculateTokenSimilarity(tokensA, tokensB []string) float64 {
	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	// Create token frequency maps
	setA := make(map[string]bool)
	setB := make(map[string]bool)

	for _, token := range tokensA {
		setA[token] = true
	}
	for _, token := range tokensB {
		setB[token] = true
	}

	// Calculate Jaccard similarity
	intersection := 0
	for token := range setA {
		if setB[token] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection

	if union == 0 {
		return 0.0
	}

	jaccard := float64(intersection) / float64(union)

	// Apply fuzzy matching if enabled
	if cm.config.TokenConfig.EnableFuzzyMatching {
		fuzzyBonus := cm.calculateFuzzyMatchBonus(tokensA, tokensB)
		jaccard = math.Min(1.0, jaccard+fuzzyBonus*0.1) // Small bonus for fuzzy matches
	}

	return jaccard
}

// calculateFuzzyMatchBonus adds similarity for near-matches
func (cm *CacheManager) calculateFuzzyMatchBonus(tokensA, tokensB []string) float64 {
	var fuzzyMatches int
	maxDistance := cm.config.TokenConfig.MaxTokenDistance

	for _, tokenA := range tokensA {
		for _, tokenB := range tokensB {
			if tokenA != tokenB && cm.editDistance(tokenA, tokenB) <= maxDistance {
				fuzzyMatches++
				break // Count each token A at most once
			}
		}
	}

	// Normalize by the smaller token set size
	minLength := min(len(tokensA), len(tokensB))
	if minLength == 0 {
		return 0.0
	}

	return float64(fuzzyMatches) / float64(minLength)
}

// editDistance calculates the Levenshtein distance between two strings
func (cm *CacheManager) editDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create a matrix to store distances
	rows := len(a) + 1
	cols := len(b) + 1
	matrix := make([][]int, rows)
	for i := range matrix {
		matrix[i] = make([]int, cols)
	}

	// Initialize first row and column
	for i := 0; i <= len(a); i++ {
		matrix[i][0] = i
	}
	for j := 0; j <= len(b); j++ {
		matrix[0][j] = j
	}

	// Fill the matrix
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}

			deletion := matrix[i-1][j] + 1
			insertion := matrix[i][j-1] + 1
			substitution := matrix[i-1][j-1] + cost

			matrix[i][j] = min(deletion, min(insertion, substitution))
		}
	}

	return matrix[len(a)][len(b)]
}

// updateEntryAccess updates access tracking for a cache entry
func (cm *CacheManager) updateEntryAccess(entry *CacheEntry) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	now := time.Now()

	// Update entry access info
	entry.AccessCount++
	entry.LastAccess = now

	// Update LRU order
	// Remove from current position
	cm.removeFromAccessOrder(entry.Key)

	// Add to end (most recently used)
	cm.accessOrder = append(cm.accessOrder, entry.Key)
}

// storeEntry stores a cache entry in the backend
func (cm *CacheManager) storeEntry(entry *CacheEntry) error {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Check memory limits before storing
	if int64(len(cm.memoryCache)) >= cm.config.MaxEntries {
		// Evict oldest entry
		if len(cm.accessOrder) > 0 {
			oldestKey := cm.accessOrder[0]
			delete(cm.memoryCache, oldestKey)
			cm.accessOrder = cm.accessOrder[1:]
		}
	}

	// Store in memory cache
	cm.memoryCache[entry.Key] = entry
	cm.accessOrder = append(cm.accessOrder, entry.Key)

	return nil
}

// clearBackend clears backend-specific storage
func (cm *CacheManager) clearBackend() error {
	// Implementation depends on backend type
	switch cm.backend {
	case BackendRedis:
		// Clear Redis cache
		return nil
	case BackendMultiTier:
		// Clear multi-tier cache
		return nil
	default:
		// Memory cache is already cleared
		return nil
	}
}

// compressEntry compresses a cache entry if compression is enabled
func (cm *CacheManager) compressEntry(entry *CacheEntry) error {
	if !cm.config.CompressionEnabled {
		return nil
	}

	// Serialize the response for compression
	responseData, err := json.Marshal(entry.Response)
	if err != nil {
		return err
	}

	entry.OriginalSize = int64(len(responseData))

	// This is a placeholder for actual compression
	// In production, you would use gzip, zstd, or other compression algorithms
	entry.Compressed = true

	return nil
}

// updateLookupStats updates cache lookup statistics
func (cm *CacheManager) updateLookupStats(result *CacheLookupResult, tenantID string) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	if result.Found {
		cm.stats.Hits++

		// Update strategy-specific stats
		switch result.Strategy {
		case StrategyExact:
			cm.stats.ExactHits++
		case StrategySemantic:
			cm.stats.SemanticHits++
		case StrategyToken:
			cm.stats.TokenHits++
		}
	} else {
		cm.stats.Misses++
	}

	// Update tenant-specific stats
	if cm.config.PerTenantLimits && tenantID != "" {
		tenantStats, exists := cm.stats.TenantStats[tenantID]
		if !exists {
			tenantStats = &TenantCacheStats{}
			cm.stats.TenantStats[tenantID] = tenantStats
		}

		if result.Found {
			tenantStats.Hits++
		} else {
			tenantStats.Misses++
		}

		total := tenantStats.Hits + tenantStats.Misses
		if total > 0 {
			tenantStats.HitRate = float64(tenantStats.Hits) / float64(total)
		}
	}

	// Update total entries count
	cm.stats.TotalEntries = int64(len(cm.memoryCache))
}

// updateStoreStats updates cache store statistics
func (cm *CacheManager) updateStoreStats(entry *CacheEntry) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	cm.stats.Stores++
	cm.stats.TotalEntries = int64(len(cm.memoryCache))

	// Update tenant-specific stats
	if cm.config.PerTenantLimits && entry.TenantID != "" {
		tenantStats, exists := cm.stats.TenantStats[entry.TenantID]
		if !exists {
			tenantStats = &TenantCacheStats{}
			cm.stats.TenantStats[entry.TenantID] = tenantStats
		}

		tenantStats.Entries++
	}
}

// updateAdaptiveLearning updates adaptive caching learning data
func (cm *CacheManager) updateAdaptiveLearning(result *CacheLookupResult, req *CacheRequest, tenantID string) {
	if cm.adaptiveState == nil {
		return
	}

	cm.adaptiveState.mutex.Lock()
	defer cm.adaptiveState.mutex.Unlock()

	cm.adaptiveState.SampleCount++

	// Update pattern detection
	if cm.config.AdaptiveConfig.EnablePatternDetection && cm.adaptiveState.PatternDetection != nil {
		cm.adaptiveState.PatternDetection.CommonModels[req.Model]++

		hour := time.Now().Hour()
		cm.adaptiveState.PatternDetection.TimePatterns[hour]++

		if tenantID != "" {
			cm.adaptiveState.PatternDetection.UserPatterns[tenantID]++
		}

		// Track query characteristics
		queryLength := cm.estimateQueryLength(req)
		cm.adaptiveState.PatternDetection.QueryLength = append(cm.adaptiveState.PatternDetection.QueryLength, queryLength)

		// Keep only recent samples (limit memory usage)
		if len(cm.adaptiveState.PatternDetection.QueryLength) > 1000 {
			cm.adaptiveState.PatternDetection.QueryLength = cm.adaptiveState.PatternDetection.QueryLength[500:]
		}
	}
}

// performAdaptiveTuning performs adaptive strategy tuning
func (cm *CacheManager) performAdaptiveTuning() {
	if cm.adaptiveState == nil || cm.config.AdaptiveConfig == nil {
		return
	}

	cm.adaptiveState.mutex.Lock()
	defer cm.adaptiveState.mutex.Unlock()

	now := time.Now()
	if now.Sub(cm.adaptiveState.LastEvaluation) < cm.config.AdaptiveConfig.LearningWindow {
		return
	}

	if cm.adaptiveState.SampleCount < cm.config.AdaptiveConfig.MinSamples {
		return
	}

	// Calculate current hit rate
	stats := cm.GetStats()
	currentHitRate := stats.HitRate

	// Determine if strategy should change
	var newStrategy CacheStrategy
	var reason string

	if currentHitRate < cm.config.AdaptiveConfig.LowHitThreshold {
		// Hit rate is low, try a different strategy
		switch cm.adaptiveState.CurrentStrategy {
		case StrategyExact:
			newStrategy = StrategySemantic
			reason = "low hit rate with exact matching"
		case StrategySemantic:
			newStrategy = StrategyToken
			reason = "low hit rate with semantic matching"
		case StrategyToken:
			newStrategy = StrategyHybrid
			reason = "low hit rate with token matching"
		default:
			newStrategy = StrategyExact
			reason = "reset to exact matching"
		}
	} else if currentHitRate > cm.config.AdaptiveConfig.HighHitThreshold {
		// Hit rate is high, we can try more aggressive caching
		if cm.adaptiveState.CurrentStrategy == StrategyExact {
			newStrategy = StrategyHybrid
			reason = "high hit rate, enabling hybrid caching"
		}
	}

	// Apply strategy change if needed
	if newStrategy != "" && newStrategy != cm.adaptiveState.CurrentStrategy {
		cm.logger.Infow("Adaptive caching strategy change",
			"from", cm.adaptiveState.CurrentStrategy,
			"to", newStrategy,
			"reason", reason,
			"hit_rate", currentHitRate)

		cm.adaptiveState.StrategyHistory = append(cm.adaptiveState.StrategyHistory, StrategyChange{
			Timestamp:    now,
			FromStrategy: cm.adaptiveState.CurrentStrategy,
			ToStrategy:   newStrategy,
			Reason:       reason,
			HitRate:      currentHitRate,
			Metrics: map[string]interface{}{
				"total_hits":   stats.Hits,
				"total_misses": stats.Misses,
				"sample_count": cm.adaptiveState.SampleCount,
			},
		})

		cm.adaptiveState.CurrentStrategy = newStrategy
	}

	cm.adaptiveState.LastEvaluation = now
	cm.adaptiveState.SampleCount = 0
}

// estimateQueryLength estimates the length/complexity of a query
func (cm *CacheManager) estimateQueryLength(req *CacheRequest) int {
	totalLength := 0
	for _, message := range req.Messages {
		if message.Content != nil {
			if message.Content.String != nil {
				totalLength += len(*message.Content.String)
			} else if message.Content.Parts != nil {
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						totalLength += len(part.Content.TextContent.Text)
					}
				}
			}
		}
	}
	return totalLength
}

// Helper functions

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
