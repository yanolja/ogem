package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

func TestGenerateChatCompletionSuccess(t *testing.T) {
	t.Run("successful completion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
				Id: "test-id",
				Choices: []openai.Choice{{
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: ptr("I'm doing well, thank you!"),
						},
					},
				}},
			})
		}))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)

		defer endpoint.Shutdown()

		request := &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{{
				Role: "user",
				Content: &openai.MessageContent{
					String: ptr("Hello, how are you?"),
				},
			}},
		}

		expectedResponse := &openai.ChatCompletionResponse{
			Id: "test-id",
			Choices: []openai.Choice{{
				Message: openai.Message{
					Role: "assistant",
					Content: &openai.MessageContent{
						String: ptr("I'm doing well, thank you!"),
					},
				},
			}},
		}

		ctx := context.Background()
		result, err := endpoint.GenerateChatCompletion(ctx, request)

		assert.NoError(t, err)
		assert.Equal(t, expectedResponse, result)
	})
}

func TestGenerateChatCompletionQuotaExceeded(t *testing.T) {
	t.Run("quota exceeded error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": {"message": "Rate limit exceeded"}}`))
		}))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		request := &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: ptr("Hello"),
					},
				},
			},
		}

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, request)

		expectedError := `quota exceeded: {"error": {"message": "Rate limit exceeded"}}`
		assert.Error(t, err)
		assert.Contains(t, err.Error(), expectedError)
	})
}

func TestGenerateChatCompletionInternalServerError(t *testing.T) {
	t.Run("internal server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": {"message": "Internal server error"}}`))
		}))

		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		request := &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: ptr("Hello"),
					},
				},
			},
		}

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Internal server error")
	})
}

func TestGenerateChatCompletionBadRequest(t *testing.T) {
	t.Run("bad request error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error": {"message": "Invalid request payload"}}`))
		}))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		request := &openai.ChatCompletionRequest{}

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Invalid request payload")
	})
}

func ptr(s string) *string {
	return &s
}
