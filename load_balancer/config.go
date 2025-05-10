package load_balancer

import (
	"os"

	"github.com/yanolja/ogem/config"
)

// LoadBalancerConfig extends the main config with load balancing settings
type LoadBalancerConfig struct {
	DefaultStrategy RoutingStrategy `yaml:"default_strategy"`
	Weights         struct {
		Latency float64 `yaml:"latency"`
		Cost    float64 `yaml:"cost"`
		Quota   float64 `yaml:"quota"`
	} `yaml:"weights"`

	Regional struct {
		Enabled bool   `yaml:"enabled"`
		Region  string `yaml:"region"`
	} `yaml:"regional"`
	Fallback struct {
		Enabled bool     `yaml:"enabled"`
		Chain   []string `yaml:"chain"`
	} `yaml:"fallback"`

	ModelCapabilities map[string]ModelCapability `yaml:"model_capabilities"`
}

func NewConfigFromGlobal(cfg *config.Config) *RoutingConfig {
	return &RoutingConfig{
		Strategy:         StrategyBalanced,
		LatencyWeight:    0.4,
		CostWeight:       0.3,
		QuotaWeight:      0.3,
		RegionalAffinity: true,
		Region:           os.Getenv("OGEM_REGION"),
		FallbackChain:    []string{"openai", "anthropic", "google"},
	}
}

func DefaultModelCapabilities() map[string]ModelCapability {
	return map[string]ModelCapability{
		"gpt-4": {
			Name:           "gpt-4",
			MaxTokens:      8192,
			InputCost:      0.03,
			OutputCost:     0.06,
			Specialization: []string{"code", "reasoning", "math"},
			Provider:       "openai",
		},
		"gpt-3.5-turbo": {
			Name:           "gpt-3.5-turbo",
			MaxTokens:      4096,
			InputCost:      0.0015,
			OutputCost:     0.002,
			Specialization: []string{"general", "chat"},
			Provider:       "openai",
		},
		"claude-2": {
			Name:           "claude-2",
			MaxTokens:      100000,
			InputCost:      0.01102,
			OutputCost:     0.03268,
			Specialization: []string{"analysis", "writing", "math"},
			Provider:       "anthropic",
		},
		"gemini-pro": {
			Name:           "gemini-pro",
			MaxTokens:      32768,
			InputCost:      0.00025,
			OutputCost:     0.0005,
			Specialization: []string{"general", "code", "math"},
			Provider:       "google",
		},
	}
}
