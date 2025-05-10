package load_balancer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/openai"
	"go.uber.org/zap"
)

// MockAiEndpoint implements provider.AiEndpoint for testing
type MockAiEndpoint struct {
	mock.Mock
}

func (m *MockAiEndpoint) Provider() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockAiEndpoint) Region() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockAiEndpoint) BaseUrl() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockAiEndpoint) GenerateChatCompletion(ctx context.Context, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	args := m.Called(ctx, request)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*openai.ChatCompletionResponse), args.Error(1)
}

func (m *MockAiEndpoint) Ping(ctx context.Context) (time.Duration, error) {
	args := m.Called(ctx)
	return args.Get(0).(time.Duration), args.Error(1)
}

func (m *MockAiEndpoint) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

func TestNew(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := &RoutingConfig{
		Strategy:         StrategyBalanced,
		LatencyWeight:    0.4,
		CostWeight:       0.3,
		QuotaWeight:      0.3,
		RegionalAffinity: true,
		Region:          "us-east",
	}

	lb := New(config, logger)
	assert.NotNil(t, lb)
	assert.Equal(t, config, lb.config)
	assert.NotNil(t, lb.endpoints)
	assert.NotNil(t, lb.modelCapabilities)
}

func TestRegisterEndpoint(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := &RoutingConfig{Strategy: StrategyBalanced}
	lb := New(config, logger)

	mockEndpoint := new(MockAiEndpoint)
	mockEndpoint.On("Provider").Return("openai")
	mockEndpoint.On("Region").Return("us-east")
	mockEndpoint.On("Ping", mock.Anything).Return(time.Duration(50*time.Millisecond), nil)
	mockEndpoint.On("Shutdown").Return(nil)

	modelStatus := &ogem.SupportedModel{
		Name:                "gpt-4",
		MaxRequestsPerMinute: 100,
	}

	// Test registering new endpoint
	lb.RegisterEndpoint(mockEndpoint, modelStatus)
	
	key := getEndpointKey("openai", "us-east")
	endpoints, exists := lb.endpoints[key]
	assert.True(t, exists)
	assert.Len(t, endpoints, 1)
	assert.Equal(t, mockEndpoint, endpoints[0].Endpoint)
	assert.Equal(t, modelStatus, endpoints[0].ModelStatus)
	assert.True(t, endpoints[0].IsAvailable)
	assert.Equal(t, 1.0, endpoints[0].SuccessRate)

	// Test updating existing endpoint
	updatedStatus := &ogem.SupportedModel{
		Name:                "gpt-4",
		MaxRequestsPerMinute: 200,
	}
	lb.RegisterEndpoint(mockEndpoint, updatedStatus)
	
	endpoints, exists = lb.endpoints[key]
	assert.True(t, exists)
	assert.Len(t, endpoints, 1)
	assert.Equal(t, updatedStatus, endpoints[0].ModelStatus)
}

func TestSelectEndpoint(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := &RoutingConfig{
		Strategy:         StrategyLatencyOptimized,
		RegionalAffinity: true,
		Region:          "us-east",
	}
	lb := New(config, logger)

	// Create mock endpoints with different characteristics
	fastEndpoint := new(MockAiEndpoint)
	fastEndpoint.On("Provider").Return("openai")
	fastEndpoint.On("Region").Return("us-east")
	fastEndpoint.On("Ping", mock.Anything).Return(time.Duration(50*time.Millisecond), nil)
	fastEndpoint.On("Shutdown").Return(nil)

	slowEndpoint := new(MockAiEndpoint)
	slowEndpoint.On("Provider").Return("anthropic")
	slowEndpoint.On("Region").Return("us-west")
	slowEndpoint.On("Ping", mock.Anything).Return(time.Duration(200*time.Millisecond), nil)
	slowEndpoint.On("Shutdown").Return(nil)

	// Register endpoints with different latencies
	lb.RegisterEndpoint(fastEndpoint, &ogem.SupportedModel{Name: "gpt-4"})
	lb.RegisterEndpoint(slowEndpoint, &ogem.SupportedModel{Name: "claude-2"})

	// Update status to simulate different performance characteristics
	lb.UpdateEndpointStatus("openai", "us-east", 50*time.Millisecond, true)
	lb.UpdateEndpointStatus("anthropic", "us-west", 200*time.Millisecond, true)

	// Test endpoint selection
	selected, err := lb.SelectEndpoint(context.Background(), "gpt-4")
	assert.NoError(t, err)
	assert.Equal(t, fastEndpoint, selected)
}

func TestGetFallbackEndpoint(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := &RoutingConfig{
		FallbackChain: []string{"openai", "anthropic", "google"},
	}
	lb := New(config, logger)

	// Create mock endpoints
	openaiEndpoint := new(MockAiEndpoint)
	openaiEndpoint.On("Provider").Return("openai")
	openaiEndpoint.On("Region").Return("us-east")
	openaiEndpoint.On("Ping", mock.Anything).Return(time.Duration(50*time.Millisecond), nil)
	openaiEndpoint.On("Shutdown").Return(nil)

	claudeEndpoint := new(MockAiEndpoint)
	claudeEndpoint.On("Provider").Return("anthropic")
	claudeEndpoint.On("Region").Return("us-east")
	claudeEndpoint.On("Ping", mock.Anything).Return(time.Duration(100*time.Millisecond), nil)
	claudeEndpoint.On("Shutdown").Return(nil)

	// Register endpoints
	lb.RegisterEndpoint(openaiEndpoint, &ogem.SupportedModel{Name: "gpt-4"})
	lb.RegisterEndpoint(claudeEndpoint, &ogem.SupportedModel{Name: "claude-2"})

	// Test fallback when OpenAI fails
	endpoint, err := lb.GetFallbackEndpoint(context.Background(), []string{"openai"})
	assert.NoError(t, err)
	assert.Equal(t, "anthropic", endpoint.Provider())

	// Test fallback when all providers fail
	endpoint, err = lb.GetFallbackEndpoint(context.Background(), []string{"openai", "anthropic", "google"})
	assert.Error(t, err)
	assert.Nil(t, endpoint)
}

func TestCalculateScore(t *testing.T) {
	logger := zap.NewNop().Sugar()
	config := &RoutingConfig{
		Strategy:         StrategyBalanced,
		RegionalAffinity: true,
		Region:          "us-east",
	}
	lb := New(config, logger)

	mockEndpoint := new(MockAiEndpoint)
	mockEndpoint.On("Provider").Return("openai")
	mockEndpoint.On("Region").Return("us-east")
	mockEndpoint.On("Ping", mock.Anything).Return(time.Duration(50*time.Millisecond), nil)
	mockEndpoint.On("Shutdown").Return(nil)

	tests := []struct {
		name     string
		status   EndpointStatus
		strategy RoutingStrategy
		want     float64
	}{
		{
			name: "Perfect endpoint same region",
			status: EndpointStatus{
				Endpoint:    mockEndpoint,
				Latency:    50 * time.Millisecond,
				SuccessRate: 1.0,
				IsAvailable: true,
			},
			strategy: StrategyBalanced,
			want:     0.61, // Normalized latency score with regional bonus
		},
		{
			name: "High latency endpoint",
			status: EndpointStatus{
				Endpoint:    mockEndpoint,
				Latency:    500 * time.Millisecond,
				SuccessRate: 1.0,
				IsAvailable: true,
			},
			strategy: StrategyLatencyOptimized,
			want:     0.36, // Lower score due to higher latency
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lb.config.Strategy = tt.strategy
			got := lb.calculateScore(tt.status)
			assert.InDelta(t, tt.want, got, 0.1)
		})
	}
}

func TestCalculateQuotaUsage(t *testing.T) {
	tests := []struct {
		name  string
		model *ogem.SupportedModel
		want  float64
	}{
		{
			name: "Model with RPM limit",
			model: &ogem.SupportedModel{
				MaxRequestsPerMinute: 100,
			},
			want: 0.5,
		},
		{
			name: "Model with TPM limit",
			model: &ogem.SupportedModel{
				MaxTokensPerMinute: 40000,
			},
			want: 0.5,
		},
		{
			name:  "Model without limits",
			model: &ogem.SupportedModel{},
			want:  0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateQuotaUsage(tt.model)
			assert.Equal(t, tt.want, got)
		})
	}
}
