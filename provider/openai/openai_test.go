package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

type testCase struct {
	name           string
	request        *openai.ChatCompletionRequest
	serverResponse http.HandlerFunc
	expectedError  string
	expectedResult *openai.ChatCompletionResponse
}

func TestGenerateChatCompletionSuccess(t *testing.T) {
	successTestCase := testCase{
		name: "successful completion",
		request: &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{{
				Role: "user",
				Content: &openai.MessageContent{
					String: &[]string{"Hello, how are you?"}[0],
				},
			}},
		},
		serverResponse: func(w http.ResponseWriter, r *http.Request) {
			json.NewEncoder(w).Encode(openai.ChatCompletionResponse{
				Id: "test-id",
				Choices: []openai.Choice{{
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: &[]string{"I'm doing well, thank you!"}[0],
						},
					},
				}},
			})
		},
		expectedResult: &openai.ChatCompletionResponse{
			Id: "test-id",
			Choices: []openai.Choice{{
				Message: openai.Message{
					Role: "assistant",
					Content: &openai.MessageContent{
						String: &[]string{"I'm doing well, thank you!"}[0],
					},
				},
			}},
		},
	}

	t.Run(successTestCase.name, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(successTestCase.serverResponse))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		ctx := context.Background()
		result, err := endpoint.GenerateChatCompletion(ctx, successTestCase.request)

		assert.NoError(t, err)
		assert.Equal(t, successTestCase.expectedResult, result)
	})
}

func TestGenerateChatCompletionQuotaExceeded(t *testing.T) {
	quotaExceededTestCase := testCase{
		name: "quota exceeded error",
		request: &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: &[]string{"Hello"}[0],
					},
				},
			},
		},
		serverResponse: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": {"message": "Rate limit exceeded"}}`))
		},
		expectedError: "quota exceeded: {\"error\": {\"message\": \"Rate limit exceeded\"}}",
	}

	t.Run(quotaExceededTestCase.name, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(quotaExceededTestCase.serverResponse))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, quotaExceededTestCase.request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), quotaExceededTestCase.expectedError)
	})
}

func TestGenerateChatCompletionBatchJobNotFound(t *testing.T) {
	batchJobNotFoundTestCase := testCase{
		name: "batch model request",
		request: &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo@batch",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: &[]string{"Hello"}[0],
					},
				},
			},
		},
		serverResponse: func(w http.ResponseWriter, r *http.Request) {
			// Should be handled by batch processing internally
			t.Error("Server should not be called for batch requests")
		},
		expectedError: "batch job not found",
	}

	t.Run(batchJobNotFoundTestCase.name, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(batchJobNotFoundTestCase.serverResponse))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		jobId := generateJobId(batchJobNotFoundTestCase.request)

		endpoint.batchJobMutex.Lock()
		endpoint.batchJobs[jobId] = &BatchJob{
			Id:     jobId,
			Method: "POST",
			Url:    "/v1/chat/completions",
			Body:   batchJobNotFoundTestCase.request,
			Status: BatchJobStatusFailed,
			Error:  fmt.Errorf("batch job not found"),
		}
		endpoint.batchJobMutex.Unlock()

		// To prevent real API calls by using a mock HTTP client
		endpoint.client = &http.Client{
			Transport: &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("batch job not found")
				},
			},
		}

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, batchJobNotFoundTestCase.request)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), batchJobNotFoundTestCase.expectedError)
	})
}

// MockHTTPClient provides a custom RoundTrip implementation to mock HTTP responses.
// This is necessary because http.Client does not allow direct overriding of the Do method.
// Instead, it calls the RoundTrip method of its Transport, which we override here
// to intercept HTTP requests and return mock responses without making real network calls.
// Ref: https://go.dev/src/net/http/client.go#L114
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}
