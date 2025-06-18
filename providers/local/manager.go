package local

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yanolja/ogem/providers"
	"github.com/yanolja/ogem/types"
)

// LocalProviderManager manages local AI providers
type LocalProviderManager struct {
	providers map[string]providers.Provider
	config    *LocalConfig
	mu        sync.RWMutex
}

// LocalConfig holds configuration for all local providers
type LocalConfig struct {
	Ollama   *OllamaConfig   `json:"ollama,omitempty" yaml:"ollama,omitempty"`
	VLLM     *VLLMConfig     `json:"vllm,omitempty" yaml:"vllm,omitempty"`
	LMStudio *LMStudioConfig `json:"lmstudio,omitempty" yaml:"lmstudio,omitempty"`
	
	// Global settings
	HealthCheckInterval time.Duration `json:"health_check_interval" yaml:"health_check_interval"`
	AutoDiscovery       bool          `json:"auto_discovery" yaml:"auto_discovery"`
	DefaultTimeout      time.Duration `json:"default_timeout" yaml:"default_timeout"`
}

// NewLocalProviderManager creates a new local provider manager
func NewLocalProviderManager(config *LocalConfig) *LocalProviderManager {
	if config == nil {
		config = &LocalConfig{}
	}
	
	if config.HealthCheckInterval == 0 {
		config.HealthCheckInterval = 30 * time.Second
	}
	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 30 * time.Second
	}

	manager := &LocalProviderManager{
		providers: make(map[string]providers.Provider),
		config:    config,
	}

	manager.initializeProviders()
	return manager
}

// initializeProviders sets up all configured local providers
func (m *LocalProviderManager) initializeProviders() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize Ollama if configured
	if m.config.Ollama != nil {
		if m.config.Ollama.Timeout == 0 {
			m.config.Ollama.Timeout = m.config.DefaultTimeout
		}
		ollama := NewOllamaProvider(m.config.Ollama)
		m.providers["ollama"] = ollama
	}

	// Initialize vLLM if configured
	if m.config.VLLM != nil {
		if m.config.VLLM.Timeout == 0 {
			m.config.VLLM.Timeout = m.config.DefaultTimeout
		}
		vllm := NewVLLMProvider(m.config.VLLM)
		m.providers["vllm"] = vllm
	}

	// Initialize LM Studio if configured
	if m.config.LMStudio != nil {
		if m.config.LMStudio.Timeout == 0 {
			m.config.LMStudio.Timeout = m.config.DefaultTimeout
		}
		lmstudio := NewLMStudioProvider(m.config.LMStudio)
		m.providers["lmstudio"] = lmstudio
	}

	// Auto-discovery of local providers
	if m.config.AutoDiscovery {
		m.discoverProviders()
	}
}

// discoverProviders attempts to auto-discover local AI providers
func (m *LocalProviderManager) discoverProviders() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to discover Ollama
	if _, exists := m.providers["ollama"]; !exists {
		ollamaConfig := &OllamaConfig{
			BaseURL: "http://localhost:11434",
			Timeout: m.config.DefaultTimeout,
		}
		ollama := NewOllamaProvider(ollamaConfig)
		if err := ollama.Health(ctx); err == nil {
			m.providers["ollama"] = ollama
		}
	}

	// Try to discover vLLM
	if _, exists := m.providers["vllm"]; !exists {
		vllmConfig := &VLLMConfig{
			BaseURL: "http://localhost:8000",
			Timeout: m.config.DefaultTimeout,
		}
		vllm := NewVLLMProvider(vllmConfig)
		if err := vllm.Health(ctx); err == nil {
			m.providers["vllm"] = vllm
		}
	}

	// Try to discover LM Studio
	if _, exists := m.providers["lmstudio"]; !exists {
		lmstudioConfig := &LMStudioConfig{
			BaseURL: "http://localhost:1234",
			Timeout: m.config.DefaultTimeout,
		}
		lmstudio := NewLMStudioProvider(lmstudioConfig)
		if err := lmstudio.Health(ctx); err == nil {
			m.providers["lmstudio"] = lmstudio
		}
	}
}

// GetProvider returns a specific local provider
func (m *LocalProviderManager) GetProvider(name string) (providers.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	provider, exists := m.providers[name]
	if !exists {
		return nil, fmt.Errorf("local provider %s not found", name)
	}

	return provider, nil
}

// GetProviders returns all available local providers
func (m *LocalProviderManager) GetProviders() map[string]providers.Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]providers.Provider)
	for name, provider := range m.providers {
		result[name] = provider
	}

	return result
}

// GetAllModels returns all models from all local providers
func (m *LocalProviderManager) GetAllModels(ctx context.Context) (map[string][]types.Model, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]types.Model)
	
	for name, provider := range m.providers {
		models, err := provider.GetModels(ctx)
		if err != nil {
			// Log error but continue with other providers
			continue
		}
		result[name] = models
	}

	return result, nil
}

// HealthCheck performs health checks on all providers
func (m *LocalProviderManager) HealthCheck(ctx context.Context) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]error)
	
	for name, provider := range m.providers {
		err := provider.Health(ctx)
		result[name] = err
	}

	return result
}

// StartHealthMonitoring starts periodic health monitoring
func (m *LocalProviderManager) StartHealthMonitoring(ctx context.Context) {
	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.performHealthCheck()
		}
	}
}

// performHealthCheck performs health checks and removes unhealthy providers
func (m *LocalProviderManager) performHealthCheck() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	healthResults := m.HealthCheck(ctx)
	
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, err := range healthResults {
		if err != nil {
			// Remove unhealthy provider
			delete(m.providers, name)
		}
	}

	// Try to rediscover providers if auto-discovery is enabled
	if m.config.AutoDiscovery {
		m.discoverProviders()
	}
}

// AddProvider dynamically adds a provider
func (m *LocalProviderManager) AddProvider(name string, provider providers.Provider) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[name]; exists {
		return fmt.Errorf("provider %s already exists", name)
	}

	m.providers[name] = provider
	return nil
}

// RemoveProvider removes a provider
func (m *LocalProviderManager) RemoveProvider(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.providers[name]; !exists {
		return fmt.Errorf("provider %s not found", name)
	}

	delete(m.providers, name)
	return nil
}

// GetProviderForModel returns the best provider for a given model
func (m *LocalProviderManager) GetProviderForModel(ctx context.Context, modelID string) (providers.Provider, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var lastErr error
	
	// First, try to find exact model match
	for _, provider := range m.providers {
		models, err := provider.GetModels(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		for _, model := range models {
			if model.ID == modelID {
				return provider, nil
			}
		}
	}

	// If no exact match, try pattern matching
	for _, provider := range m.providers {
		models, err := provider.GetModels(ctx)
		if err != nil {
			lastErr = err
			continue
		}

		for _, model := range models {
			// Check if model ID contains the requested model name
			if contains(model.ID, modelID) {
				return provider, nil
			}
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("no local provider found for model %s (last error: %w)", modelID, lastErr)
	}
	return nil, fmt.Errorf("no local provider found for model %s", modelID)
}

// GetStatus returns the status of all local providers
func (m *LocalProviderManager) GetStatus(ctx context.Context) *LocalProviderStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := &LocalProviderStatus{
		TotalProviders: len(m.providers),
		Providers:      make(map[string]*ProviderStatus),
	}

	healthResults := m.HealthCheck(ctx)
	allModels := make(map[string][]types.Model)

	for name, provider := range m.providers {
		providerStatus := &ProviderStatus{
			Name:    name,
			Healthy: healthResults[name] == nil,
		}

		if healthResults[name] != nil {
			providerStatus.Error = healthResults[name].Error()
		}

		// Get models
		if models, err := provider.GetModels(ctx); err == nil {
			providerStatus.Models = models
			providerStatus.ModelCount = len(models)
			allModels[name] = models
		}

		status.Providers[name] = providerStatus
		if providerStatus.Healthy {
			status.HealthyProviders++
		}
	}

	status.AllModels = allModels
	return status
}

// LocalProviderStatus represents the status of all local providers
type LocalProviderStatus struct {
	TotalProviders   int                        `json:"total_providers"`
	HealthyProviders int                        `json:"healthy_providers"`
	Providers        map[string]*ProviderStatus `json:"providers"`
	AllModels        map[string][]types.Model   `json:"all_models"`
}

// ProviderStatus represents the status of a single provider
type ProviderStatus struct {
	Name       string         `json:"name"`
	Healthy    bool           `json:"healthy"`
	Error      string         `json:"error,omitempty"`
	Models     []types.Model  `json:"models,omitempty"`
	ModelCount int            `json:"model_count"`
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || 
		s[:len(substr)] == substr || 
		s[len(s)-len(substr):] == substr)
}