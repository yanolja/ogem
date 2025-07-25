package config

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"

	"github.com/yanolja/ogem"
	"github.com/yanolja/ogem/routing"
	"github.com/yanolja/ogem/utils/env"
)

// Config represents the full application configuration
type Config struct {
	// Valkey (open-source version of Redis) endpoint to store rate limiting information.
	// E.g., localhost:6379
	ValkeyEndpoint string `yaml:"valkey_endpoint"`

	// API key to access the Ogem service. The user should provide this key in the Authorization header with the Bearer scheme.
	OgemApiKey string

	// Project ID of the Google Cloud project to use Vertex AI.
	// E.g., my-project-12345
	GoogleCloudProject string `yaml:"google_cloud_project"`

	// API key to access the GenAI Studio service.
	GenaiStudioApiKey string

	// API key to access the OpenAI service.
	OpenAiApiKey string

	// API key to access the Claude service.
	ClaudeApiKey string

	// API key to access the Mistral AI service.
	MistralApiKey string

	// API key to access the xAI service.
	XAIApiKey string

	// API key to access the Groq service.
	GroqApiKey string

	// API key to access the OpenRouter service.
	OpenRouterApiKey string

	// Interval to retry when no available endpoints are found. E.g., 10m
	RetryInterval string `yaml:"retry_interval"`

	// Interval to update the status of the providers. E.g., 1h30m
	PingInterval string `yaml:"ping_interval"`

	// Port to listen for incoming requests.
	Port int `yaml:"port"`

	// Configuration for each provider.
	Providers ogem.ProvidersStatus `yaml:"providers"`

	// Enable virtual key authentication system
	EnableVirtualKeys bool `yaml:"enable_virtual_keys"`

	// Master API key for managing virtual keys
	MasterApiKey string

	// Advanced routing configuration
	Routing *routing.RoutingConfig `yaml:"routing,omitempty"`
}

// LoadConfig loads the configuration from the specified path
func LoadConfig(path string, logger *zap.SugaredLogger) (*Config, error) {
	// Setting default values
	config := Config{
		ValkeyEndpoint: "",
		OgemApiKey:     "",
		RetryInterval:  "1m",
		PingInterval:   "1h",
		Port:           8080,
		Providers:      ogem.ProvidersStatus{},
	}

	// Checks if config is specified via environment variable.
	configSource := env.OptionalStringVariable("CONFIG_SOURCE", path)
	configToken := env.OptionalStringVariable("CONFIG_TOKEN", "")
	configData, err := func(configSource string, configToken string) ([]byte, error) {
		// Handle URL or local path
		if strings.HasPrefix(configSource, "http://") || strings.HasPrefix(configSource, "https://") {
			logger.Infow("Fetching remote config", "url", configSource)
			return fetchRemoteConfig(configSource, configToken)
		}
		logger.Infow("Loading local config", "path", configSource)
		return os.ReadFile(configSource)
	}(configSource, configToken)

	if err != nil {
		return nil, fmt.Errorf("failed to get config data: %v", err)
	}

	// Overrides config with the YAML data.
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %v", err)
	}

	// Overrides config with environment variables.
	// Therefore, the values from the environment variables precede the values from the YAML file.
	config.ValkeyEndpoint = env.OptionalStringVariable("VALKEY_ENDPOINT", config.ValkeyEndpoint)
	config.OgemApiKey = env.OptionalStringVariable("OPEN_GEMINI_API_KEY", config.OgemApiKey)
	config.GoogleCloudProject = env.OptionalStringVariable("GOOGLE_CLOUD_PROJECT", config.GoogleCloudProject)
	config.OpenAiApiKey = env.OptionalStringVariable("OPENAI_API_KEY", config.OpenAiApiKey)
	config.GenaiStudioApiKey = env.OptionalStringVariable("GENAI_STUDIO_API_KEY", config.GenaiStudioApiKey)
	config.ClaudeApiKey = env.OptionalStringVariable("CLAUDE_API_KEY", config.ClaudeApiKey)
	config.MistralApiKey = env.OptionalStringVariable("MISTRAL_API_KEY", config.MistralApiKey)
	config.XAIApiKey = env.OptionalStringVariable("XAI_API_KEY", config.XAIApiKey)
	config.GroqApiKey = env.OptionalStringVariable("GROQ_API_KEY", config.GroqApiKey)
	config.OpenRouterApiKey = env.OptionalStringVariable("OPENROUTER_API_KEY", config.OpenRouterApiKey)
	config.RetryInterval = env.OptionalStringVariable("RETRY_INTERVAL", config.RetryInterval)
	config.PingInterval = env.OptionalStringVariable("PING_INTERVAL", config.PingInterval)
	config.Port = env.OptionalIntVariable("PORT", config.Port)
	config.EnableVirtualKeys = env.OptionalBoolVariable("ENABLE_VIRTUAL_KEYS", config.EnableVirtualKeys)
	config.MasterApiKey = env.OptionalStringVariable("MASTER_API_KEY", config.MasterApiKey)

	return &config, nil
}

func fetchRemoteConfig(url string, token string) ([]byte, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch config: HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
