package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/yanolja/ogem/auth"
	"github.com/yanolja/ogem/config"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/server"
	"github.com/yanolja/ogem/state"
)

var (
	testServer *httptest.Server
	testProxy  *server.ModelProxy
	testAPIKey = "test-api-key"
)

func setupTestServer(t *testing.T) {
	if testServer != nil {
		return // Already set up
	}

	// Create test config
	cfg := &config.Config{
		OgemApiKey: testAPIKey,
		Port:       8080,
		// Add minimal provider config for testing
		Providers: map[string]interface{}{},
	}

	// Create test state manager
	stateManager := state.NewMemoryManager()

	// Create test auth manager
	authManager := auth.NewMemoryManager()

	// Create test proxy
	proxy, err := server.NewModelProxy(cfg, stateManager, authManager)
	if err != nil {
		t.Fatalf("Failed to create test proxy: %v", err)
	}

	testProxy = proxy

	// Create test HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/images/generations", proxy.HandleAuthentication(proxy.HandleImages))
	mux.HandleFunc("/v1/moderations", proxy.HandleAuthentication(proxy.HandleModerations))
	mux.HandleFunc("/v1/audio/transcriptions", proxy.HandleAuthentication(proxy.HandleAudioTranscriptions))
	mux.HandleFunc("/v1/audio/translations", proxy.HandleAuthentication(proxy.HandleAudioTranslations))
	mux.HandleFunc("/v1/audio/speech", proxy.HandleAuthentication(proxy.HandleAudioSpeech))

	testServer = httptest.NewServer(mux)
}

func makeRequest(t *testing.T, method, path string, body interface{}) ([]byte, int, error) {
	setupTestServer(t)

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to marshal request: %v", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequest(method, testServer.URL+path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+testAPIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("failed to read response: %v", err)
	}

	return respBody, resp.StatusCode, nil
}

func generateImage(ctx context.Context, request *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	// Skip if no API key configured
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set - skipping actual API call")
	}

	// This would make real API calls in integration tests
	// For unit tests, we'd use a mock
	return nil, fmt.Errorf("not yet implemented - need actual provider configuration")
}

func moderateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	// Skip if no API key configured
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set - skipping actual API call")
	}

	return nil, fmt.Errorf("not yet implemented - need actual provider configuration")
}

func transcribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	// Skip if no API key configured
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set - skipping actual API call")
	}

	return nil, fmt.Errorf("not yet implemented - need actual provider configuration")
}

func translateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	// Skip if no API key configured
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set - skipping actual API call")
	}

	return nil, fmt.Errorf("not yet implemented - need actual provider configuration")
}

func generateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	// Skip if no API key configured
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY not set - skipping actual API call")
	}

	return nil, fmt.Errorf("not yet implemented - need actual provider configuration")
}
