package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"go.uber.org/zap"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/config"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
	"github.com/yanolja/ogem/provider/claude"
	openaiProvider "github.com/yanolja/ogem/provider/openai"
	"github.com/yanolja/ogem/provider/studio"
	"github.com/yanolja/ogem/provider/vclaude"
	"github.com/yanolja/ogem/provider/vertex"
	"github.com/yanolja/ogem/state"
	"github.com/yanolja/ogem/utils/array"
	"github.com/yanolja/ogem/utils/copy"
	"github.com/yanolja/ogem/utils/env"
)

type (
	BadRequestError     struct{ error }
	InternalServerError struct{ error }
	RateLimitError      struct{ error }
	RequestTimeoutError struct{ error }
	UnavailableError    struct{ error }
)

type endpointStatus struct {
	// Endpoint to use for generating completions.
	endpoint provider.AiEndpoint

	// Latency of the endpoint.
	latency time.Duration

	// Model status (latency and rate limiting information) of the endpoint.
	modelStatus *ogem.SupportedModel
}

type ModelProxy struct {
	// Endpoints to use for generating completions.
	endpoints []provider.AiEndpoint

	// Status of the endpoints. Used for latency checking and rate limiting.
	endpointStatus ogem.ProvidersStatus

	// Mutex to synchronize access to the endpoints and endpointStatus.
	mutex sync.RWMutex

	// State manager for rate limiting and caching
	stateManager state.Manager

	// Cleanup function from memory manager if using in-memory state
	cleanup func()

	// Interval to retry when no available endpoints are found.
	retryInterval time.Duration

	// Interval to update the status of the providers.
	pingInterval time.Duration

	// Configuration for the proxy server.
	config *config.Config

	// Logger for the proxy server.
	logger *zap.SugaredLogger
}

func newEndpoint(provider string, region string, config *config.Config) (provider.AiEndpoint, error) {
	switch provider {
	case "claude":
		if region != "claude" {
			return nil, fmt.Errorf("region is not supported for claude provider")
		}
		return claude.NewEndpoint(config.ClaudeApiKey)
	case "vclaude":
		return vclaude.NewEndpoint(config.GoogleCloudProject, region)
	case "vertex":
		return vertex.NewEndpoint(config.GoogleCloudProject, region)
	case "studio":
		if region != "studio" {
			return nil, fmt.Errorf("region is not supported for studio provider")
		}
		return studio.NewEndpoint(config.GenaiStudioApiKey)
	case "openai":
		if region != "openai" {
			return nil, fmt.Errorf("region is not supported for openai provider")
		}
		return openaiProvider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", config.OpenAiApiKey)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func newCustomEndpoint(providerName string, protocol string, baseUrl string, apiKeyEnv string, region string) (provider.AiEndpoint, error) {
	switch protocol {
	case "openai":
		if region != providerName {
			return nil, fmt.Errorf("region is not supported for custom openai provider; region field must match provider name")
		}
		return openaiProvider.NewEndpoint(providerName, region, baseUrl, env.RequiredStringVariable(apiKeyEnv))
	default:
		return nil, fmt.Errorf("unsupported protocol: %s, only openai is supported", protocol)
	}
}

func NewProxyServer(stateManager state.Manager, cleanup func(), config *config.Config, logger *zap.SugaredLogger) (*ModelProxy, error) {
	retryInterval, err := time.ParseDuration(config.RetryInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid retry interval: %v", err)
	}

	pingInterval, err := time.ParseDuration(config.PingInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid ping interval: %v", err)
	}

	endpointStatus, err := copy.Deep(config.Providers)
	if err != nil {
		return nil, fmt.Errorf("failed to deep copy provider status: %v", err)
	}

	endpoints := []provider.AiEndpoint{}
	endpointStatus.ForEach(func(
		providerName string,
		providerData ogem.ProviderStatus,
		region string,
		_ ogem.RegionStatus,
		models []*ogem.SupportedModel,
	) bool {
		var endpoint provider.AiEndpoint
		var err error
		if providerData.BaseUrl == "" {
			endpoint, err = newEndpoint(providerName, region, config)
		} else {
			endpoint, err = newCustomEndpoint(
				providerName,
				providerData.Protocol,
				providerData.BaseUrl,
				providerData.ApiKeyEnv,
				region,
			)
		}
		if err != nil {
			logger.Warnw("Failed to create endpoint", "provider", providerName, "region", region, "error", err)
			return false
		}
		endpoints = append(endpoints, endpoint)
		return false
	})

	return &ModelProxy{
		endpoints:      endpoints,
		endpointStatus: endpointStatus,
		stateManager:   stateManager,
		cleanup:        cleanup,
		retryInterval:  retryInterval,
		pingInterval:   pingInterval,
		config:         config,
		logger:         logger,
	}, nil
}

func (s *ModelProxy) HandleChatCompletions(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var openAiRequest openai.ChatCompletionRequest
	if err := json.Unmarshal(bodyBytes, &openAiRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	models := strings.Split(openAiRequest.Model, ",")
	s.logger.Infow("Received chat completions request", "models", models)

	// Check if streaming is requested
	if openAiRequest.Stream != nil && *openAiRequest.Stream {
		s.handleStreamingChatCompletions(httpResponse, httpRequest, &openAiRequest, models)
		return
	}

	var openAiResponse *openai.ChatCompletionResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		openAiRequest.Model = strings.TrimSpace(model)
		openAiResponse, err = s.generateChatCompletion(httpRequest.Context(), &openAiRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to get chat completions", "error", err, "model", model)
			lastError = err
			continue
		}

		if len(openAiResponse.Choices) > 0 && openAiResponse.Choices[0].FinishReason == "stop" {
			break
		}
	}

	if openAiResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(openAiResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) handleStreamingChatCompletions(httpResponse http.ResponseWriter, httpRequest *http.Request, openAiRequest *openai.ChatCompletionRequest, models []string) {
	// Set SSE headers
	httpResponse.Header().Set("Content-Type", "text/event-stream")
	httpResponse.Header().Set("Cache-Control", "no-cache")
	httpResponse.Header().Set("Connection", "keep-alive")
	httpResponse.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := httpResponse.(http.Flusher)
	if !ok {
		http.Error(httpResponse, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		openAiRequest.Model = strings.TrimSpace(model)
		success, err := s.generateStreamingChatCompletion(httpRequest.Context(), openAiRequest, httpResponse, flusher, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to get streaming chat completions", "error", err, "model", model)
			lastError = err
			continue
		}
		if success {
			break
		}
	}

	if lastError != nil {
		// Write error as SSE event
		errorData := fmt.Sprintf(`{"error": {"message": "%s", "type": "server_error"}}`, lastError.Error())
		fmt.Fprintf(httpResponse, "data: %s\n\n", errorData)
		flusher.Flush()
	}

	// Send termination signal
	fmt.Fprintf(httpResponse, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *ModelProxy) generateStreamingChatCompletion(ctx context.Context, openAiRequest *openai.ChatCompletionRequest, httpResponse http.ResponseWriter, flusher http.Flusher, keepRetry bool) (bool, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(openAiRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", openAiRequest.Model)
		return false, BadRequestError{fmt.Errorf("invalid model name: %s", openAiRequest.Model)}
	}

	if len(openAiRequest.Messages) == 0 {
		s.logger.Warn("No messages provided")
		return false, BadRequestError{fmt.Errorf("no messages provided")}
	}

	endpoints, err := s.sortedEndpoints(endpointProvider, endpointRegion, modelOrAlias)
	if err != nil || len(endpoints) == 0 {
		s.logger.Warnw("Failed to select endpoint", "error", err, "provider", endpointProvider, "region", endpointRegion, "model", modelOrAlias)
		return false, UnavailableError{fmt.Errorf("no available endpoints")}
	}

	for {
		var bestEndpoint *endpointStatus
		var shortestWaiting time.Duration
		for _, endpoint := range endpoints {
			if ctx.Err() != nil {
				s.logger.Warn("Request canceled")
				return false, RequestTimeoutError{fmt.Errorf("request canceled")}
			}

			accepted, waiting, err := s.stateManager.Allow(
				ctx,
				endpoint.endpoint.Provider(),
				endpoint.endpoint.Region(),
				modelOrAlias,
				requestInterval(endpoint.modelStatus),
			)
			if err != nil {
				s.logger.Warnw("Failed to check rate limit", "error", err, "provider", endpoint.endpoint.Provider(), "region", endpoint.endpoint.Region(), "model", modelOrAlias)
				return false, InternalServerError{fmt.Errorf("rate limit check failed")}
			}
			if !accepted {
				if bestEndpoint == nil || waiting < shortestWaiting {
					bestEndpoint = endpoint
					shortestWaiting = waiting
				}
				s.logger.Infow("Rate limit exceeded", "provider", endpoint.endpoint.Provider(), "region", endpoint.endpoint.Region(), "model", modelOrAlias, "waiting", waiting)
				continue
			}

			openAiRequest.Model = endpoint.modelStatus.Name
			responseCh, errorCh := endpoint.endpoint.GenerateChatCompletionStream(ctx, openAiRequest)

			// Stream the response
			for {
				select {
				case streamResponse, ok := <-responseCh:
					if !ok {
						// Channel closed, streaming finished successfully
						return true, nil
					}

					// Finalize the stream response
					finalizedResponse := openai.FinalizeStreamResponse(endpoint.endpoint.Provider(), endpoint.endpoint.Region(), openAiRequest.Model, streamResponse)

					responseData, err := json.Marshal(finalizedResponse)
					if err != nil {
						s.logger.Warnw("Failed to marshal stream response", "error", err)
						continue
					}

					fmt.Fprintf(httpResponse, "data: %s\n\n", string(responseData))
					flusher.Flush()

				case err, ok := <-errorCh:
					if !ok {
						// Error channel closed
						return true, nil
					}
					if err != nil {
						loweredError := strings.ToLower(err.Error())
						if strings.Contains(loweredError, "429") ||
							strings.Contains(loweredError, "quota") ||
							strings.Contains(loweredError, "exceeded") ||
							strings.Contains(loweredError, "throughput") ||
							strings.Contains(loweredError, "exhausted") {
							s.stateManager.Disable(ctx, endpoint.endpoint.Provider(), endpoint.endpoint.Region(), modelOrAlias, 1*time.Minute)
							return false, err
						}
						s.logger.Warnw("Failed to generate streaming completion", "error", err)
						return false, InternalServerError{fmt.Errorf("failed to generate streaming completion")}
					}

				case <-ctx.Done():
					s.logger.Warn("Request canceled during streaming")
					return false, RequestTimeoutError{fmt.Errorf("request canceled")}
				}
			}
		}

		if bestEndpoint == nil {
			if keepRetry {
				s.logger.Warnw("No available endpoints", "waiting", s.retryInterval)
				time.Sleep(s.retryInterval)
				continue
			}
			s.logger.Warn("No available endpoints")
			return false, UnavailableError{fmt.Errorf("no available endpoints")}
		}
		time.Sleep(shortestWaiting)
	}
}

func (s *ModelProxy) HandleAuthentication(handler http.HandlerFunc) http.HandlerFunc {
	return func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
		if s.config.OgemApiKey == "" {
			handler(httpResponse, httpRequest)
			return
		}

		headerSplit := strings.Split(httpRequest.Header.Get("Authorization"), " ")
		if len(headerSplit) != 2 ||
			strings.ToLower(headerSplit[0]) != "bearer" ||
			(headerSplit[1] != "" && headerSplit[1] != s.config.OgemApiKey) {
			http.Error(httpResponse, "Unauthorized", http.StatusUnauthorized)
			return
		}

		handler(httpResponse, httpRequest)
	}
}

func (s *ModelProxy) PingInterval() time.Duration {
	return s.pingInterval
}

func (s *ModelProxy) StartPingLoop(ctx context.Context) {
	if s.pingInterval <= 0 {
		return
	}

	ticker := time.NewTicker(s.pingInterval)
	defer ticker.Stop()

	// This ensures we have initial data without waiting for the first tick,
	// which occurs after a full interval.
	s.pingAllEndpoints(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.pingAllEndpoints(ctx)
		}
	}
}

func (s *ModelProxy) Shutdown() {
	s.logger.Info("Shutting down ModelProxy")
	if s.cleanup != nil {
		s.cleanup()
	}
	for _, endpoint := range s.endpoints {
		if err := endpoint.Shutdown(); err != nil {
			s.logger.Warnw("Failed to shutdown endpoint", "error", err)
		}
	}
}

func handleError(w http.ResponseWriter, err error) {
	switch err.(type) {
	case BadRequestError:
		http.Error(w, "Invalid request", http.StatusBadRequest)
	case UnavailableError:
		http.Error(w, "No available endpoints", http.StatusServiceUnavailable)
	case RateLimitError:
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
	case RequestTimeoutError:
		http.Error(w, "Request timed out", http.StatusRequestTimeout)
	case InternalServerError:
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	default:
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) generateChatCompletion(ctx context.Context, openAiRequest *openai.ChatCompletionRequest, keepRetry bool) (*openai.ChatCompletionResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(openAiRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", openAiRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", openAiRequest.Model)}
	}

	if len(openAiRequest.Messages) == 0 {
		s.logger.Warn("No messages provided")
		return nil, BadRequestError{fmt.Errorf("no messages provided")}
	}

	cacheable := openAiRequest.Temperature != nil && math.Abs(float64(*openAiRequest.Temperature)-float64(0)) < math.SmallestNonzeroFloat32

	if cacheable {
		cachedResponse, err := s.cachedResponse(ctx, openAiRequest)
		if err != nil {
			s.logger.Warnw("Failed to get cached response", "error", err)
		} else if cachedResponse != nil {
			s.logger.Infow("Returning cached response", "model", openAiRequest.Model)
			return cachedResponse, nil
		}
	}

	endpoints, err := s.sortedEndpoints(endpointProvider, endpointRegion, modelOrAlias)
	if err != nil || len(endpoints) == 0 {
		s.logger.Warnw("Failed to select endpoint", "error", err, "provider", endpointProvider, "region", endpointRegion, "model", modelOrAlias)
		return nil, UnavailableError{fmt.Errorf("no available endpoints")}
	}

	for {
		var bestEndpoint *endpointStatus
		var shortestWaiting time.Duration
		for _, endpoint := range endpoints {
			if ctx.Err() != nil {
				s.logger.Warn("Request canceled")
				return nil, RequestTimeoutError{fmt.Errorf("request canceled")}
			}

			accepted, waiting, err := s.stateManager.Allow(
				ctx,
				endpoint.endpoint.Provider(),
				endpoint.endpoint.Region(),
				modelOrAlias,
				requestInterval(endpoint.modelStatus),
			)
			if err != nil {
				s.logger.Warnw("Failed to check rate limit", "error", err, "provider", endpoint.endpoint.Provider(), "region", endpoint.endpoint.Region(), "model", modelOrAlias)
				return nil, InternalServerError{fmt.Errorf("rate limit check failed")}
			}
			if !accepted {
				if bestEndpoint == nil || waiting < shortestWaiting {
					bestEndpoint = endpoint
					shortestWaiting = waiting
				}
				s.logger.Infow("Rate limit exceeded", "provider", endpoint.endpoint.Provider(), "region", endpoint.endpoint.Region(), "model", modelOrAlias, "waiting", waiting)
				continue
			}

			openAiRequest.Model = endpoint.modelStatus.Name
			openAiResponse, err := endpoint.endpoint.GenerateChatCompletion(ctx, openAiRequest)
			if err != nil {
				loweredError := strings.ToLower(err.Error())
				if strings.Contains(loweredError, "429") ||
					strings.Contains(loweredError, "quota") ||
					strings.Contains(loweredError, "exceeded") ||
					strings.Contains(loweredError, "throughput") ||
					strings.Contains(loweredError, "exhausted") {
					s.stateManager.Disable(ctx, endpoint.endpoint.Provider(), endpoint.endpoint.Region(), modelOrAlias, 1*time.Minute)
					continue
				}
				s.logger.Warnw("Failed to generate completion", "error", err, "request", openAiRequest, "response", openAiResponse)
				return nil, InternalServerError{fmt.Errorf("failed to generate completion")}
			}

			if cacheable {
				// Caching should be done even if the request has been canceled.
				err := s.storeResponseInCache(context.Background(), openAiRequest, openAiResponse)
				if err != nil {
					s.logger.Warnw("Failed to cache response", "error", err)
				}
			}
			return openAiResponse, nil
		}
		if bestEndpoint == nil {
			if keepRetry {
				s.logger.Warnw("No available endpoints", "waiting", s.retryInterval)
				time.Sleep(s.retryInterval)
				continue
			}
			s.logger.Warn("No available endpoints")
			return nil, UnavailableError{fmt.Errorf("no available endpoints")}
		}
		time.Sleep(shortestWaiting)
	}
}

func parseModelIdentifier(modelIdentifier string) (provider string, region string, model string, err error) {
	parts := strings.Split(modelIdentifier, "/")

	switch len(parts) {
	case 1:
		return "", "", modelIdentifier, nil
	case 2:
		return parts[0], "", parts[1], nil
	case 3:
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf("invalid model name: %s", modelIdentifier)
	}
}

func (s *ModelProxy) sortedEndpoints(desiredProvider string, desiredRegion string, model string) ([]*endpointStatus, error) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	endpoints := []*endpointStatus{}
	s.endpointStatus.ForEach(func(provider string, _ ogem.ProviderStatus, region string, regionStatus ogem.RegionStatus, models []*ogem.SupportedModel) bool {
		if desiredProvider != "" && desiredProvider != provider {
			return false
		}
		if desiredRegion != "" && desiredRegion != region {
			return false
		}
		modelStatus, found := array.Find(models, func(m *ogem.SupportedModel) bool {
			return m.Name == model || array.Contains(m.OtherNames, model)
		})
		if !found {
			return false
		}

		endpoint, err := s.endpoint(provider, region)
		if err != nil {
			s.logger.Warnw("Failed to get endpoint", "provider", provider, "region", region, "error", err)
			return false
		}
		endpoints = append(endpoints, &endpointStatus{
			endpoint:    endpoint,
			latency:     regionStatus.Latency,
			modelStatus: modelStatus,
		})
		return false
	})

	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].latency < endpoints[j].latency
	})

	s.logger.Infow("Selected endpoint", "endpoints", array.Map(endpoints, func(e *endpointStatus) string {
		return fmt.Sprintf("%s/%s/%s", e.endpoint.Provider(), e.endpoint.Region(), model)
	}), "model", model)
	return endpoints, nil
}

func (s *ModelProxy) endpoint(provider string, region string) (provider.AiEndpoint, error) {
	for _, endpoint := range s.endpoints {
		if endpoint.Provider() == provider && endpoint.Region() == region {
			return endpoint, nil
		}
	}
	return nil, fmt.Errorf("endpoint not found for provider: %s, region: %s", provider, region)
}

func (s *ModelProxy) pingAllEndpoints(ctx context.Context) {
	for _, endpoint := range s.endpoints {
		latency, err := endpoint.Ping(ctx)
		if err != nil {
			s.logger.Warnw("Failed to ping endpoint", "provider", endpoint.Provider(), "region", endpoint.Region(), "error", err)
			continue
		}
		s.updateEndpointStatus(endpoint.Provider(), endpoint.Region(), latency)
	}
}

func (s *ModelProxy) updateEndpointStatus(provider string, region string, latency time.Duration) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.endpointStatus.Update(provider, region, func(regionStatus *ogem.RegionStatus) error {
		regionStatus.Latency = latency
		regionStatus.LastChecked = time.Now()
		return nil
	})
}

func (s *ModelProxy) cachedResponse(ctx context.Context, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	cacheKey, err := generateCacheKey(request)
	if err != nil {
		return nil, fmt.Errorf("failed to build cache key: %v", err)
	}
	s.logger.Infow("Checking cache", "key", cacheKey)

	data, err := s.stateManager.LoadCache(ctx, cacheKey)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, nil
	}

	var cachedResponse openai.ChatCompletionResponse
	if err := json.Unmarshal(data, &cachedResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached response: %v", err)
	}
	return &cachedResponse, nil
}

func (s *ModelProxy) storeResponseInCache(ctx context.Context, request *openai.ChatCompletionRequest, response *openai.ChatCompletionResponse) error {
	cacheKey, err := generateCacheKey(request)
	if err != nil {
		return fmt.Errorf("failed to build cache key: %v", err)
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to marshal response for caching: %v", err)
	}

	return s.stateManager.SaveCache(ctx, cacheKey, jsonBytes, 24*time.Hour)
}

func generateCacheKey(request *openai.ChatCompletionRequest) (string, error) {
	hasher := sha256.New()
	requestBytes, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	hasher.Write(requestBytes)
	hash := hex.EncodeToString(hasher.Sum(nil))
	return fmt.Sprintf("ogem:cache:%s", hash), nil
}

func requestInterval(modelStatus *ogem.SupportedModel) time.Duration {
	if modelStatus == nil || modelStatus.MaxRequestsPerMinute == 0 {
		return time.Millisecond
	}
	return time.Duration(time.Minute.Nanoseconds() / int64(modelStatus.MaxRequestsPerMinute))
}
