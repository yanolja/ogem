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
	// Define a test case for successful completion
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
		// Mock server response to simulate OpenAI API response
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
		// Expected result to be returned by the API
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

	// Run the test case
	t.Run(successTestCase.name, func(t *testing.T) {
		// Create a new server to mock the OpenAI API response
		server := httptest.NewServer(http.HandlerFunc(successTestCase.serverResponse))
		defer server.Close()

		// Create a new endpoint to test the OpenAI API
		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		// Call the GenerateChatCompletion method
		ctx := context.Background()
		result, err := endpoint.GenerateChatCompletion(ctx, successTestCase.request)

		// Assert that the result is as expected (no error occured and the result is as expected)
		assert.NoError(t, err)
		assert.Equal(t, successTestCase.expectedResult, result)
	})
}

func TestGenerateChatCompletionQuotaExceeded(t *testing.T) {
	// Define a test case for quota exceeded error (rate limit)
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
		// Mock server response to simulate a 429 too many requests error
		serverResponse: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": {"message": "Rate limit exceeded"}}`))
		},
		// Expected error message to be returned by the API
		expectedError: "quota exceeded: {\"error\": {\"message\": \"Rate limit exceeded\"}}",
	}

	// Run the test case
	t.Run(quotaExceededTestCase.name, func(t *testing.T) {
		// Create a new server to mock the OpenAI API response
		server := httptest.NewServer(http.HandlerFunc(quotaExceededTestCase.serverResponse))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, quotaExceededTestCase.request)

		// Assert that an error occured and the error message is as expected
		assert.Error(t, err)
		assert.Contains(t, err.Error(), quotaExceededTestCase.expectedError)
	})
}

func TestGenerateChatCompletionBatchJobNotFound(t *testing.T) {
	// Define a test case to simulate a batch model request where the batch job is not found
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
		// Mock server response that should not be triggered in batch processing
		serverResponse: func(w http.ResponseWriter, r *http.Request) {
			// This should be handled by batch processing internally
			t.Error("Server should not be called for batch requests")
		},
		// Expected error message to be returned by the API
		expectedError: "batch job not found", // This is expected as we're not implementing batch processing in the test
	}

	t.Run(batchJobNotFoundTestCase.name, func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(batchJobNotFoundTestCase.serverResponse))
		defer server.Close()

		endpoint, err := NewEndpoint("test-provider", "test-region", server.URL, "test-api-key")
		assert.NoError(t, err)
		defer endpoint.Shutdown()

		// Generate a job ID for the batch job
		jobId := generateJobId(batchJobNotFoundTestCase.request)

		// Lock the batch job mutex and add the batch job to the batch jobs map
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

		// Prevent real API calls by using a mock HTTP client
		endpoint.client = &http.Client{
			Transport: &MockHTTPClient{
				DoFunc: func(req *http.Request) (*http.Response, error) {
					return nil, fmt.Errorf("batch job not found")
				},
			},
		}

		ctx := context.Background()
		_, err = endpoint.GenerateChatCompletion(ctx, batchJobNotFoundTestCase.request)

		// Assert that an error occured and the error message is as expected
		assert.Error(t, err)
		assert.Contains(t, err.Error(), batchJobNotFoundTestCase.expectedError)
	})
}

// MockHTTPClient overrides the Do function of http.Client
type MockHTTPClient struct {
	DoFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockHTTPClient) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.DoFunc(req)
}
