package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem/openai"
)

// Mock endpoint for testing
type mockEndpoint struct {
	provider string
	region   string
}

func (m *mockEndpoint) Provider() string { return m.provider }
func (m *mockEndpoint) Region() string   { return m.region }
func (m *mockEndpoint) Ping(ctx context.Context) (time.Duration, error) {
	return 100 * time.Millisecond, nil
}
func (m *mockEndpoint) GenerateChatCompletion(ctx context.Context, req *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) GenerateChatCompletionStream(ctx context.Context, req *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	return nil, nil
}
func (m *mockEndpoint) GenerateEmbedding(ctx context.Context, req *openai.EmbeddingRequest) (*openai.EmbeddingResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) GenerateImage(ctx context.Context, req *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) TranscribeAudio(ctx context.Context, req *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) TranslateAudio(ctx context.Context, req *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) GenerateSpeech(ctx context.Context, req *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) ModerateContent(ctx context.Context, req *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, nil
}
func (m *mockEndpoint) CreateFineTuningJob(ctx context.Context, req *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, nil
}
func (m *mockEndpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, nil
}
func (m *mockEndpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, nil
}
func (m *mockEndpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, nil
}
func (m *mockEndpoint) Shutdown() error { return nil }

func TestNewAPIHandler(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)

	handler := NewAPIHandler(router, logger)

	assert.NotNil(t, handler)
	assert.Equal(t, router, handler.router)
	assert.Equal(t, logger, handler.logger)
}

func TestAPIHandler_HandleRoutingStats(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	tests := []struct {
		name           string
		method         string
		expectedStatus int
	}{
		{
			name:           "GET request",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST request (not allowed)",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT request (not allowed)",
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/v1/routing/stats", nil)
			recorder := httptest.NewRecorder()

			handler.HandleRoutingStats(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var stats map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &stats)
				assert.NoError(t, err)
				assert.Contains(t, stats, "strategy")
				assert.Contains(t, stats, "config")
			}
		})
	}
}

func TestAPIHandler_HandleEndpointMetrics(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	// Add some test metrics
	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}
	router.RecordRequestResult(endpoint, 100*time.Millisecond, 0.001, true, "")

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		checkContent   bool
	}{
		{
			name:           "GET all endpoints",
			method:         http.MethodGet,
			path:           "/v1/routing/endpoints/",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
		{
			name:           "GET specific endpoint with metrics",
			method:         http.MethodGet,
			path:           "/v1/routing/endpoints/openai/us-east-1",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
		{
			name:           "GET specific endpoint without metrics",
			method:         http.MethodGet,
			path:           "/v1/routing/endpoints/nonexistent/region",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
		{
			name:           "POST request (not allowed)",
			method:         http.MethodPost,
			path:           "/v1/routing/endpoints/",
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "Invalid path format",
			method:         http.MethodGet,
			path:           "/v1/routing/endpoints/openai",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			recorder := httptest.NewRecorder()

			handler.HandleEndpointMetrics(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedStatus == http.StatusOK && tt.checkContent {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err)

				if strings.Contains(tt.path, "openai/us-east-1") {
					assert.Contains(t, response, "openai/us-east-1")
					endpointData := response["openai/us-east-1"]
					if endpointData != nil {
						// Should contain actual metrics
						metrics, ok := endpointData.(*EndpointMetrics)
						if ok {
							assert.Greater(t, metrics.TotalRequests, int64(0))
						}
					}
				} else if strings.Contains(tt.path, "nonexistent/region") {
					assert.Contains(t, response, "nonexistent/region")
					endpointData := response["nonexistent/region"].(map[string]interface{})
					assert.Equal(t, "no_metrics_available", endpointData["status"])
				} else {
					// All endpoints or invalid format
					assert.Contains(t, response, "message")
				}
			}
		})
	}
}

func TestAPIHandler_HandleUpdateRoutingConfig(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	tests := []struct {
		name           string
		method         string
		body           interface{}
		expectedStatus int
		checkResponse  bool
	}{
		{
			name:   "valid config update",
			method: http.MethodPatch,
			body: map[string]interface{}{
				"strategy":            "cost",
				"fallback_strategy":   "latency",
				"cost_weight":         0.5,
				"latency_weight":      0.3,
				"success_rate_weight": 0.2,
				"endpoint_weights": map[string]float64{
					"openai/us-east-1":    2.0,
					"anthropic/us-west-2": 1.0,
				},
			},
			expectedStatus: http.StatusOK,
			checkResponse:  true,
		},
		{
			name:   "partial config update",
			method: http.MethodPatch,
			body: map[string]interface{}{
				"strategy": "round_robin",
			},
			expectedStatus: http.StatusOK,
			checkResponse:  true,
		},
		{
			name:           "GET request (not allowed)",
			method:         http.MethodGet,
			body:           nil,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "invalid JSON body",
			method:         http.MethodPatch,
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty body",
			method:         http.MethodPatch,
			body:           map[string]interface{}{},
			expectedStatus: http.StatusOK,
			checkResponse:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var reqBody *bytes.Buffer
			if tt.body != nil {
				if bodyStr, ok := tt.body.(string); ok {
					reqBody = bytes.NewBufferString(bodyStr)
				} else {
					bodyBytes, err := json.Marshal(tt.body)
					require.NoError(t, err)
					reqBody = bytes.NewBuffer(bodyBytes)
				}
			} else {
				reqBody = bytes.NewBuffer(nil)
			}

			req := httptest.NewRequest(tt.method, "/v1/routing/config", reqBody)
			recorder := httptest.NewRecorder()

			handler.HandleUpdateRoutingConfig(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedStatus == http.StatusOK && tt.checkResponse {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "updated", response["status"])
				assert.Contains(t, response, "config")

				// Verify weights are normalized if they were updated
				if configBody, ok := tt.body.(map[string]interface{}); ok {
					if _, hasCostWeight := configBody["cost_weight"]; hasCostWeight {
						configMap, ok := response["config"].(map[string]interface{})
						assert.True(t, ok, "config should be a map")

						costWeight, _ := configMap["CostWeight"].(float64)
						latencyWeight, _ := configMap["LatencyWeight"].(float64)
						successRateWeight, _ := configMap["SuccessRateWeight"].(float64)
						loadWeight, _ := configMap["LoadWeight"].(float64)

						totalWeight := costWeight + latencyWeight + successRateWeight + loadWeight
						assert.InDelta(t, 1.0, totalWeight, 0.001)
					}
				}
			}
		})
	}
}

func TestAPIHandler_HandleRoutingHealth(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()

	tests := []struct {
		name           string
		router         *Router
		method         string
		expectedStatus int
		checkHealth    bool
	}{
		{
			name:           "healthy router",
			router:         NewRouter(nil, nil, logger),
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			checkHealth:    true,
		},
		{
			name: "adaptive router",
			router: NewRouter(&RoutingConfig{
				Strategy: StrategyAdaptive,
			}, nil, logger),
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			checkHealth:    true,
		},
		{
			name:           "nil router",
			router:         nil,
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			checkHealth:    true,
		},
		{
			name:           "POST request (not allowed)",
			router:         NewRouter(nil, nil, logger),
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewAPIHandler(tt.router, logger)

			req := httptest.NewRequest(tt.method, "/v1/routing/health", nil)
			recorder := httptest.NewRecorder()

			handler.HandleRoutingHealth(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedStatus == http.StatusOK && tt.checkHealth {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var health map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &health)
				assert.NoError(t, err)

				assert.Contains(t, health, "status")
				assert.Equal(t, "healthy", health["status"])
				assert.Contains(t, health, "router_initialized")

				if tt.router != nil {
					assert.True(t, health["router_initialized"].(bool))
					assert.Contains(t, health, "strategy")
					assert.Contains(t, health, "monitoring_enabled")

					if tt.router.adaptiveState != nil {
						assert.Contains(t, health, "adaptive_strategy")
						assert.Contains(t, health, "last_evaluation")
						assert.Contains(t, health, "strategy_changes")
					}
				} else {
					assert.False(t, health["router_initialized"].(bool))
				}
			}
		})
	}
}

func TestAPIHandler_HandleCircuitBreakerStatus(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	// Add some test metrics with circuit breaker data
	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}
	router.RecordRequestResult(endpoint, 100*time.Millisecond, 0.001, false, "error")

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		checkContent   bool
	}{
		{
			name:           "GET specific endpoint with circuit breaker data",
			method:         http.MethodGet,
			path:           "/v1/routing/circuit-breakers/openai/us-east-1",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
		{
			name:           "GET endpoint without circuit breaker data",
			method:         http.MethodGet,
			path:           "/v1/routing/circuit-breakers/nonexistent/region",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
		{
			name:           "GET invalid path",
			method:         http.MethodGet,
			path:           "/v1/routing/circuit-breakers/",
			expectedStatus: http.StatusOK,
			checkContent:   true,
		},
		{
			name:           "POST request (not allowed)",
			method:         http.MethodPost,
			path:           "/v1/routing/circuit-breakers/openai/us-east-1",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			recorder := httptest.NewRecorder()

			handler.HandleCircuitBreakerStatus(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedStatus == http.StatusOK && tt.checkContent {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err)

				if strings.Contains(tt.path, "openai/us-east-1") {
					assert.Contains(t, response, "openai/us-east-1")
					circuitData := response["openai/us-east-1"].(map[string]interface{})
					assert.Contains(t, circuitData, "circuit_state")
					assert.Contains(t, circuitData, "consecutive_failures")
					assert.Contains(t, circuitData, "consecutive_successes")
					assert.Contains(t, circuitData, "last_failure_time")
				} else if strings.Contains(tt.path, "nonexistent/region") {
					assert.Contains(t, response, "nonexistent/region")
					circuitData := response["nonexistent/region"].(map[string]interface{})
					assert.Equal(t, "no_circuit_breaker_data", circuitData["status"])
				} else {
					assert.Contains(t, response, "message")
				}
			}
		})
	}
}

func TestAPIHandler_HandleResetCircuitBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	// Add metrics for an endpoint with failed requests
	endpoint := &EndpointStatus{
		Endpoint: &mockEndpoint{provider: "openai", region: "us-east-1"},
	}
	for i := 0; i < 3; i++ {
		router.RecordRequestResult(endpoint, 100*time.Millisecond, 0.001, false, "error")
	}

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		checkResponse  bool
	}{
		{
			name:           "reset existing circuit breaker",
			method:         http.MethodPost,
			path:           "/v1/routing/circuit-breakers/openai/us-east-1/reset",
			expectedStatus: http.StatusOK,
			checkResponse:  true,
		},
		{
			name:           "reset non-existent endpoint",
			method:         http.MethodPost,
			path:           "/v1/routing/circuit-breakers/nonexistent/region/reset",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "invalid path format",
			method:         http.MethodPost,
			path:           "/v1/routing/circuit-breakers/openai/reset",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "missing reset action",
			method:         http.MethodPost,
			path:           "/v1/routing/circuit-breakers/openai/us-east-1",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "GET request (not allowed)",
			method:         http.MethodGet,
			path:           "/v1/routing/circuit-breakers/openai/us-east-1/reset",
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			recorder := httptest.NewRecorder()

			handler.HandleResetCircuitBreaker(recorder, req)

			assert.Equal(t, tt.expectedStatus, recorder.Code)

			if tt.expectedStatus == http.StatusOK && tt.checkResponse {
				assert.Equal(t, "application/json", recorder.Header().Get("Content-Type"))

				var response map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err)

				assert.Equal(t, "reset", response["status"])
				assert.Equal(t, "openai/us-east-1", response["endpoint"])
				assert.Equal(t, "closed", response["circuit_state"])

				// Verify circuit breaker was actually reset
				endpointKey := "openai/us-east-1"
				metrics := router.endpointMetrics[endpointKey]
				assert.Equal(t, CircuitClosed, metrics.CircuitState)
				assert.Equal(t, 0, metrics.ConsecutiveFailures)
				assert.Equal(t, 0, metrics.ConsecutiveSuccesses)
			}
		})
	}
}

func TestAPIHandler_RegisterRoutes(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test that routes are registered by making requests
	testCases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/v1/routing/stats"},
		{http.MethodGet, "/v1/routing/health"},
		{http.MethodPatch, "/v1/routing/config"},
		{http.MethodGet, "/v1/routing/endpoints/openai/us-east-1"},
		{http.MethodGet, "/v1/routing/circuit-breakers/openai/us-east-1"},
		{http.MethodPost, "/v1/routing/circuit-breakers/openai/us-east-1/reset"},
	}

	for _, tc := range testCases {
		t.Run(tc.method+"_"+tc.path, func(t *testing.T) {
			var reqBody *bytes.Buffer
			if tc.method == http.MethodPatch {
				reqBody = bytes.NewBufferString("{}")
			} else {
				reqBody = bytes.NewBuffer(nil)
			}

			req := httptest.NewRequest(tc.method, tc.path, reqBody)
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, req)

			// Should not return 404 (route not found)
			assert.NotEqual(t, http.StatusNotFound, recorder.Code)
		})
	}
}

func TestDummyEndpoint(t *testing.T) {
	dummy := &dummyEndpoint{
		provider: "test-provider",
		region:   "test-region",
	}

	assert.Equal(t, "test-provider", dummy.Provider())
	assert.Equal(t, "test-region", dummy.Region())

	// Test that all methods exist and don't panic
	ctx := context.Background()

	latency, err := dummy.Ping(ctx)
	assert.Equal(t, time.Duration(0), latency)
	assert.NoError(t, err)

	chatResp, err := dummy.GenerateChatCompletion(ctx, nil)
	assert.Nil(t, chatResp)
	assert.NoError(t, err)

	chatChan, errChan := dummy.GenerateChatCompletionStream(ctx, nil)
	assert.Nil(t, chatChan)
	assert.Nil(t, errChan)

	embResp, err := dummy.GenerateEmbedding(ctx, nil)
	assert.Nil(t, embResp)
	assert.NoError(t, err)

	imgResp, err := dummy.GenerateImage(ctx, nil)
	assert.Nil(t, imgResp)
	assert.NoError(t, err)

	audioResp, err := dummy.TranscribeAudio(ctx, nil)
	assert.Nil(t, audioResp)
	assert.NoError(t, err)

	transResp, err := dummy.TranslateAudio(ctx, nil)
	assert.Nil(t, transResp)
	assert.NoError(t, err)

	speechResp, err := dummy.GenerateSpeech(ctx, nil)
	assert.Nil(t, speechResp)
	assert.NoError(t, err)

	modResp, err := dummy.ModerateContent(ctx, nil)
	assert.Nil(t, modResp)
	assert.NoError(t, err)

	ftJob, err := dummy.CancelFineTuningJob(ctx, "test-job")
	assert.Nil(t, ftJob)
	assert.NoError(t, err)

	err = dummy.Shutdown()
	assert.NoError(t, err)
}

func TestAPIHandler_EdgeCases(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	// Test with malformed JSON in config update
	t.Run("malformed JSON config update", func(t *testing.T) {
		reqBody := bytes.NewBufferString(`{"strategy": "invalid_strategy", "cost_weight": "not_a_number"}`)
		req := httptest.NewRequest(http.MethodPatch, "/v1/routing/config", reqBody)
		recorder := httptest.NewRecorder()

		handler.HandleUpdateRoutingConfig(recorder, req)

		// Should handle gracefully - the JSON will unmarshal with string value
		// but the type assertion will use the default value
		assert.Equal(t, http.StatusOK, recorder.Code)
	})

	// Test endpoint metrics with empty path segments
	t.Run("endpoint metrics with empty segments", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/routing/endpoints//", nil)
		recorder := httptest.NewRecorder()

		handler.HandleEndpointMetrics(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "message")
	})

	// Test circuit breaker status with partial path
	t.Run("circuit breaker status partial path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/routing/circuit-breakers/openai", nil)
		recorder := httptest.NewRecorder()

		handler.HandleCircuitBreakerStatus(recorder, req)

		assert.Equal(t, http.StatusOK, recorder.Code)

		var response map[string]interface{}
		err := json.Unmarshal(recorder.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Contains(t, response, "message")
	})

	// Test reset circuit breaker with extra path segments
	t.Run("reset circuit breaker extra segments", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/routing/circuit-breakers/openai/us-east-1/reset/extra", nil)
		recorder := httptest.NewRecorder()

		handler.HandleResetCircuitBreaker(recorder, req)

		assert.Equal(t, http.StatusBadRequest, recorder.Code)
	})
}

func TestAPIHandler_ConcurrentAccess(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	// Test concurrent access to different endpoints
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()

			// Make requests to different endpoints concurrently
			endpoints := []string{
				"/v1/routing/stats",
				"/v1/routing/health",
				"/v1/routing/endpoints/openai/us-east-1",
				"/v1/routing/circuit-breakers/openai/us-east-1",
			}

			for _, endpoint := range endpoints {
				req := httptest.NewRequest(http.MethodGet, endpoint, nil)
				recorder := httptest.NewRecorder()

				switch {
				case strings.Contains(endpoint, "stats"):
					handler.HandleRoutingStats(recorder, req)
				case strings.Contains(endpoint, "health"):
					handler.HandleRoutingHealth(recorder, req)
				case strings.Contains(endpoint, "endpoints"):
					handler.HandleEndpointMetrics(recorder, req)
				case strings.Contains(endpoint, "circuit-breakers"):
					handler.HandleCircuitBreakerStatus(recorder, req)
				}

				assert.Equal(t, http.StatusOK, recorder.Code)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestAPIHandler_Configuration_Validation(t *testing.T) {
	logger := zaptest.NewLogger(t).Sugar()
	router := NewRouter(nil, nil, logger)
	handler := NewAPIHandler(router, logger)

	tests := []struct {
		name   string
		config map[string]interface{}
		valid  bool
	}{
		{
			name: "valid strategies",
			config: map[string]interface{}{
				"strategy":          "latency",
				"fallback_strategy": "round_robin",
			},
			valid: true,
		},
		{
			name: "valid weights",
			config: map[string]interface{}{
				"cost_weight":         0.25,
				"latency_weight":      0.25,
				"success_rate_weight": 0.25,
				"load_weight":         0.25,
			},
			valid: true,
		},
		{
			name: "valid endpoint weights",
			config: map[string]interface{}{
				"endpoint_weights": map[string]float64{
					"openai/us-east-1":    2.0,
					"anthropic/us-west-2": 1.5,
					"google/europe-west1": 1.0,
				},
			},
			valid: true,
		},
		{
			name: "mixed valid configuration",
			config: map[string]interface{}{
				"strategy":    "performance_based",
				"cost_weight": 0.4,
				"load_weight": 0.1,
			},
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodyBytes, err := json.Marshal(tt.config)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPatch, "/v1/routing/config", bytes.NewBuffer(bodyBytes))
			recorder := httptest.NewRecorder()

			handler.HandleUpdateRoutingConfig(recorder, req)

			if tt.valid {
				assert.Equal(t, http.StatusOK, recorder.Code)

				var response map[string]interface{}
				err := json.Unmarshal(recorder.Body.Bytes(), &response)
				assert.NoError(t, err)
				assert.Equal(t, "updated", response["status"])
			} else {
				assert.NotEqual(t, http.StatusOK, recorder.Code)
			}
		})
	}
}
