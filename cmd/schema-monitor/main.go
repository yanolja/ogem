package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/yanolja/ogem/monitor/schema"
	"github.com/yanolja/ogem/state"
	"go.uber.org/zap"

	"github.com/yanolja/ogem/utils"
)

func main() {
	logger := utils.Must(zap.NewProduction())
	defer logger.Sync()
	sugar := logger.Sugar()

	valkeyEndpoint := os.Getenv("VALKEY_ENDPOINT")
	if valkeyEndpoint == "" {
		sugar.Fatal("VALKEY_ENDPOINT environment variable is required")
	}

	slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackWebhookURL == "" {
		sugar.Fatal("SLACK_WEBHOOK_URL environment variable is required")
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	memoryManager, _ := state.NewMemoryManager(1000) // Cache size of 1000 entries
	notifier := schema.NewSlackNotifier(slackWebhookURL)
	monitor := schema.NewMonitor(sugar, httpClient, memoryManager, notifier)

	if err := monitor.CheckSchemas(context.Background()); err != nil {
		sugar.Fatalw("Schema check failed", "error", err)
	}

	sugar.Info("Schema check completed successfully")
}
