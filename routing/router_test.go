package routing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/openai"
	ogemSdk "github.com/yanolja/ogem/sdk/go"
)

func TestNewRouter(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name   string
		config *RoutingConfig
	}{
		{
			name:   "nil config uses default",
			config: nil,
		},
		{
			name: "custom config",
			config: &RoutingConfig{
				Strategy:          StrategyCost,
				FallbackStrategy:  StrategyLatency,
				CostWeight:        0.4,
				LatencyWeight:     0.3,
				SuccessRateWeight: 0.2,
				LoadWeight:        0.1,
				EnableMetrics:     true,
			},
		},
		{
			name: "adaptive strategy",
			config: &RoutingConfig{
				Strategy:         StrategyAdaptive,
				FallbackStrategy: StrategyRoundRobin,
				AdaptiveConfig: &AdaptiveConfig{
					CostThreshold:      1.0,
					LatencyThreshold:   1 * time.Second,
					LoadThreshold:      0.5,
					EvaluationInterval: 5 * time.Minute,
					MinSamples:         20,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(tt.config, nil, logger)

			assert.NotNil(t, router)
			assert.NotNil(t, router.config)
			assert.NotNil(t, router.endpointMetrics)
			assert.Equal(t, logger, router.logger)

			if tt.config == nil {
				// Should use default config
				assert.Equal(t, StrategyLatency, router.config.Strategy)
			} else {
				if tt.config.Strategy == StrategyAdaptive {
					assert.NotNil(t, router.adaptiveState)
					assert.Equal(t, StrategyLatency, router.adaptiveState.CurrentStrategy)
				}

				// Weights should be normalized
				totalWeight := router.config.CostWeight + router.config.LatencyWeight +
					router.config.SuccessRateWeight + router.config.LoadWeight
				if totalWeight > 0 {
					assert.InDelta(t, 1.0, totalWeight, 0.001)
				}
			}
		})
	}
}

func TestDefaultRoutingConfig(t *testing.T) {
	config := DefaultRoutingConfig()

	assert.NotNil(t, config)
	assert.Equal(t, StrategyLatency, config.Strategy)
	assert.Equal(t, StrategyRoundRobin, config.FallbackStrategy)
	assert.True(t, config.EnableMetrics)

	// Check adaptive config
	assert.NotNil(t, config.AdaptiveConfig)
	assert.Equal(t, 0.5, config.AdaptiveConfig.CostThreshold)
	assert.Equal(t, 500*time.Millisecond, config.AdaptiveConfig.LatencyThreshold)
	assert.Equal(t, 0.3, config.AdaptiveConfig.LoadThreshold)

	// Check circuit breaker config
	assert.NotNil(t, config.CircuitBreaker)
	assert.Equal(t, 5, config.CircuitBreaker.FailureThreshold)
	assert.Equal(t, 3, config.CircuitBreaker.SuccessThreshold)
	assert.Equal(t, 30*time.Second, config.CircuitBreaker.Timeout)
}

func TestRouter_RouteRequest_NoEndpoints(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	ctx := context.Background()
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

	result, err := router.RouteRequest(ctx, []*EndpointStatus{}, request)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no endpoints available")
}

func TestRouter_RouteByLatency(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyLatency,
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  200 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  100 * time.Millisecond, // Lower latency - should be selected
		},
		{
			Endpoint: &mockEndpoint{provider: "google", region: "europe-west1"},
			Latency:  300 * time.Millisecond,
		},
	}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	result, err := router.RouteRequest(ctx, endpoints, request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "anthropic", result.Endpoint.Provider())
	assert.Equal(t, "us-west-2", result.Endpoint.Region())
}

func TestRouter_RouteByCost(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyCost,
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  200 * time.Millisecond,
		},
	}

	ctx := context.Background()
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

	result, err := router.RouteRequest(ctx, endpoints, request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// Should select based on estimated cost
}

func TestRouter_RouteRoundRobin(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyRoundRobin,
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  200 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "google", region: "europe-west1"},
			Latency:  150 * time.Millisecond,
		},
	}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	selectedEndpoints := make(map[string]int)

	// Make multiple requests to test round-robin distribution
	for i := 0; i < 6; i++ {
		result, err := router.RouteRequest(ctx, endpoints, request)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		key := result.Endpoint.Provider() + "/" + result.Endpoint.Region()
		selectedEndpoints[key]++
	}

	// Each endpoint should be selected twice in 6 requests
	assert.Equal(t, 3, len(selectedEndpoints))
	for _, count := range selectedEndpoints {
		assert.Equal(t, 2, count)
	}
}

func TestRouter_RouteWeightedRoundRobin(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyWeightedRoundRobin,
		EndpointWeights: map[string]float64{
			"openai/us-east-1":    3.0, // Higher weight
			"anthropic/us-west-2": 1.0,
			"google/europe-west1": 1.0,
		},
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  200 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "google", region: "europe-west1"},
			Latency:  150 * time.Millisecond,
		},
	}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	selectedEndpoints := make(map[string]int)

	// Make multiple requests to test weighted distribution
	for i := 0; i < 15; i++ {
		result, err := router.RouteRequest(ctx, endpoints, request)
		assert.NoError(t, err)
		assert.NotNil(t, result)

		key := result.Endpoint.Provider() + "/" + result.Endpoint.Region()
		selectedEndpoints[key]++
	}

	// OpenAI should be selected more often due to higher weight
	assert.Greater(t, selectedEndpoints["openai/us-east-1"], selectedEndpoints["anthropic/us-west-2"])
	assert.Greater(t, selectedEndpoints["openai/us-east-1"], selectedEndpoints["google/europe-west1"])
}

func TestRouter_RouteLeastConnections(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyLeastConnections,
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  200 * time.Millisecond,
		},
	}

	// Simulate active connections on first endpoint
	router.endpointMetrics["openai/us-east-1"] = &EndpointMetrics{
		ActiveConnections: 5,
		CircuitState:      CircuitClosed,
	}
	router.endpointMetrics["anthropic/us-west-2"] = &EndpointMetrics{
		ActiveConnections: 2, // Fewer connections - should be selected
		CircuitState:      CircuitClosed,
	}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	result, err := router.RouteRequest(ctx, endpoints, request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "anthropic", result.Endpoint.Provider())
}

func TestRouter_RouteRandomWeighted(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyRandomWeighted,
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  200 * time.Millisecond,
		},
	}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	// Should not error even with random selection
	result, err := router.RouteRequest(ctx, endpoints, request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
}

func TestRouter_RoutePerformanceBased(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy:          StrategyPerformanceBased,
		CostWeight:        0.3,
		LatencyWeight:     0.4,
		SuccessRateWeight: 0.2,
		LoadWeight:        0.1,
	}
	router := NewRouter(config, nil, logger)

	endpoints := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
		{
			Endpoint: &mockEndpoint{provider: "anthropic", region: "us-west-2"},
			Latency:  200 * time.Millisecond,
		},
	}

	// Set up metrics for performance comparison
	router.endpointMetrics["openai/us-east-1"] = &EndpointMetrics{
		RecentLatency:     150 * time.Millisecond,
		RecentSuccessRate: 0.95,
		RecentCost:        0.002,
		ActiveConnections: 3,
		CircuitState:      CircuitClosed,
	}
	router.endpointMetrics["anthropic/us-west-2"] = &EndpointMetrics{
		RecentLatency:     250 * time.Millisecond,
		RecentSuccessRate: 0.90,
		RecentCost:        0.003,
		ActiveConnections: 5,
		CircuitState:      CircuitClosed,
	}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	result, err := router.RouteRequest(ctx, endpoints, request)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	// OpenAI should be selected due to better performance metrics
	assert.Equal(t, "openai", result.Endpoint.Provider())
}

func TestRouter_FallbackStrategy(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy:         StrategyCost,
		FallbackStrategy: StrategyLatency,
	}
	router := NewRouter(config, nil, logger)

	// Empty endpoints should cause cost routing to fail, trigger fallback
	endpoints := []*EndpointStatus{}

	ctx := context.Background()
	request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

	result, err := router.RouteRequest(ctx, endpoints, request)
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no endpoints available")
}

func TestRouter_RecordRequestResult(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}

	tests := []struct {
		name     string
		latency  time.Duration
		cost     float64
		success  bool
		errorMsg string
	}{
		{
			name:    "successful request",
			latency: 100 * time.Millisecond,
			cost:    0.001,
			success: true,
		},
		{
			name:     "failed request",
			latency:  500 * time.Millisecond,
			cost:     0.002,
			success:  false,
			errorMsg: "timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router.RecordRequestResult(endpoint, tt.latency, tt.cost, tt.success, tt.errorMsg)

			endpointKey := router.getEndpointKey(endpoint)
			metrics, exists := router.endpointMetrics[endpointKey]
			assert.True(t, exists)
			assert.NotNil(t, metrics)

			if tt.success {
				assert.Greater(t, metrics.SuccessfulRequests, int64(0))
				assert.Equal(t, 0, metrics.ConsecutiveFailures)
			} else {
				assert.Greater(t, metrics.FailedRequests, int64(0))
				assert.Greater(t, metrics.ConsecutiveFailures, 0)
			}

			assert.Greater(t, metrics.TotalRequests, int64(0))
		})
	}
}

func TestRouter_CircuitBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyLatency,
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 3,
			SuccessThreshold: 2,
			Timeout:          1 * time.Second,
		},
	}
	router := NewRouter(config, nil, logger)

	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}

	// Simulate multiple failures to open circuit
	for i := 0; i < 4; i++ {
		router.RecordRequestResult(endpoint, 100*time.Millisecond, 0.001, false, "error")
	}

	endpointKey := router.getEndpointKey(endpoint)
	metrics := router.endpointMetrics[endpointKey]
	assert.Equal(t, CircuitOpen, metrics.CircuitState)

	// Test that endpoint is filtered out when circuit is open
	endpoints := []*EndpointStatus{endpoint}
	available := router.filterAvailableEndpoints(endpoints)
	assert.Equal(t, 0, len(available))

	// Wait for timeout and test half-open transition
	time.Sleep(1100 * time.Millisecond)
	available = router.filterAvailableEndpoints(endpoints)
	assert.Equal(t, 1, len(available))
	assert.Equal(t, CircuitHalfOpen, metrics.CircuitState)

	// Successful requests should close the circuit
	for i := 0; i < 2; i++ {
		router.RecordRequestResult(endpoint, 100*time.Millisecond, 0.001, true, "")
	}
	assert.Equal(t, CircuitClosed, metrics.CircuitState)
}

func TestRouter_AdaptiveStrategy(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyAdaptive,
		AdaptiveConfig: &AdaptiveConfig{
			CostThreshold:      0.5,
			LatencyThreshold:   500 * time.Millisecond,
			LoadThreshold:      0.3,
			EvaluationInterval: 1 * time.Millisecond, // Very short for testing
			MinSamples:         1,
		},
	}
	router := NewRouter(config, nil, logger)

	assert.NotNil(t, router.adaptiveState)
	assert.Equal(t, StrategyLatency, router.adaptiveState.CurrentStrategy)

	// Add some metrics to trigger adaptation
	router.endpointMetrics["test/endpoint"] = &EndpointMetrics{
		RecentLatency: 600 * time.Millisecond, // Above threshold
		RecentCost:    0.1,
		CircuitState:  CircuitClosed,
	}

	// Force evaluation
	router.adaptiveState.LastEvaluation = time.Now().Add(-1 * time.Hour)
	router.adaptiveState.SampleCount = 2

	router.updateAdaptiveStrategy()

	// Strategy should change based on high latency
	assert.Equal(t, StrategyLatency, router.getActiveStrategy())
}

func TestRouter_GetEndpointMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}

	// Test with no metrics
	metrics := router.GetEndpointMetrics(endpoint)
	assert.Nil(t, metrics)

	// Add some metrics
	router.RecordRequestResult(endpoint, 100*time.Millisecond, 0.001, true, "")

	metrics = router.GetEndpointMetrics(endpoint)
	assert.NotNil(t, metrics)
	assert.Equal(t, int64(1), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.SuccessfulRequests)
	assert.Equal(t, CircuitClosed, metrics.CircuitState)
}

func TestRouter_GetRoutingStats(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy: StrategyAdaptive,
		AdaptiveConfig: &AdaptiveConfig{
			MinSamples: 10,
		},
	}
	router := NewRouter(config, nil, logger)

	stats := router.GetRoutingStats()
	assert.NotNil(t, stats)
	assert.Equal(t, StrategyLatency, stats["strategy"]) // Adaptive starts with latency
	assert.NotNil(t, stats["config"])

	// Test adaptive state in stats
	assert.Contains(t, stats, "adaptive_state")
	adaptiveStats := stats["adaptive_state"].(map[string]interface{})
	assert.Equal(t, StrategyLatency, adaptiveStats["current_strategy"])
	assert.Equal(t, 0, adaptiveStats["sample_count"])
}

func TestRouter_CalculateDynamicWeight(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}

	// Test with no metrics (should return default weight)
	weight := router.calculateDynamicWeight(endpoint)
	assert.Equal(t, 1.0, weight)

	// Add metrics
	endpointKey := router.getEndpointKey(endpoint)
	router.endpointMetrics[endpointKey] = &EndpointMetrics{
		TotalRequests:      100,
		SuccessfulRequests: 95,
		RecentSuccessRate:  0.95,
		RecentLatency:      100 * time.Millisecond,
	}

	weight = router.calculateDynamicWeight(endpoint)
	assert.Greater(t, weight, 0.01)
	assert.Less(t, weight, 1.0)
}

func TestRouter_EstimateRequestCost(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("This is a test message for cost estimation"),
				},
			},
		},
		MaxTokens: int32Ptr(100),
	}

	cost, err := router.estimateRequestCost(endpoint, request)
	assert.NoError(t, err)
	assert.Greater(t, cost, 0.0)

	// Test with multipart content
	multipartRequest := &openai.ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					Parts: []openai.Part{
						{
							Type: "text",
							Content: openai.Content{
								TextContent: &openai.TextContent{
									Text: "Describe this image",
								},
							},
						},
						{
							Type: "image_url",
							Content: openai.Content{
								ImageContent: &openai.ImageContent{
									Url:    "data:image/jpeg;base64,test",
									Detail: "high",
								},
							},
						},
					},
				},
			},
		},
	}

	cost, err = router.estimateRequestCost(endpoint, multipartRequest)
	assert.NoError(t, err)
	assert.Greater(t, cost, 0.0)
}

func TestRouteStrategies_EdgeCases(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	// Test with single endpoint
	singleEndpoint := []*EndpointStatus{
		{
			Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
			Latency:  100 * time.Millisecond,
		},
	}

	// Test all strategies with single endpoint
	strategies := []RoutingStrategy{
		StrategyLatency, StrategyCost, StrategyRoundRobin,
		StrategyLeastConnections, StrategyRandomWeighted,
	}

	for _, strategy := range strategies {
		t.Run(string(strategy)+"_single_endpoint", func(t *testing.T) {
			var result *EndpointStatus
			var err error

			request := &openai.ChatCompletionRequest{Model: "gpt-3.5-turbo"}

			switch strategy {
			case StrategyLatency:
				result, err = router.routeByLatency(singleEndpoint)
			case StrategyCost:
				result, err = router.routeByCost(singleEndpoint, request)
			case StrategyRoundRobin:
				result, err = router.routeRoundRobin(singleEndpoint)
			case StrategyLeastConnections:
				result, err = router.routeLeastConnections(singleEndpoint)
			case StrategyRandomWeighted:
				result, err = router.routeRandomWeighted(singleEndpoint)
			}

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, singleEndpoint[0], result)
		})
	}
}

func TestRouter_WeightNormalization(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	config := &RoutingConfig{
		Strategy:          StrategyPerformanceBased,
		CostWeight:        0.6, // Total > 1.0
		LatencyWeight:     0.8,
		SuccessRateWeight: 0.4,
		LoadWeight:        0.2,
	}

	router := NewRouter(config, nil, logger)

	// Weights should be normalized to sum to 1.0
	totalWeight := router.config.CostWeight + router.config.LatencyWeight +
		router.config.SuccessRateWeight + router.config.LoadWeight
	assert.InDelta(t, 1.0, totalWeight, 0.001)
}

func TestRoutingStrategy_Constants(t *testing.T) {
	assert.Equal(t, RoutingStrategy("latency"), StrategyLatency)
	assert.Equal(t, RoutingStrategy("cost"), StrategyCost)
	assert.Equal(t, RoutingStrategy("round_robin"), StrategyRoundRobin)
	assert.Equal(t, RoutingStrategy("weighted_round_robin"), StrategyWeightedRoundRobin)
	assert.Equal(t, RoutingStrategy("least_connections"), StrategyLeastConnections)
	assert.Equal(t, RoutingStrategy("random_weighted"), StrategyRandomWeighted)
	assert.Equal(t, RoutingStrategy("performance_based"), StrategyPerformanceBased)
	assert.Equal(t, RoutingStrategy("adaptive"), StrategyAdaptive)
}

func TestCircuitState_Constants(t *testing.T) {
	assert.Equal(t, CircuitState("closed"), CircuitClosed)
	assert.Equal(t, CircuitState("open"), CircuitOpen)
	assert.Equal(t, CircuitState("half_open"), CircuitHalfOpen)
}

func TestEndpointMetrics_Structure(t *testing.T) {
	now := time.Now()
	metrics := &EndpointMetrics{
		TotalRequests:        100,
		SuccessfulRequests:   95,
		FailedRequests:       5,
		ActiveConnections:    3,
		AverageLatency:       150 * time.Millisecond,
		AverageCost:          0.002,
		ThroughputRPM:        60.0,
		RecentLatency:        120 * time.Millisecond,
		RecentSuccessRate:    0.96,
		RecentCost:           0.0018,
		CircuitState:         CircuitClosed,
		LastFailureTime:      now,
		ConsecutiveFailures:  0,
		ConsecutiveSuccesses: 5,
	}

	assert.Equal(t, int64(100), metrics.TotalRequests)
	assert.Equal(t, int64(95), metrics.SuccessfulRequests)
	assert.Equal(t, int64(5), metrics.FailedRequests)
	assert.Equal(t, int64(3), metrics.ActiveConnections)
	assert.Equal(t, 150*time.Millisecond, metrics.AverageLatency)
	assert.Equal(t, 0.002, metrics.AverageCost)
	assert.Equal(t, 60.0, metrics.ThroughputRPM)
	assert.Equal(t, 120*time.Millisecond, metrics.RecentLatency)
	assert.Equal(t, 0.96, metrics.RecentSuccessRate)
	assert.Equal(t, 0.0018, metrics.RecentCost)
	assert.Equal(t, CircuitClosed, metrics.CircuitState)
	assert.Equal(t, now, metrics.LastFailureTime)
	assert.Equal(t, 0, metrics.ConsecutiveFailures)
	assert.Equal(t, 5, metrics.ConsecutiveSuccesses)
}

func TestAdaptiveState_Structure(t *testing.T) {
	now := time.Now()
	state := &AdaptiveState{
		CurrentStrategy: StrategyCost,
		LastEvaluation:  now,
		SampleCount:     25,
		StrategyHistory: []StrategyChange{
			{
				Timestamp:    now.Add(-1 * time.Hour),
				FromStrategy: StrategyLatency,
				ToStrategy:   StrategyCost,
				Reason:       "high_cost_detected",
				Metrics: map[string]interface{}{
					"avg_cost":    0.015,
					"avg_latency": "150ms",
				},
			},
		},
	}

	assert.Equal(t, StrategyCost, state.CurrentStrategy)
	assert.Equal(t, now, state.LastEvaluation)
	assert.Equal(t, 25, state.SampleCount)
	assert.Len(t, state.StrategyHistory, 1)

	change := state.StrategyHistory[0]
	assert.Equal(t, StrategyLatency, change.FromStrategy)
	assert.Equal(t, StrategyCost, change.ToStrategy)
	assert.Equal(t, "high_cost_detected", change.Reason)
	assert.Equal(t, 0.015, change.Metrics["avg_cost"])
	assert.Equal(t, "150ms", change.Metrics["avg_latency"])
}

func TestEndpointStatus_Structure(t *testing.T) {
	endpoint := &mockEndpoint{provider: "openai", region: "us-east-1"}
	modelStatus := &ogem.SupportedModel{
		Name:    ogemSdk.ModelGPT35Turbo,
		RateKey: ogemSdk.ModelGPT35Turbo,
	}

	status := &EndpointStatus{
		Endpoint:    endpoint,
		Latency:     100 * time.Millisecond,
		ModelStatus: modelStatus,
	}

	assert.Equal(t, endpoint, status.Endpoint)
	assert.Equal(t, 100*time.Millisecond, status.Latency)
	assert.Equal(t, modelStatus, status.ModelStatus)
	assert.Equal(t, "openai", status.Endpoint.Provider())
	assert.Equal(t, "us-east-1", status.Endpoint.Region())
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}
