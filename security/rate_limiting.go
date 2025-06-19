package security

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RateLimitConfig configures advanced rate limiting
type RateLimitConfig struct {
	// Enable rate limiting
	Enabled bool `yaml:"enabled"`
	
	// Global rate limits
	GlobalLimits []RateLimit `yaml:"global_limits,omitempty"`
	
	// Per-user rate limits
	UserLimits []RateLimit `yaml:"user_limits,omitempty"`
	
	// Per-endpoint rate limits
	EndpointLimits map[string][]RateLimit `yaml:"endpoint_limits,omitempty"`
	
	// Per-model rate limits
	ModelLimits map[string][]RateLimit `yaml:"model_limits,omitempty"`
	
	// Burst allowance configuration
	BurstConfig *BurstConfig `yaml:"burst_config,omitempty"`
	
	// Sliding window configuration
	SlidingWindow *SlidingWindowConfig `yaml:"sliding_window,omitempty"`
	
	// Adaptive rate limiting
	AdaptiveConfig *AdaptiveRateLimitConfig `yaml:"adaptive_config,omitempty"`
	
	// Whitelist/blacklist configuration
	AccessControl *AccessControlConfig `yaml:"access_control,omitempty"`
}

// RateLimit defines a rate limiting rule
type RateLimit struct {
	// Limit type
	Type RateLimitType `yaml:"type"`
	
	// Maximum requests allowed
	Limit int64 `yaml:"limit"`
	
	// Time window
	Window time.Duration `yaml:"window"`
	
	// Priority for conflicting rules
	Priority int `yaml:"priority"`
	
	// Action to take when limit exceeded
	Action RateLimitAction `yaml:"action"`
	
	// Custom error message
	ErrorMessage string `yaml:"error_message,omitempty"`
}

// RateLimitType represents different types of rate limits
type RateLimitType string

const (
	RateLimitTypeRequests RateLimitType = "requests"
	RateLimitTypeTokens   RateLimitType = "tokens"
	RateLimitTypeCost     RateLimitType = "cost"
	RateLimitTypeConcurrent RateLimitType = "concurrent"
)

// RateLimitAction represents actions to take when rate limit is exceeded
type RateLimitAction string

const (
	RateLimitActionBlock   RateLimitAction = "block"
	RateLimitActionDelay   RateLimitAction = "delay"
	RateLimitActionQueue   RateLimitAction = "queue"
	RateLimitActionWarn    RateLimitAction = "warn"
)

// BurstConfig configures burst allowance
type BurstConfig struct {
	// Enable burst allowance
	Enabled bool `yaml:"enabled"`
	
	// Maximum burst size (multiplier of base limit)
	BurstMultiplier float64 `yaml:"burst_multiplier"`
	
	// Burst recovery rate (tokens per second)
	RecoveryRate float64 `yaml:"recovery_rate"`
	
	// Minimum time between bursts
	CooldownPeriod time.Duration `yaml:"cooldown_period"`
}

// SlidingWindowConfig configures sliding window rate limiting
type SlidingWindowConfig struct {
	// Enable sliding window
	Enabled bool `yaml:"enabled"`
	
	// Window size for sliding calculations
	WindowSize time.Duration `yaml:"window_size"`
	
	// Number of sub-windows for precision
	SubWindows int `yaml:"sub_windows"`
}

// AdaptiveRateLimitConfig configures adaptive rate limiting
type AdaptiveRateLimitConfig struct {
	// Enable adaptive rate limiting
	Enabled bool `yaml:"enabled"`
	
	// Factor to adjust limits based on system load
	LoadFactor float64 `yaml:"load_factor"`
	
	// Factor to adjust limits based on error rates
	ErrorFactor float64 `yaml:"error_factor"`
	
	// Minimum adjustment interval
	AdjustmentInterval time.Duration `yaml:"adjustment_interval"`
	
	// Maximum adjustment percentage
	MaxAdjustment float64 `yaml:"max_adjustment"`
}

// AccessControlConfig configures IP-based access control
type AccessControlConfig struct {
	// Whitelisted IP addresses/ranges
	Whitelist []string `yaml:"whitelist,omitempty"`
	
	// Blacklisted IP addresses/ranges
	Blacklist []string `yaml:"blacklist,omitempty"`
	
	// Trusted proxy headers
	TrustedProxies []string `yaml:"trusted_proxies,omitempty"`
	
	// Geolocation restrictions
	GeoRestrictions *GeoRestrictions `yaml:"geo_restrictions,omitempty"`
}

// GeoRestrictions configures geographic access restrictions
type GeoRestrictions struct {
	// Allowed countries (ISO 3166-1 alpha-2)
	AllowedCountries []string `yaml:"allowed_countries,omitempty"`
	
	// Blocked countries
	BlockedCountries []string `yaml:"blocked_countries,omitempty"`
	
	// Default action for unknown locations
	DefaultAction string `yaml:"default_action"` // "allow" or "block"
}

// RateLimitResult represents the result of a rate limit check
type RateLimitResult struct {
	// Whether the request is allowed
	Allowed bool
	
	// Rate limit that was applied
	AppliedLimit *RateLimit
	
	// Current usage count
	Current int64
	
	// Time until reset
	ResetTime time.Time
	
	// Remaining requests
	Remaining int64
	
	// Action to take
	Action RateLimitAction
	
	// Error message if blocked
	ErrorMessage string
	
	// Delay duration if action is delay
	DelayDuration time.Duration
}

// RateLimitState tracks state for a specific rate limit
type RateLimitState struct {
	// Current count
	Count int64
	
	// Window start time
	WindowStart time.Time
	
	// Last access time
	LastAccess time.Time
	
	// Burst tokens available
	BurstTokens float64
	
	// Historical data for sliding window
	History []int64
	
	// Concurrent requests counter
	Concurrent int64
	
	// Total cost accumulated
	TotalCost float64
	
	// Mutex for thread safety
	mutex sync.RWMutex
}

// AdvancedRateLimiter provides sophisticated rate limiting capabilities
type AdvancedRateLimiter struct {
	config *RateLimitConfig
	states map[string]*RateLimitState // key -> state
	logger *zap.SugaredLogger
	mutex  sync.RWMutex
}

// NewAdvancedRateLimiter creates a new advanced rate limiter
func NewAdvancedRateLimiter(config *RateLimitConfig, logger *zap.SugaredLogger) *AdvancedRateLimiter {
	if config == nil {
		config = DefaultRateLimitConfig()
	}

	limiter := &AdvancedRateLimiter{
		config: config,
		states: make(map[string]*RateLimitState),
		logger: logger,
	}

	// Start cleanup goroutine
	go limiter.cleanupExpiredStates()

	return limiter
}

// DefaultRateLimitConfig returns default rate limiting configuration
func DefaultRateLimitConfig() *RateLimitConfig {
	return &RateLimitConfig{
		Enabled: true,
		GlobalLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  1000,
				Window: time.Hour,
				Action: RateLimitActionBlock,
			},
		},
		UserLimits: []RateLimit{
			{
				Type:   RateLimitTypeRequests,
				Limit:  100,
				Window: time.Hour,
				Action: RateLimitActionBlock,
			},
		},
		BurstConfig: &BurstConfig{
			Enabled:         true,
			BurstMultiplier: 2.0,
			RecoveryRate:    1.0,
			CooldownPeriod:  time.Minute,
		},
		SlidingWindow: &SlidingWindowConfig{
			Enabled:    true,
			WindowSize: time.Hour,
			SubWindows: 60,
		},
		AdaptiveConfig: &AdaptiveRateLimitConfig{
			Enabled:            false,
			LoadFactor:         0.8,
			ErrorFactor:        0.5,
			AdjustmentInterval: 5 * time.Minute,
			MaxAdjustment:      0.5,
		},
	}
}

// CheckRateLimit checks if a request should be rate limited
func (r *AdvancedRateLimiter) CheckRateLimit(ctx context.Context, userID, endpoint, model string, tokenCount int64, cost float64) (*RateLimitResult, error) {
	if !r.config.Enabled {
		return &RateLimitResult{Allowed: true}, nil
	}

	// Check access control first
	if !r.checkAccessControl(ctx) {
		return &RateLimitResult{
			Allowed:      false,
			Action:       RateLimitActionBlock,
			ErrorMessage: "Access denied by IP restrictions",
		}, nil
	}

	// Collect all applicable rate limits
	limits := r.getApplicableLimits(userID, endpoint, model)
	if len(limits) == 0 {
		return &RateLimitResult{Allowed: true}, nil
	}

	// Check each limit
	for _, limit := range limits {
		key := r.generateKey(limit, userID, endpoint, model)
		result := r.checkLimit(key, limit, tokenCount, cost)
		
		if !result.Allowed {
			r.logger.Infow("Rate limit exceeded",
				"user_id", userID,
				"endpoint", endpoint,
				"model", model,
				"limit_type", limit.Type,
				"current", result.Current,
				"limit", limit.Limit)
			
			return result, nil
		}
	}

	return &RateLimitResult{Allowed: true}, nil
}

// checkLimit checks a specific rate limit
func (r *AdvancedRateLimiter) checkLimit(key string, limit RateLimit, tokenCount int64, cost float64) *RateLimitResult {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	state, exists := r.states[key]
	if !exists {
		state = &RateLimitState{
			WindowStart: time.Now(),
			LastAccess:  time.Now(),
			BurstTokens: float64(limit.Limit) * r.config.BurstConfig.BurstMultiplier,
		}
		if r.config.SlidingWindow.Enabled {
			state.History = make([]int64, r.config.SlidingWindow.SubWindows)
		}
		r.states[key] = state
	}

	state.mutex.Lock()
	defer state.mutex.Unlock()

	now := time.Now()
	
	// Update state based on limit type
	switch limit.Type {
	case RateLimitTypeRequests:
		return r.checkRequestsLimit(state, limit, now, 1)
	case RateLimitTypeTokens:
		return r.checkTokensLimit(state, limit, now, tokenCount)
	case RateLimitTypeCost:
		return r.checkCostLimit(state, limit, now, cost)
	case RateLimitTypeConcurrent:
		return r.checkConcurrentLimit(state, limit, now)
	default:
		return &RateLimitResult{Allowed: true}
	}
}

// checkRequestsLimit checks request-based rate limit
func (r *AdvancedRateLimiter) checkRequestsLimit(state *RateLimitState, limit RateLimit, now time.Time, increment int64) *RateLimitResult {
	// Reset window if expired
	if now.Sub(state.WindowStart) >= limit.Window {
		state.Count = 0
		state.WindowStart = now
		if r.config.SlidingWindow.Enabled {
			state.History = make([]int64, r.config.SlidingWindow.SubWindows)
		}
	}

	// Update sliding window if enabled
	if r.config.SlidingWindow.Enabled {
		r.updateSlidingWindow(state, now, increment, limit.Window)
	}

	// Check burst allowance
	if r.config.BurstConfig.Enabled {
		if state.BurstTokens >= float64(increment) {
			state.BurstTokens -= float64(increment)
			state.Count += increment
			state.LastAccess = now
			return &RateLimitResult{
				Allowed:   true,
				Current:   state.Count,
				Remaining: limit.Limit - state.Count,
				ResetTime: state.WindowStart.Add(limit.Window),
			}
		}
	}

	// Check standard limit
	if state.Count+increment > limit.Limit {
		return &RateLimitResult{
			Allowed:       false,
			AppliedLimit:  &limit,
			Current:       state.Count,
			Remaining:     0,
			ResetTime:     state.WindowStart.Add(limit.Window),
			Action:        limit.Action,
			ErrorMessage:  r.getErrorMessage(limit),
			DelayDuration: r.calculateDelay(state, limit, now),
		}
	}

	state.Count += increment
	state.LastAccess = now

	return &RateLimitResult{
		Allowed:   true,
		Current:   state.Count,
		Remaining: limit.Limit - state.Count,
		ResetTime: state.WindowStart.Add(limit.Window),
	}
}

// checkTokensLimit checks token-based rate limit
func (r *AdvancedRateLimiter) checkTokensLimit(state *RateLimitState, limit RateLimit, now time.Time, tokenCount int64) *RateLimitResult {
	// Similar to requests but counting tokens
	if now.Sub(state.WindowStart) >= limit.Window {
		state.Count = 0
		state.WindowStart = now
	}

	if state.Count+tokenCount > limit.Limit {
		return &RateLimitResult{
			Allowed:      false,
			AppliedLimit: &limit,
			Current:      state.Count,
			Remaining:    0,
			ResetTime:    state.WindowStart.Add(limit.Window),
			Action:       limit.Action,
			ErrorMessage: r.getErrorMessage(limit),
		}
	}

	state.Count += tokenCount
	state.LastAccess = now

	return &RateLimitResult{
		Allowed:   true,
		Current:   state.Count,
		Remaining: limit.Limit - state.Count,
		ResetTime: state.WindowStart.Add(limit.Window),
	}
}

// checkCostLimit checks cost-based rate limit
func (r *AdvancedRateLimiter) checkCostLimit(state *RateLimitState, limit RateLimit, now time.Time, cost float64) *RateLimitResult {
	if now.Sub(state.WindowStart) >= limit.Window {
		state.TotalCost = 0
		state.WindowStart = now
	}

	costLimit := float64(limit.Limit) / 1000.0 // Convert to dollars
	if state.TotalCost+cost > costLimit {
		return &RateLimitResult{
			Allowed:      false,
			AppliedLimit: &limit,
			Current:      int64(state.TotalCost * 1000),
			Remaining:    0,
			ResetTime:    state.WindowStart.Add(limit.Window),
			Action:       limit.Action,
			ErrorMessage: r.getErrorMessage(limit),
		}
	}

	state.TotalCost += cost
	state.LastAccess = now

	return &RateLimitResult{
		Allowed:   true,
		Current:   int64(state.TotalCost * 1000),
		Remaining: int64((costLimit - state.TotalCost) * 1000),
		ResetTime: state.WindowStart.Add(limit.Window),
	}
}

// checkConcurrentLimit checks concurrent request limit
func (r *AdvancedRateLimiter) checkConcurrentLimit(state *RateLimitState, limit RateLimit, now time.Time) *RateLimitResult {
	if state.Concurrent >= limit.Limit {
		return &RateLimitResult{
			Allowed:      false,
			AppliedLimit: &limit,
			Current:      state.Concurrent,
			Remaining:    0,
			Action:       limit.Action,
			ErrorMessage: r.getErrorMessage(limit),
		}
	}

	state.Concurrent++
	state.LastAccess = now

	return &RateLimitResult{
		Allowed:   true,
		Current:   state.Concurrent,
		Remaining: limit.Limit - state.Concurrent,
	}
}

// ReleaseResource decrements concurrent request counter
func (r *AdvancedRateLimiter) ReleaseResource(userID, endpoint, model string) {
	concurrentLimit := RateLimit{Type: RateLimitTypeConcurrent}
	key := r.generateKey(concurrentLimit, userID, endpoint, model)

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if state, exists := r.states[key]; exists {
		state.mutex.Lock()
		if state.Concurrent > 0 {
			state.Concurrent--
		}
		state.mutex.Unlock()
	}
}

// updateSlidingWindow updates sliding window counters
func (r *AdvancedRateLimiter) updateSlidingWindow(state *RateLimitState, now time.Time, increment int64, window time.Duration) {
	subWindowDuration := window / time.Duration(len(state.History))
	subWindowIndex := int(now.Sub(state.WindowStart) / subWindowDuration)
	
	if subWindowIndex >= len(state.History) {
		// Shift history
		shift := subWindowIndex - len(state.History) + 1
		for i := 0; i < len(state.History)-shift; i++ {
			state.History[i] = state.History[i+shift]
		}
		for i := len(state.History) - shift; i < len(state.History); i++ {
			state.History[i] = 0
		}
		subWindowIndex = len(state.History) - 1
		state.WindowStart = now.Add(-time.Duration(subWindowIndex) * subWindowDuration)
	}
	
	state.History[subWindowIndex] += increment
}

// getApplicableLimits returns all rate limits applicable to the request
func (r *AdvancedRateLimiter) getApplicableLimits(userID, endpoint, model string) []RateLimit {
	var limits []RateLimit

	// Global limits
	limits = append(limits, r.config.GlobalLimits...)

	// User limits
	if userID != "" {
		limits = append(limits, r.config.UserLimits...)
	}

	// Endpoint limits
	if endpointLimits, exists := r.config.EndpointLimits[endpoint]; exists {
		limits = append(limits, endpointLimits...)
	}

	// Model limits
	if modelLimits, exists := r.config.ModelLimits[model]; exists {
		limits = append(limits, modelLimits...)
	}

	return limits
}

// generateKey generates a unique key for rate limit tracking
func (r *AdvancedRateLimiter) generateKey(limit RateLimit, userID, endpoint, model string) string {
	var keyParts []string

	switch limit.Type {
	case RateLimitTypeRequests, RateLimitTypeTokens, RateLimitTypeCost:
		keyParts = append(keyParts, string(limit.Type))
		if userID != "" {
			keyParts = append(keyParts, "user", userID)
		}
		if endpoint != "" {
			keyParts = append(keyParts, "endpoint", endpoint)
		}
		if model != "" {
			keyParts = append(keyParts, "model", model)
		}
	case RateLimitTypeConcurrent:
		keyParts = []string{"concurrent", userID, endpoint, model}
	}

	return fmt.Sprintf("ratelimit:%s:%d", strings.Join(keyParts, ":"), limit.Window.Nanoseconds())
}

// checkAccessControl checks IP-based access control
func (r *AdvancedRateLimiter) checkAccessControl(ctx context.Context) bool {
	if r.config.AccessControl == nil {
		return true
	}

	// Extract IP from context
	ip := ""
	if clientIP := ctx.Value("client_ip"); clientIP != nil {
		if ipStr, ok := clientIP.(string); ok {
			ip = ipStr
		}
	}

	if ip == "" {
		return true // Allow if no IP available
	}

	// Check blacklist first
	for _, blockedIP := range r.config.AccessControl.Blacklist {
		if r.matchesIPPattern(ip, blockedIP) {
			return false
		}
	}

	// Check whitelist if configured
	if len(r.config.AccessControl.Whitelist) > 0 {
		for _, allowedIP := range r.config.AccessControl.Whitelist {
			if r.matchesIPPattern(ip, allowedIP) {
				return true
			}
		}
		return false // Not in whitelist
	}

	return true
}

// matchesIPPattern checks if IP matches a pattern (simplified implementation)
func (r *AdvancedRateLimiter) matchesIPPattern(ip, pattern string) bool {
	// Simplified pattern matching - in production, use proper CIDR matching
	return ip == pattern || pattern == "*"
}

// getErrorMessage returns appropriate error message for rate limit
func (r *AdvancedRateLimiter) getErrorMessage(limit RateLimit) string {
	if limit.ErrorMessage != "" {
		return limit.ErrorMessage
	}

	switch limit.Type {
	case RateLimitTypeRequests:
		return fmt.Sprintf("Request rate limit exceeded: %d requests per %v", limit.Limit, limit.Window)
	case RateLimitTypeTokens:
		return fmt.Sprintf("Token rate limit exceeded: %d tokens per %v", limit.Limit, limit.Window)
	case RateLimitTypeCost:
		return fmt.Sprintf("Cost rate limit exceeded: $%.3f per %v", float64(limit.Limit)/1000.0, limit.Window)
	case RateLimitTypeConcurrent:
		return fmt.Sprintf("Concurrent request limit exceeded: %d concurrent requests", limit.Limit)
	default:
		return "Rate limit exceeded"
	}
}

// calculateDelay calculates delay duration for delay action
func (r *AdvancedRateLimiter) calculateDelay(state *RateLimitState, limit RateLimit, now time.Time) time.Duration {
	if limit.Action != RateLimitActionDelay {
		return 0
	}

	// Calculate delay based on how much the limit was exceeded
	excess := float64(state.Count) / float64(limit.Limit)
	baseDelay := time.Second
	
	return time.Duration(float64(baseDelay) * excess)
}

// cleanupExpiredStates removes expired rate limit states
func (r *AdvancedRateLimiter) cleanupExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		r.mutex.Lock()
		now := time.Now()
		
		for key, state := range r.states {
			state.mutex.RLock()
			expired := now.Sub(state.LastAccess) > time.Hour
			state.mutex.RUnlock()
			
			if expired {
				delete(r.states, key)
			}
		}
		r.mutex.Unlock()
	}
}

// GetRateLimitStats returns rate limiting statistics
func (r *AdvancedRateLimiter) GetRateLimitStats() map[string]interface{} {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	return map[string]interface{}{
		"enabled":            r.config.Enabled,
		"active_states":      len(r.states),
		"global_limits":      len(r.config.GlobalLimits),
		"user_limits":        len(r.config.UserLimits),
		"endpoint_limits":    len(r.config.EndpointLimits),
		"model_limits":       len(r.config.ModelLimits),
		"burst_enabled":      r.config.BurstConfig.Enabled,
		"sliding_window":     r.config.SlidingWindow.Enabled,
		"adaptive_enabled":   r.config.AdaptiveConfig.Enabled,
	}
}