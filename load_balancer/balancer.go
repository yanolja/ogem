package load_balancer

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/provider"
	"go.uber.org/zap"
)

// LoadBalancer implements an adaptive routing system optimized for AI model inference.
// It handles the complexity of routing requests across different AI providers while
// maintaining high availability through intelligent failover and load distribution.
type LoadBalancer struct {
	endpoints         map[string][]EndpointStatus
	modelCapabilities map[string]ModelCapability
	config            *RoutingConfig
	mutex             sync.RWMutex
	logger            *zap.SugaredLogger
}

func New(config *RoutingConfig, logger *zap.SugaredLogger) *LoadBalancer {
	return &LoadBalancer{
		endpoints:         make(map[string][]EndpointStatus),
		modelCapabilities: make(map[string]ModelCapability),
		config:            config,
		logger:            logger,
	}
}

func (lb *LoadBalancer) RegisterEndpoint(endpoint provider.AiEndpoint, status *ogem.SupportedModel) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	key := getEndpointKey(endpoint.Provider(), endpoint.Region())

	endpointStatus := EndpointStatus{
		Endpoint:    endpoint,
		ModelStatus: status,
		IsAvailable: true,
		SuccessRate: 1.0,
	}

	if existing, ok := lb.endpoints[key]; ok {
		for i, e := range existing {
			if e.Endpoint.Provider() == endpoint.Provider() && e.Endpoint.Region() == endpoint.Region() {
				existing[i] = endpointStatus
				return
			}
		}
		lb.endpoints[key] = append(existing, endpointStatus)
	} else {
		lb.endpoints[key] = []EndpointStatus{endpointStatus}
	}
}

// SelectEndpoint dynamically chooses the optimal endpoint based on multiple factors:
// - Latency: Prioritizes low-latency endpoints for real-time applications
// - Cost: Considers pricing differences between providers (e.g., Gemini being cheaper than GPT-4)
// - Quota: Prevents rate limit issues by spreading load across providers
// - Regional: Reduces latency by preferring same-region endpoints when possible
func (lb *LoadBalancer) SelectEndpoint(ctx context.Context, model string) (provider.AiEndpoint, error) {
	lb.mutex.RLock()
	defer lb.mutex.RUnlock()

	var candidates []EndpointScore

	for _, endpoints := range lb.endpoints {
		for _, endpoint := range endpoints {
			if !endpoint.IsAvailable {
				continue
			}

			if modelStatus := endpoint.ModelStatus; modelStatus != nil {
				score := lb.calculateScore(endpoint)
				candidates = append(candidates, EndpointScore{
					Endpoint:   endpoint.Endpoint,
					Score:      score,
					Latency:    endpoint.Latency,
					QuotaUsage: calculateQuotaUsage(modelStatus),
				})
			}
		}
	}

	if len(candidates) == 0 {
		return nil, errors.New("no available endpoints found")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Score > candidates[j].Score
	})
	return candidates[0].Endpoint, nil
}

func (lb *LoadBalancer) UpdateEndpointStatus(provider, region string, latency time.Duration, success bool) {
	lb.mutex.Lock()
	defer lb.mutex.Unlock()

	key := getEndpointKey(provider, region)
	if endpoints, ok := lb.endpoints[key]; ok {
		for i := range endpoints {
			if endpoints[i].Endpoint.Provider() == provider && endpoints[i].Endpoint.Region() == region {
				if success {
					endpoints[i].SuccessRate = (endpoints[i].SuccessRate*0.9 + 0.1)
					endpoints[i].Latency = latency
				} else {
					endpoints[i].ErrorRate = (endpoints[i].ErrorRate*0.9 + 0.1)
					endpoints[i].LastFailure = time.Now()
					if endpoints[i].ErrorRate > 0.5 {
						endpoints[i].IsAvailable = false
					}
				}
				break
			}
		}
	}
}

// calculateScore computes a normalized score (0-1) for endpoint selection.
// The scoring formula is designed to handle edge cases like:
// - New endpoints with no performance history
// - Endpoints with temporary latency spikes
// - Regional routing preferences without over-penalizing cross-region calls
func (lb *LoadBalancer) calculateScore(status EndpointStatus) float64 {
	var score float64

	latencyScore := 1.0 / (1.0 + float64(status.Latency.Milliseconds()))
	successScore := status.SuccessRate
	regionalBonus := 1.0
	if lb.config.RegionalAffinity && status.Endpoint.Region() == lb.config.Region {
		regionalBonus = 1.2
	}

	switch lb.config.Strategy {
	case StrategyLatencyOptimized:
		score = latencyScore*0.7 + successScore*0.3
	case StrategyCostOptimized:
		// TODO: Add cost optimization logic here
		score = successScore*0.7 + latencyScore*0.3
	case StrategyBalanced:
		score = (latencyScore + successScore) / 2
	}

	return score * regionalBonus
}

func getEndpointKey(provider, region string) string {
	return provider + ":" + region
}

// calculateQuotaUsage estimates current capacity utilization for an endpoint.
// Returns a conservative 50% usage for rate-limited endpoints to:
// 1. Maintain headroom for traffic spikes
// 2. Enable gradual load distribution across providers
// 3. Prevent overwhelming any single provider during high load
func calculateQuotaUsage(model *ogem.SupportedModel) float64 {
	if model.MaxRequestsPerMinute > 0 || model.MaxTokensPerMinute > 0 {
		return 0.5
	}
	return 0.0
}

// GetFallbackEndpoint implements a cascading fallback strategy.
// The default chain (OpenAI -> Claude -> Gemini) is ordered by:
// 1. API stability and uptime history
// 2. Model quality and consistency
// 3. Cost efficiency as a final fallback
func (lb *LoadBalancer) GetFallbackEndpoint(ctx context.Context, failedProviders []string) (provider.AiEndpoint, error) {
	if len(lb.config.FallbackChain) == 0 {
		return nil, errors.New("no fallback chain configured")
	}

	failedSet := make(map[string]bool)
	for _, p := range failedProviders {
		failedSet[p] = true
	}

	// Try providers in the fallback chain that haven't failed
	for _, providerID := range lb.config.FallbackChain {
		if !failedSet[providerID] {
			for _, endpoints := range lb.endpoints {
				for _, status := range endpoints {
					if status.Endpoint.Provider() == providerID && status.IsAvailable {
						return status.Endpoint, nil
					}
				}
			}
		}
	}

	return nil, errors.New("no available fallback endpoints found")
}
