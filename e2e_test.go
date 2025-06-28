package ogem_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem/cache"
	"github.com/yanolja/ogem/monitoring"
	"github.com/yanolja/ogem/openai"
	openaiProvider "github.com/yanolja/ogem/provider/openai"
	"github.com/yanolja/ogem/routing"
	"github.com/yanolja/ogem/security"
	"github.com/yanolja/ogem/tenancy"
)

// TestCompleteWorkflow tests the entire OGEM system working together
func TestCompleteWorkflow(t *testing.T) {
	if os.Getenv("OGEM_E2E_TESTS") != "true" {
		t.Skip("End-to-end tests disabled. Set OGEM_E2E_TESTS=true to enable.")
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping e2e test - OPENAI_API_KEY not set")
	}

	logger := zaptest.NewLogger(t).Sugar()
	ctx := context.Background()

	// 1. Initialize monitoring
	monitoringConfig := monitoring.DefaultMonitoringConfig()
	monitor, err := monitoring.NewMonitoringManager(monitoringConfig, logger)
	require.NoError(t, err)
	defer monitor.Close()

	// 2. Initialize tenant management
	tenantConfig := &tenancy.TenantConfig{
		Enabled:              true,
		TrackUsage:           true,
		DefaultTenantType:    tenancy.TenantTypeTeam,
		EnforceLimits:        true,
		SoftLimitThreshold:   0.8,
		UsageResetInterval:   24 * time.Hour,
		CleanupInterval:      1 * time.Hour,
	}
	// Need security manager for tenant manager
	securityConfig := security.DefaultSecurityConfig()
	securityManager, err := security.NewSecurityManager(securityConfig, logger)
	require.NoError(t, err)

	tenantManager, err := tenancy.NewTenantManager(tenantConfig, securityManager, monitor, logger)
	require.NoError(t, err)

	// 3. Initialize caching
	cacheConfig := cache.DefaultCacheConfig()
	cacheConfig.Strategy = cache.StrategyExact
	cacheConfig.EnableMetrics = true
	cacheManager, err := cache.NewCacheManager(cacheConfig, monitor, logger)
	require.NoError(t, err)
	defer cacheManager.Stop()

	// 4. Initialize routing
	routingConfig := routing.DefaultRoutingConfig()
	routingConfig.Strategy = routing.StrategyLatency
	routingConfig.EnableMetrics = true
	router := routing.NewRouter(routingConfig, monitor, logger)

	// 5. Set up provider endpoints
	endpoint, err := openaiProvider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"))
	require.NoError(t, err)
	defer endpoint.Shutdown()

	endpoints := []*routing.EndpointStatus{
		{
			Endpoint: endpoint,
			Latency:  100 * time.Millisecond,
		},
	}

	// 6. Create test tenant
	tenant := &tenancy.Tenant{
		ID:     "test-tenant-e2e",
		Name:   "Test Tenant E2E",
		Status: tenancy.TenantStatusActive,
		Subscription: &tenancy.Subscription{
			PlanName:  "premium",
			Status:    "active",
			StartDate: time.Now(),
			RenewalDate: time.Now().Add(30 * 24 * time.Hour),
		},
		Limits: &tenancy.TenantLimits{
			RequestsPerHour: 3000,
			TokensPerDay:    5000,
			CostPerDay:      5.0,
		},
		CreatedAt: time.Now(),
	}

	err = tenantManager.CreateTenant(ctx, tenant)
	require.NoError(t, err)

	t.Run("complete_request_flow", func(t *testing.T) {
		// Test the complete flow: routing -> provider -> caching -> monitoring
		request := &openai.ChatCompletionRequest{
			Model: "gpt-4.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Hello! Please respond with just 'Hi there!'"),
					},
				},
			},
			MaxTokens:   int32Ptr(10),
			Temperature: float32Ptr(0.1),
		}

		// Step 1: Check tenant access
		accessResult, err := tenantManager.CheckAccess(ctx, tenant.ID, "user-123", "chat.completion", "create")
		require.NoError(t, err)
		require.True(t, accessResult.Allowed)

		// Step 2: Check cache (should miss first time)
		cacheResult, err := cacheManager.Lookup(ctx, request, tenant.ID)
		require.NoError(t, err)
		assert.False(t, cacheResult.Found)

		// Step 3: Route request
		selectedEndpoint, err := router.RouteRequest(ctx, endpoints, request)
		require.NoError(t, err)
		require.NotNil(t, selectedEndpoint)

		// Step 4: Execute request
		start := time.Now()
		response, err := selectedEndpoint.Endpoint.GenerateChatCompletion(ctx, request)
		duration := time.Since(start)
		require.NoError(t, err)
		require.NotNil(t, response)

		// Step 5: Record metrics and usage
		router.RecordRequestResult(selectedEndpoint, duration, 0.001, true, "")

		err = tenantManager.RecordUsage(ctx, tenant.ID, &tenancy.UsageRecord{
			Type:      "request",
			Count:     1,
			Amount:    0.001,
			Model:     request.Model,
			Provider:  "openai",
			Timestamp: time.Now(),
		})
		require.NoError(t, err)

		// Record token usage
		err = tenantManager.RecordUsage(ctx, tenant.ID, &tenancy.UsageRecord{
			Type:      "token",
			Count:     int64(response.Usage.TotalTokens),
			Amount:    0,
			Model:     request.Model,
			Provider:  "openai",
			Timestamp: time.Now(),
		})
		require.NoError(t, err)

		// Step 6: Cache the response
		err = cacheManager.Store(ctx, request, response, tenant.ID)
		require.NoError(t, err)

		// Step 7: Verify cache hit on second request
		cacheResult, err = cacheManager.Lookup(ctx, request, tenant.ID)
		require.NoError(t, err)
		assert.True(t, cacheResult.Found)
		assert.Equal(t, response.Id, cacheResult.Entry.Response.Id)

		// Validate response structure
		assert.NotEmpty(t, response.Id)
		assert.NotEmpty(t, response.Choices)
		assert.Equal(t, "assistant", response.Choices[0].Message.Role)
		assert.NotEmpty(t, *response.Choices[0].Message.Content.String)
	})

	t.Run("rate_limiting", func(t *testing.T) {
		// Test rate limiting functionality
		request := &openai.ChatCompletionRequest{
			Model: "gpt-4.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Rate limit test"),
					},
				},
			},
			MaxTokens: int32Ptr(5),
		}

		// Make requests up to the rate limit
		rateLimitTenant := &tenancy.Tenant{
			ID:     "rate-limit-tenant",
			Name:   "Rate Limit Test Tenant",
			Status: tenancy.TenantStatusActive,
			Limits: &tenancy.TenantLimits{
				RequestsPerHour: 120, // Very low limit for testing (2 per minute)
			},
			CreatedAt: time.Now(),
		}

		err = tenantManager.CreateTenant(ctx, rateLimitTenant)
		require.NoError(t, err)

		// First request should succeed
		accessResult, err := tenantManager.CheckAccess(ctx, rateLimitTenant.ID, "user-456", "chat.completion", "create")
		require.NoError(t, err)
		assert.True(t, accessResult.Allowed)

		// Record usage for first request
		err = tenantManager.RecordUsage(ctx, rateLimitTenant.ID, &tenancy.UsageRecord{
			Type:      "request",
			Count:     1,
			Amount:    0,
			Model:     request.Model,
			Provider:  "openai",
			Timestamp: time.Now(),
		})
		require.NoError(t, err)

		// Second request should succeed
		accessResult, err = tenantManager.CheckAccess(ctx, rateLimitTenant.ID, "user-456", "chat.completion", "create")
		require.NoError(t, err)
		assert.True(t, accessResult.Allowed)

		// Record usage for second request
		err = tenantManager.RecordUsage(ctx, rateLimitTenant.ID, &tenancy.UsageRecord{
			Type:      "request",
			Count:     1,
			Amount:    0,
			Model:     request.Model,
			Provider:  "openai",
			Timestamp: time.Now(),
		})
		require.NoError(t, err)

		// Third request should be rate limited
		accessResult, err = tenantManager.CheckAccess(ctx, rateLimitTenant.ID, "user-456", "chat.completion", "create")
		require.NoError(t, err)
		assert.False(t, accessResult.Allowed)
		assert.Contains(t, accessResult.Reason, "rate limit")
	})

	t.Run("adaptive_routing", func(t *testing.T) {
		// Test adaptive routing with multiple endpoints
		adaptiveConfig := &routing.RoutingConfig{
			Strategy: routing.StrategyAdaptive,
			AdaptiveConfig: &routing.AdaptiveConfig{
				CostThreshold:      0.5,
				LatencyThreshold:   500 * time.Millisecond,
				LoadThreshold:      0.3,
				EvaluationInterval: 1 * time.Second,
				MinSamples:         1,
			},
		}
		adaptiveRouter := routing.NewRouter(adaptiveConfig, monitor, logger)

		request := &openai.ChatCompletionRequest{
			Model: "gpt-4.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Adaptive routing test"),
					},
				},
			},
			MaxTokens: int32Ptr(5),
		}

		// Test routing with adaptive strategy
		selectedEndpoint, err := adaptiveRouter.RouteRequest(ctx, endpoints, request)
		require.NoError(t, err)
		require.NotNil(t, selectedEndpoint)

		// Record some metrics to trigger adaptation
		adaptiveRouter.RecordRequestResult(selectedEndpoint, 200*time.Millisecond, 0.002, true, "")

		// Check routing stats
		stats := adaptiveRouter.GetRoutingStats()
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "strategy")
		assert.Contains(t, stats, "adaptive_state")
	})

	t.Run("cache_strategies", func(t *testing.T) {
		// Test different caching strategies
		strategies := []cache.CacheStrategy{
			cache.StrategyExact,
			cache.StrategySemantic,
			cache.StrategyToken,
		}

		for _, strategy := range strategies {
			t.Run(string(strategy), func(t *testing.T) {
				strategyConfig := cache.DefaultCacheConfig()
				strategyConfig.Strategy = strategy
				strategyConfig.EnableMetrics = false // Disable to avoid conflicts

				strategyManager, err := cache.NewCacheManager(strategyConfig, monitor, logger)
				require.NoError(t, err)
				defer strategyManager.Stop()

				request := &openai.ChatCompletionRequest{
					Model: "gpt-4o",
					Messages: []openai.Message{
						{
							Role: "user",
							Content: &openai.MessageContent{
								String: stringPtr("Cache strategy test for " + string(strategy)),
							},
						},
					},
					MaxTokens: int32Ptr(10),
				}

				// Test cache miss
				result, err := strategyManager.Lookup(ctx, request, tenant.ID)
				require.NoError(t, err)
				assert.False(t, result.Found)

				// Create and store a response
				response := &openai.ChatCompletionResponse{
					Id:      "test-response-" + string(strategy),
					Object:  "chat.completion",
					Created: time.Now().Unix(),
					Model:   request.Model,
					Choices: []openai.Choice{
						{
							Index: 0,
							Message: openai.Message{
								Role: "assistant",
								Content: &openai.MessageContent{
									String: stringPtr("Test response for " + string(strategy)),
								},
							},
							FinishReason: "stop",
						},
					},
				}

				err = strategyManager.Store(ctx, request, response, tenant.ID)
				require.NoError(t, err)

				// Test cache hit
				result, err = strategyManager.Lookup(ctx, request, tenant.ID)
				require.NoError(t, err)
				assert.True(t, result.Found)
				assert.Equal(t, strategy, result.Strategy)
			})
		}
	})

	t.Run("monitoring_integration", func(t *testing.T) {
		// Test monitoring integration with all components
		request := &openai.ChatCompletionRequest{
			Model: "gpt-4.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Monitoring integration test"),
					},
				},
			},
			MaxTokens: int32Ptr(10),
		}

		// Execute request through the full pipeline
		selectedEndpoint, err := router.RouteRequest(ctx, endpoints, request)
		require.NoError(t, err)

		start := time.Now()
		response, err := selectedEndpoint.Endpoint.GenerateChatCompletion(ctx, request)
		duration := time.Since(start)
		require.NoError(t, err)

		// Record metrics
		requestMetrics := &monitoring.RequestMetrics{
			Provider:     selectedEndpoint.Endpoint.Provider(),
			Model:        request.Model,
			Endpoint:     "/v1/chat/completions",
			Method:       "POST",
			StatusCode:   200,
			Duration:     duration,
			InputTokens:  int64(response.Usage.PromptTokens),
			OutputTokens: int64(response.Usage.CompletionTokens),
			TotalTokens:  int64(response.Usage.TotalTokens),
			Cost:         0.001,
			UserID:       "test-user",
			TeamID:       tenant.ID,
			CacheHit:     false,
		}

		err = monitor.RecordRequestMetrics(requestMetrics)
		require.NoError(t, err)

		// Record custom metric
		customMetric := &monitoring.Metric{
			Name:      "e2e_test_completion",
			Type:      monitoring.MetricTypeCounter,
			Value:     1.0,
			Labels:    map[string]string{"test": "e2e", "status": "success"},
			Timestamp: time.Now(),
		}

		err = monitor.RecordMetric(customMetric)
		require.NoError(t, err)

		// Test health metrics
		healthMetrics := &monitoring.HealthMetrics{
			CPUUsage:    45.2,
			MemoryUsage: 67.8,
			DiskUsage:   23.4,
			Uptime:      time.Hour,
			Version:     "1.0.0-test",
		}

		err = monitor.RecordHealthMetrics(healthMetrics)
		require.NoError(t, err)
	})

	t.Run("tenant_isolation", func(t *testing.T) {
		// Test that tenants are properly isolated
		tenant1 := &tenancy.Tenant{
			ID:        "isolation-tenant-1",
			Name:      "Isolation Test Tenant 1",
			Status:    tenancy.TenantStatusActive,
			CreatedAt: time.Now(),
		}

		tenant2 := &tenancy.Tenant{
			ID:        "isolation-tenant-2",
			Name:      "Isolation Test Tenant 2",
			Status:    tenancy.TenantStatusActive,
			CreatedAt: time.Now(),
		}

		err = tenantManager.CreateTenant(ctx, tenant1)
		require.NoError(t, err)

		err = tenantManager.CreateTenant(ctx, tenant2)
		require.NoError(t, err)

		request := &openai.ChatCompletionRequest{
			Model: "gpt-4.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Tenant isolation test"),
					},
				},
			},
			MaxTokens: int32Ptr(10),
		}

		response := &openai.ChatCompletionResponse{
			Id:     "isolation-test-response",
			Object: "chat.completion",
			Model:  request.Model,
			Choices: []openai.Choice{
				{
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: stringPtr("Isolation test response"),
						},
					},
				},
			},
		}

		// Store response for tenant1
		err = cacheManager.Store(ctx, request, response, tenant1.ID)
		require.NoError(t, err)

		// Verify tenant1 can access their cached response
		result, err := cacheManager.Lookup(ctx, request, tenant1.ID)
		require.NoError(t, err)
		assert.True(t, result.Found)

		// Verify tenant2 cannot access tenant1's cached response
		result, err = cacheManager.Lookup(ctx, request, tenant2.ID)
		require.NoError(t, err)
		assert.False(t, result.Found)

		// Test usage tracking isolation
		err = tenantManager.RecordUsage(ctx, tenant1.ID, &tenancy.UsageRecord{
			Type:      "request",
			Count:     1,
			Amount:    0,
			Timestamp: time.Now(),
		})
		require.NoError(t, err)

		err = tenantManager.RecordUsage(ctx, tenant2.ID, &tenancy.UsageRecord{
			Type:      "request",
			Count:     1,
			Amount:    0,
			Timestamp: time.Now(),
		})
		require.NoError(t, err)

		// Verify usage stats are isolated
		stats1, err := tenantManager.GetUsageMetrics(ctx, tenant1.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stats1.RequestsThisHour, int64(1))

		stats2, err := tenantManager.GetUsageMetrics(ctx, tenant2.ID)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, stats2.RequestsThisHour, int64(1))
	})

	// Cleanup
	t.Cleanup(func() {
		tenantManager.DeleteTenant(ctx, tenant.ID)
		cacheManager.Clear()
		monitor.Flush()
	})
}

// TestSystemResilience tests the system's ability to handle failures gracefully
func TestSystemResilience(t *testing.T) {
	if os.Getenv("OGEM_E2E_TESTS") != "true" {
		t.Skip("End-to-end tests disabled. Set OGEM_E2E_TESTS=true to enable.")
	}

	logger := zaptest.NewLogger(t).Sugar()
	ctx := context.Background()

	t.Run("cache_failure_graceful_degradation", func(t *testing.T) {
		// Test that the system continues to work even if cache fails
		config := cache.DefaultCacheConfig()
		config.Strategy = cache.StrategyExact
		cacheManager, err := cache.NewCacheManager(config, nil, logger)
		require.NoError(t, err)

		request := &openai.ChatCompletionRequest{
			Model: "gpt-4.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Resilience test"),
					},
				},
			},
		}

		// Cache lookup should not fail even with disabled cache
		result, err := cacheManager.Lookup(ctx, request, "test-tenant")
		assert.NoError(t, err)
		assert.False(t, result.Found)

		cacheManager.Stop()
	})

	t.Run("monitoring_failure_graceful_degradation", func(t *testing.T) {
		// Test that the system continues to work even if monitoring fails
		disabledConfig := &monitoring.MonitoringConfig{
			Enabled: false,
		}
		monitor, err := monitoring.NewMonitoringManager(disabledConfig, logger)
		require.NoError(t, err)

		// Operations should succeed even with disabled monitoring
		err = monitor.RecordMetric(&monitoring.Metric{
			Name:  "test_metric",
			Type:  monitoring.MetricTypeCounter,
			Value: 1.0,
		})
		assert.NoError(t, err)

		err = monitor.RecordRequestMetrics(&monitoring.RequestMetrics{
			Provider:   "test",
			Model:      "test-model",
			StatusCode: 200,
			Duration:   100 * time.Millisecond,
		})
		assert.NoError(t, err)

		monitor.Close()
	})

	t.Run("tenant_service_failure", func(t *testing.T) {
		// Test behavior when tenant service is unavailable
		config := &tenancy.TenantConfig{
			Enabled: false, // Disable tenancy
			TrackUsage: false, // Disable tracking
		}
		tenantManager, err := tenancy.NewTenantManager(config, nil, nil, logger)
		require.NoError(t, err)

		// Operations should still work with minimal tenant functionality
		accessResult, err := tenantManager.CheckAccess(ctx, "any-tenant", "any-user", "any-resource", "any-action")
		assert.NoError(t, err)
		assert.True(t, accessResult.Allowed) // Should allow when isolation is disabled
	})
}

// TestPerformanceBenchmarks runs basic performance tests
func TestPerformanceBenchmarks(t *testing.T) {
	if os.Getenv("OGEM_E2E_TESTS") != "true" {
		t.Skip("End-to-end tests disabled. Set OGEM_E2E_TESTS=true to enable.")
	}

	logger := zaptest.NewLogger(t).Sugar()

	t.Run("cache_performance", func(t *testing.T) {
		config := cache.DefaultCacheConfig()
		config.Strategy = cache.StrategyExact
		config.MaxEntries = 1000
		cacheManager, err := cache.NewCacheManager(config, nil, logger)
		require.NoError(t, err)
		defer cacheManager.Stop()

		ctx := context.Background()
		tenantID := "perf-test-tenant"

		// Pre-populate cache
		for i := 0; i < 100; i++ {
			request := &openai.ChatCompletionRequest{
				Model: "gpt-4o",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr(fmt.Sprintf("Performance test message %d", i)),
						},
					},
				},
			}

			response := &openai.ChatCompletionResponse{
				Id:    fmt.Sprintf("perf-response-%d", i),
				Model: request.Model,
				Choices: []openai.Choice{
					{
						Message: openai.Message{
							Role: "assistant",
							Content: &openai.MessageContent{
								String: stringPtr(fmt.Sprintf("Response %d", i)),
							},
						},
					},
				},
			}

			err = cacheManager.Store(ctx, request, response, tenantID)
			require.NoError(t, err)
		}

		// Benchmark cache lookups
		start := time.Now()
		hits := 0
		misses := 0

		for i := 0; i < 1000; i++ {
			request := &openai.ChatCompletionRequest{
				Model: "gpt-4o",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr(fmt.Sprintf("Performance test message %d", i%100)),
						},
					},
				},
			}

			result, err := cacheManager.Lookup(ctx, request, tenantID)
			require.NoError(t, err)

			if result.Found {
				hits++
			} else {
				misses++
			}
		}

		duration := time.Since(start)
		t.Logf("Cache performance: 1000 lookups in %v, %d hits, %d misses", duration, hits, misses)
		assert.Less(t, duration, 1*time.Second, "1000 cache lookups should complete in less than 1 second")
		assert.Greater(t, float64(hits)/1000, 0.9, "Cache hit rate should be > 90%")
	})
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func float32Ptr(f float32) *float32 {
	return &f
}
