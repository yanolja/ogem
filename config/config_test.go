package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoadConfig(t *testing.T) {
	logger := zap.NewNop().Sugar()

	t.Run("loads config from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")
		configContent := `
port: 9090
retry_interval: "2m"
ping_interval: "30m"
valkey_endpoint: "localhost:6379"
file_storage_dir: "/tmp/files"
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		config, err := LoadConfig(configFile, logger)
		require.NoError(t, err)
		assert.Equal(t, 9090, config.Port)
		assert.Equal(t, "2m", config.RetryInterval)
		assert.Equal(t, "30m", config.PingInterval)
		assert.Equal(t, "localhost:6379", config.ValkeyEndpoint)
		assert.Equal(t, "/tmp/files", config.FileStorageDir)
	})

	t.Run("uses default values", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")
		configContent := `# empty config`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		config, err := LoadConfig(configFile, logger)
		require.NoError(t, err)
		assert.Equal(t, 8080, config.Port)
		assert.Equal(t, "1m", config.RetryInterval)
		assert.Equal(t, "1h", config.PingInterval)
		assert.Equal(t, "./data/files", config.FileStorageDir)
	})

	t.Run("overrides with environment variables", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")
		configContent := `
port: 8080
valkey_endpoint: "file-value"
`
		err := os.WriteFile(configFile, []byte(configContent), 0644)
		require.NoError(t, err)

		// Set environment variables
		os.Setenv("PORT", "9999")
		os.Setenv("VALKEY_ENDPOINT", "env-value")
		os.Setenv("COHERE_API_KEY", "test-key")
		os.Setenv("FILE_STORAGE_DIR", "/env/files")
		defer func() {
			os.Unsetenv("PORT")
			os.Unsetenv("VALKEY_ENDPOINT")
			os.Unsetenv("COHERE_API_KEY")
			os.Unsetenv("FILE_STORAGE_DIR")
		}()

		config, err := LoadConfig(configFile, logger)
		require.NoError(t, err)
		assert.Equal(t, 9999, config.Port)
		assert.Equal(t, "env-value", config.ValkeyEndpoint)
		assert.Equal(t, "test-key", config.CohereApiKey)
		assert.Equal(t, "/env/files", config.FileStorageDir)
	})

	t.Run("loads config from remote URL", func(t *testing.T) {
		// Create a test server
		configContent := `
port: 7777
retry_interval: "5m"
`
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(configContent))
		}))
		defer server.Close()

		// Set environment variable to use remote URL
		os.Setenv("CONFIG_SOURCE", server.URL)
		defer os.Unsetenv("CONFIG_SOURCE")

		config, err := LoadConfig("local.yaml", logger)
		require.NoError(t, err)
		assert.Equal(t, 7777, config.Port)
		assert.Equal(t, "5m", config.RetryInterval)
	})

	t.Run("loads config with authentication token", func(t *testing.T) {
		// Create a test server that checks for token
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`port: 8888`))
		}))
		defer server.Close()

		os.Setenv("CONFIG_SOURCE", server.URL)
		os.Setenv("CONFIG_TOKEN", "test-token")
		defer func() {
			os.Unsetenv("CONFIG_SOURCE")
			os.Unsetenv("CONFIG_TOKEN")
		}()

		config, err := LoadConfig("local.yaml", logger)
		require.NoError(t, err)
		assert.Equal(t, 8888, config.Port)
	})

	t.Run("fails on invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")
		invalidYAML := `
port: not-a-number
invalid yaml [[[
`
		err := os.WriteFile(configFile, []byte(invalidYAML), 0644)
		require.NoError(t, err)

		_, err = LoadConfig(configFile, logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse config")
	})

	t.Run("fails on missing file", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/config.yaml", logger)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get config data")
	})

	t.Run("handles all API keys from environment", func(t *testing.T) {
		tmpDir := t.TempDir()
		configFile := filepath.Join(tmpDir, "config.yaml")
		err := os.WriteFile(configFile, []byte(`# minimal config`), 0644)
		require.NoError(t, err)

		os.Setenv("OPENAI_API_KEY", "openai-key")
		os.Setenv("CLAUDE_API_KEY", "claude-key")
		os.Setenv("MISTRAL_API_KEY", "mistral-key")
		os.Setenv("XAI_API_KEY", "xai-key")
		os.Setenv("GROQ_API_KEY", "groq-key")
		os.Setenv("OPENROUTER_API_KEY", "openrouter-key")
		os.Setenv("GENAI_STUDIO_API_KEY", "studio-key")
		os.Setenv("GOOGLE_CLOUD_PROJECT", "project-123")
		os.Setenv("MASTER_API_KEY", "master-key")
		os.Setenv("ENABLE_VIRTUAL_KEYS", "true")

		defer func() {
			os.Unsetenv("OPENAI_API_KEY")
			os.Unsetenv("CLAUDE_API_KEY")
			os.Unsetenv("MISTRAL_API_KEY")
			os.Unsetenv("XAI_API_KEY")
			os.Unsetenv("GROQ_API_KEY")
			os.Unsetenv("OPENROUTER_API_KEY")
			os.Unsetenv("GENAI_STUDIO_API_KEY")
			os.Unsetenv("GOOGLE_CLOUD_PROJECT")
			os.Unsetenv("MASTER_API_KEY")
			os.Unsetenv("ENABLE_VIRTUAL_KEYS")
		}()

		config, err := LoadConfig(configFile, logger)
		require.NoError(t, err)
		assert.Equal(t, "openai-key", config.OpenAiApiKey)
		assert.Equal(t, "claude-key", config.ClaudeApiKey)
		assert.Equal(t, "mistral-key", config.MistralApiKey)
		assert.Equal(t, "xai-key", config.XAIApiKey)
		assert.Equal(t, "groq-key", config.GroqApiKey)
		assert.Equal(t, "openrouter-key", config.OpenRouterApiKey)
		assert.Equal(t, "studio-key", config.GenaiStudioApiKey)
		assert.Equal(t, "project-123", config.GoogleCloudProject)
		assert.Equal(t, "master-key", config.MasterApiKey)
		assert.True(t, config.EnableVirtualKeys)
	})
}

func Test_fetchRemoteConfig(t *testing.T) {
	t.Run("fetches config successfully", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("test config content"))
		}))
		defer server.Close()

		data, err := fetchRemoteConfig(server.URL, "")
		require.NoError(t, err)
		assert.Equal(t, "test config content", string(data))
	})

	t.Run("includes bearer token when provided", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			assert.Equal(t, "Bearer my-token", auth)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("authenticated content"))
		}))
		defer server.Close()

		data, err := fetchRemoteConfig(server.URL, "my-token")
		require.NoError(t, err)
		assert.Equal(t, "authenticated content", string(data))
	})

	t.Run("fails on HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		_, err := fetchRemoteConfig(server.URL, "")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HTTP 500")
	})

	t.Run("fails on invalid URL", func(t *testing.T) {
		_, err := fetchRemoteConfig("not-a-valid-url", "")
		assert.Error(t, err)
	})

	t.Run("fails on connection refused", func(t *testing.T) {
		_, err := fetchRemoteConfig("http://localhost:99999", "")
		assert.Error(t, err)
	})
}
