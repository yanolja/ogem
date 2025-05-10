package load_balancer

import (
	"time"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/provider"
)

type EndpointScore struct {
	Endpoint    provider.AiEndpoint
	Score       float64
	Latency     time.Duration
	CostPerCall float64
	QuotaUsage  float64
}

// RoutingStrategy defines how requests should be distributed
type RoutingStrategy int

const (
	StrategyLatencyOptimized RoutingStrategy = iota
	StrategyCostOptimized
	StrategyBalanced
)

// RoutingConfig holds configuration for the load balancer
type RoutingConfig struct {
	Strategy         RoutingStrategy
	LatencyWeight    float64
	CostWeight       float64
	QuotaWeight      float64
	RegionalAffinity bool
	Region           string
	FallbackChain    []string // List of provider IDs to try in order
}

type ModelCapability struct {
	Name           string
	MaxTokens      int
	InputCost      float64
	OutputCost     float64
	Specialization []string
	Provider       string
}

type EndpointStatus struct {
	Endpoint    provider.AiEndpoint
	Latency     time.Duration
	ModelStatus *ogem.SupportedModel
	SuccessRate float64
	ErrorRate   float64
	LastFailure time.Time
	IsAvailable bool
}
