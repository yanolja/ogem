package routing

import (
	"context"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/cost"
	"github.com/yanolja/ogem/monitoring"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
)

// RoutingStrategy defines different routing strategies
type RoutingStrategy string

const (
	// StrategyLatency routes to the endpoint with lowest latency (default)
	StrategyLatency RoutingStrategy = "latency"
	
	// StrategyCost routes to the endpoint with lowest cost
	StrategyCost RoutingStrategy = "cost"
	
	// StrategyRoundRobin distributes requests evenly across endpoints
	StrategyRoundRobin RoutingStrategy = "round_robin"
	
	// StrategyWeightedRoundRobin distributes requests based on endpoint weights
	StrategyWeightedRoundRobin RoutingStrategy = "weighted_round_robin"
	
	// StrategyLeastConnections routes to endpoint with fewest active connections
	StrategyLeastConnections RoutingStrategy = "least_connections"
	
	// StrategyRandomWeighted randomly selects endpoint based on weights
	StrategyRandomWeighted RoutingStrategy = "random_weighted"
	
	// StrategyPerformanceBased routes based on combined performance metrics
	StrategyPerformanceBased RoutingStrategy = "performance_based"
	
	// StrategyAdaptive dynamically adapts strategy based on conditions
	StrategyAdaptive RoutingStrategy = "adaptive"
)

// RoutingConfig represents routing configuration
type RoutingConfig struct {
	// Primary routing strategy
	Strategy RoutingStrategy `yaml:"strategy"`
	
	// Fallback strategy if primary fails
	FallbackStrategy RoutingStrategy `yaml:"fallback_strategy"`
	
	// Cost weight factor (0.0 to 1.0) for performance-based routing
	CostWeight float64 `yaml:"cost_weight"`
	
	// Latency weight factor (0.0 to 1.0) for performance-based routing
	LatencyWeight float64 `yaml:"latency_weight"`
	
	// Success rate weight factor (0.0 to 1.0) for performance-based routing
	SuccessRateWeight float64 `yaml:"success_rate_weight"`
	
	// Load weight factor (0.0 to 1.0) for performance-based routing
	LoadWeight float64 `yaml:"load_weight"`
	
	// Adaptive thresholds
	AdaptiveConfig *AdaptiveConfig `yaml:"adaptive,omitempty"`
	
	// Endpoint weights for weighted strategies
	EndpointWeights map[string]float64 `yaml:"endpoint_weights,omitempty"`
	
	// Circuit breaker configuration
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker,omitempty"`
	
	// Enable request routing metrics collection
	EnableMetrics bool `yaml:"enable_metrics"`
}

// AdaptiveConfig configures adaptive routing behavior
type AdaptiveConfig struct {
	// Switch to cost-based routing when cost difference exceeds threshold
	CostThreshold float64 `yaml:"cost_threshold"`
	
	// Switch to latency-based routing when latency difference exceeds threshold (ms)
	LatencyThreshold time.Duration `yaml:"latency_threshold"`
	
	// Switch to least-connections when load difference exceeds threshold
	LoadThreshold float64 `yaml:"load_threshold"`
	
	// Evaluation interval for strategy adaptation
	EvaluationInterval time.Duration `yaml:"evaluation_interval"`
	
	// Minimum samples before switching strategies
	MinSamples int `yaml:"min_samples"`
}

// CircuitBreakerConfig configures circuit breaker behavior
type CircuitBreakerConfig struct {
	// Failure threshold to open circuit
	FailureThreshold int `yaml:"failure_threshold"`
	
	// Success threshold to close circuit
	SuccessThreshold int `yaml:"success_threshold"`
	
	// Timeout before attempting to close circuit
	Timeout time.Duration `yaml:"timeout"`
}

// EndpointMetrics tracks performance metrics for an endpoint
type EndpointMetrics struct {
	// Basic metrics
	TotalRequests     int64         `json:"total_requests"`
	SuccessfulRequests int64        `json:"successful_requests"`
	FailedRequests    int64         `json:"failed_requests"`
	ActiveConnections int64         `json:"active_connections"`
	
	// Performance metrics
	AverageLatency    time.Duration `json:"average_latency"`
	AverageCost      float64       `json:"average_cost"`
	ThroughputRPM    float64       `json:"throughput_rpm"`
	
	// Time-windowed metrics (last 5 minutes)
	RecentLatency     time.Duration `json:"recent_latency"`
	RecentSuccessRate float64       `json:"recent_success_rate"`
	RecentCost       float64       `json:"recent_cost"`
	
	// Circuit breaker state
	CircuitState      CircuitState  `json:"circuit_state"`
	LastFailureTime  time.Time     `json:"last_failure_time"`
	ConsecutiveFailures int         `json:"consecutive_failures"`
	ConsecutiveSuccesses int        `json:"consecutive_successes"`
	
	// Internal tracking
	lastUpdated      time.Time
	mutex           sync.RWMutex
}

// CircuitState represents circuit breaker state
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

// Router provides intelligent routing capabilities
type Router struct {
	config          *RoutingConfig
	endpointMetrics map[string]*EndpointMetrics
	roundRobinIndex int
	adaptiveState   *AdaptiveState
	// costCalculator removed as cost package has standalone functions
	monitor         *monitoring.MonitoringManager
	logger          *zap.SugaredLogger
	mutex           sync.RWMutex
}

// AdaptiveState tracks adaptive routing decisions
type AdaptiveState struct {
	CurrentStrategy  RoutingStrategy
	LastEvaluation  time.Time
	SampleCount     int
	StrategyHistory []StrategyChange
	mutex          sync.RWMutex
}

// StrategyChange records when and why routing strategy changed
type StrategyChange struct {
	Timestamp    time.Time       `json:"timestamp"`
	FromStrategy RoutingStrategy `json:"from_strategy"`
	ToStrategy   RoutingStrategy `json:"to_strategy"`
	Reason       string          `json:"reason"`
	Metrics      map[string]interface{} `json:"metrics"`
}

// NewRouter creates a new intelligent router
func NewRouter(config *RoutingConfig, monitor *monitoring.MonitoringManager, logger *zap.SugaredLogger) *Router {
	if config == nil {
		config = DefaultRoutingConfig()
	}
	
	// Normalize weights for performance-based routing
	totalWeight := config.CostWeight + config.LatencyWeight + config.SuccessRateWeight + config.LoadWeight
	if totalWeight > 0 {
		config.CostWeight /= totalWeight
		config.LatencyWeight /= totalWeight
		config.SuccessRateWeight /= totalWeight
		config.LoadWeight /= totalWeight
	}
	
	router := &Router{
		config:          config,
		endpointMetrics: make(map[string]*EndpointMetrics),
		monitor:         monitor,
		logger:          logger,
	}
	
	// Initialize adaptive state if using adaptive routing
	if config.Strategy == StrategyAdaptive {
		router.adaptiveState = &AdaptiveState{
			CurrentStrategy:  StrategyLatency, // Start with latency-based
			LastEvaluation:  time.Now(),
			StrategyHistory: make([]StrategyChange, 0),
		}
	}
	
	return router
}

// DefaultRoutingConfig returns default routing configuration
func DefaultRoutingConfig() *RoutingConfig {
	return &RoutingConfig{
		Strategy:          StrategyLatency,
		FallbackStrategy:  StrategyRoundRobin,
		CostWeight:        0.3,
		LatencyWeight:     0.4,
		SuccessRateWeight: 0.2,
		LoadWeight:        0.1,
		AdaptiveConfig: &AdaptiveConfig{
			CostThreshold:      0.5,  // $0.50 difference
			LatencyThreshold:   500 * time.Millisecond,
			LoadThreshold:      0.3,  // 30% load difference
			EvaluationInterval: 2 * time.Minute,
			MinSamples:        10,
		},
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 3,
			Timeout:         30 * time.Second,
		},
		EnableMetrics: true,
	}
}

// RouteRequest intelligently routes a request to the best endpoint
func (r *Router) RouteRequest(ctx context.Context, endpoints []*EndpointStatus, request *openai.ChatCompletionRequest) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	// Filter out endpoints with open circuits
	availableEndpoints := r.filterAvailableEndpoints(endpoints)
	if len(availableEndpoints) == 0 {
		r.logger.Warn("All endpoints have open circuits, using original list")
		availableEndpoints = endpoints
	}
	
	// Update adaptive strategy if needed
	if r.config.Strategy == StrategyAdaptive {
		r.updateAdaptiveStrategy()
	}
	
	strategy := r.getActiveStrategy()
	
	// Route based on selected strategy
	var selectedEndpoint *EndpointStatus
	var err error
	
	switch strategy {
	case StrategyCost:
		selectedEndpoint, err = r.routeByCost(availableEndpoints, request)
	case StrategyRoundRobin:
		selectedEndpoint, err = r.routeRoundRobin(availableEndpoints)
	case StrategyWeightedRoundRobin:
		selectedEndpoint, err = r.routeWeightedRoundRobin(availableEndpoints)
	case StrategyLeastConnections:
		selectedEndpoint, err = r.routeLeastConnections(availableEndpoints)
	case StrategyRandomWeighted:
		selectedEndpoint, err = r.routeRandomWeighted(availableEndpoints)
	case StrategyPerformanceBased:
		selectedEndpoint, err = r.routePerformanceBased(availableEndpoints, request)
	default: // StrategyLatency
		selectedEndpoint, err = r.routeByLatency(availableEndpoints)
	}
	
	// Try fallback strategy if primary fails
	if err != nil && r.config.FallbackStrategy != strategy {
		r.logger.Warnw("Primary routing strategy failed, trying fallback", 
			"primary", strategy, "fallback", r.config.FallbackStrategy, "error", err)
		
		switch r.config.FallbackStrategy {
		case StrategyCost:
			selectedEndpoint, err = r.routeByCost(availableEndpoints, request)
		case StrategyRoundRobin:
			selectedEndpoint, err = r.routeRoundRobin(availableEndpoints)
		case StrategyLeastConnections:
			selectedEndpoint, err = r.routeLeastConnections(availableEndpoints)
		default:
			selectedEndpoint, err = r.routeByLatency(availableEndpoints)
		}
	}
	
	if err != nil {
		return nil, fmt.Errorf("routing failed: %v", err)
	}
	
	// Update metrics
	r.incrementActiveConnections(selectedEndpoint)
	
	// Record routing decision for monitoring
	if r.config.EnableMetrics && r.monitor != nil {
		r.recordRoutingMetrics(strategy, selectedEndpoint, len(availableEndpoints))
	}
	
	return selectedEndpoint, nil
}

// RecordRequestResult updates endpoint metrics based on request outcome
func (r *Router) RecordRequestResult(endpoint *EndpointStatus, latency time.Duration, cost float64, success bool, errorMsg string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	endpointKey := r.getEndpointKey(endpoint)
	metrics, exists := r.endpointMetrics[endpointKey]
	if !exists {
		metrics = &EndpointMetrics{
			CircuitState: CircuitClosed,
		}
		r.endpointMetrics[endpointKey] = metrics
	}
	
	metrics.mutex.Lock()
	defer metrics.mutex.Unlock()
	
	// Update basic metrics
	metrics.TotalRequests++
	metrics.ActiveConnections--
	if metrics.ActiveConnections < 0 {
		metrics.ActiveConnections = 0
	}
	
	if success {
		metrics.SuccessfulRequests++
		metrics.ConsecutiveSuccesses++
		metrics.ConsecutiveFailures = 0
		
		// Update circuit breaker state
		if metrics.CircuitState == CircuitHalfOpen && 
		   metrics.ConsecutiveSuccesses >= r.config.CircuitBreaker.SuccessThreshold {
			metrics.CircuitState = CircuitClosed
			r.logger.Infow("Circuit breaker closed", "endpoint", endpointKey)
		}
	} else {
		metrics.FailedRequests++
		metrics.ConsecutiveFailures++
		metrics.ConsecutiveSuccesses = 0
		metrics.LastFailureTime = time.Now()
		
		// Update circuit breaker state
		if metrics.CircuitState == CircuitClosed && 
		   metrics.ConsecutiveFailures >= r.config.CircuitBreaker.FailureThreshold {
			metrics.CircuitState = CircuitOpen
			r.logger.Warnw("Circuit breaker opened", "endpoint", endpointKey, "failures", metrics.ConsecutiveFailures)
		}
	}
	
	// Update performance metrics
	if metrics.TotalRequests == 1 {
		metrics.AverageLatency = latency
		metrics.AverageCost = cost
	} else {
		// Exponentially weighted moving average
		alpha := 0.1
		metrics.AverageLatency = time.Duration(float64(metrics.AverageLatency)*(1-alpha) + float64(latency)*alpha)
		metrics.AverageCost = metrics.AverageCost*(1-alpha) + cost*alpha
	}
	
	// Update recent metrics (5-minute window)
	now := time.Now()
	if now.Sub(metrics.lastUpdated) > 5*time.Minute {
		metrics.RecentLatency = latency
		metrics.RecentSuccessRate = 1.0
		if !success {
			metrics.RecentSuccessRate = 0.0
		}
		metrics.RecentCost = cost
	} else {
		windowAlpha := 0.2
		metrics.RecentLatency = time.Duration(float64(metrics.RecentLatency)*(1-windowAlpha) + float64(latency)*windowAlpha)
		metrics.RecentCost = metrics.RecentCost*(1-windowAlpha) + cost*windowAlpha
		
		recentSuccessRate := 1.0
		if !success {
			recentSuccessRate = 0.0
		}
		metrics.RecentSuccessRate = metrics.RecentSuccessRate*(1-windowAlpha) + recentSuccessRate*windowAlpha
	}
	
	metrics.lastUpdated = now
	
	// Update throughput (requests per minute)
	if metrics.TotalRequests > 1 {
		minutes := now.Sub(metrics.lastUpdated).Minutes()
		if minutes > 0 {
			metrics.ThroughputRPM = float64(metrics.TotalRequests) / minutes
		}
	}
}

// routeByLatency routes to endpoint with lowest latency
func (r *Router) routeByLatency(endpoints []*EndpointStatus) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Latency < endpoints[j].Latency
	})
	
	return endpoints[0], nil
}

// routeByCost routes to endpoint with lowest cost
func (r *Router) routeByCost(endpoints []*EndpointStatus, request *openai.ChatCompletionRequest) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	type endpointCost struct {
		endpoint *EndpointStatus
		cost     float64
	}
	
	var endpointCosts []endpointCost
	
	for _, endpoint := range endpoints {
		// Estimate cost for this endpoint
		estimatedCost, err := r.estimateRequestCost(endpoint, request)
		if err != nil {
			r.logger.Warnw("Failed to estimate cost for endpoint", "endpoint", r.getEndpointKey(endpoint), "error", err)
			// Use average cost from metrics if estimation fails
			endpointKey := r.getEndpointKey(endpoint)
			if metrics, exists := r.endpointMetrics[endpointKey]; exists {
				estimatedCost = metrics.AverageCost
			} else {
				estimatedCost = 0.001 // Default fallback cost
			}
		}
		
		endpointCosts = append(endpointCosts, endpointCost{
			endpoint: endpoint,
			cost:     estimatedCost,
		})
	}
	
	// Sort by cost (ascending)
	sort.Slice(endpointCosts, func(i, j int) bool {
		return endpointCosts[i].cost < endpointCosts[j].cost
	})
	
	return endpointCosts[0].endpoint, nil
}

// routeRoundRobin distributes requests evenly
func (r *Router) routeRoundRobin(endpoints []*EndpointStatus) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	r.roundRobinIndex = (r.roundRobinIndex + 1) % len(endpoints)
	return endpoints[r.roundRobinIndex], nil
}

// routeWeightedRoundRobin distributes requests based on weights
func (r *Router) routeWeightedRoundRobin(endpoints []*EndpointStatus) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	// If no weights configured, fall back to normal round robin
	if len(r.config.EndpointWeights) == 0 {
		return r.routeRoundRobin(endpoints)
	}
	
	// Calculate total weight
	var totalWeight float64
	for _, endpoint := range endpoints {
		weight := r.getEndpointWeight(endpoint)
		totalWeight += weight
	}
	
	if totalWeight == 0 {
		return r.routeRoundRobin(endpoints)
	}
	
	// Select based on weight distribution
	r.mutex.Lock()
	target := float64(r.roundRobinIndex) / float64(len(endpoints)) * totalWeight
	r.roundRobinIndex = (r.roundRobinIndex + 1) % len(endpoints)
	r.mutex.Unlock()
	
	var currentWeight float64
	for _, endpoint := range endpoints {
		currentWeight += r.getEndpointWeight(endpoint)
		if currentWeight >= target {
			return endpoint, nil
		}
	}
	
	// Fallback to first endpoint
	return endpoints[0], nil
}

// routeLeastConnections routes to endpoint with fewest active connections
func (r *Router) routeLeastConnections(endpoints []*EndpointStatus) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	minConnections := int64(math.MaxInt64)
	var selectedEndpoint *EndpointStatus
	
	for _, endpoint := range endpoints {
		endpointKey := r.getEndpointKey(endpoint)
		metrics, exists := r.endpointMetrics[endpointKey]
		var connections int64
		if exists {
			connections = metrics.ActiveConnections
		}
		
		if connections < minConnections {
			minConnections = connections
			selectedEndpoint = endpoint
		}
	}
	
	if selectedEndpoint == nil {
		return endpoints[0], nil
	}
	
	return selectedEndpoint, nil
}

// routeRandomWeighted randomly selects endpoint based on weights
func (r *Router) routeRandomWeighted(endpoints []*EndpointStatus) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	// Calculate weights based on performance metrics
	var totalWeight float64
	weights := make([]float64, len(endpoints))
	
	for i, endpoint := range endpoints {
		weight := r.calculateDynamicWeight(endpoint)
		weights[i] = weight
		totalWeight += weight
	}
	
	if totalWeight == 0 {
		return endpoints[0], nil
	}
	
	// Random selection based on weights
	target := totalWeight * (float64(time.Now().UnixNano()%1000000) / 1000000.0)
	var currentWeight float64
	
	for i, weight := range weights {
		currentWeight += weight
		if currentWeight >= target {
			return endpoints[i], nil
		}
	}
	
	return endpoints[len(endpoints)-1], nil
}

// routePerformanceBased routes based on combined performance metrics
func (r *Router) routePerformanceBased(endpoints []*EndpointStatus, request *openai.ChatCompletionRequest) (*EndpointStatus, error) {
	if len(endpoints) == 0 {
		return nil, fmt.Errorf("no endpoints available")
	}
	
	type endpointScore struct {
		endpoint *EndpointStatus
		score    float64
	}
	
	var endpointScores []endpointScore
	
	for _, endpoint := range endpoints {
		score := r.calculatePerformanceScore(endpoint, request)
		endpointScores = append(endpointScores, endpointScore{
			endpoint: endpoint,
			score:    score,
		})
	}
	
	// Sort by score (higher is better)
	sort.Slice(endpointScores, func(i, j int) bool {
		return endpointScores[i].score > endpointScores[j].score
	})
	
	return endpointScores[0].endpoint, nil
}

// Helper methods

func (r *Router) getEndpointKey(endpoint *EndpointStatus) string {
	return fmt.Sprintf("%s/%s", endpoint.Endpoint.Provider(), endpoint.Endpoint.Region())
}

func (r *Router) getEndpointWeight(endpoint *EndpointStatus) float64 {
	key := r.getEndpointKey(endpoint)
	if weight, exists := r.config.EndpointWeights[key]; exists {
		return weight
	}
	return 1.0 // Default weight
}

func (r *Router) filterAvailableEndpoints(endpoints []*EndpointStatus) []*EndpointStatus {
	var available []*EndpointStatus
	
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	now := time.Now()
	
	for _, endpoint := range endpoints {
		endpointKey := r.getEndpointKey(endpoint)
		metrics, exists := r.endpointMetrics[endpointKey]
		
		if !exists {
			// No metrics yet, consider available
			available = append(available, endpoint)
			continue
		}
		
		metrics.mutex.RLock()
		circuitState := metrics.CircuitState
		lastFailure := metrics.LastFailureTime
		metrics.mutex.RUnlock()
		
		switch circuitState {
		case CircuitClosed:
			available = append(available, endpoint)
		case CircuitOpen:
			// Check if timeout has passed
			if now.Sub(lastFailure) > r.config.CircuitBreaker.Timeout {
				// Transition to half-open
				metrics.mutex.Lock()
				metrics.CircuitState = CircuitHalfOpen
				metrics.mutex.Unlock()
				available = append(available, endpoint)
				r.logger.Infow("Circuit breaker transitioned to half-open", "endpoint", endpointKey)
			}
		case CircuitHalfOpen:
			available = append(available, endpoint)
		}
	}
	
	return available
}

func (r *Router) incrementActiveConnections(endpoint *EndpointStatus) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	endpointKey := r.getEndpointKey(endpoint)
	metrics, exists := r.endpointMetrics[endpointKey]
	if !exists {
		metrics = &EndpointMetrics{
			CircuitState: CircuitClosed,
		}
		r.endpointMetrics[endpointKey] = metrics
	}
	
	metrics.mutex.Lock()
	metrics.ActiveConnections++
	metrics.mutex.Unlock()
}

func (r *Router) calculateDynamicWeight(endpoint *EndpointStatus) float64 {
	endpointKey := r.getEndpointKey(endpoint)
	
	r.mutex.RLock()
	metrics, exists := r.endpointMetrics[endpointKey]
	r.mutex.RUnlock()
	
	if !exists {
		return 1.0 // Default weight for new endpoints
	}
	
	metrics.mutex.RLock()
	defer metrics.mutex.RUnlock()
	
	// Base weight on success rate and inverse of latency
	successRate := metrics.RecentSuccessRate
	if metrics.TotalRequests > 0 {
		successRate = float64(metrics.SuccessfulRequests) / float64(metrics.TotalRequests)
	}
	
	latencyMs := float64(metrics.RecentLatency.Milliseconds())
	if latencyMs == 0 {
		latencyMs = 1 // Avoid division by zero
	}
	
	// Weight = success_rate * (1 / latency_factor)
	latencyFactor := math.Log(latencyMs + 1) // Logarithmic scaling
	weight := successRate * (1.0 / latencyFactor)
	
	return math.Max(weight, 0.01) // Minimum weight
}

func (r *Router) calculatePerformanceScore(endpoint *EndpointStatus, request *openai.ChatCompletionRequest) float64 {
	endpointKey := r.getEndpointKey(endpoint)
	
	r.mutex.RLock()
	metrics, exists := r.endpointMetrics[endpointKey]
	r.mutex.RUnlock()
	
	if !exists {
		return 0.5 // Neutral score for new endpoints
	}
	
	metrics.mutex.RLock()
	defer metrics.mutex.RUnlock()
	
	// Calculate normalized scores for each factor
	var costScore, latencyScore, successRateScore, loadScore float64
	
	// Cost score (lower cost = higher score)
	if r.config.CostWeight > 0 {
		estimatedCost, _ := r.estimateRequestCost(endpoint, request)
		if estimatedCost > 0 {
			maxCost := 1.0 // Assume $1 as max cost for normalization
			costScore = math.Max(0, (maxCost-estimatedCost)/maxCost)
		} else {
			costScore = 1.0
		}
	}
	
	// Latency score (lower latency = higher score)
	if r.config.LatencyWeight > 0 {
		latencyMs := float64(metrics.RecentLatency.Milliseconds())
		maxLatency := 5000.0 // Assume 5s as max latency for normalization
		latencyScore = math.Max(0, (maxLatency-latencyMs)/maxLatency)
	}
	
	// Success rate score
	if r.config.SuccessRateWeight > 0 {
		successRateScore = metrics.RecentSuccessRate
	}
	
	// Load score (lower load = higher score)
	if r.config.LoadWeight > 0 {
		maxConnections := 100.0 // Assume 100 as max connections for normalization
		loadScore = math.Max(0, (maxConnections-float64(metrics.ActiveConnections))/maxConnections)
	}
	
	// Weighted combination
	totalScore := costScore*r.config.CostWeight +
		latencyScore*r.config.LatencyWeight +
		successRateScore*r.config.SuccessRateWeight +
		loadScore*r.config.LoadWeight
	
	return totalScore
}

func (r *Router) estimateRequestCost(endpoint *EndpointStatus, request *openai.ChatCompletionRequest) (float64, error) {
	// Estimate input tokens
	inputTokens := 0
	for _, message := range request.Messages {
		if message.Content != nil {
			if message.Content.String != nil {
				inputTokens += len(*message.Content.String) / 4 // Rough estimation: 4 chars per token
			} else if message.Content.Parts != nil {
				// Estimate tokens for multipart content
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						inputTokens += len(part.Content.TextContent.Text) / 4
					}
					// Add some tokens for non-text parts (images, etc.)
					if part.Content.ImageContent != nil {
						inputTokens += 100 // Rough estimate for image description
					}
				}
			}
		}
	}
	
	// Estimate output tokens based on max_tokens or default
	outputTokens := 100 // Default estimation
	if request.MaxTokens != nil {
		outputTokens = int(*request.MaxTokens)
	}
	
	usage := openai.Usage{
		PromptTokens:     int32(inputTokens),
		CompletionTokens: int32(outputTokens),
		TotalTokens:      int32(inputTokens + outputTokens),
	}
	
	return cost.CalculateChatCost(request.Model, usage), nil
}

func (r *Router) getActiveStrategy() RoutingStrategy {
	if r.config.Strategy == StrategyAdaptive && r.adaptiveState != nil {
		r.adaptiveState.mutex.RLock()
		defer r.adaptiveState.mutex.RUnlock()
		return r.adaptiveState.CurrentStrategy
	}
	return r.config.Strategy
}

func (r *Router) updateAdaptiveStrategy() {
	if r.adaptiveState == nil || r.config.AdaptiveConfig == nil {
		return
	}
	
	r.adaptiveState.mutex.Lock()
	defer r.adaptiveState.mutex.Unlock()
	
	now := time.Now()
	if now.Sub(r.adaptiveState.LastEvaluation) < r.config.AdaptiveConfig.EvaluationInterval {
		return
	}
	
	if r.adaptiveState.SampleCount < r.config.AdaptiveConfig.MinSamples {
		r.adaptiveState.SampleCount++
		return
	}
	
	// Analyze current performance and decide if strategy should change
	newStrategy := r.evaluateBestStrategy()
	if newStrategy != r.adaptiveState.CurrentStrategy {
		r.logger.Infow("Adaptive routing strategy change",
			"from", r.adaptiveState.CurrentStrategy,
			"to", newStrategy,
			"reason", "performance_optimization")
		
		r.adaptiveState.StrategyHistory = append(r.adaptiveState.StrategyHistory, StrategyChange{
			Timestamp:    now,
			FromStrategy: r.adaptiveState.CurrentStrategy,
			ToStrategy:   newStrategy,
			Reason:       "performance_optimization",
		})
		
		r.adaptiveState.CurrentStrategy = newStrategy
	}
	
	r.adaptiveState.LastEvaluation = now
	r.adaptiveState.SampleCount = 0
}

func (r *Router) evaluateBestStrategy() RoutingStrategy {
	// This is a simplified adaptive algorithm
	// In a real implementation, you might use machine learning or more sophisticated analysis
	
	// Analyze recent performance across all endpoints
	var avgLatency time.Duration
	var avgCost float64
	var avgLoad float64
	var endpointCount int
	
	r.mutex.RLock()
	for _, metrics := range r.endpointMetrics {
		metrics.mutex.RLock()
		avgLatency += metrics.RecentLatency
		avgCost += metrics.RecentCost
		avgLoad += float64(metrics.ActiveConnections)
		endpointCount++
		metrics.mutex.RUnlock()
	}
	r.mutex.RUnlock()
	
	if endpointCount == 0 {
		return StrategyLatency // Default fallback
	}
	
	avgLatency /= time.Duration(endpointCount)
	avgCost /= float64(endpointCount)
	avgLoad /= float64(endpointCount)
	
	// Decision logic based on thresholds
	if avgCost > r.config.AdaptiveConfig.CostThreshold {
		return StrategyCost
	}
	
	if avgLatency > r.config.AdaptiveConfig.LatencyThreshold {
		return StrategyLatency
	}
	
	if avgLoad > r.config.AdaptiveConfig.LoadThreshold {
		return StrategyLeastConnections
	}
	
	// Default to performance-based if no specific condition is met
	return StrategyPerformanceBased
}

func (r *Router) recordRoutingMetrics(strategy RoutingStrategy, endpoint *EndpointStatus, totalEndpoints int) {
	if r.monitor == nil {
		return
	}
	
	labels := map[string]string{
		"strategy":        string(strategy),
		"provider":        endpoint.Endpoint.Provider(),
		"region":          endpoint.Endpoint.Region(),
		"total_endpoints": fmt.Sprintf("%d", totalEndpoints),
	}
	
	metric := &monitoring.Metric{
		Name:      "routing_decisions_total",
		Type:      monitoring.MetricTypeCounter,
		Value:     1,
		Labels:    labels,
		Timestamp: time.Now(),
	}
	
	if err := r.monitor.RecordMetric(metric); err != nil {
		r.logger.Warnw("Failed to record routing metrics", "error", err)
	}
}

// GetEndpointMetrics returns current metrics for an endpoint
func (r *Router) GetEndpointMetrics(endpoint *EndpointStatus) *EndpointMetrics {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	endpointKey := r.getEndpointKey(endpoint)
	if metrics, exists := r.endpointMetrics[endpointKey]; exists {
		// Return a copy to avoid race conditions
		metrics.mutex.RLock()
		defer metrics.mutex.RUnlock()
		
		return &EndpointMetrics{
			TotalRequests:        metrics.TotalRequests,
			SuccessfulRequests:   metrics.SuccessfulRequests,
			FailedRequests:       metrics.FailedRequests,
			ActiveConnections:    metrics.ActiveConnections,
			AverageLatency:       metrics.AverageLatency,
			AverageCost:         metrics.AverageCost,
			ThroughputRPM:       metrics.ThroughputRPM,
			RecentLatency:       metrics.RecentLatency,
			RecentSuccessRate:   metrics.RecentSuccessRate,
			RecentCost:         metrics.RecentCost,
			CircuitState:        metrics.CircuitState,
			LastFailureTime:     metrics.LastFailureTime,
			ConsecutiveFailures: metrics.ConsecutiveFailures,
			ConsecutiveSuccesses: metrics.ConsecutiveSuccesses,
		}
	}
	
	return nil
}

// GetRoutingStats returns overall routing statistics
func (r *Router) GetRoutingStats() map[string]interface{} {
	stats := map[string]interface{}{
		"strategy": r.getActiveStrategy(),
		"config":   r.config,
	}
	
	if r.adaptiveState != nil {
		r.adaptiveState.mutex.RLock()
		stats["adaptive_state"] = map[string]interface{}{
			"current_strategy":  r.adaptiveState.CurrentStrategy,
			"last_evaluation":  r.adaptiveState.LastEvaluation,
			"sample_count":     r.adaptiveState.SampleCount,
			"strategy_history": r.adaptiveState.StrategyHistory,
		}
		r.adaptiveState.mutex.RUnlock()
	}
	
	return stats
}

// EndpointStatus represents the status of an endpoint for routing
type EndpointStatus struct {
	Endpoint    provider.AiEndpoint
	Latency     time.Duration
	ModelStatus *ogem.SupportedModel
}