package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/goccy/go-json"
	"github.com/rs/cors"
	"github.com/valkey-io/valkey-go"
	ogem "github.com/yanolja/ogem/api"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
	"github.com/yanolja/ogem/provider/claude"
	openaiProvider "github.com/yanolja/ogem/provider/openai"
	"github.com/yanolja/ogem/provider/studio"
	"github.com/yanolja/ogem/provider/vclaude"
	"github.com/yanolja/ogem/provider/vertex"
	"github.com/yanolja/ogem/rate"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/array"
	"github.com/yanolja/ogem/utils/copy"
	"github.com/yanolja/ogem/utils/env"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

type (
	BadRequestError     struct{ error }
	InternalServerError struct{ error }
	RateLimitError      struct{ error }
	RequestTimeoutError struct{ error }
	UnavailableError    struct{ error }
)

type Config struct {
	// Valkey (open-source version of Redis) endpoint to store rate limiting information.
	// E.g., localhost:6379
	ValkeyEndpoint string `yaml:"valkey_endpoint"`

	// API key to access the Ogem service. The user should provide this key in the Authorization header with the Bearer scheme.
	OgemApiKey string `yaml:"api_key"`

	// Project ID of the Google Cloud project to use Vertex AI.
	// E.g., my-project-12345
	GoogleCloudProject string `yaml:"google_cloud_project"`

	// API key to access the GenAI Studio service.
	GenaiStudioApiKey string `yaml:"genai_studio_api_key"`

	// API key to access the OpenAI service.
	OpenAiApiKey string `yaml:"openai_api_key"`

	// API key to access the Claude service.
	ClaudeApiKey string `yaml:"claude_api_key"`

	// Interval to retry when no available endpoints are found. E.g., 10m
	RetryInterval string `yaml:"retry_interval"`

	// Interval to update the status of the providers. E.g., 1h30m
	PingInterval string `yaml:"ping_interval"`

	// Port to listen for incoming requests.
	Port int `yaml:"port"`

	// Configuration for each provider.
	Providers ogem.ProvidersStatus `yaml:"providers"`
}

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

	// Valkey client to access the Valkey service. Used for caching and rate limiting.
	valkeyClient valkey.Client

	// Limiter to limit the number of requests per minute.
	limiter *rate.Limiter

	// Interval to retry when no available endpoints are found.
	retryInterval time.Duration

	// Interval to update the status of the providers.
	pingInterval time.Duration

	// Configuration for the proxy server.
	config Config

	// Logger for the proxy server.
	logger *zap.SugaredLogger
}

func newEndpoint(provider string, region string, config *Config) (provider.AiEndpoint, error) {
	switch provider {
	case "claude":
		return claude.NewEndpoint(config.ClaudeApiKey)
	case "vclaude":
		return vclaude.NewEndpoint(config.GoogleCloudProject, region)
	case "vertex":
		return vertex.NewEndpoint(config.GoogleCloudProject, region)
	case "studio":
		return studio.NewEndpoint(config.GenaiStudioApiKey)
	case "openai":
		return openaiProvider.NewEndpoint(config.OpenAiApiKey), nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func NewProxyServer(valkeyClient valkey.Client, config Config, logger *zap.SugaredLogger) (*ModelProxy, error) {
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
		provider string,
		_ ogem.ProviderStatus,
		region string,
		_ ogem.RegionStatus,
		models []*ogem.SupportedModel,
	) bool {
		endpoint, err := newEndpoint(provider, region, &config)
		if err != nil {
			logger.Warnw("Failed to create endpoint", "provider", provider, "region", region, "error", err)
			return false
		}
		endpoints = append(endpoints, endpoint)
		return false
	})

	return &ModelProxy{
		endpoints:      endpoints,
		endpointStatus: endpointStatus,
		valkeyClient:   valkeyClient,
		limiter:        rate.NewLimiter(valkeyClient, logger),
		retryInterval:  retryInterval,
		pingInterval:   pingInterval,
		config:         config,
		logger:         logger,
	}, nil
}

func (s *ModelProxy) handleChatCompletions(httpResponse http.ResponseWriter, httpRequest *http.Request) {
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

			accepted, waiting, err := s.limiter.CanProceed(
				ctx, endpoint.endpoint.Provider(), endpoint.endpoint.Region(), modelOrAlias, requestInterval(endpoint.modelStatus))
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
					s.limiter.DisableEndpointTemporarily(ctx, endpoint.endpoint.Provider(), endpoint.endpoint.Region(), modelOrAlias, 1*time.Minute)
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

func (s *ModelProxy) handleAuthentication(handler http.HandlerFunc) http.HandlerFunc {
	return func(httpResponse http.ResponseWriter, httpRequest *http.Request) {
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
	return newEndpoint(provider, region, &s.config)
}

func (s *ModelProxy) startPingLoop(ctx context.Context) {
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

	valkeyResponse := s.valkeyClient.Do(ctx, s.valkeyClient.B().Get().Key(cacheKey).Build())
	if err := valkeyResponse.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	jsonBytes, err := valkeyResponse.AsBytes()
	if err != nil {
		return nil, fmt.Errorf("failed to get cached response: %v", err)
	}

	var cachedResponse openai.ChatCompletionResponse
	if err := json.Unmarshal(jsonBytes, &cachedResponse); err != nil {
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

	return s.valkeyClient.Do(
		ctx, s.valkeyClient.B().Set().Key(cacheKey).Value(string(jsonBytes)).Build(),
	).Error()
}

func (s *ModelProxy) shutdown() {
	s.logger.Info("Shutting down ModelProxy")
	s.valkeyClient.Close()
	for _, endpoint := range s.endpoints {
		if err := endpoint.Shutdown(); err != nil {
			s.logger.Warnw("Failed to shutdown endpoint", "error", err)
		}
	}
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

func loadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

	// Setting default values.
	config := Config{
		ValkeyEndpoint: "localhost:6379",
		OgemApiKey:     "",
		RetryInterval:  "1m",
		PingInterval:   "1h",
		Port:           8080,
		Providers: ogem.ProvidersStatus{
			"openai": &ogem.ProviderStatus{
				Regions: map[string]*ogem.RegionStatus{
					"default": {
						Models: []*ogem.SupportedModel{
							{
								Name:                 "gpt-4o-mini",
								RateKey:              "gpt-4o-mini",
								MaxRequestsPerMinute: 500,
								MaxTokensPerMinute:   200_000,
							},
						},
					},
				},
			},
			"studio": &ogem.ProviderStatus{
				Regions: map[string]*ogem.RegionStatus{
					"default": {
						Models: []*ogem.SupportedModel{
							{
								Name:                 "gemini-1.5-flash",
								RateKey:              "gemini-1.5-flash",
								MaxRequestsPerMinute: 500,
								MaxTokensPerMinute:   200_000,
							},
						},
					},
				},
			},
		},
	}

	// Overrides config with the given YAML file.
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %v", err)
	}

	// Overrides config with environment variables.
	// Therefore, the values from the environment variables precede the values from the YAML file.
	config.ValkeyEndpoint = env.OptionalStringVariable("VALKEY_ENDPOINT", config.ValkeyEndpoint)
	config.OgemApiKey = env.OptionalStringVariable("OPEN_GEMINI_API_KEY", config.OgemApiKey)
	config.GenaiStudioApiKey = env.OptionalStringVariable("GENAI_STUDIO_API_KEY", config.GenaiStudioApiKey)
	config.GoogleCloudProject = env.OptionalStringVariable("GOOGLE_CLOUD_PROJECT", config.GoogleCloudProject)
	config.OpenAiApiKey = env.OptionalStringVariable("OPENAI_API_KEY", config.OpenAiApiKey)
	config.ClaudeApiKey = env.OptionalStringVariable("CLAUDE_API_KEY", config.ClaudeApiKey)
	config.RetryInterval = env.OptionalStringVariable("RETRY_INTERVAL", config.RetryInterval)
	config.PingInterval = env.OptionalStringVariable("PING_INTERVAL", config.PingInterval)
	config.Port = env.OptionalIntVariable("PORT", config.Port)

	return &config, nil
}

func main() {
	logger := utils.Must(zap.NewProduction())
	defer logger.Sync()
	sugar := logger.Sugar()

	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()
	config, err := loadConfig(*configPath)
	if err != nil {
		sugar.Fatalw("Failed to load config", "error", err)
	}

	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{config.ValkeyEndpoint},
	})
	if err != nil {
		sugar.Fatalw("Failed to create Valkey client", "error", err)
	}
	defer valkeyClient.Close()

	sugar.Infow("Loaded config", "config", config)

	proxy, err := NewProxyServer(valkeyClient, *config, sugar)
	if err != nil {
		sugar.Fatalw("Failed to create proxy server", "error", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", proxy.handleAuthentication(proxy.handleChatCompletions))

	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		Debug:          false,
	})

	port := env.OptionalStringVariable("PORT", "8080")
	address := fmt.Sprintf(":%s", port)

	httpServer := &http.Server{
		Addr:    address,
		Handler: corsMiddleware.Handler(mux),
	}

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proxy.startPingLoop(ctx)

	go func() {
		<-shutdownSignal
		sugar.Infow("Shutting down server...")

		proxy.shutdown()

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			sugar.Fatalw("Server forced to shutdown", "error", err)
		}
	}()

	sugar.Infow("Starting server", "address", address)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		sugar.Fatalw("Failed to start server", "error", err)
	}

	sugar.Infow("Server exited gracefully")
}
