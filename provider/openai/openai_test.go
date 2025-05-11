package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

func TestNewEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		region       string
		baseUrl      string
		apiKey       string
		wantErr      bool
	}{
		{
			name:         "valid endpoint",
			providerName: "openai",
			region:       "us",
			baseUrl:      "https://api.openai.com/v1",
			apiKey:       "test-key",
			wantErr:      false,
		},
		{
			name:         "invalid url",
			providerName: "openai",
			region:       "us",
			baseUrl:      "://invalid-url",
			apiKey:       "test-key",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := NewEndpoint(tt.providerName, tt.region, tt.baseUrl, tt.apiKey)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, endpoint)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, endpoint)
				assert.Equal(t, tt.providerName, endpoint.Provider())
				assert.Equal(t, tt.region, endpoint.Region())
				endpoint.Shutdown()
			}
		})
	}
}

func TestGenerateChatCompletion(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var req openai.ChatCompletionRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		assert.NoError(t, err)

		resp := openai.ChatCompletionResponse{
			Id:      "test-id",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Choices: []openai.Choice{
				{
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: stringPtr("Test response"),
						},
					},
					FinishReason: "stop",
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("openai", "us", server.URL, "test-key")
	assert.NoError(t, err)
	defer endpoint.Shutdown()

	req := &openai.ChatCompletionRequest{
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

	resp, err := endpoint.GenerateChatCompletion(context.Background(), req)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "Test response", *resp.Choices[0].Message.Content.String)
}

func TestGenerateBatchChatCompletion(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header for all requests
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"message": "Incorrect API key provided",
					"type":    "invalid_request_error",
					"code":    "invalid_api_key",
				},
			})
			return
		}

		// For file upload
		if strings.HasPrefix(r.URL.Path, "/v1/files") && r.Method == "POST" {
			// Check Content-Type
			contentType := r.Header.Get("Content-Type")
			if !strings.HasPrefix(contentType, "multipart/form-data") {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "request Content-Type isn't multipart/form-data",
				})
				return
			}

			// Parse the multipart form
			err := r.ParseMultipartForm(10 << 20) // 10 MB
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": err.Error(),
				})
				return
			}

			// Verify the file purpose
			purpose := r.FormValue("purpose")
			if purpose != "batch" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": "invalid purpose",
				})
				return
			}

			// Get the file from the form
			file, _, err := r.FormFile("file")
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": err.Error(),
				})
				return
			}
			defer file.Close()

			// Read the file content
			content, err := io.ReadAll(file)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error": err.Error(),
				})
				return
			}

			// Split content into lines and decode each line as JSON
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				if line == "" {
					continue
				}
				var job BatchJob
				err = json.NewDecoder(strings.NewReader(line)).Decode(&job)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error": err.Error(),
					})
					return
				}
				
				// Verify job fields
				if job.Id == "" || job.Method != "POST" || job.Url != "/v1/chat/completions" || job.Body == nil {
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]interface{}{
						"error": "invalid job fields",
					})
					return
				}
			}

			// Mock file upload response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"id": "test-file-id"})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/v1/batches") && r.Method == "POST" {
			// Verify batch creation request
			var requestBody map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&requestBody)
			assert.NoError(t, err)
			assert.Equal(t, "test-file-id", requestBody["input_file_id"])
			assert.Equal(t, "/v1/chat/completions", requestBody["endpoint"])
			assert.Equal(t, "24h", requestBody["completion_window"])

			// Mock batch job creation response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"id": "test-batch-id"})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/v1/batches/test-batch-id") && r.Method == "GET" {
			// Mock batch status check response
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":        "completed",
				"output_file_id": "test-output-file-id",
			})
			return
		}

		if strings.HasPrefix(r.URL.Path, "/v1/files/test-output-file-id/content") && r.Method == "GET" {
			// Mock file download response
			result := openai.ChatCompletionResponse{
				Id:      "test-batch-response-id",
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   "gpt-3.5-turbo",
				Choices: []openai.Choice{
					{
						Message: openai.Message{
							Role: "assistant",
							Content: &openai.MessageContent{
								String: stringPtr("Batch response"),
							},
						},
						FinishReason: "stop",
					},
				},
			}

			responseBytes, err := json.Marshal(result)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusOK)
			w.Write(responseBytes)
			return
		}

		// Default case
		t.Errorf("Unexpected path: %s", r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("openai", "us", server.URL, "test-key")
	assert.NoError(t, err)

	// Create a context that will be cancelled after the test
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := &openai.ChatCompletionRequest{
		Model: "gpt-3.5-turbo@batch",
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello"),
				},
			},
		},
	}

	resp, err := endpoint.GenerateBatchChatCompletion(ctx, req)
	if !assert.NoError(t, err) {
		return
	}
	if !assert.NotNil(t, resp) {
		return
	}
	if !assert.NotNil(t, resp.Choices) {
		return
	}
	if !assert.Greater(t, len(resp.Choices), 0) {
		return
	}
	if !assert.NotNil(t, resp.Choices[0].Message.Content) {
		return
	}
	if !assert.NotNil(t, resp.Choices[0].Message.Content.String) {
		return
	}
	assert.Equal(t, "Batch response", *resp.Choices[0].Message.Content.String)

	// Clean up
	endpoint.Shutdown()
}

func TestPing(t *testing.T) {
	// Create a test server that responds after a short delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond) // Add a small delay to ensure measurable duration
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("openai", "us", server.URL, "test-key")
	assert.NoError(t, err)
	defer endpoint.Shutdown()

	duration, err := endpoint.Ping(context.Background())
	assert.NoError(t, err)
	assert.True(t, duration > 0)
}

func TestGenerateJobId(t *testing.T) {
	req1 := &openai.ChatCompletionRequest{
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

	req2 := &openai.ChatCompletionRequest{
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

	// Same content should generate same ID
	id1 := generateJobId(req1)
	id2 := generateJobId(req2)
	assert.Equal(t, id1, id2)

	// Different content should generate different ID
	req2.Messages[0].Content.String = stringPtr("Different")
	id3 := generateJobId(req2)
	assert.NotEqual(t, id1, id3)
}

func stringPtr(s string) *string {
	return &s
}
