package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/cors"
	"github.com/valkey-io/valkey-go"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/server"
	"github.com/yanolja/ogem/state"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/env"
)

func loadConfig(path string) (*server.Config, error) {
	// Setting default values.
	config := server.Config{
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

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %v", err)
	}
	defer file.Close()

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

func setupStateManager(valkeyEndpoint string) (state.Manager, func(), error) {
	if valkeyEndpoint == "" {
		// Maximum memory usage of 2GB.
		memoryManager, cleanup := state.NewMemoryManager(2 * 1024 * 1024 * 1024)
		return memoryManager, cleanup, nil
	}

	valkeyClient, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{valkeyEndpoint},
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create Valkey client: %v", err)
	}
	return state.NewValkeyManager(valkeyClient), nil, nil
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

	stateManager, cleanup, err := setupStateManager(config.ValkeyEndpoint)
	if err != nil {
		sugar.Fatalw("Failed to setup state manager", "error", err)
	}

	sugar.Infow("Loaded config", "config", config)

	proxy, err := server.NewProxyServer(stateManager, cleanup, *config, sugar)
	if err != nil {
		sugar.Fatalw("Failed to create proxy server", "error", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", proxy.HandleAuthentication(proxy.HandleChatCompletions))

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

	go proxy.StartPingLoop(ctx)

	go func() {
		<-shutdownSignal
		sugar.Infow("Shutting down server...")

		proxy.Shutdown()

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
