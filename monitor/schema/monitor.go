package schema

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/getkin/kin-openapi/openapi3"
	"go.uber.org/zap"
)

const (
	OpenAISchemaURL = "https://raw.githubusercontent.com/openai/openai-openapi/master/openapi.yaml"
	GeminiSchemaURL = "https://raw.githubusercontent.com/googleapis/google-cloud-go/main/vertexai/generativelanguage/apiv1/generativelanguage_v1.swagger.json"
	ClaudeSchemaURL = "https://raw.githubusercontent.com/anthropics/anthropic-openapi/main/openapi.yaml"
	cacheKeyPrefix  = "schema_cache:"
)

// Provider represents supported LLM API providers.
// These specific providers were chosen because:
// - They offer production-ready, stable APIs
// - Maintain public OpenAPI specifications
// - Have significant market adoption
// - Provide complementary capabilities
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderGoogle    Provider = "google"
	ProviderAnthropic Provider = "anthropic"
)

func GetSchemaURL(provider Provider) string {
	switch provider {
	case ProviderOpenAI:
		return OpenAISchemaURL
	case ProviderGoogle:
		return GeminiSchemaURL
	case ProviderAnthropic:
		return ClaudeSchemaURL
	default:
		return ""
	}
}

// HTTPClient abstracts HTTP operations to enable:
// - Mocking in tests without real network calls
// - Custom timeout and retry policies
// - Request/response logging and monitoring
// - Circuit breaking for fault tolerance
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// Monitor handles schema monitoring for various API providers
type Monitor struct {
	logger     *zap.SugaredLogger
	httpClient HTTPClient
	cache      Cache
	notifier   Notifier
}

type Cache interface {
	Get(key string) (string, error)
	Set(key string, value string) error
}
type Notifier interface {
	NotifySchemaChange(provider string, oldHash, newHash string) error
}

func NewMonitor(logger *zap.SugaredLogger, httpClient HTTPClient, cache Cache, notifier Notifier) *Monitor {
	return &Monitor{
		logger:     logger,
		httpClient: httpClient,
		cache:      cache,
		notifier:   notifier,
	}
}

// CheckSchemas checks for changes in all supported API schemas
func (m *Monitor) CheckSchemas(ctx context.Context) error {
	m.logger.Info("Starting schema checks for all providers...")
	providers := []Provider{ProviderOpenAI, ProviderGoogle, ProviderAnthropic}

	for _, provider := range providers {
		if err := m.checkProviderSchema(ctx, provider); err != nil {
			m.logger.Errorw("failed to check schema", "provider", provider, "error", err)
		}
	}
	m.logger.Info("Completed schema checks for all providers")
	return nil
}

// checkProviderSchema implements a robust schema monitoring process:
// 1. Fetches latest schema from provider's GitHub
// 2. Validates schema structure to catch malformed updates
// 3. Uses hash comparison to detect any changes efficiently
// 4. Maintains state in Redis for persistence across runs
// 5. Notifies team when breaking changes might affect integration
func (m *Monitor) checkProviderSchema(ctx context.Context, provider Provider) error {
	m.logger.Infow("Checking schema for provider", "provider", provider)

	schemaURL := GetSchemaURL(provider)
	if schemaURL == "" {
		return fmt.Errorf("unknown provider: %s", provider)
	}

	schema, err := m.fetchSchema(ctx, schemaURL)
	if err != nil {
		return fmt.Errorf("failed to fetch schema for %s: %w", provider, err)
	}
	if err := m.validateSchema(schema); err != nil {
		m.logger.Warnw("Schema validation failed", "provider", provider, "error", err)
	}

	hash, err := m.calculateSchemaHash(schema)
	if err != nil {
		return fmt.Errorf("failed to calculate hash for %s: %w", provider, err)
	}
	cacheKey := cacheKeyPrefix + string(provider)
	previousHash, err := m.cache.Get(cacheKey)
	if err != nil {
		m.logger.Warnw("failed to get previous hash from cache", "provider", provider, "error", err)
	}

	if previousHash != "" && hash != previousHash {
		m.logger.Infow("Schema change detected",
			"provider", provider,
			"oldHash", previousHash,
			"newHash", hash,
		)
		if err := m.notifier.NotifySchemaChange(string(provider), previousHash, hash); err != nil {
			m.logger.Errorw("failed to send notification", "provider", provider, "error", err)
		}
	}

	if err := m.cache.Set(cacheKey, hash); err != nil {
		return fmt.Errorf("failed to update cache for %s: %w", provider, err)
	}

	m.logger.Infow("Schema check completed", "provider", provider)
	return nil
}

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

func (m *Monitor) validateSchema(data []byte) error {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return fmt.Errorf("failed to load OpenAPI schema: %w", err)
	}

	if doc.Info == nil {
		return fmt.Errorf("missing info section")
	}
	if doc.Info.Version == "" {
		return fmt.Errorf("missing version in info section")
	}
	if doc.Info.Title == "" {
		return fmt.Errorf("missing title in info section")
	}

	return nil
}

func (m *Monitor) calculateSchemaHash(data []byte) (string, error) {
	var normalized interface{}
	if err := json.Unmarshal(data, &normalized); err == nil {
		normalizedData, err := json.Marshal(normalized)
		if err != nil {
			return "", fmt.Errorf("failed to marshal schema: %w", err)
		}
		data = normalizedData
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
