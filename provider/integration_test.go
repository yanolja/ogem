package provider_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
	"github.com/yanolja/ogem/provider/azure"
	"github.com/yanolja/ogem/provider/claude"
	openai_provider "github.com/yanolja/ogem/provider/openai"
)

// TestProviderIntegration tests basic provider functionality
func TestProviderIntegration(t *testing.T) {
	// Skip integration tests if not explicitly enabled
	if os.Getenv("OGEM_INTEGRATION_TESTS") != "true" {
		t.Skip("Integration tests disabled. Set OGEM_INTEGRATION_TESTS=true to enable.")
	}

	tests := []struct {
		name     string
		provider func() (provider.AiEndpoint, error)
		envKey   string
	}{
		{
			name: "openai_provider",
			provider: func() (provider.AiEndpoint, error) {
				apiKey := os.Getenv("OPENAI_API_KEY")
				if apiKey == "" {
					return nil, nil // Skip if no API key
				}
				return openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", apiKey)
			},
			envKey: "OPENAI_API_KEY",
		},
		{
			name: "claude_provider",
			provider: func() (provider.AiEndpoint, error) {
				apiKey := os.Getenv("ANTHROPIC_API_KEY")
				if apiKey == "" {
					return nil, nil // Skip if no API key
				}
				return claude.NewEndpoint(apiKey)
			},
			envKey: "ANTHROPIC_API_KEY",
		},
		{
			name: "azure_provider",
			provider: func() (provider.AiEndpoint, error) {
				apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
				endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
				deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")
				if apiKey == "" || endpoint == "" || deployment == "" {
					return nil, nil // Skip if missing config
				}
				return azure.NewEndpoint("azure", endpoint, apiKey, "2024-02-15-preview")
			},
			envKey: "AZURE_OPENAI_API_KEY",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if os.Getenv(tt.envKey) == "" {
				t.Skipf("Skipping %s test - %s not set", tt.name, tt.envKey)
			}

			endpoint, err := tt.provider()
			if endpoint == nil {
				t.Skipf("Skipping %s test - endpoint creation returned nil", tt.name)
			}
			require.NoError(t, err)
			require.NotNil(t, endpoint)

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// Test basic functionality
			t.Run("ping", func(t *testing.T) {
				latency, err := endpoint.Ping(ctx)
				assert.NoError(t, err)
				assert.Greater(t, latency, time.Duration(0))
				assert.Less(t, latency, 10*time.Second)
			})

			t.Run("chat_completion", func(t *testing.T) {
				request := &openai.ChatCompletionRequest{
					Model: getModelForProvider(tt.name),
					Messages: []openai.Message{
						{
							Role: "user",
							Content: &openai.MessageContent{
								String: stringPtr("Hello, can you respond with just 'Hi'?"),
							},
						},
					},
					MaxTokens:   int32Ptr(10),
					Temperature: float32Ptr(0.7),
				}

				response, err := endpoint.GenerateChatCompletion(ctx, request)
				assert.NoError(t, err)
				assert.NotNil(t, response)
				assert.NotEmpty(t, response.Id)
				assert.NotEmpty(t, response.Choices)
				assert.NotEmpty(t, response.Choices[0].Message.Content)
			})

			// Test shutdown
			t.Run("shutdown", func(t *testing.T) {
				err := endpoint.Shutdown()
				assert.NoError(t, err)
			})
		})
	}
}

// TestProviderCompatibility tests provider compatibility with OpenAI API format
func TestProviderCompatibility(t *testing.T) {
	if os.Getenv("OGEM_INTEGRATION_TESTS") != "true" {
		t.Skip("Integration tests disabled. Set OGEM_INTEGRATION_TESTS=true to enable.")
	}

	testCases := []struct {
		name    string
		request *openai.ChatCompletionRequest
	}{
		{
			name: "simple_message",
			request: &openai.ChatCompletionRequest{
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("What is 2+2?"),
						},
					},
				},
				MaxTokens:   int32Ptr(20),
				Temperature: float32Ptr(0.1),
			},
		},
		{
			name: "conversation",
			request: &openai.ChatCompletionRequest{
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("Hello"),
						},
					},
					{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: stringPtr("Hi there! How can I help you?"),
						},
					},
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("Tell me a very short joke"),
						},
					},
				},
				MaxTokens:   int32Ptr(50),
				Temperature: float32Ptr(0.7),
			},
		},
		{
			name: "with_system_message",
			request: &openai.ChatCompletionRequest{
				Messages: []openai.Message{
					{
						Role: "system",
						Content: &openai.MessageContent{
							String: stringPtr("You are a helpful assistant that always responds with exactly one word."),
						},
					},
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("How are you?"),
						},
					},
				},
				MaxTokens:   int32Ptr(5),
				Temperature: float32Ptr(0.1),
			},
		},
	}

	providers := []struct {
		name     string
		endpoint func() (provider.AiEndpoint, error)
		envKey   string
	}{
		{
			name: "openai",
			endpoint: func() (provider.AiEndpoint, error) {
				apiKey := os.Getenv("OPENAI_API_KEY")
				if apiKey == "" {
					return nil, nil
				}
				return openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", apiKey)
			},
			envKey: "OPENAI_API_KEY",
		},
	}

	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			if os.Getenv(provider.envKey) == "" {
				t.Skipf("Skipping %s test - %s not set", provider.name, provider.envKey)
			}

			endpoint, err := provider.endpoint()
			if endpoint == nil {
				t.Skipf("Skipping %s test - endpoint creation returned nil", provider.name)
			}
			require.NoError(t, err)
			require.NotNil(t, endpoint)
			defer endpoint.Shutdown()

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					defer cancel()

					tc.request.Model = getModelForProvider(provider.name)

					response, err := endpoint.GenerateChatCompletion(ctx, tc.request)
					assert.NoError(t, err)
					assert.NotNil(t, response)

					// Validate response structure
					assert.NotEmpty(t, response.Id)
					assert.NotEmpty(t, response.Object)
					assert.Greater(t, response.Created, int64(0))
					assert.NotEmpty(t, response.Model)
					assert.NotEmpty(t, response.Choices)

					// Validate first choice
					choice := response.Choices[0]
					assert.GreaterOrEqual(t, choice.Index, 0)
					assert.NotNil(t, choice.Message)
					assert.Equal(t, "assistant", choice.Message.Role)
					assert.NotNil(t, choice.Message.Content)
					assert.NotEmpty(t, *choice.Message.Content.String)

					// Validate usage
					assert.Greater(t, response.Usage.TotalTokens, int32(0))
					assert.GreaterOrEqual(t, response.Usage.PromptTokens, int32(0))
					assert.GreaterOrEqual(t, response.Usage.CompletionTokens, int32(0))
				})
			}
		})
	}
}

// TestProviderErrorHandling tests how providers handle various error conditions
func TestProviderErrorHandling(t *testing.T) {
	if os.Getenv("OGEM_INTEGRATION_TESTS") != "true" {
		t.Skip("Integration tests disabled. Set OGEM_INTEGRATION_TESTS=true to enable.")
	}

	// Test with invalid API key
	t.Run("invalid_api_key", func(t *testing.T) {
		endpoint, err := openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", "invalid-api-key")
		require.NoError(t, err)
		defer endpoint.Shutdown()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		request := &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: stringPtr("Hello"),
					},
				},
			},
		}

		response, err := endpoint.GenerateChatCompletion(ctx, request)
		assert.Error(t, err)
		assert.Nil(t, response)
		assert.Contains(t, err.Error(), "401")
	})

	// Test with invalid model
	if os.Getenv("OPENAI_API_KEY") != "" {
		t.Run("invalid_model", func(t *testing.T) {
			endpoint, err := openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"))
			require.NoError(t, err)
			defer endpoint.Shutdown()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			request := &openai.ChatCompletionRequest{
				Model: "invalid-model-name",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("Hello"),
						},
					},
				},
			}

			response, err := endpoint.GenerateChatCompletion(ctx, request)
			assert.Error(t, err)
			assert.Nil(t, response)
		})
	}

	// Test with context timeout
	if os.Getenv("OPENAI_API_KEY") != "" {
		t.Run("context_timeout", func(t *testing.T) {
			endpoint, err := openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"))
			require.NoError(t, err)
			defer endpoint.Shutdown()

			// Very short timeout
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
			defer cancel()

			request := &openai.ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("Hello"),
						},
					},
				},
			}

			response, err := endpoint.GenerateChatCompletion(ctx, request)
			assert.Error(t, err)
			assert.Nil(t, response)
			assert.Contains(t, err.Error(), "context deadline exceeded")
		})
	}
}

// TestProviderStreamingSupport tests streaming capabilities where supported
func TestProviderStreamingSupport(t *testing.T) {
	if os.Getenv("OGEM_INTEGRATION_TESTS") != "true" {
		t.Skip("Integration tests disabled. Set OGEM_INTEGRATION_TESTS=true to enable.")
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping streaming test - OPENAI_API_KEY not set")
	}

	endpoint, err := openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"))
	require.NoError(t, err)
	defer endpoint.Shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	request := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Count from 1 to 5, one number per line."),
				},
			},
		},
		MaxTokens:   int32Ptr(50),
		Temperature: float32Ptr(0.1),
		Stream:      boolPtr(true),
	}

	responseChan, errorChan := endpoint.GenerateChatCompletionStream(ctx, request)

	var chunks []*openai.ChatCompletionStreamResponse
	var streamErr error

	for {
		select {
		case chunk, ok := <-responseChan:
			if !ok {
				// Channel closed, streaming complete
				goto StreamComplete
			}
			chunks = append(chunks, chunk)

		case err, ok := <-errorChan:
			if ok {
				streamErr = err
			}
			goto StreamComplete

		case <-ctx.Done():
			streamErr = ctx.Err()
			goto StreamComplete
		}
	}

StreamComplete:
	if streamErr != nil {
		t.Errorf("Streaming error: %v", streamErr)
		return
	}

	assert.Greater(t, len(chunks), 0, "Should receive at least one chunk")

	// Validate chunk structure
	for i, chunk := range chunks {
		assert.NotEmpty(t, chunk.Id, "Chunk %d should have ID", i)
		assert.NotEmpty(t, chunk.Object, "Chunk %d should have object type", i)
		assert.Greater(t, chunk.Created, int64(0), "Chunk %d should have created timestamp", i)
		assert.NotEmpty(t, chunk.Model, "Chunk %d should have model", i)
		assert.NotEmpty(t, chunk.Choices, "Chunk %d should have choices", i)

		choice := chunk.Choices[0]
		assert.GreaterOrEqual(t, choice.Index, 0, "Chunk %d choice should have valid index", i)

		// Last chunk should have finish_reason
		if i == len(chunks)-1 {
			assert.NotEmpty(t, choice.FinishReason, "Last chunk should have finish_reason")
		}
	}
}

// TestProviderConcurrency tests provider behavior under concurrent load
func TestProviderConcurrency(t *testing.T) {
	if os.Getenv("OGEM_INTEGRATION_TESTS") != "true" {
		t.Skip("Integration tests disabled. Set OGEM_INTEGRATION_TESTS=true to enable.")
	}

	if os.Getenv("OPENAI_API_KEY") == "" {
		t.Skip("Skipping concurrency test - OPENAI_API_KEY not set")
	}

	endpoint, err := openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", os.Getenv("OPENAI_API_KEY"))
	require.NoError(t, err)
	defer endpoint.Shutdown()

	concurrency := 5
	requests := 10

	type result struct {
		response *openai.ChatCompletionResponse
		err      error
		duration time.Duration
	}

	results := make(chan result, concurrency*requests)

	// Launch concurrent requests
	for i := 0; i < concurrency; i++ {
		go func(workerID int) {
			for j := 0; j < requests; j++ {
				start := time.Now()

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

				request := &openai.ChatCompletionRequest{
					Model: "gpt-3.5-turbo",
					Messages: []openai.Message{
						{
							Role: "user",
							Content: &openai.MessageContent{
								String: stringPtr(fmt.Sprintf("Worker %d, request %d: What is %d + %d?", workerID, j, j, j+1)),
							},
						},
					},
					MaxTokens:   int32Ptr(20),
					Temperature: float32Ptr(0.1),
				}

				response, err := endpoint.GenerateChatCompletion(ctx, request)
				duration := time.Since(start)

				results <- result{
					response: response,
					err:      err,
					duration: duration,
				}

				cancel()
			}
		}(i)
	}

	// Collect results
	successCount := 0
	errorCount := 0
	totalDuration := time.Duration(0)

	for i := 0; i < concurrency*requests; i++ {
		result := <-results
		if result.err != nil {
			errorCount++
			t.Logf("Request failed: %v", result.err)
		} else {
			successCount++
			assert.NotNil(t, result.response)
			assert.NotEmpty(t, result.response.Choices)
		}
		totalDuration += result.duration
	}

	assert.Greater(t, successCount, 0, "At least some requests should succeed")
	assert.Less(t, float64(errorCount)/float64(concurrency*requests), 0.1, "Error rate should be less than 10%")

	avgDuration := totalDuration / time.Duration(concurrency*requests)
	t.Logf("Concurrent test results: %d success, %d errors, avg duration: %v", successCount, errorCount, avgDuration)
}

// Helper functions

func getModelForProvider(providerName string) string {
	switch providerName {
	case "openai", "openai_provider":
		return "gpt-4o"
	case "claude", "claude_provider":
		return "claude-3-5-haiku-20241022"
	case "azure", "azure_provider":
		return "gpt-35-turbo" // Azure often uses different model names
	case "vertex", "vertex_provider":
		return "gemini-2.5-pro"
	default:
		return "gpt-4o"
	}
}

func stringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func float32Ptr(f float32) *float32 {
	return &f
}

func boolPtr(b bool) *bool {
	return &b
}

// TestProviderInterface ensures all providers implement the AiEndpoint interface correctly
func TestProviderInterface(t *testing.T) {
	// This test verifies that our provider implementations satisfy the interface
	// without requiring actual API calls

	tests := []struct {
		name     string
		provider func() provider.AiEndpoint
	}{
		{
			name: "openai_endpoint",
			provider: func() provider.AiEndpoint {
				endpoint, _ := openai_provider.NewEndpoint("openai", "openai", "https://api.openai.com/v1", "test-key")
				return endpoint
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := tt.provider()
			assert.NotNil(t, provider)

			// Test interface methods exist
			assert.NotEmpty(t, provider.Provider())
			assert.NotEmpty(t, provider.Region())

			// Test shutdown doesn't panic
			err := provider.Shutdown()
			assert.NoError(t, err)
		})
	}
}

// TestProviderConfiguration tests various provider configuration scenarios
func TestProviderConfiguration(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		apiKey    string
		wantError bool
	}{
		{
			name:      "valid_config",
			baseURL:   "https://api.openai.com/v1",
			apiKey:    "test-key",
			wantError: false,
		},
		{
			name:      "invalid_url",
			baseURL:   "not-a-valid-url",
			apiKey:    "test-key",
			wantError: true,
		},
		{
			name:      "empty_api_key",
			baseURL:   "https://api.openai.com/v1",
			apiKey:    "",
			wantError: false, // Some providers may allow empty keys for testing
		},
		{
			name:      "custom_base_url",
			baseURL:   "https://custom-endpoint.example.com/v1",
			apiKey:    "custom-key",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := openai_provider.NewEndpoint("openai", "openai", tt.baseURL, tt.apiKey)

			if tt.wantError {
				assert.Error(t, err)
				assert.Nil(t, endpoint)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, endpoint)
				assert.Equal(t, "openai", endpoint.Provider())
				assert.Equal(t, "openai", endpoint.Region())

				// Clean up
				if endpoint != nil {
					endpoint.Shutdown()
				}
			}
		})
	}
}
