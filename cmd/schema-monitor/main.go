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

	// Configuration is managed through environment variables to:
	// - Support secure credential management in CI/CD
	// - Enable easy configuration across environments
	// - Prevent accidental credential commits
	valkeyEndpoint := os.Getenv("VALKEY_ENDPOINT")
	if valkeyEndpoint == "" {
		sugar.Fatal("VALKEY_ENDPOINT environment variable is required")
	}

	slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackWebhookURL == "" {
		sugar.Fatal("SLACK_WEBHOOK_URL environment variable is required")
	}

	// Initialize monitor components with carefully chosen defaults:
	// - 30s timeout balances thorough schema parsing with fail-fast
	// - Redis provides persistent, concurrent-safe schema caching
	// - Slack enables immediate team-wide change visibility
	httpClient := &http.Client{Timeout: 30 * time.Second}
	cache := schema.NewRedisCache(valkeyEndpoint)
	notifier := schema.NewSlackNotifier(slackWebhookURL)
	monitor := schema.NewMonitor(sugar, httpClient, cache, notifier)

	// Execute schema check as a one-shot operation.
	// GitHub Actions handles scheduling, which provides:
	// - Reliable cron-based execution
	// - Built-in retry mechanisms
	// - Automatic failure notifications
	if err := monitor.CheckSchemas(context.Background()); err != nil {
		sugar.Fatalw("Schema check failed", "error", err)
	}

	sugar.Info("Schema check completed successfully")
}
