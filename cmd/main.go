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

	"go.uber.org/zap"

	"github.com/rs/cors"
	"github.com/valkey-io/valkey-go"
	"github.com/yanolja/ogem/auth"
	"github.com/yanolja/ogem/config"
	"github.com/yanolja/ogem/server"
	"github.com/yanolja/ogem/state"
	"github.com/yanolja/ogem/utils"
)

func main() {
	logger := utils.Must(zap.NewProduction())
	defer logger.Sync()
	sugar := logger.Sugar()

	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()
	config, err := config.LoadConfig(*configPath, sugar)
	if err != nil {
		sugar.Fatalw("Failed to load config", "error", err)
	}

	stateManager, cleanup, err := setupStateManager(config.ValkeyEndpoint)
	if err != nil {
		sugar.Fatalw("Failed to setup state manager", "error", err)
	}

	sugar.Infow("Loaded config", "config", config)

	// Setup auth manager
	var authManager auth.Manager
	if config.EnableVirtualKeys {
		authManager = auth.NewMemoryManager()
		sugar.Infow("Virtual keys authentication enabled")
	}

	proxy, err := server.NewProxyServer(stateManager, cleanup, config, authManager, sugar)
	if err != nil {
		sugar.Fatalw("Failed to create proxy server", "error", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", proxy.HandleAuthentication(proxy.HandleChatCompletions))
	mux.HandleFunc("/v1/embeddings", proxy.HandleAuthentication(proxy.HandleEmbeddings))
	mux.HandleFunc("/v1/images/generations", proxy.HandleAuthentication(proxy.HandleImages))
	mux.HandleFunc("/v1/audio/transcriptions", proxy.HandleAuthentication(proxy.HandleAudioTranscriptions))
	mux.HandleFunc("/v1/audio/translations", proxy.HandleAuthentication(proxy.HandleAudioTranslations))
	mux.HandleFunc("/v1/audio/speech", proxy.HandleAuthentication(proxy.HandleAudioSpeech))
	mux.HandleFunc("/v1/moderations", proxy.HandleAuthentication(proxy.HandleModerations))
	mux.HandleFunc("/v1/cost/estimate", proxy.HandleCostEstimate) // No authentication required for cost estimation
	
	// Virtual key management endpoints (only available if virtual keys are enabled)
	if config.EnableVirtualKeys {
		mux.HandleFunc("/v1/keys", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodPost:
				proxy.HandleCreateKey(w, r)
			case http.MethodGet:
				proxy.HandleListKeys(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
		mux.HandleFunc("/v1/keys/", func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				proxy.HandleGetKey(w, r)
			case http.MethodDelete:
				proxy.HandleDeleteKey(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		})
	}

	httpServer := setupServer(config.Port, mux)

	shutdownSignal := make(chan os.Signal, 1)
	signal.Notify(shutdownSignal, os.Interrupt, syscall.SIGTERM)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if pingInterval := proxy.PingInterval(); pingInterval > 0 {
		sugar.Infow("Starting ping loop", "interval", pingInterval)
		go proxy.StartPingLoop(ctx)
	} else {
		sugar.Infow("Ping loop disabled")
	}

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

	sugar.Infow("Starting server", "address", httpServer.Addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		sugar.Fatalw("Failed to start server", "error", err)
	}

	sugar.Infow("Server exited gracefully")
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

func setupServer(port int, handler http.Handler) *http.Server {
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
		Debug:          false,
	})

	address := fmt.Sprintf(":%d", port)

	return &http.Server{
		Addr:    address,
		Handler: corsMiddleware.Handler(handler),
	}
}
