package openai

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

type BatchChatCompletionTestCase struct {
	name           string
	request        *openai.ChatCompletionRequest
	batchJobSetup  func(*Endpoint)
	expectedError  string
	expectedResult *openai.ChatCompletionResponse
}

// MockHTTPClient provides a custom RoundTrip implementation to mock HTTP responses.
// This is necessary because http.Client does not allow direct overriding of the Do method.
// Instead, it calls the RoundTrip method of its Transport, which we override here
// to intercept HTTP requests and return mock responses without making real network calls.
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}

func TestGenerateBatchChatCompletionSuccess(t *testing.T) {
	successTestCase := BatchChatCompletionTestCase{
		name: "successful batch completion",
		request: &openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: &[]string{"Hello, how are you?"}[0],
					},
				},
			},
		},
		batchJobSetup: func(endpoint *Endpoint) {
			jobId := generateJobId(&openai.ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: &[]string{"Hello, how are you?"}[0],
						},
					},
				},
			})
			// Lock the batch job mutex before modifying batchJobs to prevent race conditions.
			// This ensures thread safety when updating shared resources.
			endpoint.batchJobMutex.Lock()
			endpoint.batchJobs[jobId] = &BatchJob{
				Id:     jobId,
				Method: "POST",
				Url:    "/v1/chat/completions",
				Body: &openai.ChatCompletionRequest{
					Model: "gpt-3.5-turbo",
					Messages: []openai.Message{
						{
							Role: "user",
							Content: &openai.MessageContent{
								String: &[]string{"Hello, how are you?"}[0],
							},
						},
					},
				},
				Status: BatchJobStatusCompleted,
				Result: &openai.ChatCompletionResponse{
					Id: "test-id",
					Choices: []openai.Choice{
						{
							Message: openai.Message{
								Role: "assistant",
								Content: &openai.MessageContent{
									String: &[]string{"I'm doing well, thank you!"}[0],
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
				},
			}
			endpoint.batchJobMutex.Unlock()
		},
		expectedResult: &openai.ChatCompletionResponse{
			Id: "test-id",
			Choices: []openai.Choice{
				{
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: &[]string{"I'm doing well, thank you!"}[0],
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
		},
	}

	t.Run(successTestCase.name, func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		successTestCase.batchJobSetup(endpoint)

		ctx := context.Background()
		result, err := endpoint.GenerateBatchChatCompletion(ctx, successTestCase.request)

		assert.NoError(t, err)
		assert.Equal(t, successTestCase.expectedResult, result)
	})
}

func TestGenerateBatchChatCompletionNotFound(t *testing.T) {
	notFoundTestCase := BatchChatCompletionTestCase{
		name: "batch job not found",
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
		batchJobSetup: func(endpoint *Endpoint) {
			// There is no batch job setup - should trigger "not found" error
		},
		expectedError: "batch job not found",
	}

	t.Run(notFoundTestCase.name, func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		endpoint.client = &http.Client{
			Transport: &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("batch job not found")
				},
			},
		}

		notFoundTestCase.batchJobSetup(endpoint)

		ctx := context.Background()
		result, err := endpoint.GenerateBatchChatCompletion(ctx, notFoundTestCase.request)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), notFoundTestCase.expectedError)
	})
}

func TestGenerateBatchChatCompletionFailed(t *testing.T) {
	failureTestCase := BatchChatCompletionTestCase{
		name: "batch job failed",
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
		batchJobSetup: func(endpoint *Endpoint) {
			jobId := generateJobId(&openai.ChatCompletionRequest{
				Model: "gpt-3.5-turbo",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: &[]string{"Hello"}[0],
						},
					},
				},
			})
			endpoint.batchJobMutex.Lock()
			endpoint.batchJobs[jobId] = &BatchJob{
				Id:     jobId,
				Method: "POST",
				Url:    "/v1/chat/completions",
				Body: &openai.ChatCompletionRequest{
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
				Status: BatchJobStatusFailed,
				Error:  fmt.Errorf("batch processing failed"),
			}
			endpoint.batchJobMutex.Unlock()
		},
		expectedError: "batch processing failed",
	}

	t.Run(failureTestCase.name, func(t *testing.T) {
		endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		failureTestCase.batchJobSetup(endpoint)

		ctx := context.Background()
		result, err := endpoint.GenerateBatchChatCompletion(ctx, failureTestCase.request)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), failureTestCase.expectedError)
	})
}
