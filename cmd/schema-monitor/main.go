package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/monitor/schema"
	"github.com/yanolja/ogem/utils"
)

func main() {
	logger := utils.Must(zap.NewProduction())
	defer logger.Sync()
	sugar := logger.Sugar()

	// Get configuration from environment variables
	valkeyEndpoint := os.Getenv("VALKEY_ENDPOINT")
	if valkeyEndpoint == "" {
		sugar.Fatal("VALKEY_ENDPOINT environment variable is required")
	}

	slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackWebhookURL == "" {
		sugar.Fatal("SLACK_WEBHOOK_URL environment variable is required")
	}

	// Initialize schema monitor
	httpClient := &http.Client{Timeout: 30 * time.Second}
	cache := schema.NewRedisCache(valkeyEndpoint)
	notifier := schema.NewSlackNotifier(slackWebhookURL)
	monitor := schema.NewMonitor(sugar, httpClient, cache, notifier)

	// Run a single check
	ctx := context.Background()
	if err := monitor.CheckSchemas(ctx); err != nil {
		sugar.Fatalw("Schema check failed", "error", err)
	}

	sugar.Info("Schema check completed successfully")
}
