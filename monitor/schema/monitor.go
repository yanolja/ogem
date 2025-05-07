package schema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"go.uber.org/zap"
)

const (
	OpenAISchemaURL = "https://raw.githubusercontent.com/openai/openai-openapi/master/openapi.yaml"
	cacheKeyPrefix  = "schema_cache:"
)

// Monitor handles schema monitoring for various API providers
type Monitor struct {
	logger     *zap.SugaredLogger
	httpClient *http.Client
	cache      Cache
	notifier   Notifier
}

// Cache interface for storing schema hashes
type Cache interface {
	Get(key string) (string, error)
	Set(key string, value string) error
}

// Notifier interface for sending notifications about schema changes
type Notifier interface {
	NotifySchemaChange(provider string, oldHash, newHash string) error
}

// NewMonitor creates a new schema monitor
func NewMonitor(logger *zap.SugaredLogger, cache Cache, notifier Notifier) *Monitor {
	return &Monitor{
		logger: logger,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		cache:    cache,
		notifier: notifier,
	}
}

// CheckOpenAISchema fetches and compares the OpenAI schema
func (m *Monitor) CheckOpenAISchema(ctx context.Context) error {
	m.logger.Info("Checking OpenAI schema for changes...")

	// Fetch current schema
	schema, err := m.fetchSchema(ctx, OpenAISchemaURL)
	if err != nil {
		return fmt.Errorf("failed to fetch OpenAI schema: %w", err)
	}

	// Calculate hash of new schema
	newHash := calculateSchemaHash(schema)

	// Get previous hash from cache
	cacheKey := cacheKeyPrefix + "openai"
	oldHash, err := m.cache.Get(cacheKey)
	if err != nil {
		m.logger.Warnw("Failed to get previous schema hash from cache", "error", err)
		// Continue execution to store the new hash
	}

	// If we have an old hash and it's different from the new one
	if oldHash != "" && oldHash != newHash {
		m.logger.Infow("Schema change detected",
			"provider", "OpenAI",
			"oldHash", oldHash,
			"newHash", newHash,
		)

		// Notify about the change
		if err := m.notifier.NotifySchemaChange("OpenAI", oldHash, newHash); err != nil {
			m.logger.Errorw("Failed to send schema change notification", "error", err)
		}
	}

	// Store new hash in cache
	if err := m.cache.Set(cacheKey, newHash); err != nil {
		return fmt.Errorf("failed to cache schema hash: %w", err)
	}

	// Validate schema
	if err := m.validateSchema(schema); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	m.logger.Info("OpenAI schema check completed successfully")
	return nil
}

// fetchSchema downloads the OpenAPI schema from the given URL
func (m *Monitor) fetchSchema(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch schema: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

// validateSchema ensures the schema is valid OpenAPI 3.0
func (m *Monitor) validateSchema(schemaData []byte) error {
	if len(schemaData) == 0 {
		return fmt.Errorf("empty schema")
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(schemaData)
	if err != nil {
		return fmt.Errorf("invalid OpenAPI schema: %w", err)
	}

	// Additional validation checks
	if doc.Info == nil || doc.Info.Version == "" {
		return fmt.Errorf("missing required field: info.version")
	}

	return nil
}

// calculateSchemaHash generates a SHA-256 hash of the normalized schema content
func calculateSchemaHash(data []byte) string {
	// Normalize JSON by parsing and re-marshaling
	var normalized interface{}
	if err := json.Unmarshal(data, &normalized); err == nil {
		if normalizedData, err := json.Marshal(normalized); err == nil {
			data = normalizedData
		}
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
