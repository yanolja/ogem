package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/monitoring"
	"github.com/yanolja/ogem/openai"
)

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// CacheStrategy defines different caching strategies
type CacheStrategy string

const (
	// StrategyNone disables caching
	StrategyNone CacheStrategy = "none"
	
	// StrategyExact only caches exact request matches
	StrategyExact CacheStrategy = "exact"
	
	// StrategySemantic caches based on semantic similarity
	StrategySemantic CacheStrategy = "semantic"
	
	// StrategyToken caches based on token similarity with fuzzy matching
	StrategyToken CacheStrategy = "token"
	
	// StrategyHybrid combines multiple caching strategies
	StrategyHybrid CacheStrategy = "hybrid"
	
	// StrategyAdaptive dynamically adjusts caching based on patterns
	StrategyAdaptive CacheStrategy = "adaptive"
)

// CacheBackend defines different cache storage backends
type CacheBackend string

const (
	BackendMemory    CacheBackend = "memory"
	BackendRedis     CacheBackend = "redis"
	BackendMultiTier CacheBackend = "multi_tier"
)

// CacheConfig configures the caching system
type CacheConfig struct {
	// Enable caching
	Enabled bool `yaml:"enabled"`
	
	// Primary caching strategy
	Strategy CacheStrategy `yaml:"strategy"`
	
	// Cache backend
	Backend CacheBackend `yaml:"backend"`
	
	// TTL settings
	DefaultTTL        time.Duration `yaml:"default_ttl"`
	MaxTTL           time.Duration `yaml:"max_ttl"`
	SemanticTTL      time.Duration `yaml:"semantic_ttl"`
	
	// Cache size limits
	MaxEntries       int64 `yaml:"max_entries"`
	MaxMemoryMB      int64 `yaml:"max_memory_mb"`
	
	// Semantic caching configuration
	SemanticConfig   *SemanticConfig `yaml:"semantic_config,omitempty"`
	
	// Token-based caching configuration
	TokenConfig      *TokenConfig `yaml:"token_config,omitempty"`
	
	// Multi-tier cache configuration
	MultiTierConfig  *MultiTierConfig `yaml:"multi_tier_config,omitempty"`
	
	// Adaptive caching configuration
	AdaptiveConfig   *AdaptiveConfig `yaml:"adaptive_config,omitempty"`
	
	// Performance settings
	CompressionEnabled bool    `yaml:"compression_enabled"`
	CompressionLevel   int     `yaml:"compression_level"`
	
	// Invalidation settings
	InvalidationPolicy InvalidationPolicy `yaml:"invalidation_policy"`
	
	// Redis configuration (if using Redis backend)
	RedisConfig      *RedisConfig `yaml:"redis_config,omitempty"`
	
	// Tenant-specific settings
	PerTenantLimits  bool `yaml:"per_tenant_limits"`
	TenantTTLOverrides map[string]time.Duration `yaml:"tenant_ttl_overrides,omitempty"`
	
	// Monitoring
	EnableMetrics    bool `yaml:"enable_metrics"`
	MetricsInterval  time.Duration `yaml:"metrics_interval"`
}

// SemanticConfig configures semantic similarity caching
type SemanticConfig struct {
	// Similarity threshold (0.0 to 1.0)
	SimilarityThreshold float64 `yaml:"similarity_threshold"`
	
	// Embedding service configuration
	EmbeddingProvider   string `yaml:"embedding_provider"`
	EmbeddingModel      string `yaml:"embedding_model"`
	EmbeddingEndpoint   string `yaml:"embedding_endpoint,omitempty"`
	
	// Vector similarity algorithm
	SimilarityAlgorithm string `yaml:"similarity_algorithm"` // "cosine", "euclidean", "dot_product"
	
	// Maximum embeddings to compute per hour
	MaxEmbeddingsPerHour int64 `yaml:"max_embeddings_per_hour"`
	
	// Cache embeddings for reuse
	CacheEmbeddings     bool `yaml:"cache_embeddings"`
}

// TokenConfig configures token-based caching
type TokenConfig struct {
	// Token similarity threshold
	TokenSimilarityThreshold float64 `yaml:"token_similarity_threshold"`
	
	// Maximum token distance for fuzzy matching
	MaxTokenDistance         int     `yaml:"max_token_distance"`
	
	// Enable fuzzy matching
	EnableFuzzyMatching      bool    `yaml:"enable_fuzzy_matching"`
	
	// Token normalization
	NormalizeTokens          bool    `yaml:"normalize_tokens"`
	IgnoreCase              bool    `yaml:"ignore_case"`
	RemovePunctuation       bool    `yaml:"remove_punctuation"`
}

// MultiTierConfig configures multi-tier caching
type MultiTierConfig struct {
	// L1 cache (fastest, smallest)
	L1Config *TierConfig `yaml:"l1_config"`
	
	// L2 cache (medium speed/size)
	L2Config *TierConfig `yaml:"l2_config"`
	
	// L3 cache (slower, largest)
	L3Config *TierConfig `yaml:"l3_config"`
	
	// Promotion/demotion thresholds
	L1PromotionThreshold int `yaml:"l1_promotion_threshold"`
	L2PromotionThreshold int `yaml:"l2_promotion_threshold"`
}

// TierConfig configures a cache tier
type TierConfig struct {
	Backend    CacheBackend  `yaml:"backend"`
	MaxEntries int64         `yaml:"max_entries"`
	TTL        time.Duration `yaml:"ttl"`
}

// AdaptiveConfig configures adaptive caching behavior
type AdaptiveConfig struct {
	// Learning window for pattern analysis
	LearningWindow    time.Duration `yaml:"learning_window"`
	
	// Minimum samples before adaptation
	MinSamples        int `yaml:"min_samples"`
	
	// Adaptation sensitivity (0.0 to 1.0)
	Sensitivity       float64 `yaml:"sensitivity"`
	
	// Cache hit rate thresholds for strategy switching
	HighHitThreshold  float64 `yaml:"high_hit_threshold"`
	LowHitThreshold   float64 `yaml:"low_hit_threshold"`
	
	// Pattern detection
	EnablePatternDetection bool `yaml:"enable_pattern_detection"`
	
	// Auto-tuning parameters
	EnableAutoTuning       bool `yaml:"enable_auto_tuning"`
	TuningInterval        time.Duration `yaml:"tuning_interval"`
}

// InvalidationPolicy defines cache invalidation behavior
type InvalidationPolicy string

const (
	InvalidationLRU    InvalidationPolicy = "lru"
	InvalidationLFU    InvalidationPolicy = "lfu"
	InvalidationTTL    InvalidationPolicy = "ttl"
	InvalidationRandom InvalidationPolicy = "random"
)

// RedisConfig configures Redis backend
type RedisConfig struct {
	Address     string        `yaml:"address"`
	Password    string        `yaml:"password,omitempty"`
	Database    int           `yaml:"database"`
	PoolSize    int           `yaml:"pool_size"`
	MaxRetries  int           `yaml:"max_retries"`
	DialTimeout time.Duration `yaml:"dial_timeout"`
	ReadTimeout time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

// CacheEntry represents a cached item
type CacheEntry struct {
	Key        string                 `json:"key"`
	Hash       string                 `json:"hash"`
	Request    *CacheRequest          `json:"request"`
	Response   *openai.ChatCompletionResponse `json:"response"`
	CreatedAt  time.Time              `json:"created_at"`
	ExpiresAt  time.Time              `json:"expires_at"`
	AccessCount int64                 `json:"access_count"`
	LastAccess time.Time              `json:"last_access"`
	TenantID   string                 `json:"tenant_id,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	
	// Semantic caching fields
	Embedding  []float32              `json:"embedding,omitempty"`
	Similarity float64                `json:"similarity,omitempty"`
	
	// Compression
	Compressed bool                   `json:"compressed"`
	OriginalSize int64                `json:"original_size"`
}

// CacheRequest represents a cacheable request
type CacheRequest struct {
	Model    string            `json:"model"`
	Messages []openai.Message  `json:"messages"`
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// CacheManager provides intelligent caching capabilities
type CacheManager struct {
	config    *CacheConfig
	backend   CacheBackend
	strategy  CacheStrategy
	monitor   *monitoring.MonitoringManager
	logger    *zap.SugaredLogger
	
	// Memory cache
	memoryCache map[string]*CacheEntry
	memoryMutex sync.RWMutex
	
	// LRU tracking
	accessOrder []string
	
	// Metrics
	stats       *CacheStats
	statsMutex  sync.RWMutex
	
	// Adaptive state
	adaptiveState *AdaptiveState
	
	// Background services
	cleanupTicker *time.Ticker
	metricsTicker *time.Ticker
	stopChan      chan struct{}
}

// CacheStats tracks cache performance metrics
type CacheStats struct {
	Hits              int64                    `json:"hits"`
	Misses            int64                    `json:"misses"`
	Stores            int64                    `json:"stores"`
	Evictions         int64                    `json:"evictions"`
	TotalEntries      int64                    `json:"total_entries"`
	MemoryUsageMB     float64                  `json:"memory_usage_mb"`
	HitRate           float64                  `json:"hit_rate"`
	AverageLatency    time.Duration            `json:"average_latency"`
	
	// Strategy-specific stats
	ExactHits         int64                    `json:"exact_hits"`
	SemanticHits      int64                    `json:"semantic_hits"`
	TokenHits         int64                    `json:"token_hits"`
	
	// Per-tenant stats
	TenantStats       map[string]*TenantCacheStats `json:"tenant_stats,omitempty"`
	
	LastUpdated       time.Time                `json:"last_updated"`
}

// TenantCacheStats tracks per-tenant cache statistics
type TenantCacheStats struct {
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	Entries    int64   `json:"entries"`
	MemoryMB   float64 `json:"memory_mb"`
}

// AdaptiveState tracks adaptive caching decisions
type AdaptiveState struct {
	CurrentStrategy    CacheStrategy            `json:"current_strategy"`
	StrategyHistory    []StrategyChange         `json:"strategy_history"`
	LearningData       map[string]interface{}   `json:"learning_data"`
	LastEvaluation     time.Time                `json:"last_evaluation"`
	SampleCount        int                      `json:"sample_count"`
	PatternDetection   *PatternData             `json:"pattern_detection,omitempty"`
	mutex              sync.RWMutex
}

// StrategyChange records when and why caching strategy changed
type StrategyChange struct {
	Timestamp    time.Time     `json:"timestamp"`
	FromStrategy CacheStrategy `json:"from_strategy"`
	ToStrategy   CacheStrategy `json:"to_strategy"`
	Reason       string        `json:"reason"`
	HitRate      float64       `json:"hit_rate"`
	Metrics      map[string]interface{} `json:"metrics"`
}

// PatternData tracks detected usage patterns
type PatternData struct {
	CommonModels     map[string]int64 `json:"common_models"`
	TimePatterns     map[int]int64    `json:"time_patterns"` // hour -> count
	UserPatterns     map[string]int64 `json:"user_patterns"`
	QueryLength      []int            `json:"query_length"`
	ResponseSize     []int            `json:"response_size"`
	LastAnalysis     time.Time        `json:"last_analysis"`
}

// CacheLookupResult represents the result of a cache lookup
type CacheLookupResult struct {
	Found      bool                           `json:"found"`
	Entry      *CacheEntry                    `json:"entry,omitempty"`
	Strategy   CacheStrategy                  `json:"strategy"`
	Similarity float64                        `json:"similarity,omitempty"`
	Latency    time.Duration                  `json:"latency"`
	Source     string                         `json:"source"` // "l1", "l2", "l3", "redis", "memory"
}

// NewCacheManager creates a new cache manager
func NewCacheManager(config *CacheConfig, monitor *monitoring.MonitoringManager, logger *zap.SugaredLogger) (*CacheManager, error) {
	if config == nil {
		config = DefaultCacheConfig()
	}
	
	manager := &CacheManager{
		config:      config,
		backend:     config.Backend,
		strategy:    config.Strategy,
		monitor:     monitor,
		logger:      logger,
		memoryCache: make(map[string]*CacheEntry),
		accessOrder: make([]string, 0),
		stats:       &CacheStats{TenantStats: make(map[string]*TenantCacheStats)},
		stopChan:    make(chan struct{}),
	}
	
	// Initialize adaptive state if using adaptive strategy
	if config.Strategy == StrategyAdaptive {
		manager.adaptiveState = &AdaptiveState{
			CurrentStrategy:  StrategyExact, // Start with exact matching
			StrategyHistory:  make([]StrategyChange, 0),
			LearningData:     make(map[string]interface{}),
			LastEvaluation:   time.Now(),
			PatternDetection: &PatternData{
				CommonModels:  make(map[string]int64),
				TimePatterns:  make(map[int]int64),
				UserPatterns:  make(map[string]int64),
				QueryLength:   make([]int, 0),
				ResponseSize:  make([]int, 0),
			},
		}
	}
	
	// Initialize backend-specific components
	if err := manager.initializeBackend(); err != nil {
		return nil, fmt.Errorf("failed to initialize cache backend: %v", err)
	}
	
	// Start background services
	if config.Enabled {
		manager.startBackgroundServices()
	}
	
	return manager, nil
}

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		Enabled:          true,
		Strategy:         StrategyExact,
		Backend:          BackendMemory,
		DefaultTTL:       1 * time.Hour,
		MaxTTL:          24 * time.Hour,
		SemanticTTL:     30 * time.Minute,
		MaxEntries:      10000,
		MaxMemoryMB:     500,
		CompressionEnabled: true,
		CompressionLevel:   6,
		InvalidationPolicy: InvalidationLRU,
		PerTenantLimits:    true,
		EnableMetrics:      true,
		MetricsInterval:    5 * time.Minute,
		
		SemanticConfig: &SemanticConfig{
			SimilarityThreshold:  0.95,
			EmbeddingProvider:   "openai",
			EmbeddingModel:      "text-embedding-3-small",
			SimilarityAlgorithm: "cosine",
			MaxEmbeddingsPerHour: 1000,
			CacheEmbeddings:     true,
		},
		
		TokenConfig: &TokenConfig{
			TokenSimilarityThreshold: 0.9,
			MaxTokenDistance:        5,
			EnableFuzzyMatching:     true,
			NormalizeTokens:         true,
			IgnoreCase:             true,
			RemovePunctuation:      false,
		},
		
		AdaptiveConfig: &AdaptiveConfig{
			LearningWindow:         24 * time.Hour,
			MinSamples:            100,
			Sensitivity:           0.1,
			HighHitThreshold:      0.8,
			LowHitThreshold:       0.3,
			EnablePatternDetection: true,
			EnableAutoTuning:       true,
			TuningInterval:        time.Hour,
		},
	}
}

// Lookup searches for a cached response
func (cm *CacheManager) Lookup(ctx context.Context, request *openai.ChatCompletionRequest, tenantID string) (*CacheLookupResult, error) {
	if !cm.config.Enabled {
		return &CacheLookupResult{Found: false, Strategy: StrategyNone}, nil
	}
	
	startTime := time.Now()
	
	// Convert request to cacheable format
	cacheReq := cm.convertToCacheRequest(request)
	
	// Determine active strategy
	strategy := cm.getActiveStrategy()
	
	var result *CacheLookupResult
	var err error
	
	// Perform lookup based on strategy
	switch strategy {
	case StrategyExact:
		result, err = cm.lookupExact(cacheReq, tenantID)
	case StrategySemantic:
		result, err = cm.lookupSemantic(ctx, cacheReq, tenantID)
	case StrategyToken:
		result, err = cm.lookupToken(cacheReq, tenantID)
	case StrategyHybrid:
		result, err = cm.lookupHybrid(ctx, cacheReq, tenantID)
	default:
		result = &CacheLookupResult{Found: false, Strategy: strategy}
	}
	
	if err != nil {
		cm.logger.Warnw("Cache lookup failed", "error", err, "strategy", strategy)
		return &CacheLookupResult{Found: false, Strategy: strategy}, nil
	}
	
	result.Latency = time.Since(startTime)
	
	// Update statistics
	cm.updateLookupStats(result, tenantID)
	
	// Update adaptive learning
	if cm.adaptiveState != nil {
		cm.updateAdaptiveLearning(result, cacheReq, tenantID)
	}
	
	return result, nil
}

// Store caches a response
func (cm *CacheManager) Store(ctx context.Context, request *openai.ChatCompletionRequest, response *openai.ChatCompletionResponse, tenantID string) error {
	if !cm.config.Enabled {
		return nil
	}
	
	// Convert request to cacheable format
	cacheReq := cm.convertToCacheRequest(request)
	
	// Generate cache key
	key := cm.generateCacheKey(cacheReq, tenantID)
	
	// Calculate TTL
	ttl := cm.calculateTTL(cacheReq, tenantID)
	
	// Create cache entry
	entry := &CacheEntry{
		Key:         key,
		Hash:        cm.generateHash(cacheReq),
		Request:     cacheReq,
		Response:    response,
		CreatedAt:   time.Now(),
		ExpiresAt:   time.Now().Add(ttl),
		AccessCount: 1,
		LastAccess:  time.Now(),
		TenantID:    tenantID,
		Metadata:    make(map[string]interface{}),
	}
	
	// Add semantic embedding if using semantic caching
	if cm.strategy == StrategySemantic || cm.strategy == StrategyHybrid {
		if embedding, err := cm.generateEmbedding(ctx, cacheReq); err == nil {
			entry.Embedding = embedding
		}
	}
	
	// Compress if enabled
	if cm.config.CompressionEnabled {
		if err := cm.compressEntry(entry); err != nil {
			cm.logger.Warnw("Failed to compress cache entry", "error", err)
		}
	}
	
	// Store in backend
	if err := cm.storeEntry(entry); err != nil {
		cm.logger.Errorw("Failed to store cache entry", "error", err, "key", key)
		return err
	}
	
	// Update statistics
	cm.updateStoreStats(entry)
	
	return nil
}

// Clear removes all cached entries
func (cm *CacheManager) Clear() error {
	cm.memoryMutex.Lock()
	defer cm.memoryMutex.Unlock()
	
	// Clear memory cache
	cm.memoryCache = make(map[string]*CacheEntry)
	cm.accessOrder = make([]string, 0)
	
	// Clear backend-specific storage
	if err := cm.clearBackend(); err != nil {
		return err
	}
	
	// Reset statistics
	cm.statsMutex.Lock()
	cm.stats = &CacheStats{TenantStats: make(map[string]*TenantCacheStats)}
	cm.statsMutex.Unlock()
	
	cm.logger.Info("Cache cleared successfully")
	return nil
}

// ClearTenant removes all cached entries for a specific tenant
func (cm *CacheManager) ClearTenant(tenantID string) error {
	cm.memoryMutex.Lock()
	defer cm.memoryMutex.Unlock()
	
	// Remove tenant entries from memory cache
	for key, entry := range cm.memoryCache {
		if entry.TenantID == tenantID {
			delete(cm.memoryCache, key)
			cm.removeFromAccessOrder(key)
		}
	}
	
	// Clear tenant statistics
	cm.statsMutex.Lock()
	delete(cm.stats.TenantStats, tenantID)
	cm.statsMutex.Unlock()
	
	cm.logger.Infow("Tenant cache cleared", "tenant_id", tenantID)
	return nil
}

// GetStats returns current cache statistics
func (cm *CacheManager) GetStats() *CacheStats {
	cm.statsMutex.RLock()
	defer cm.statsMutex.RUnlock()
	
	// Create a copy to prevent modification
	statsCopy := *cm.stats
	statsCopy.TenantStats = make(map[string]*TenantCacheStats)
	for k, v := range cm.stats.TenantStats {
		tenantStatsCopy := *v
		statsCopy.TenantStats[k] = &tenantStatsCopy
	}
	
	// Update hit rate
	total := statsCopy.Hits + statsCopy.Misses
	if total > 0 {
		statsCopy.HitRate = float64(statsCopy.Hits) / float64(total)
	}
	
	// Update memory usage (without additional locking since we're already locked)
	cm.memoryMutex.RLock()
	entryCount := len(cm.memoryCache)
	cm.memoryMutex.RUnlock()
	
	averageEntrySize := 10.0 // KB estimate
	statsCopy.MemoryUsageMB = float64(entryCount) * averageEntrySize / 1024.0 // Convert to MB
	statsCopy.LastUpdated = time.Now()
	
	return &statsCopy
}

// Helper methods

func (cm *CacheManager) convertToCacheRequest(req *openai.ChatCompletionRequest) *CacheRequest {
	cacheReq := &CacheRequest{
		Model:    req.Model,
		Messages: req.Messages,
		Settings: make(map[string]interface{}),
	}
	
	// Include relevant settings that affect response
	if req.Temperature != nil {
		cacheReq.Settings["temperature"] = *req.Temperature
	}
	if req.MaxTokens != nil {
		cacheReq.Settings["max_tokens"] = *req.MaxTokens
	}
	if req.TopP != nil {
		cacheReq.Settings["top_p"] = *req.TopP
	}
	if req.FrequencyPenalty != nil {
		cacheReq.Settings["frequency_penalty"] = *req.FrequencyPenalty
	}
	if req.PresencePenalty != nil {
		cacheReq.Settings["presence_penalty"] = *req.PresencePenalty
	}
	
	return cacheReq
}

func (cm *CacheManager) generateCacheKey(req *CacheRequest, tenantID string) string {
	// Create a deterministic key based on request content
	keyData := map[string]interface{}{
		"model":     req.Model,
		"messages":  req.Messages,
		"settings":  req.Settings,
		"tenant_id": tenantID,
	}
	
	jsonData, err := json.Marshal(keyData)
	if err != nil {
		// Fallback to simple string-based key if marshaling fails
		var messageContent string
		if len(req.Messages) > 0 && req.Messages[0].Content != nil && req.Messages[0].Content.String != nil {
			content := *req.Messages[0].Content.String
			if len(content) > 50 {
				messageContent = content[:50]
			} else {
				messageContent = content
			}
		} else {
			messageContent = "empty"
		}
		fallbackKey := fmt.Sprintf("%s:%s:%s", req.Model, tenantID, messageContent)
		hash := sha256.Sum256([]byte(fallbackKey))
		return hex.EncodeToString(hash[:])
	}
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])
}

func (cm *CacheManager) generateHash(req *CacheRequest) string {
	// Generate a hash for content matching
	contentData := map[string]interface{}{
		"model":    req.Model,
		"messages": req.Messages,
		"settings": req.Settings,
	}
	
	jsonData, err := json.Marshal(contentData)
	if err != nil {
		// Fallback to model name + timestamp if marshaling fails  
		fallbackData := fmt.Sprintf("%s:%d", req.Model, time.Now().Unix())
		hash := sha256.Sum256([]byte(fallbackData))
		return hex.EncodeToString(hash[:])[:16]
	}
	hash := sha256.Sum256(jsonData)
	return hex.EncodeToString(hash[:])[:16] // Shortened hash for display
}

func (cm *CacheManager) calculateTTL(req *CacheRequest, tenantID string) time.Duration {
	// Check for tenant-specific TTL overrides
	if cm.config.TenantTTLOverrides != nil {
		if ttl, exists := cm.config.TenantTTLOverrides[tenantID]; exists {
			return ttl
		}
	}
	
	// Use default TTL or strategy-specific TTL
	switch cm.getActiveStrategy() {
	case StrategySemantic:
		return cm.config.SemanticTTL
	default:
		return cm.config.DefaultTTL
	}
}

func (cm *CacheManager) getActiveStrategy() CacheStrategy {
	if cm.strategy == StrategyAdaptive && cm.adaptiveState != nil {
		cm.adaptiveState.mutex.RLock()
		defer cm.adaptiveState.mutex.RUnlock()
		return cm.adaptiveState.CurrentStrategy
	}
	return cm.strategy
}

func (cm *CacheManager) initializeBackend() error {
	// Initialize backend-specific components
	switch cm.backend {
	case BackendRedis:
		// Redis initialization would go here
		cm.logger.Info("Redis cache backend initialized")
	case BackendMultiTier:
		// Multi-tier initialization would go here
		cm.logger.Info("Multi-tier cache backend initialized")
	default:
		cm.logger.Info("Memory cache backend initialized")
	}
	return nil
}

func (cm *CacheManager) startBackgroundServices() {
	// Cleanup service
	cm.cleanupTicker = time.NewTicker(time.Hour)
	go cm.runCleanupService()
	
	// Metrics service
	if cm.config.EnableMetrics {
		cm.metricsTicker = time.NewTicker(cm.config.MetricsInterval)
		go cm.runMetricsService()
	}
	
	// Adaptive tuning service
	if cm.adaptiveState != nil && cm.config.AdaptiveConfig.EnableAutoTuning {
		go cm.runAdaptiveTuningService()
	}
}

func (cm *CacheManager) runCleanupService() {
	for {
		select {
		case <-cm.cleanupTicker.C:
			cm.performCleanup()
		case <-cm.stopChan:
			return
		}
	}
}

func (cm *CacheManager) runMetricsService() {
	for {
		select {
		case <-cm.metricsTicker.C:
			cm.recordMetrics()
		case <-cm.stopChan:
			return
		}
	}
}

func (cm *CacheManager) runAdaptiveTuningService() {
	ticker := time.NewTicker(cm.config.AdaptiveConfig.TuningInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			cm.performAdaptiveTuning()
		case <-cm.stopChan:
			return
		}
	}
}

func (cm *CacheManager) performCleanup() {
	cm.memoryMutex.Lock()
	defer cm.memoryMutex.Unlock()
	
	now := time.Now()
	expiredKeys := make([]string, 0)
	
	// Find expired entries
	for key, entry := range cm.memoryCache {
		if now.After(entry.ExpiresAt) {
			expiredKeys = append(expiredKeys, key)
		}
	}
	
	// Remove expired entries
	for _, key := range expiredKeys {
		delete(cm.memoryCache, key)
		cm.removeFromAccessOrder(key)
	}
	
	// Enforce memory limits
	if int64(len(cm.memoryCache)) > cm.config.MaxEntries {
		cm.evictOldestEntries(int64(len(cm.memoryCache)) - cm.config.MaxEntries)
	}
	
	cm.statsMutex.Lock()
	cm.stats.Evictions += int64(len(expiredKeys))
	cm.statsMutex.Unlock()
	
	if len(expiredKeys) > 0 {
		cm.logger.Infow("Cache cleanup completed", "expired_entries", len(expiredKeys))
	}
}

func (cm *CacheManager) recordMetrics() {
	if cm.monitor == nil {
		return
	}
	
	stats := cm.GetStats()
	
	// Record cache metrics
	metrics := []*monitoring.Metric{
		{
			Name:      "cache_hits_total",
			Type:      monitoring.MetricTypeCounter,
			Value:     float64(stats.Hits),
			Timestamp: time.Now(),
		},
		{
			Name:      "cache_misses_total",
			Type:      monitoring.MetricTypeCounter,
			Value:     float64(stats.Misses),
			Timestamp: time.Now(),
		},
		{
			Name:      "cache_hit_rate",
			Type:      monitoring.MetricTypeGauge,
			Value:     stats.HitRate,
			Timestamp: time.Now(),
		},
		{
			Name:      "cache_entries_total",
			Type:      monitoring.MetricTypeGauge,
			Value:     float64(stats.TotalEntries),
			Timestamp: time.Now(),
		},
		{
			Name:      "cache_memory_usage_mb",
			Type:      monitoring.MetricTypeGauge,
			Value:     stats.MemoryUsageMB,
			Timestamp: time.Now(),
		},
	}
	
	for _, metric := range metrics {
		if err := cm.monitor.RecordMetric(metric); err != nil {
			cm.logger.Warnw("Failed to record cache metric", "error", err)
		}
	}
}

func (cm *CacheManager) calculateMemoryUsage() float64 {
	cm.memoryMutex.RLock()
	defer cm.memoryMutex.RUnlock()
	
	// This is a simplified calculation
	// In production, you would measure actual memory usage
	entryCount := len(cm.memoryCache)
	averageEntrySize := 10.0 // KB estimate
	return float64(entryCount) * averageEntrySize / 1024.0 // Convert to MB
}

func (cm *CacheManager) removeFromAccessOrder(key string) {
	for i, k := range cm.accessOrder {
		if k == key {
			cm.accessOrder = append(cm.accessOrder[:i], cm.accessOrder[i+1:]...)
			break
		}
	}
}

func (cm *CacheManager) evictOldestEntries(count int64) {
	// This is a simplified LRU implementation
	for i := int64(0); i < count && len(cm.accessOrder) > 0; i++ {
		oldestKey := cm.accessOrder[0]
		delete(cm.memoryCache, oldestKey)
		cm.accessOrder = cm.accessOrder[1:]
	}
}

// Stop gracefully stops the cache manager
func (cm *CacheManager) Stop() {
	close(cm.stopChan)
	
	if cm.cleanupTicker != nil {
		cm.cleanupTicker.Stop()
	}
	if cm.metricsTicker != nil {
		cm.metricsTicker.Stop()
	}
	
	cm.logger.Info("Cache manager stopped")
}