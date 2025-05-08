package schema

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.uber.org/zap"
)

type MockCache struct {
	mock.Mock
}

func (m *MockCache) Get(key string) (string, error) {
	args := m.Called(key)
	return args.String(0), args.Error(1)
}

func (m *MockCache) Set(key string, value string) error {
	args := m.Called(key, value)
	return args.Error(0)
}

type MockNotifier struct {
	mock.Mock
}

type MockHTTPClient struct {
	mock.Mock
}

func (m *MockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	return args.Get(0).(*http.Response), args.Error(1)
}

func (m *MockNotifier) NotifySchemaChange(provider string, oldHash, newHash string) error {
	args := m.Called(provider, oldHash, newHash)
	return args.Error(0)
}

func TestMonitor_CheckSchemas(t *testing.T) {
	logger := zap.NewNop().Sugar()

	// Mock HTTP client
	httpClient := &MockHTTPClient{}

	tests := []struct {
		name          string
		provider      Provider
		schemaContent string
		setup         func(*MockCache, *MockNotifier)
	}{
		{
			name:     "OpenAI schema change detected",
			provider: ProviderOpenAI,
			schemaContent: `{
				"openapi": "3.0.0",
				"info": {
					"title": "OpenAI API",
					"version": "1.0.0"
				},
				"paths": {}
			}`,
			setup: func(cache *MockCache, notifier *MockNotifier) {
				cache.On("Get", "schema_cache:openai").Return("old-hash", nil)
				cache.On("Set", "schema_cache:openai", mock.AnythingOfType("string")).Return(nil)
				notifier.On("NotifySchemaChange", "openai", "old-hash", mock.AnythingOfType("string")).Return(nil)
			},
		},
		{
			name:     "Google schema change detected",
			provider: ProviderGoogle,
			schemaContent: `{
				"openapi": "3.0.0",
				"info": {
					"title": "Gemini API",
					"version": "1.0.0"
				},
				"paths": {}
			}`,
			setup: func(cache *MockCache, notifier *MockNotifier) {
				cache.On("Get", "schema_cache:google").Return("old-hash", nil)
				cache.On("Set", "schema_cache:google", mock.AnythingOfType("string")).Return(nil)
				notifier.On("NotifySchemaChange", "google", "old-hash", mock.AnythingOfType("string")).Return(nil)
			},
		},
		{
			name:     "Anthropic schema change detected",
			provider: ProviderAnthropic,
			schemaContent: `{
				"openapi": "3.0.0",
				"info": {
					"title": "Claude API",
					"version": "1.0.0"
				},
				"paths": {}
			}`,
			setup: func(cache *MockCache, notifier *MockNotifier) {
				cache.On("Get", "schema_cache:anthropic").Return("old-hash", nil)
				cache.On("Set", "schema_cache:anthropic", mock.AnythingOfType("string")).Return(nil)
				notifier.On("NotifySchemaChange", "anthropic", "old-hash", mock.AnythingOfType("string")).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := &MockCache{}
			notifier := &MockNotifier{}
			tt.setup(cache, notifier)

			// Set up HTTP client mock
			httpClient.On("Do", mock.AnythingOfType("*http.Request")).Return(&http.Response{
				StatusCode: http.StatusOK,
				Body:      io.NopCloser(strings.NewReader(tt.schemaContent)),
			}, nil)

			monitor := NewMonitor(logger, httpClient, cache, notifier)
			err := monitor.checkProviderSchema(context.Background(), tt.provider)

			assert.NoError(t, err)
			cache.AssertExpectations(t)
			notifier.AssertExpectations(t)
			httpClient.AssertExpectations(t)
		})
	}
}

func TestGetSchemaURL(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
		want     string
	}{
		{
			name:     "OpenAI URL",
			provider: ProviderOpenAI,
			want:     OpenAISchemaURL,
		},
		{
			name:     "Google URL",
			provider: ProviderGoogle,
			want:     GeminiSchemaURL,
		},
		{
			name:     "Anthropic URL",
			provider: ProviderAnthropic,
			want:     ClaudeSchemaURL,
		},
		{
			name:     "Unknown provider",
			provider: "unknown",
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSchemaURL(tt.provider)
			assert.Equal(t, tt.want, got)
		})
	}
}



func TestCalculateSchemaHash(t *testing.T) {
	monitor := NewMonitor(zap.NewNop().Sugar(), &http.Client{}, &MockCache{}, &MockNotifier{})

	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "simple string",
			data: []byte("test data"),
			want: "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9",
		},
		{
			name: "empty data",
			data: []byte{},
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // SHA-256 of empty string
		},
		{
			name: "OpenAPI schema - normalized",
			data: []byte(`{"info":{"title":"Test API","version":"1.0.0"},"openapi":"3.0.0"}`),
			want: "65b2e6596adb1111c95c38723de4cf0e84f72a38791c44ba1041f25aefc89ff2",
		},
		{
			name: "OpenAPI schema - with whitespace",
			data: []byte(`{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API",
					"version": "1.0.0"
				}
			}`),
			want: "65b2e6596adb1111c95c38723de4cf0e84f72a38791c44ba1041f25aefc89ff2", // Same as normalized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := monitor.calculateSchemaHash(tt.data)
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestHashConsistency ensures that the same input always produces the same hash
func TestHashConsistency(t *testing.T) {
	monitor := NewMonitor(zap.NewNop().Sugar(), &http.Client{}, &MockCache{}, &MockNotifier{})
	data := []byte("test data")

	hash1, err1 := monitor.calculateSchemaHash(data)
	assert.NoError(t, err1)

	hash2, err2 := monitor.calculateSchemaHash(data)
	assert.NoError(t, err2)

	assert.Equal(t, hash1, hash2, "Same input should produce same hash")
}

// TestHashDifferentInputs ensures that different inputs produce different hashes
func TestHashDifferentInputs(t *testing.T) {
	monitor := NewMonitor(zap.NewNop().Sugar(), &http.Client{}, &MockCache{}, &MockNotifier{})
	data1 := []byte("test data 1")
	data2 := []byte("test data 2")

	hash1, err1 := monitor.calculateSchemaHash(data1)
	assert.NoError(t, err1)

	hash2, err2 := monitor.calculateSchemaHash(data2)
	assert.NoError(t, err2)

	assert.NotEqual(t, hash1, hash2, "Different inputs should produce different hashes")
}

// TestValidateSchema tests the schema validation functionality
func TestValidateSchema(t *testing.T) {
	monitor := NewMonitor(zap.NewNop().Sugar(), &http.Client{}, &MockCache{}, &MockNotifier{})

	tests := []struct {
		name    string
		schema  string
		wantErr bool
	}{
		{
			name: "Valid OpenAPI schema",
			schema: `{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API",
					"version": "1.0.0"
				},
				"paths": {}
			}`,
			wantErr: false,
		},
		{
			name: "Invalid OpenAPI schema - missing version",
			schema: `{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API"
				},
				"paths": {}
			}`,
			wantErr: true,
		},
		{
			name: "Invalid JSON",
			schema: `{"invalid json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monitor.validateSchema([]byte(tt.schema))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
