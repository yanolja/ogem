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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"go.uber.org/zap"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/admin"
	"github.com/yanolja/ogem/auth"
	"github.com/yanolja/ogem/config"
	"github.com/yanolja/ogem/cost"
	"github.com/yanolja/ogem/image"
	"github.com/yanolja/ogem/monitoring"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
	"github.com/yanolja/ogem/provider/claude"
	"github.com/yanolja/ogem/provider/groq"
	"github.com/yanolja/ogem/provider/mistral"
	openaiProvider "github.com/yanolja/ogem/provider/openai"
	"github.com/yanolja/ogem/provider/openrouter"
	"github.com/yanolja/ogem/provider/studio"
	"github.com/yanolja/ogem/provider/vclaude"
	"github.com/yanolja/ogem/provider/vertex"
	"github.com/yanolja/ogem/provider/xai"
	"github.com/yanolja/ogem/routing"
	ogemSdk "github.com/yanolja/ogem/sdk/go"
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

	// Auth manager for virtual keys
	authManager auth.Manager

	// Image downloader for vision support
	imageDownloader *image.Downloader

	// Admin server for management UI
	adminServer *admin.AdminServer

	// Advanced router for intelligent request routing
	router *routing.Router

	// Monitoring manager for metrics and observability
	monitor *monitoring.MonitoringManager
}

func newEndpoint(provider string, region string, config *config.Config) (provider.AiEndpoint, error) {
	switch provider {
	case "claude":
		if region != "claude" {
			return nil, fmt.Errorf("region is not supported for claude provider")
		}
		return claude.NewEndpoint(config.ClaudeApiKey)
	case "mistral":
		if region != "mistral" {
			return nil, fmt.Errorf("region is not supported for mistral provider")
		}
		return mistral.NewEndpoint(region, "", config.MistralApiKey)
	case "xai":
		if region != "xai" {
			return nil, fmt.Errorf("region is not supported for xai provider")
		}
		return xai.NewEndpoint(region, "", config.XAIApiKey)
	case "groq":
		if region != "groq" {
			return nil, fmt.Errorf("region is not supported for groq provider")
		}
		return groq.NewEndpoint(region, "", config.GroqApiKey)
	case "openrouter":
		if region != "openrouter" {
			return nil, fmt.Errorf("region is not supported for openrouter provider")
		}
		return openrouter.NewEndpoint(region, "", config.OpenRouterApiKey)
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

func NewProxyServer(stateManager state.Manager, cleanup func(), config *config.Config, authManager auth.Manager, logger *zap.SugaredLogger) (*ModelProxy, error) {
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

	imageCache := image.NewStateCacheManager(stateManager)
	imageDownloader := image.NewDownloader(imageCache)

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

		// Set image downloader for vision support
		if claudeEndpoint, ok := endpoint.(*claude.Endpoint); ok {
			claudeEndpoint.SetImageDownloader(imageDownloader)
		}
		if studioEndpoint, ok := endpoint.(*studio.Endpoint); ok {
			studioEndpoint.SetImageDownloader(imageDownloader)
		}
		if vclaudeEndpoint, ok := endpoint.(*vclaude.Endpoint); ok {
			vclaudeEndpoint.SetImageDownloader(imageDownloader)
		}
		if vertexEndpoint, ok := endpoint.(*vertex.Endpoint); ok {
			vertexEndpoint.SetImageDownloader(imageDownloader)
		}

		endpoints = append(endpoints, endpoint)
		return false
	})

	// Initialize monitoring system
	var monitor *monitoring.MonitoringManager
	if config.Routing != nil && config.Routing.EnableMetrics {
		monitoringConfig := monitoring.DefaultMonitoringConfig()
		var err error
		monitor, err = monitoring.NewMonitoringManager(monitoringConfig, logger)
		if err != nil {
			logger.Warnw("Failed to initialize monitoring manager", "error", err)
		}
	}

	// Initialize intelligent router
	var router *routing.Router
	if config.Routing != nil {
		router = routing.NewRouter(config.Routing, monitor, logger)
	} else {
		// Use default routing configuration
		defaultConfig := routing.DefaultRoutingConfig()
		router = routing.NewRouter(defaultConfig, monitor, logger)
	}

	// Initialize admin server
	var adminServer *admin.AdminServer
	if authManager != nil {
		// Since Manager embeds VirtualKeyManager, we can use authManager directly
		//calculator := cost.NewCalculator()
		adminServer = admin.NewAdminServer(authManager, stateManager)
	}

	return &ModelProxy{
		endpoints:       endpoints,
		endpointStatus:  endpointStatus,
		stateManager:    stateManager,
		cleanup:         cleanup,
		retryInterval:   retryInterval,
		pingInterval:    pingInterval,
		config:          config,
		logger:          logger,
		authManager:     authManager,
		imageDownloader: imageDownloader,
		adminServer:     adminServer,
		router:          router,
		monitor:         monitor,
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

func (s *ModelProxy) HandleImages(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var imageRequest openai.ImageGenerationRequest
	if err := json.Unmarshal(bodyBytes, &imageRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set default model if not specified
	if imageRequest.Model == nil {
		defaultModel := ogemSdk.ModelDALLE3
		imageRequest.Model = &defaultModel
	}

	models := strings.Split(*imageRequest.Model, ",")
	s.logger.Infow("Received image generation request", "models", models)

	var imageResponse *openai.ImageGenerationResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		*imageRequest.Model = strings.TrimSpace(model)
		imageResponse, err = s.generateImage(httpRequest.Context(), &imageRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to generate image", "error", err, "model", model)
			lastError = err
			continue
		}
		break
	}

	if imageResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(imageResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleEmbeddings(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var embeddingRequest openai.EmbeddingRequest
	if err := json.Unmarshal(bodyBytes, &embeddingRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	models := strings.Split(embeddingRequest.Model, ",")
	s.logger.Infow("Received embeddings request", "models", models)

	var embeddingResponse *openai.EmbeddingResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		embeddingRequest.Model = strings.TrimSpace(model)
		embeddingResponse, err = s.generateEmbedding(httpRequest.Context(), &embeddingRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to get embeddings", "error", err, "model", model)
			lastError = err
			continue
		}
		break
	}

	if embeddingResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(embeddingResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) generateEmbedding(ctx context.Context, embeddingRequest *openai.EmbeddingRequest, keepRetry bool) (*openai.EmbeddingResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(embeddingRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", embeddingRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", embeddingRequest.Model)}
	}

	if len(embeddingRequest.Input) == 0 {
		s.logger.Warn("No input provided")
		return nil, BadRequestError{fmt.Errorf("no input provided")}
	}

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, embeddingRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", embeddingRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
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

			embeddingRequest.Model = endpoint.modelStatus.Name
			embeddingResponse, err := endpoint.endpoint.GenerateEmbedding(ctx, embeddingRequest)
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
				s.logger.Warnw("Failed to generate embedding", "error", err, "request", embeddingRequest, "response", embeddingResponse)
				return nil, InternalServerError{fmt.Errorf("failed to generate embedding")}
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				tokens := int64(embeddingResponse.Usage.TotalTokens)
				calculatedCost := cost.CalculateEmbeddingCost(embeddingRequest.Model, embeddingResponse.Usage)
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
				}
			}

			return embeddingResponse, nil
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

func (s *ModelProxy) generateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest, keepRetry bool) (*openai.ImageGenerationResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(*imageRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", *imageRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", *imageRequest.Model)}
	}

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, *imageRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", *imageRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
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

			*imageRequest.Model = endpoint.modelStatus.Name
			imageResponse, err := endpoint.endpoint.GenerateImage(ctx, imageRequest)
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
				s.logger.Warnw("Failed to generate image", "error", err, "request", imageRequest, "response", imageResponse)
				return nil, InternalServerError{fmt.Errorf("failed to generate image")}
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				numImages := len(imageResponse.Data)
				tokens := int64(numImages) // Use number of images as token count for simplicity
				calculatedCost := cost.CalculateImageCost(*imageRequest.Model, imageRequest, numImages)
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
				}
			}

			return imageResponse, nil
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

func (s *ModelProxy) transcribeAudio(ctx context.Context, audioRequest *openai.AudioTranscriptionRequest, keepRetry bool) (*openai.AudioTranscriptionResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(audioRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", audioRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", audioRequest.Model)}
	}

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, audioRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", audioRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
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

			audioRequest.Model = endpoint.modelStatus.Name
			audioResponse, err := endpoint.endpoint.TranscribeAudio(ctx, audioRequest)
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
				s.logger.Warnw("Failed to transcribe audio", "error", err, "request", audioRequest, "response", audioResponse)
				return nil, InternalServerError{fmt.Errorf("failed to transcribe audio")}
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				tokens := int64(100) // Approximate token usage for audio transcription
				calculatedCost := cost.CalculateChatCost(audioRequest.Model, openai.Usage{PromptTokens: int32(tokens)})
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
				}
			}

			return audioResponse, nil
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

func (s *ModelProxy) translateAudio(ctx context.Context, audioRequest *openai.AudioTranslationRequest, keepRetry bool) (*openai.AudioTranslationResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(audioRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", audioRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", audioRequest.Model)}
	}

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, audioRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", audioRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
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

			audioRequest.Model = endpoint.modelStatus.Name
			audioResponse, err := endpoint.endpoint.TranslateAudio(ctx, audioRequest)
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
				s.logger.Warnw("Failed to translate audio", "error", err, "request", audioRequest, "response", audioResponse)
				return nil, InternalServerError{fmt.Errorf("failed to translate audio")}
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				tokens := int64(100) // Approximate token usage for audio translation
				calculatedCost := cost.CalculateChatCost(audioRequest.Model, openai.Usage{PromptTokens: int32(tokens)})
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
				}
			}

			return audioResponse, nil
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

func (s *ModelProxy) generateSpeech(ctx context.Context, speechRequest *openai.TextToSpeechRequest, keepRetry bool) (*openai.TextToSpeechResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(speechRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", speechRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", speechRequest.Model)}
	}

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, speechRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", speechRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
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

			speechRequest.Model = endpoint.modelStatus.Name
			speechResponse, err := endpoint.endpoint.GenerateSpeech(ctx, speechRequest)
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
				s.logger.Warnw("Failed to generate speech", "error", err, "request", speechRequest, "response", speechResponse)
				return nil, InternalServerError{fmt.Errorf("failed to generate speech")}
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				// Estimate token usage based on input text length
				inputLength := len(speechRequest.Input)
				tokens := int64(inputLength) // Approximate token usage
				calculatedCost := cost.CalculateChatCost(speechRequest.Model, openai.Usage{PromptTokens: int32(tokens)})
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
				}
			}

			return speechResponse, nil
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

func (s *ModelProxy) moderateContent(ctx context.Context, moderationRequest *openai.ModerationRequest, keepRetry bool) (*openai.ModerationResponse, error) {
	endpointProvider, endpointRegion, modelOrAlias, err := parseModelIdentifier(*moderationRequest.Model)
	if err != nil {
		s.logger.Warnw("Invalid model name", "error", err, "model", *moderationRequest.Model)
		return nil, BadRequestError{fmt.Errorf("invalid model name: %s", *moderationRequest.Model)}
	}

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, *moderationRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", *moderationRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
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

			*moderationRequest.Model = endpoint.modelStatus.Name
			moderationResponse, err := endpoint.endpoint.ModerateContent(ctx, moderationRequest)
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
				s.logger.Warnw("Failed to moderate content", "error", err, "request", moderationRequest, "response", moderationResponse)
				return nil, InternalServerError{fmt.Errorf("failed to moderate content")}
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				// Estimate token usage based on input text length
				totalInputLength := 0
				for _, input := range moderationRequest.Input {
					totalInputLength += len(input)
				}
				tokens := int64(totalInputLength / 4) // Rough token estimate (4 chars per token)
				calculatedCost := cost.CalculateChatCost(*moderationRequest.Model, openai.Usage{PromptTokens: int32(tokens)})
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
				}
			}

			return moderationResponse, nil
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

func (s *ModelProxy) HandleAuthentication(handler http.HandlerFunc) http.HandlerFunc {
	return func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
		// Extract authorization header
		authHeader := httpRequest.Header.Get("Authorization")
		if authHeader == "" {
			if s.config.OgemApiKey == "" && !s.config.EnableVirtualKeys {
				handler(httpResponse, httpRequest)
				return
			}
			http.Error(httpResponse, "Authorization header required", http.StatusUnauthorized)
			return
		}

		headerSplit := strings.Split(authHeader, " ")
		if len(headerSplit) != 2 || strings.ToLower(headerSplit[0]) != "bearer" {
			http.Error(httpResponse, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		token := headerSplit[1]

		// If virtual keys are enabled, try to validate as virtual key first
		if s.config.EnableVirtualKeys && s.authManager != nil {
			virtualKey, err := s.authManager.GetKeyByValue(httpRequest.Context(), token)
			if err == nil {
				// Valid virtual key - add to request context
				ctx := context.WithValue(httpRequest.Context(), "virtual_key", virtualKey)
				httpRequest = httpRequest.WithContext(ctx)
				handler(httpResponse, httpRequest)
				return
			}
		}

		// Fall back to master API key authentication
		if s.config.OgemApiKey != "" && token == s.config.OgemApiKey {
			handler(httpResponse, httpRequest)
			return
		}

		http.Error(httpResponse, "Unauthorized", http.StatusUnauthorized)
	}
}

func (s *ModelProxy) HandleCreateKey(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	// Only allow master key to create virtual keys
	if !s.isMasterKeyRequest(httpRequest) {
		http.Error(httpResponse, "Master key required", http.StatusForbidden)
		return
	}

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var keyRequest auth.KeyRequest
	if err := json.Unmarshal(bodyBytes, &keyRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	virtualKey, err := s.authManager.CreateKey(httpRequest.Context(), &keyRequest)
	if err != nil {
		s.logger.Errorw("Failed to create virtual key", "error", err)
		http.Error(httpResponse, "Failed to create key", http.StatusInternalServerError)
		return
	}

	response := auth.KeyResponse{
		ID:          virtualKey.ID,
		Key:         virtualKey.Key,
		Name:        virtualKey.Name,
		Description: virtualKey.Description,
		Models:      virtualKey.Models,
		MaxTokens:   virtualKey.MaxTokens,
		MaxRequests: virtualKey.MaxRequests,
		Budget:      virtualKey.Budget,
		Metadata:    virtualKey.Metadata,
		CreatedAt:   virtualKey.CreatedAt,
		ExpiresAt:   virtualKey.ExpiresAt,
		IsActive:    virtualKey.IsActive,
		UsageStats:  virtualKey.UsageStats,
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(response); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleListKeys(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	// Only allow master key to list virtual keys
	if !s.isMasterKeyRequest(httpRequest) {
		http.Error(httpResponse, "Master key required", http.StatusForbidden)
		return
	}

	keys, err := s.authManager.ListKeys(httpRequest.Context())
	if err != nil {
		s.logger.Errorw("Failed to list virtual keys", "error", err)
		http.Error(httpResponse, "Failed to list keys", http.StatusInternalServerError)
		return
	}

	responses := make([]auth.KeyResponse, len(keys))
	for i, key := range keys {
		responses[i] = auth.KeyResponse{
			ID:          key.ID,
			Key:         key.Key,
			Name:        key.Name,
			Description: key.Description,
			Models:      key.Models,
			MaxTokens:   key.MaxTokens,
			MaxRequests: key.MaxRequests,
			Budget:      key.Budget,
			Metadata:    key.Metadata,
			CreatedAt:   key.CreatedAt,
			ExpiresAt:   key.ExpiresAt,
			IsActive:    key.IsActive,
			UsageStats:  key.UsageStats,
		}
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(responses); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleGetKey(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	// Allow both master key and the key owner to view key details
	keyID := strings.TrimPrefix(httpRequest.URL.Path, "/v1/keys/")
	if keyID == "" {
		http.Error(httpResponse, "Key ID required", http.StatusBadRequest)
		return
	}

	// Check if it's master key or the key itself
	isMasterKey := s.isMasterKeyRequest(httpRequest)
	var requestKey *auth.VirtualKey
	if !isMasterKey {
		// Try to validate this request as the key itself
		authHeader := httpRequest.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(httpResponse, "Authorization required", http.StatusUnauthorized)
			return
		}
		headerSplit := strings.Split(authHeader, " ")
		if len(headerSplit) != 2 || strings.ToLower(headerSplit[0]) != "bearer" {
			http.Error(httpResponse, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}
		token := headerSplit[1]

		var err error
		requestKey, err = s.authManager.GetKeyByValue(httpRequest.Context(), token)
		if err != nil || requestKey.ID != keyID {
			http.Error(httpResponse, "Forbidden", http.StatusForbidden)
			return
		}
	}

	key, err := s.authManager.GetKey(httpRequest.Context(), keyID)
	if err != nil {
		s.logger.Errorw("Failed to get virtual key", "error", err, "key_id", keyID)
		http.Error(httpResponse, "Key not found", http.StatusNotFound)
		return
	}

	response := auth.KeyResponse{
		ID:          key.ID,
		Key:         key.Key,
		Name:        key.Name,
		Description: key.Description,
		Models:      key.Models,
		MaxTokens:   key.MaxTokens,
		MaxRequests: key.MaxRequests,
		Budget:      key.Budget,
		Metadata:    key.Metadata,
		CreatedAt:   key.CreatedAt,
		ExpiresAt:   key.ExpiresAt,
		IsActive:    key.IsActive,
		UsageStats:  key.UsageStats,
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(response); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleCostEstimate(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var costRequest cost.CostEstimateRequest
	if err := json.Unmarshal(bodyBytes, &costRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	if costRequest.Model == "" {
		http.Error(httpResponse, "Model is required", http.StatusBadRequest)
		return
	}

	costResponse, err := cost.EstimateCost(costRequest)
	if err != nil {
		s.logger.Warnw("Failed to estimate cost", "error", err, "model", costRequest.Model)
		http.Error(httpResponse, fmt.Sprintf("Failed to estimate cost: %v", err), http.StatusBadRequest)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(costResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleDeleteKey(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	// Only allow master key to delete virtual keys
	if !s.isMasterKeyRequest(httpRequest) {
		http.Error(httpResponse, "Master key required", http.StatusForbidden)
		return
	}

	keyID := strings.TrimPrefix(httpRequest.URL.Path, "/v1/keys/")
	if keyID == "" {
		http.Error(httpResponse, "Key ID required", http.StatusBadRequest)
		return
	}

	err := s.authManager.DeleteKey(httpRequest.Context(), keyID)
	if err != nil {
		s.logger.Errorw("Failed to delete virtual key", "error", err, "key_id", keyID)
		http.Error(httpResponse, "Failed to delete key", http.StatusInternalServerError)
		return
	}

	httpResponse.WriteHeader(http.StatusNoContent)
}

func (s *ModelProxy) HandleAudioTranscriptions(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	// Parse multipart form
	err := httpRequest.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		s.logger.Warnw("Failed to parse multipart form", "error", err)
		http.Error(httpResponse, "Invalid multipart form", http.StatusBadRequest)
		return
	}

	// Extract form values
	audioRequest := &openai.AudioTranscriptionRequest{
		Model: httpRequest.FormValue("model"),
	}

	if language := httpRequest.FormValue("language"); language != "" {
		audioRequest.Language = &language
	}
	if prompt := httpRequest.FormValue("prompt"); prompt != "" {
		audioRequest.Prompt = &prompt
	}
	if responseFormat := httpRequest.FormValue("response_format"); responseFormat != "" {
		audioRequest.ResponseFormat = &responseFormat
	}
	if tempStr := httpRequest.FormValue("temperature"); tempStr != "" {
		if temp, err := strconv.ParseFloat(tempStr, 32); err == nil {
			tempFloat := float32(temp)
			audioRequest.Temperature = &tempFloat
		}
	}

	// Handle file upload - for now just get the filename
	file, handler, err := httpRequest.FormFile("file")
	if err != nil {
		s.logger.Warnw("Failed to get file from form", "error", err)
		http.Error(httpResponse, "File required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	audioRequest.File = handler.Filename

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		s.logger.Errorw("Failed to read file content", "error", err)
		http.Error(httpResponse, "Failed to read file content", http.StatusBadRequest)
		return
	}
	audioRequest.FileContent = fileBytes

	// Set default model if not specified
	if audioRequest.Model == "" {
		audioRequest.Model = ogemSdk.ModelOpenAIWhisper1
	}

	models := strings.Split(audioRequest.Model, ",")
	s.logger.Infow("Received audio transcription request", "models", models)

	var audioResponse *openai.AudioTranscriptionResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		audioRequest.Model = strings.TrimSpace(model)
		audioResponse, err = s.transcribeAudio(httpRequest.Context(), audioRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to transcribe audio", "error", err, "model", model)
			lastError = err
			continue
		}
		break
	}

	if audioResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(audioResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleAudioTranslations(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	// Parse multipart form
	err := httpRequest.ParseMultipartForm(32 << 20) // 32MB
	if err != nil {
		s.logger.Warnw("Failed to parse multipart form", "error", err)
		http.Error(httpResponse, "Invalid multipart form", http.StatusBadRequest)
		return
	}

	// Extract form values
	audioRequest := &openai.AudioTranslationRequest{
		Model: httpRequest.FormValue("model"),
	}

	if prompt := httpRequest.FormValue("prompt"); prompt != "" {
		audioRequest.Prompt = &prompt
	}
	if responseFormat := httpRequest.FormValue("response_format"); responseFormat != "" {
		audioRequest.ResponseFormat = &responseFormat
	}
	if tempStr := httpRequest.FormValue("temperature"); tempStr != "" {
		if temp, err := strconv.ParseFloat(tempStr, 32); err == nil {
			tempFloat := float32(temp)
			audioRequest.Temperature = &tempFloat
		}
	}

	// Handle file upload - for now just get the filename
	file, handler, err := httpRequest.FormFile("file")
	if err != nil {
		s.logger.Warnw("Failed to get file from form", "error", err)
		http.Error(httpResponse, "File required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	audioRequest.File = handler.Filename

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		s.logger.Errorw("Failed to read file content", "error", err)
		http.Error(httpResponse, "Failed to read file content", http.StatusBadRequest)
		return
	}
	audioRequest.FileContent = fileBytes

	// Set default model if not specified
	if audioRequest.Model == "" {
		audioRequest.Model = ogemSdk.ModelOpenAIWhisper1
	}

	models := strings.Split(audioRequest.Model, ",")
	s.logger.Infow("Received audio translation request", "models", models)

	var audioResponse *openai.AudioTranslationResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		audioRequest.Model = strings.TrimSpace(model)
		audioResponse, err = s.translateAudio(httpRequest.Context(), audioRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to translate audio", "error", err, "model", model)
			lastError = err
			continue
		}
		break
	}

	if audioResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(audioResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) HandleAudioSpeech(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var speechRequest openai.TextToSpeechRequest
	if err := json.Unmarshal(bodyBytes, &speechRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set default model if not specified
	if speechRequest.Model == "" {
		speechRequest.Model = ogemSdk.ModelOpenAITTS1
	}

	models := strings.Split(speechRequest.Model, ",")
	s.logger.Infow("Received speech generation request", "models", models)

	var speechResponse *openai.TextToSpeechResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		speechRequest.Model = strings.TrimSpace(model)
		speechResponse, err = s.generateSpeech(httpRequest.Context(), &speechRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to generate speech", "error", err, "model", model)
			lastError = err
			continue
		}
		break
	}

	if speechResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	// Return raw audio data
	httpResponse.Header().Set("Content-Type", "audio/mpeg")
	httpResponse.Write(speechResponse.Data)
}

func (s *ModelProxy) HandleModerations(httpResponse http.ResponseWriter, httpRequest *http.Request) {
	defer httpRequest.Body.Close()

	bodyBytes, err := io.ReadAll(httpRequest.Body)
	if err != nil {
		s.logger.Warnw("Failed to read request body", "error", err)
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	var moderationRequest openai.ModerationRequest
	if err := json.Unmarshal(bodyBytes, &moderationRequest); err != nil {
		s.logger.Warnw("Invalid request body", "error", err, "body", string(bodyBytes))
		http.Error(httpResponse, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set default model if not specified
	if moderationRequest.Model == nil {
		defaultModel := ogemSdk.ModelOpenAIModeration
		moderationRequest.Model = &defaultModel
	}

	models := strings.Split(*moderationRequest.Model, ",")
	s.logger.Infow("Received moderation request", "models", models)

	var moderationResponse *openai.ModerationResponse
	var lastError error
	lastIndex := len(models) - 1
	for index, model := range models {
		*moderationRequest.Model = strings.TrimSpace(model)
		moderationResponse, err = s.moderateContent(httpRequest.Context(), &moderationRequest, index == lastIndex)
		if err != nil {
			s.logger.Warnw("Failed to moderate content", "error", err, "model", model)
			lastError = err
			continue
		}
		break
	}

	if moderationResponse == nil {
		handleError(httpResponse, lastError)
		return
	}

	httpResponse.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(httpResponse).Encode(moderationResponse); err != nil {
		s.logger.Errorw("Failed to encode response", "error", err)
		http.Error(httpResponse, "Internal server error", http.StatusInternalServerError)
	}
}

func (s *ModelProxy) isMasterKeyRequest(httpRequest *http.Request) bool {
	if s.config.MasterApiKey == "" {
		return false
	}

	authHeader := httpRequest.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	headerSplit := strings.Split(authHeader, " ")
	if len(headerSplit) != 2 || strings.ToLower(headerSplit[0]) != "bearer" {
		return false
	}

	return headerSplit[1] == s.config.MasterApiKey
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

	// Check virtual key permissions if using virtual key authentication
	if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
		key := virtualKey.(*auth.VirtualKey)
		if _, err := s.authManager.ValidateKey(ctx, key.Key, openAiRequest.Model); err != nil {
			s.logger.Warnw("Virtual key validation failed", "error", err, "key_id", key.ID, "model", openAiRequest.Model)
			return nil, BadRequestError{fmt.Errorf("key validation failed: %v", err)}
		}
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

	// Use intelligent routing if router is available
	selectedEndpoint, allEndpoints, err := s.intelligentRouteRequest(ctx, endpointProvider, endpointRegion, modelOrAlias, openAiRequest)
	if err != nil || len(allEndpoints) == 0 {
		s.logger.Warnw("Failed to select endpoint", "error", err, "provider", endpointProvider, "region", endpointRegion, "model", modelOrAlias)
		return nil, UnavailableError{fmt.Errorf("no available endpoints")}
	}

	// Try the intelligently selected endpoint first, then fall back to others
	endpoints := []*endpointStatus{selectedEndpoint}
	for _, ep := range allEndpoints {
		if ep != selectedEndpoint {
			endpoints = append(endpoints, ep)
		}
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

			// Record start time for latency tracking
			startTime := time.Now()
			openAiResponse, err := endpoint.endpoint.GenerateChatCompletion(ctx, openAiRequest)
			requestLatency := time.Since(startTime)
			if err != nil {
				// Record failure with router
				if s.router != nil {
					routingEndpoint := &routing.EndpointStatus{
						Endpoint:    endpoint.endpoint,
						Latency:     endpoint.latency,
						ModelStatus: endpoint.modelStatus,
					}
					requestCost := cost.CalculateChatCost(openAiRequest.Model, openai.Usage{PromptTokens: 100}) // Estimate for failed requests
					s.router.RecordRequestResult(routingEndpoint, requestLatency, requestCost, false, err.Error())
				}

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

			// Calculate request cost
			calculatedCost := cost.CalculateChatCost(openAiRequest.Model, openAiResponse.Usage)

			// Record success with router
			if s.router != nil {
				routingEndpoint := &routing.EndpointStatus{
					Endpoint:    endpoint.endpoint,
					Latency:     endpoint.latency,
					ModelStatus: endpoint.modelStatus,
				}
				s.router.RecordRequestResult(routingEndpoint, requestLatency, calculatedCost, true, "")
			}

			// Update virtual key usage if using virtual key authentication
			if virtualKey := ctx.Value("virtual_key"); virtualKey != nil {
				key := virtualKey.(*auth.VirtualKey)
				tokens := int64(openAiResponse.Usage.TotalTokens)
				if err := s.authManager.UpdateUsage(ctx, key.Key, tokens, calculatedCost); err != nil {
					s.logger.Warnw("Failed to update virtual key usage", "error", err, "key_id", key.ID)
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

	// Use default latency-based sorting if no router is configured
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].latency < endpoints[j].latency
	})

	s.logger.Infow("Selected endpoint", "endpoints", array.Map(endpoints, func(e *endpointStatus) string {
		return fmt.Sprintf("%s/%s/%s", e.endpoint.Provider(), e.endpoint.Region(), model)
	}), "model", model)
	return endpoints, nil
}

// intelligentRouteRequest uses the advanced router to select the best endpoint
func (s *ModelProxy) intelligentRouteRequest(ctx context.Context, desiredProvider string, desiredRegion string, model string, request *openai.ChatCompletionRequest) (*endpointStatus, []*endpointStatus, error) {
	// Get all available endpoints
	allEndpoints, err := s.sortedEndpoints(desiredProvider, desiredRegion, model)
	if err != nil || len(allEndpoints) == 0 {
		return nil, allEndpoints, fmt.Errorf("no available endpoints")
	}

	// Convert to routing.EndpointStatus format
	var routingEndpoints []*routing.EndpointStatus
	for _, ep := range allEndpoints {
		routingEndpoints = append(routingEndpoints, &routing.EndpointStatus{
			Endpoint:    ep.endpoint,
			Latency:     ep.latency,
			ModelStatus: ep.modelStatus,
		})
	}

	// Use intelligent router if available
	if s.router != nil {
		selectedEndpoint, err := s.router.RouteRequest(ctx, routingEndpoints, request)
		if err != nil {
			s.logger.Warnw("Intelligent routing failed, falling back to default", "error", err)
			return allEndpoints[0], allEndpoints, nil
		}

		// Find the corresponding endpointStatus
		for _, ep := range allEndpoints {
			if ep.endpoint == selectedEndpoint.Endpoint {
				return ep, allEndpoints, nil
			}
		}
	}

	// Fallback to first endpoint
	return allEndpoints[0], allEndpoints, nil
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

// RegisterAdminRoutes registers admin UI routes if admin server is available
func (s *ModelProxy) RegisterAdminRoutes(mux *http.ServeMux) {
	if s.adminServer != nil {
		s.adminServer.RegisterRoutes(mux)
	}
}

// RegisterRoutingRoutes registers routing API routes if router is available
func (s *ModelProxy) RegisterRoutingRoutes(mux *http.ServeMux) {
	if s.router != nil {
		apiHandler := routing.NewAPIHandler(s.router, s.logger)
		apiHandler.RegisterRoutes(mux)
	}
}
