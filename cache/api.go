package cache

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	sdkOgem "github.com/yanolja/ogem/sdk/go"
	"go.uber.org/zap"
)

// CacheAPI provides REST API endpoints for cache management
type CacheAPI struct {
	cacheManager *CacheManager
	logger       *zap.SugaredLogger
}

// NewCacheAPI creates a new cache API instance
func NewCacheAPI(cacheManager *CacheManager, logger *zap.SugaredLogger) *CacheAPI {
	return &CacheAPI{
		cacheManager: cacheManager,
		logger:       logger,
	}
}

// RegisterRoutes registers all cache API routes
func (api *CacheAPI) RegisterRoutes(router *mux.Router) {
	// Cache statistics and monitoring
	router.HandleFunc("/cache/stats", api.GetCacheStats).Methods("GET")
	router.HandleFunc("/cache/stats/tenant/{tenant_id}", api.GetTenantCacheStats).Methods("GET")

	// Cache management
	router.HandleFunc("/cache/clear", api.ClearCache).Methods("POST")
	router.HandleFunc("/cache/clear/tenant/{tenant_id}", api.ClearTenantCache).Methods("POST")

	// Cache configuration
	router.HandleFunc("/cache/config", api.GetCacheConfig).Methods("GET")
	router.HandleFunc("/cache/config", api.UpdateCacheConfig).Methods("PUT")

	// Cache entries management
	router.HandleFunc("/cache/entries", api.ListCacheEntries).Methods("GET")
	router.HandleFunc("/cache/entries/{key}", api.GetCacheEntry).Methods("GET")
	router.HandleFunc("/cache/entries/{key}", api.DeleteCacheEntry).Methods("DELETE")

	// Adaptive caching
	router.HandleFunc("/cache/adaptive/state", api.GetAdaptiveState).Methods("GET")
	router.HandleFunc("/cache/adaptive/strategy", api.SetAdaptiveStrategy).Methods("POST")

	// Cache warming and preloading
	router.HandleFunc("/cache/warm", api.WarmCache).Methods("POST")
	router.HandleFunc("/cache/preload", api.PreloadCache).Methods("POST")

	// Cache analysis and insights
	router.HandleFunc("/cache/analysis", api.GetCacheAnalysis).Methods("GET")
	router.HandleFunc("/cache/insights", api.GetCacheInsights).Methods("GET")
}

// GetCacheStats handles GET /cache/stats
func (api *CacheAPI) GetCacheStats(w http.ResponseWriter, r *http.Request) {
	stats := api.cacheManager.GetStats()
	api.writeJSON(w, http.StatusOK, stats)
}

// GetTenantCacheStats handles GET /cache/stats/tenant/{tenant_id}
func (api *CacheAPI) GetTenantCacheStats(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]

	stats := api.cacheManager.GetStats()
	tenantStats, exists := stats.TenantStats[tenantID]
	if !exists {
		api.writeError(w, http.StatusNotFound, "tenant_not_found", "Tenant cache statistics not found")
		return
	}

	response := map[string]interface{}{
		"tenant_id": tenantID,
		"stats":     tenantStats,
	}

	api.writeJSON(w, http.StatusOK, response)
}

// ClearCache handles POST /cache/clear
func (api *CacheAPI) ClearCache(w http.ResponseWriter, r *http.Request) {
	if err := api.cacheManager.Clear(); err != nil {
		api.logger.Errorw("Failed to clear cache", "error", err)
		api.writeError(w, http.StatusInternalServerError, "clear_failed", "Failed to clear cache")
		return
	}

	response := map[string]interface{}{
		"message":   "Cache cleared successfully",
		"timestamp": time.Now(),
	}

	api.writeJSON(w, http.StatusOK, response)
}

// ClearTenantCache handles POST /cache/clear/tenant/{tenant_id}
func (api *CacheAPI) ClearTenantCache(w http.ResponseWriter, r *http.Request) {
	tenantID := mux.Vars(r)["tenant_id"]

	if err := api.cacheManager.ClearTenant(tenantID); err != nil {
		api.logger.Errorw("Failed to clear tenant cache", "error", err, "tenant_id", tenantID)
		api.writeError(w, http.StatusInternalServerError, "clear_failed", "Failed to clear tenant cache")
		return
	}

	response := map[string]interface{}{
		"message":   "Tenant cache cleared successfully",
		"tenant_id": tenantID,
		"timestamp": time.Now(),
	}

	api.writeJSON(w, http.StatusOK, response)
}

// GetCacheConfig handles GET /cache/config
func (api *CacheAPI) GetCacheConfig(w http.ResponseWriter, r *http.Request) {
	config := api.cacheManager.config
	api.writeJSON(w, http.StatusOK, config)
}

// UpdateCacheConfig handles PUT /cache/config
func (api *CacheAPI) UpdateCacheConfig(w http.ResponseWriter, r *http.Request) {
	var config CacheConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}

	// Validate configuration
	if err := api.validateCacheConfig(&config); err != nil {
		api.writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	// Update configuration (this is a simplified implementation)
	// In production, you might want to restart cache components
	api.cacheManager.config = &config

	api.logger.Infow("Cache configuration updated")
	api.writeJSON(w, http.StatusOK, config)
}

// ListCacheEntries handles GET /cache/entries
func (api *CacheAPI) ListCacheEntries(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	tenantID := r.URL.Query().Get("tenant_id")
	model := r.URL.Query().Get("model")
	strategy := r.URL.Query().Get("strategy")

	limit := 50 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	// Get cache entries
	entries := api.getCacheEntries(tenantID, model, strategy, limit, offset)

	response := ListCacheEntriesResponse{
		Entries: entries,
		Pagination: PaginationInfo{
			Limit:  limit,
			Offset: offset,
			Count:  len(entries),
		},
	}

	api.writeJSON(w, http.StatusOK, response)
}

// GetCacheEntry handles GET /cache/entries/{key}
func (api *CacheAPI) GetCacheEntry(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	api.cacheManager.mutex.RLock()
	entry, exists := api.cacheManager.memoryCache[key]
	api.cacheManager.mutex.RUnlock()

	if !exists {
		api.writeError(w, http.StatusNotFound, "entry_not_found", "Cache entry not found")
		return
	}

	api.writeJSON(w, http.StatusOK, entry)
}

// DeleteCacheEntry handles DELETE /cache/entries/{key}
func (api *CacheAPI) DeleteCacheEntry(w http.ResponseWriter, r *http.Request) {
	key := mux.Vars(r)["key"]

	api.cacheManager.mutex.Lock()
	_, exists := api.cacheManager.memoryCache[key]
	if exists {
		delete(api.cacheManager.memoryCache, key)
		api.cacheManager.removeFromAccessOrder(key)
	}
	api.cacheManager.mutex.Unlock()

	if !exists {
		api.writeError(w, http.StatusNotFound, "entry_not_found", "Cache entry not found")
		return
	}

	response := map[string]interface{}{
		"message":   "Cache entry deleted successfully",
		"key":       key,
		"timestamp": time.Now(),
	}

	api.writeJSON(w, http.StatusOK, response)
}

// GetAdaptiveState handles GET /cache/adaptive/state
func (api *CacheAPI) GetAdaptiveState(w http.ResponseWriter, r *http.Request) {
	if api.cacheManager.adaptiveState == nil {
		api.writeError(w, http.StatusNotFound, "adaptive_disabled", "Adaptive caching is not enabled")
		return
	}

	api.cacheManager.adaptiveState.mutex.RLock()
	defer api.cacheManager.adaptiveState.mutex.RUnlock()

	// Create a copy to avoid race conditions
	state := AdaptiveStateResponse{
		CurrentStrategy:  api.cacheManager.adaptiveState.CurrentStrategy,
		LastEvaluation:   api.cacheManager.adaptiveState.LastEvaluation,
		SampleCount:      api.cacheManager.adaptiveState.SampleCount,
		StrategyHistory:  api.cacheManager.adaptiveState.StrategyHistory,
		PatternDetection: api.cacheManager.adaptiveState.PatternDetection,
	}

	api.writeJSON(w, http.StatusOK, state)
}

// SetAdaptiveStrategy handles POST /cache/adaptive/strategy
func (api *CacheAPI) SetAdaptiveStrategy(w http.ResponseWriter, r *http.Request) {
	if api.cacheManager.adaptiveState == nil {
		api.writeError(w, http.StatusBadRequest, "adaptive_disabled", "Adaptive caching is not enabled")
		return
	}

	var req SetStrategyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}

	// Validate strategy
	if !api.isValidStrategy(req.Strategy) {
		api.writeError(w, http.StatusBadRequest, "invalid_strategy", "Invalid cache strategy")
		return
	}

	api.cacheManager.adaptiveState.mutex.Lock()
	oldStrategy := api.cacheManager.adaptiveState.CurrentStrategy
	api.cacheManager.adaptiveState.CurrentStrategy = req.Strategy

	// Record strategy change
	change := StrategyChange{
		Timestamp:    time.Now(),
		FromStrategy: oldStrategy,
		ToStrategy:   req.Strategy,
		Reason:       "manual_override",
		HitRate:      api.cacheManager.GetStats().HitRate,
	}
	api.cacheManager.adaptiveState.StrategyHistory = append(api.cacheManager.adaptiveState.StrategyHistory, change)
	api.cacheManager.adaptiveState.mutex.Unlock()

	api.logger.Infow("Cache strategy manually changed",
		"from", oldStrategy,
		"to", req.Strategy,
		"reason", req.Reason)

	response := map[string]interface{}{
		"message":      "Strategy updated successfully",
		"old_strategy": oldStrategy,
		"new_strategy": req.Strategy,
		"reason":       req.Reason,
		"timestamp":    time.Now(),
	}

	api.writeJSON(w, http.StatusOK, response)
}

// WarmCache handles POST /cache/warm
func (api *CacheAPI) WarmCache(w http.ResponseWriter, r *http.Request) {
	var req WarmCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}

	// This is a placeholder for cache warming implementation
	// In production, you would implement actual cache warming logic

	response := map[string]interface{}{
		"message":   "Cache warming initiated",
		"patterns":  req.Patterns,
		"tenant_id": req.TenantID,
		"timestamp": time.Now(),
	}

	api.writeJSON(w, http.StatusAccepted, response)
}

// PreloadCache handles POST /cache/preload
func (api *CacheAPI) PreloadCache(w http.ResponseWriter, r *http.Request) {
	var req PreloadCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON payload")
		return
	}

	// This is a placeholder for cache preloading implementation
	// In production, you would implement actual preloading logic

	response := map[string]interface{}{
		"message":   "Cache preloading initiated",
		"entries":   len(req.Entries),
		"timestamp": time.Now(),
	}

	api.writeJSON(w, http.StatusAccepted, response)
}

// GetCacheAnalysis handles GET /cache/analysis
func (api *CacheAPI) GetCacheAnalysis(w http.ResponseWriter, r *http.Request) {
	analysis := api.generateCacheAnalysis()
	api.writeJSON(w, http.StatusOK, analysis)
}

// GetCacheInsights handles GET /cache/insights
func (api *CacheAPI) GetCacheInsights(w http.ResponseWriter, r *http.Request) {
	insights := api.generateCacheInsights()
	api.writeJSON(w, http.StatusOK, insights)
}

// Request/Response types

type ListCacheEntriesResponse struct {
	Entries    []*CacheEntryInfo `json:"entries"`
	Pagination PaginationInfo    `json:"pagination"`
}

type CacheEntryInfo struct {
	Key         string    `json:"key"`
	Hash        string    `json:"hash"`
	Model       string    `json:"model"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	AccessCount int64     `json:"access_count"`
	LastAccess  time.Time `json:"last_access"`
	TenantID    string    `json:"tenant_id,omitempty"`
	Compressed  bool      `json:"compressed"`
	Size        int64     `json:"size"`
}

type PaginationInfo struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Count  int `json:"count"`
	Total  int `json:"total,omitempty"`
}

type AdaptiveStateResponse struct {
	CurrentStrategy  CacheStrategy    `json:"current_strategy"`
	LastEvaluation   time.Time        `json:"last_evaluation"`
	SampleCount      int              `json:"sample_count"`
	StrategyHistory  []StrategyChange `json:"strategy_history"`
	PatternDetection *PatternData     `json:"pattern_detection,omitempty"`
}

type SetStrategyRequest struct {
	Strategy CacheStrategy `json:"strategy"`
	Reason   string        `json:"reason,omitempty"`
}

type WarmCacheRequest struct {
	Patterns []string `json:"patterns"`
	TenantID string   `json:"tenant_id,omitempty"`
	Models   []string `json:"models,omitempty"`
}

type PreloadCacheRequest struct {
	Entries  []PreloadEntry `json:"entries"`
	TenantID string         `json:"tenant_id,omitempty"`
}

type PreloadEntry struct {
	Model    string   `json:"model"`
	Messages []string `json:"messages"`
	TTL      string   `json:"ttl,omitempty"`
}

type CacheAnalysis struct {
	Summary         AnalysisSummary            `json:"summary"`
	Performance     PerformanceAnalysis        `json:"performance"`
	Usage           UsageAnalysis              `json:"usage"`
	Recommendations []Recommendation           `json:"recommendations"`
	TenantAnalysis  map[string]*TenantAnalysis `json:"tenant_analysis,omitempty"`
	GeneratedAt     time.Time                  `json:"generated_at"`
}

type AnalysisSummary struct {
	TotalEntries     int64    `json:"total_entries"`
	MemoryUsageMB    float64  `json:"memory_usage_mb"`
	HitRate          float64  `json:"hit_rate"`
	AverageLatency   string   `json:"average_latency"`
	ActiveStrategies []string `json:"active_strategies"`
}

type PerformanceAnalysis struct {
	HitRateByStrategy map[string]float64 `json:"hit_rate_by_strategy"`
	LatencyByStrategy map[string]string  `json:"latency_by_strategy"`
	CacheEfficiency   float64            `json:"cache_efficiency"`
	EvictionRate      float64            `json:"eviction_rate"`
	CompressionRatio  float64            `json:"compression_ratio"`
}

type UsageAnalysis struct {
	TopModels             []ModelUsage     `json:"top_models"`
	TimeDistribution      map[string]int64 `json:"time_distribution"`
	TenantDistribution    map[string]int64 `json:"tenant_distribution"`
	QuerySizeDistribution []int            `json:"query_size_distribution"`
}

type ModelUsage struct {
	Model      string  `json:"model"`
	Requests   int64   `json:"requests"`
	HitRate    float64 `json:"hit_rate"`
	AvgLatency string  `json:"avg_latency"`
}

type TenantAnalysis struct {
	TenantID        string   `json:"tenant_id"`
	CacheEntries    int64    `json:"cache_entries"`
	HitRate         float64  `json:"hit_rate"`
	MemoryUsageMB   float64  `json:"memory_usage_mb"`
	TopModels       []string `json:"top_models"`
	Recommendations []string `json:"recommendations"`
}

type Recommendation struct {
	Type        string `json:"type"`
	Priority    string `json:"priority"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Impact      string `json:"impact"`
}

type CacheInsights struct {
	Trends      TrendAnalysis   `json:"trends"`
	Patterns    PatternAnalysis `json:"patterns"`
	Anomalies   []Anomaly       `json:"anomalies"`
	Predictions PredictionData  `json:"predictions"`
	GeneratedAt time.Time       `json:"generated_at"`
}

type TrendAnalysis struct {
	HitRateTrend     string           `json:"hit_rate_trend"`
	UsageTrend       string           `json:"usage_trend"`
	PerformanceTrend string           `json:"performance_trend"`
	HourlyPatterns   map[string]int64 `json:"hourly_patterns"`
}

type PatternAnalysis struct {
	CommonQueries    []string               `json:"common_queries"`
	SeasonalPatterns map[string]int64       `json:"seasonal_patterns"`
	UserBehavior     map[string]interface{} `json:"user_behavior"`
}

type Anomaly struct {
	Type        string    `json:"type"`
	Severity    string    `json:"severity"`
	Description string    `json:"description"`
	DetectedAt  time.Time `json:"detected_at"`
	Value       float64   `json:"value"`
	Expected    float64   `json:"expected"`
}

type PredictionData struct {
	ExpectedHitRate     float64          `json:"expected_hit_rate"`
	ExpectedLoad        float64          `json:"expected_load"`
	CapacityForecasting map[string]int64 `json:"capacity_forecasting"`
	Recommendations     []string         `json:"recommendations"`
}

// Helper methods

func (api *CacheAPI) getCacheEntries(tenantID, model, strategy string, limit, offset int) []*CacheEntryInfo {
	api.cacheManager.mutex.RLock()
	defer api.cacheManager.mutex.RUnlock()

	var entries []*CacheEntryInfo
	count := 0

	for _, entry := range api.cacheManager.memoryCache {
		// Apply filters
		if tenantID != "" && entry.TenantID != tenantID {
			continue
		}
		if model != "" && entry.Request.Model != model {
			continue
		}

		// Skip entries before offset
		if count < offset {
			count++
			continue
		}

		// Stop when limit reached
		if len(entries) >= limit {
			break
		}

		// Convert to response format
		info := &CacheEntryInfo{
			Key:         entry.Key,
			Hash:        entry.Hash,
			Model:       entry.Request.Model,
			CreatedAt:   entry.CreatedAt,
			ExpiresAt:   entry.ExpiresAt,
			AccessCount: entry.AccessCount,
			LastAccess:  entry.LastAccess,
			TenantID:    entry.TenantID,
			Compressed:  entry.Compressed,
			Size:        entry.OriginalSize,
		}

		entries = append(entries, info)
	}

	return entries
}

func (api *CacheAPI) validateCacheConfig(config *CacheConfig) error {
	if config.MaxEntries <= 0 {
		return fmt.Errorf("max_entries must be positive")
	}
	if config.MaxMemoryMB <= 0 {
		return fmt.Errorf("max_memory_mb must be positive")
	}
	if config.DefaultTTL <= 0 {
		return fmt.Errorf("default_ttl must be positive")
	}
	return nil
}

func (api *CacheAPI) isValidStrategy(strategy CacheStrategy) bool {
	validStrategies := []CacheStrategy{
		StrategyNone, StrategyExact, StrategySemantic,
		StrategyToken, StrategyHybrid, StrategyAdaptive,
	}

	for _, valid := range validStrategies {
		if strategy == valid {
			return true
		}
	}
	return false
}

func (api *CacheAPI) generateCacheAnalysis() *CacheAnalysis {
	stats := api.cacheManager.GetStats()

	// Generate summary
	summary := AnalysisSummary{
		TotalEntries:     stats.TotalEntries,
		MemoryUsageMB:    stats.MemoryUsageMB,
		HitRate:          stats.HitRate,
		AverageLatency:   stats.AverageLatency.String(),
		ActiveStrategies: []string{string(api.cacheManager.getActiveStrategy())},
	}

	// Generate performance analysis
	performance := PerformanceAnalysis{
		HitRateByStrategy: map[string]float64{
			"exact":    float64(stats.ExactHits) / float64(stats.Hits+stats.Misses),
			"semantic": float64(stats.SemanticHits) / float64(stats.Hits+stats.Misses),
			"token":    float64(stats.TokenHits) / float64(stats.Hits+stats.Misses),
		},
		CacheEfficiency:  stats.HitRate,
		EvictionRate:     float64(stats.Evictions) / float64(stats.Stores),
		CompressionRatio: 0.7, // Placeholder
	}

	// Generate usage analysis
	usage := api.generateUsageAnalysis()

	// Generate recommendations
	recommendations := api.generateRecommendations(stats)

	return &CacheAnalysis{
		Summary:         summary,
		Performance:     performance,
		Usage:           usage,
		Recommendations: recommendations,
		TenantAnalysis:  api.generateTenantAnalysis(stats),
		GeneratedAt:     time.Now(),
	}
}

func (api *CacheAPI) generateUsageAnalysis() UsageAnalysis {
	api.cacheManager.mutex.RLock()
	defer api.cacheManager.mutex.RUnlock()

	modelCount := make(map[string]int64)
	timeCount := make(map[string]int64)
	tenantCount := make(map[string]int64)

	for _, entry := range api.cacheManager.memoryCache {
		modelCount[entry.Request.Model]++

		hour := entry.CreatedAt.Format("15")
		timeCount[hour]++

		if entry.TenantID != "" {
			tenantCount[entry.TenantID]++
		}
	}

	// Convert to top models format
	var topModels []ModelUsage
	for model, count := range modelCount {
		topModels = append(topModels, ModelUsage{
			Model:    model,
			Requests: count,
			HitRate:  0.8, // Placeholder
		})
	}

	return UsageAnalysis{
		TopModels:             topModels,
		TimeDistribution:      timeCount,
		TenantDistribution:    tenantCount,
		QuerySizeDistribution: []int{100, 200, 500, 1000}, // Placeholder
	}
}

func (api *CacheAPI) generateRecommendations(stats *CacheStats) []Recommendation {
	var recommendations []Recommendation

	if stats.HitRate < 0.5 {
		recommendations = append(recommendations, Recommendation{
			Type:        "performance",
			Priority:    "high",
			Title:       "Low Cache Hit Rate",
			Description: "Consider enabling semantic or hybrid caching to improve hit rates",
			Impact:      "Could improve performance by 30-50%",
		})
	}

	if stats.MemoryUsageMB > 400 {
		recommendations = append(recommendations, Recommendation{
			Type:        "capacity",
			Priority:    "medium",
			Title:       "High Memory Usage",
			Description: "Consider enabling compression or reducing TTL values",
			Impact:      "Could reduce memory usage by 20-40%",
		})
	}

	return recommendations
}

func (api *CacheAPI) generateTenantAnalysis(stats *CacheStats) map[string]*TenantAnalysis {
	tenantAnalysis := make(map[string]*TenantAnalysis)

	for tenantID, tenantStats := range stats.TenantStats {
		analysis := &TenantAnalysis{
			TenantID:        tenantID,
			CacheEntries:    tenantStats.Entries,
			HitRate:         tenantStats.HitRate,
			MemoryUsageMB:   tenantStats.MemoryMB,
			TopModels:       []string{sdkOgem.ModelGPT4, sdkOgem.ModelGPT35Turbo}, // Placeholder
			Recommendations: []string{"Enable semantic caching for better hit rates"},
		}
		tenantAnalysis[tenantID] = analysis
	}

	return tenantAnalysis
}

func (api *CacheAPI) generateCacheInsights() *CacheInsights {
	return &CacheInsights{
		Trends: TrendAnalysis{
			HitRateTrend:     "increasing",
			UsageTrend:       "stable",
			PerformanceTrend: "improving",
			HourlyPatterns:   map[string]int64{"09": 100, "14": 150, "20": 80},
		},
		Patterns: PatternAnalysis{
			CommonQueries:    []string{"What is...", "How to...", "Explain..."},
			SeasonalPatterns: map[string]int64{"morning": 200, "afternoon": 300, "evening": 150},
			UserBehavior:     map[string]interface{}{"avg_session_length": "15m", "repeat_queries": 0.3},
		},
		Anomalies: []Anomaly{},
		Predictions: PredictionData{
			ExpectedHitRate:     0.75,
			ExpectedLoad:        1.2,
			CapacityForecasting: map[string]int64{"next_hour": 200, "next_day": 4800},
			Recommendations:     []string{"Consider scaling cache capacity during peak hours"},
		},
		GeneratedAt: time.Now(),
	}
}

func (api *CacheAPI) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		api.logger.Errorw("Failed to encode JSON response", "error", err)
	}
}

func (api *CacheAPI) writeError(w http.ResponseWriter, status int, errorType, message string) {
	errorResponse := map[string]interface{}{
		"error": map[string]interface{}{
			"type":    errorType,
			"message": message,
			"code":    status,
		},
	}

	api.writeJSON(w, status, errorResponse)
}
