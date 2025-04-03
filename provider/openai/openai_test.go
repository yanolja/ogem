package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

func TestGenerateBatchChatCompletionSuccess(t *testing.T) {
	t.Run("successful batch completion", func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		setupMockBatchJob(endpoint)

		ctx := context.Background()
		request := generateTestChatCompletionRequest()
		expectedResponse := generateTestChatCompletionResponse()

		result, err := endpoint.GenerateBatchChatCompletion(ctx, request)

		assert.NoError(t, err)
		assert.Equal(t, expectedResponse, result)
	})

	t.Run("failed batch completion due to invalid request", func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		setupMockBatchJob(endpoint)

		ctx := context.Background()
		invalidRequest := generateInvalidTestChatCompletionRequest()

		result, err := endpoint.GenerateBatchChatCompletion(ctx, invalidRequest)

		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("failed batch completion due to service error", func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		err = setupMockBatchJobWithError(endpoint)
		assert.NoError(t, err)

		ctx := context.Background()
		request := generateTestChatCompletionRequest()

		result, err := endpoint.GenerateBatchChatCompletion(ctx, request)

		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func setupMockBatchJob(endpoint *Endpoint) {
	jobId := generateJobId(generateTestChatCompletionRequest())

	endpoint.batchJobMutex.Lock()
	defer endpoint.batchJobMutex.Unlock()

	endpoint.batchJobs[jobId] = &BatchJob{
		Id:     jobId,
		Method: "POST",
		Url:    "/v1/chat/completions",
		Body:   generateTestChatCompletionRequest(),
		Status: BatchJobStatusCompleted,
		Result: generateTestChatCompletionResponse(),
	}
}

func generateTestChatCompletionRequest() *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: ptr("Hello, how are you?"),
				},
			},
		},
	}
}

func generateTestChatCompletionResponse() *openai.ChatCompletionResponse {
	return &openai.ChatCompletionResponse{
		Id: "test-id",
		Choices: []openai.Choice{
			{
				Message: openai.Message{
					Role: "assistant",
					Content: &openai.MessageContent{
						String: ptr("I'm doing well, thank you!"),
					},
				},
			},
		},
		Created:           1234567890,
		Model:             "gpt-3.5-turbo",
		SystemFingerprint: "test-fingerprint",
		Object:            "chat.completion",
		Usage: openai.Usage{
			PromptTokens:     10,
			CompletionTokens: 20,
			TotalTokens:      30,
			CompletionTokensDetails: openai.CompletionTokensDetails{
				ReasoningTokens: 15,
			},
		},
	}
}

func generateInvalidTestChatCompletionRequest() *openai.ChatCompletionRequest {
	return &openai.ChatCompletionRequest{
		Messages: nil,
	}
}
func setupMockBatchJobWithError(endpoint *Endpoint) error {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))

	parsedURL, err := url.Parse(mockServer.URL)
	if err != nil {
		return err
	}

	endpoint.baseUrl = parsedURL
	return nil
}

func ptr(s string) *string {
	return &s
}

func TestGenerateBatchChatCompletionFailed(t *testing.T) {
	t.Run("batch job failed", func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		setupMockFailedBatchJob(endpoint)

		ctx := context.Background()
		request := generateTestChatCompletionRequest()

		result, err := endpoint.GenerateBatchChatCompletion(ctx, request)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "batch processing failed")
	})
}

func setupMockFailedBatchJob(endpoint *Endpoint) {
	jobId := generateJobId(generateTestChatCompletionRequest())

	endpoint.batchJobMutex.Lock()
	defer endpoint.batchJobMutex.Unlock()

	endpoint.batchJobs[jobId] = &BatchJob{
		Id:     jobId,
		Method: "POST",
		Url:    "/v1/chat/completions",
		Body:   generateTestChatCompletionRequest(),
		Status: BatchJobStatusFailed,
		Error:  fmt.Errorf("batch processing failed"),
	}
}
