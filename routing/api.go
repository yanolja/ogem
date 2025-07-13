package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/openai"
)

// APIHandler provides HTTP handlers for routing management and statistics
type APIHandler struct {
	router *Router
	logger *zap.SugaredLogger
}

// NewAPIHandler creates a new routing API handler
func NewAPIHandler(router *Router, logger *zap.SugaredLogger) *APIHandler {
	return &APIHandler{
		router: router,
		logger: logger,
	}
}

// HandleRoutingStats returns overall routing statistics
func (h *APIHandler) HandleRoutingStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.router.GetRoutingStats()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.logger.Errorw("Failed to encode routing stats", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// HandleEndpointMetrics returns metrics for a specific endpoint or all endpoints
func (h *APIHandler) HandleEndpointMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse endpoint from URL path: /v1/routing/endpoints/{provider}/{region}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/routing/endpoints/"), "/")

	response := make(map[string]interface{})

	if len(pathParts) == 2 && pathParts[0] != "" && pathParts[1] != "" {
		// Specific endpoint requested
		provider := pathParts[0]
		region := pathParts[1]

		// Create a dummy endpoint status to get metrics
		endpointStatus := &EndpointStatus{
			Endpoint: &dummyEndpoint{provider: provider, region: region},
		}

		metrics := h.router.GetEndpointMetrics(endpointStatus)
		if metrics != nil {
			response[fmt.Sprintf("%s/%s", provider, region)] = metrics
		} else {
			response[fmt.Sprintf("%s/%s", provider, region)] = map[string]string{
				"status": "no_metrics_available",
			}
		}
	} else {
		// All endpoints requested - this would need access to endpoint list
		// For now, return message about specific endpoint query
		response["message"] = "Specify endpoint as /v1/routing/endpoints/{provider}/{region} for metrics"
		response["available_endpoints"] = []string{
			"Format: /v1/routing/endpoints/{provider}/{region}",
			"Example: /v1/routing/endpoints/openai/openai",
			"Example: /v1/routing/endpoints/vertex/us-central1",
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Errorw("Failed to encode endpoint metrics", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

type routingConfigUpdate struct {
	Strategy          *string            `json:"strategy,omitempty"`
	FallbackStrategy  *string            `json:"fallback_strategy,omitempty"`
	CostWeight        *float64           `json:"cost_weight,omitempty"`
	LatencyWeight     *float64           `json:"latency_weight,omitempty"`
	SuccessRateWeight *float64           `json:"success_rate_weight,omitempty"`
	LoadWeight        *float64           `json:"load_weight,omitempty"`
	EndpointWeights   map[string]float64 `json:"endpoint_weights,omitempty"`
}

// HandleUpdateRoutingConfig updates routing configuration
func (h *APIHandler) HandleUpdateRoutingConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var configUpdate routingConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&configUpdate); err != nil {
		h.logger.Warnw("Invalid request body", "error", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	config := h.router.config

	if configUpdate.Strategy != nil {
		config.Strategy = RoutingStrategy(*configUpdate.Strategy)
	}
	if configUpdate.FallbackStrategy != nil {
		config.FallbackStrategy = RoutingStrategy(*configUpdate.FallbackStrategy)
	}

	weightUpdated := false
	if configUpdate.CostWeight != nil {
		config.CostWeight = *configUpdate.CostWeight
		weightUpdated = true
	}
	if configUpdate.LatencyWeight != nil {
		config.LatencyWeight = *configUpdate.LatencyWeight
		weightUpdated = true
	}
	if configUpdate.SuccessRateWeight != nil {
		config.SuccessRateWeight = *configUpdate.SuccessRateWeight
		weightUpdated = true
	}
	if configUpdate.LoadWeight != nil {
		config.LoadWeight = *configUpdate.LoadWeight
		weightUpdated = true
	}

	if configUpdate.EndpointWeights != nil {
		if config.EndpointWeights == nil {
			config.EndpointWeights = make(map[string]float64)
		}
		for k, v := range configUpdate.EndpointWeights {
			config.EndpointWeights[k] = v
		}
	}

	// Normalize weights for performance-based routing if any weight was updated
	if weightUpdated {
		totalWeight := config.CostWeight + config.LatencyWeight + config.SuccessRateWeight + config.LoadWeight
		if totalWeight > 0 {
			config.CostWeight /= totalWeight
			config.LatencyWeight /= totalWeight
			config.SuccessRateWeight /= totalWeight
			config.LoadWeight /= totalWeight
		}
	}

	response := map[string]interface{}{
		"status": "updated",
		"config": config,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Errorw("Failed to encode config update response", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// HandleRoutingHealth returns routing system health status
func (h *APIHandler) HandleRoutingHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Basic health check - in a real implementation, you might check:
	// - Router is initialized
	// - Monitoring is working
	// - No critical errors in adaptive routing
	// - Circuit breakers are functioning

	health := map[string]interface{}{
		"status":             "healthy",
		"router_initialized": h.router != nil,
		"strategy":           "unknown",
		"monitoring_enabled": false,
	}

	if h.router != nil {
		health["strategy"] = h.router.getActiveStrategy()
		health["monitoring_enabled"] = h.router.monitor != nil

		if h.router.adaptiveState != nil {
			h.router.adaptiveState.mutex.RLock()
			health["adaptive_strategy"] = h.router.adaptiveState.CurrentStrategy
			health["last_evaluation"] = h.router.adaptiveState.LastEvaluation
			health["strategy_changes"] = len(h.router.adaptiveState.StrategyHistory)
			h.router.adaptiveState.mutex.RUnlock()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		h.logger.Errorw("Failed to encode routing health", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// HandleCircuitBreakerStatus returns circuit breaker status for endpoints
func (h *APIHandler) HandleCircuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse endpoint from URL path if specified
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/routing/circuit-breakers/"), "/")

	response := make(map[string]interface{})

	if len(pathParts) == 2 && pathParts[0] != "" && pathParts[1] != "" {
		// Specific endpoint requested
		provider := pathParts[0]
		region := pathParts[1]

		endpointStatus := &EndpointStatus{
			Endpoint: &dummyEndpoint{provider: provider, region: region},
		}

		metrics := h.router.GetEndpointMetrics(endpointStatus)
		if metrics != nil {
			response[fmt.Sprintf("%s/%s", provider, region)] = map[string]interface{}{
				"circuit_state":         metrics.CircuitState,
				"consecutive_failures":  metrics.ConsecutiveFailures,
				"consecutive_successes": metrics.ConsecutiveSuccesses,
				"last_failure_time":     metrics.LastFailureTime,
			}
		} else {
			response[fmt.Sprintf("%s/%s", provider, region)] = map[string]string{
				"status": "no_circuit_breaker_data",
			}
		}
	} else {
		response["message"] = "Specify endpoint as /v1/routing/circuit-breakers/{provider}/{region}"
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Errorw("Failed to encode circuit breaker status", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// HandleResetCircuitBreaker manually resets a circuit breaker
func (h *APIHandler) HandleResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse endpoint from URL path
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/routing/circuit-breakers/"), "/")

	if len(pathParts) != 3 || pathParts[0] == "" || pathParts[1] == "" || pathParts[2] != "reset" {
		http.Error(w, "Invalid endpoint format. Use: /v1/routing/circuit-breakers/{provider}/{region}/reset", http.StatusBadRequest)
		return
	}

	provider := pathParts[0]
	region := pathParts[1]

	// Reset circuit breaker by updating metrics
	h.router.mutex.Lock()
	endpointKey := fmt.Sprintf("%s/%s", provider, region)
	if metrics, exists := h.router.endpointMetrics[endpointKey]; exists {
		metrics.mutex.Lock()
		metrics.CircuitState = CircuitClosed
		metrics.ConsecutiveFailures = 0
		metrics.ConsecutiveSuccesses = 0
		metrics.mutex.Unlock()

		h.logger.Infow("Circuit breaker manually reset", "endpoint", endpointKey)

		response := map[string]interface{}{
			"status":        "reset",
			"endpoint":      endpointKey,
			"circuit_state": "closed",
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	} else {
		h.router.mutex.Unlock()
		http.Error(w, "Endpoint not found", http.StatusNotFound)
		return
	}
	h.router.mutex.Unlock()
}

// RegisterRoutes registers all routing API routes
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/v1/routing/stats", h.HandleRoutingStats)
	mux.HandleFunc("/v1/routing/health", h.HandleRoutingHealth)
	mux.HandleFunc("/v1/routing/config", h.HandleUpdateRoutingConfig)
	mux.HandleFunc("/v1/routing/endpoints/", h.HandleEndpointMetrics)
	mux.HandleFunc("/v1/routing/circuit-breakers/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/reset") {
			h.HandleResetCircuitBreaker(w, r)
		} else {
			h.HandleCircuitBreakerStatus(w, r)
		}
	})
}

// dummyEndpoint is a helper for API operations that need an endpoint interface
type dummyEndpoint struct {
	provider string
	region   string
}

func (d *dummyEndpoint) Provider() string                                { return d.provider }
func (d *dummyEndpoint) Region() string                                  { return d.region }
func (d *dummyEndpoint) Ping(ctx context.Context) (time.Duration, error) { return 0, nil }
func (d *dummyEndpoint) GenerateChatCompletion(ctx context.Context, req *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) GenerateChatCompletionStream(ctx context.Context, req *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	return nil, nil
}
func (d *dummyEndpoint) GenerateEmbedding(ctx context.Context, req *openai.EmbeddingRequest) (*openai.EmbeddingResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) GenerateImage(ctx context.Context, req *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) TranscribeAudio(ctx context.Context, req *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) TranslateAudio(ctx context.Context, req *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) GenerateSpeech(ctx context.Context, req *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) ModerateContent(ctx context.Context, req *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, nil
}
func (d *dummyEndpoint) CreateFineTuningJob(ctx context.Context, req *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, nil
}
func (d *dummyEndpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, nil
}
func (d *dummyEndpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, nil
}
func (d *dummyEndpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, nil
}
func (d *dummyEndpoint) Shutdown() error { return nil }
