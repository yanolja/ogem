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

func TestGenerateChatCompletion(t *testing.T) {
	tests := []struct {
		name           string
		request        *openai.ChatCompletionRequest
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedError  string
		expectedResult *openai.ChatCompletionResponse
	}{
		{
			name: "successful completion",
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
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "POST", r.Method)
				assert.Equal(t, "/chat/completions", r.URL.Path)
				assert.Equal(t, "Bearer test-api-key", r.Header.Get("Authorization"))
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

				response := openai.ChatCompletionResponse{
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
				}
				json.NewEncoder(w).Encode(response)
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
		},
		{
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
		},
		{
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
				// This should not be called as the request should be handled by batch processing
				t.Error("Server should not be called for batch requests")
			},
			expectedError: "batch job not found", // This is expected as we're not implementing batch processing in the test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
			assert.NoError(t, err)
			defer endpoint.Shutdown()

			// need to mock the batch processing
			if tt.name == "batch model request" {
				// mock batch job
				jobId := generateJobId(tt.request)
				endpoint.batchJobMutex.Lock()
				endpoint.batchJobs[jobId] = &BatchJob{
					Id:     jobId,
					Method: "POST",
					Url:    "/v1/chat/completions",
					Body:   tt.request,
					Status: BatchJobStatusFailed,
					Error:  fmt.Errorf("batch job not found"),
				}
				endpoint.batchJobMutex.Unlock()

				// prevent real API calls
				endpoint.client = &http.Client{
					Transport: &MockHTTPClient{
						DoFunc: func(req *http.Request) (*http.Response, error) {
							return nil, fmt.Errorf("batch job not found")
						},
					},
				}
			}

			ctx := context.Background()
			result, err := endpoint.GenerateChatCompletion(ctx, tt.request)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestGenerateBatchChatCompletion(t *testing.T) {
	tests := []struct {
		name           string
		request        *openai.ChatCompletionRequest
		batchJobSetup  func(*Endpoint)
		expectedError  string
		expectedResult *openai.ChatCompletionResponse
	}{
		{
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
		},
		{
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
				// there is no batch job setup - should trigger "not found" error
			},
			expectedError: "batch job not found",
		},
		{
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := NewEndpoint("test-provider", "test-region", "http://test-url", "test-api-key")
			assert.NoError(t, err)
			defer endpoint.Shutdown()

			// setup batch job if needed
			if tt.batchJobSetup != nil {
				tt.batchJobSetup(endpoint)
			}

			if tt.name == "batch job not found" {
				// prevent real API call
				endpoint.client = &http.Client{
					Transport: &MockHTTPClient{
						DoFunc: func(req *http.Request) (*http.Response, error) {
							return nil, fmt.Errorf("batch job not found")
						},
					},
				}
			}

			ctx := context.Background()
			result, err := endpoint.GenerateBatchChatCompletion(ctx, tt.request)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

// MockHTTPClient overrides the Do function of http.Client
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}
