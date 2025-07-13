package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yanolja/ogem/openai"
)

func TestNewEndpoint(t *testing.T) {
	tests := []struct {
		name         string
		providerName string
		region       string
		baseUrl      string
		apiKey       string
		expectError  bool
	}{
		{
			name:         "valid endpoint",
			providerName: "test-provider",
			region:       "us-east-1",
			baseUrl:      "https://api.openai.com",
			apiKey:       "test-key",
			expectError:  false,
		},
		{
			name:         "invalid URL scheme",
			providerName: "test-provider",
			region:       "us-east-1",
			baseUrl:      "invalid-url",
			apiKey:       "test-key",
			expectError:  true,
		},
		{
			name:         "missing host",
			providerName: "test-provider",
			region:       "us-east-1",
			baseUrl:      "https://",
			apiKey:       "test-key",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := NewEndpoint(tt.providerName, tt.region, tt.baseUrl, tt.apiKey)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if endpoint == nil {
				t.Errorf("expected endpoint but got nil")
			}
		})
	}
}

func TestGenerateChatCompletion_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("expected /chat/completions endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.ChatCompletionRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if request.Model == "" {
			t.Errorf("model field is required")
		}
		if len(request.Messages) == 0 {
			t.Errorf("messages field is required and cannot be empty")
		}

		for i, msg := range request.Messages {
			if msg.Role == "" {
				t.Errorf("message %d: role field is required", i)
			}
			if msg.Content == nil {
				t.Errorf("message %d: content field is required", i)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: stringPtr("Hello")},
			},
		},
	}

	_, err = endpoint.GenerateChatCompletion(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateChatCompletion_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := openai.ChatCompletionResponse{
			Id:      "chatcmpl-test-123",
			Object:  "chat.completion",
			Created: 1234567890,
			Model:   "gpt-4",
			Choices: []openai.Choice{
				{
					Index: 0,
					Message: openai.Message{
						Role:    "assistant",
						Content: &openai.MessageContent{String: stringPtr("This is a test response")},
					},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{
				PromptTokens:     15,
				CompletionTokens: 8,
				TotalTokens:      23,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ChatCompletionRequest{}
	response, err := endpoint.GenerateChatCompletion(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if response.Id != "chatcmpl-test-123" {
		t.Errorf("expected ID 'chatcmpl-test-123', got %s", response.Id)
	}
	if response.Object != "chat.completion" {
		t.Errorf("expected Object 'chat.completion', got %s", response.Object)
	}
	if response.Created != 1234567890 {
		t.Errorf("expected Created 1234567890, got %d", response.Created)
	}
	if response.Model != "gpt-4" {
		t.Errorf("expected Model 'gpt-4', got %s", response.Model)
	}

	if len(response.Choices) != 1 {
		t.Errorf("expected 1 choice, got %d", len(response.Choices))
	}
	choice := response.Choices[0]
	if choice.Index != 0 {
		t.Errorf("expected choice index 0, got %d", choice.Index)
	}
	if choice.Message.Role != "assistant" {
		t.Errorf("expected assistant role, got %s", choice.Message.Role)
	}
	if choice.Message.Content == nil || choice.Message.Content.String == nil {
		t.Errorf("expected message content, got nil")
	} else if *choice.Message.Content.String != "This is a test response" {
		t.Errorf("expected content 'This is a test response', got %s", *choice.Message.Content.String)
	}
	if choice.FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got %s", choice.FinishReason)
	}

	if response.Usage.PromptTokens != 15 {
		t.Errorf("expected prompt tokens 15, got %d", response.Usage.PromptTokens)
	}
	if response.Usage.CompletionTokens != 8 {
		t.Errorf("expected completion tokens 8, got %d", response.Usage.CompletionTokens)
	}
	if response.Usage.TotalTokens != 23 {
		t.Errorf("expected total tokens 23, got %d", response.Usage.TotalTokens)
	}
}

func TestGenerateChatCompletion_AdvancedParameters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.ChatCompletionRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if request.Temperature != nil && (*request.Temperature < 0 || *request.Temperature > 2) {
			t.Errorf("temperature must be between 0 and 2, got %f", *request.Temperature)
		}

		if request.MaxTokens != nil && *request.MaxTokens < 1 {
			t.Errorf("max_tokens must be at least 1, got %d", *request.MaxTokens)
		}

		if request.TopP != nil && (*request.TopP < 0 || *request.TopP > 1) {
			t.Errorf("top_p must be between 0 and 1, got %f", *request.TopP)
		}

		if request.FrequencyPenalty != nil && (*request.FrequencyPenalty < -2 || *request.FrequencyPenalty > 2) {
			t.Errorf("frequency_penalty must be between -2 and 2, got %f", *request.FrequencyPenalty)
		}

		if request.PresencePenalty != nil && (*request.PresencePenalty < -2 || *request.PresencePenalty > 2) {
			t.Errorf("presence_penalty must be between -2 and 2, got %f", *request.PresencePenalty)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	temp := float32(0.7)
	maxTokens := int32(100)
	topP := float32(0.9)
	freqPenalty := float32(0.5)
	presencePenalty := float32(0.5)

	request := &openai.ChatCompletionRequest{
		Model:            "gpt-4",
		Temperature:      &temp,
		MaxTokens:        &maxTokens,
		TopP:             &topP,
		FrequencyPenalty: &freqPenalty,
		PresencePenalty:  &presencePenalty,
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: stringPtr("Hello")},
			},
		},
	}

	_, err = endpoint.GenerateChatCompletion(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateChatCompletionStream_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("expected /chat/completions endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}

		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("expected Accept: text/event-stream, got %s", r.Header.Get("Accept"))
		}

		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.ChatCompletionRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if request.Stream == nil || !*request.Stream {
			t.Errorf("stream field must be true for streaming requests")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		responses := []string{
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}

		for _, response := range responses {
			fmt.Fprintf(w, "data: %s\n\n", response)
			w.(http.Flusher).Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: stringPtr("Hello")},
			},
		},
	}

	responseCh, errorCh := endpoint.GenerateChatCompletionStream(context.Background(), request)

	count := 0
	for response := range responseCh {
		if response == nil {
			t.Errorf("received nil response")
		}
		count++
	}

	if err := <-errorCh; err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if count == 0 {
		t.Errorf("expected at least one response")
	}
}

func TestGenerateChatCompletionStream_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		responses := []string{
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{"content":"Hello"},"finish_reason":null}]}`,
			`{"id":"chatcmpl-test","object":"chat.completion.chunk","created":1234567890,"model":"gpt-4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}

		for _, response := range responses {
			fmt.Fprintf(w, "data: %s\n\n", response)
			w.(http.Flusher).Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ChatCompletionRequest{}
	responseCh, errorCh := endpoint.GenerateChatCompletionStream(context.Background(), request)

	var responses []*openai.ChatCompletionStreamResponse
	for resp := range responseCh {
		if resp == nil {
			t.Errorf("received nil response")
			continue
		}
		responses = append(responses, resp)
	}

	if err := <-errorCh; err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(responses) != 3 {
		t.Errorf("expected 3 streamed responses, got %d", len(responses))
	}
	if len(responses) > 0 && (responses[0].Choices[0].Delta.Role == nil || *responses[0].Choices[0].Delta.Role != "assistant") {
		t.Errorf("expected role 'assistant' in first chunk")
	}
	if len(responses) > 1 && (responses[1].Choices[0].Delta.Content == nil || *responses[1].Choices[0].Delta.Content != "Hello") {
		t.Errorf("expected content 'Hello' in second chunk")
	}
	if len(responses) > 2 && (responses[2].Choices[0].FinishReason == nil || *responses[2].Choices[0].FinishReason != "stop") {
		t.Errorf("expected finish_reason 'stop' in last chunk")
	}
}

func TestGenerateEmbedding_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/embeddings") {
			t.Errorf("expected /embeddings endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.EmbeddingRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if request.Model == "" {
			t.Errorf("model field is required")
		}
		if len(request.Input) == 0 {
			t.Errorf("input field is required and cannot be empty")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.EmbeddingRequest{
		Model: "text-embedding-ada-002",
		Input: []string{"Hello world"},
	}

	_, err = endpoint.GenerateEmbedding(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateEmbedding_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := openai.EmbeddingResponse{
			Object: "list",
			Data: []openai.EmbeddingObject{
				{
					Object:    "embedding",
					Embedding: []float32{0.1, 0.2, 0.3},
					Index:     0,
				},
			},
			Model: "text-embedding-ada-002",
			Usage: openai.EmbeddingUsage{
				PromptTokens: 5,
				TotalTokens:  5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.EmbeddingRequest{}
	resp, err := endpoint.GenerateEmbedding(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %s", resp.Object)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 embedding, got %d", len(resp.Data))
	}
	if resp.Data[0].Index != 0 {
		t.Errorf("expected index 0, got %d", resp.Data[0].Index)
	}
	if resp.Model != "text-embedding-ada-002" {
		t.Errorf("expected model 'text-embedding-ada-002', got %s", resp.Model)
	}
	if resp.Usage.PromptTokens != 5 || resp.Usage.TotalTokens != 5 {
		t.Errorf("unexpected usage: %+v", resp.Usage)
	}
}

func TestGenerateImage_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/images/generations") {
			t.Errorf("expected /images/generations endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.ImageGenerationRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if request.Prompt == "" {
			t.Errorf("prompt field is required")
		}

		if request.N != nil && (*request.N < 1 || *request.N > 10) {
			t.Errorf("n must be between 1 and 10, got %d", *request.N)
		}

		if request.Size != nil {
			validSizes := map[string]bool{"256x256": true, "512x512": true, "1024x1024": true, "1792x1024": true, "1024x1792": true}
			if !validSizes[*request.Size] {
				t.Errorf("invalid size: %s", *request.Size)
			}
		}

		if request.Quality != nil {
			validQualities := map[string]bool{"standard": true, "hd": true}
			if !validQualities[*request.Quality] {
				t.Errorf("invalid quality: %s", *request.Quality)
			}
		}

		if request.ResponseFormat != nil {
			validFormats := map[string]bool{"url": true, "b64_json": true}
			if !validFormats[*request.ResponseFormat] {
				t.Errorf("invalid response_format: %s", *request.ResponseFormat)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	n := int32(1)
	size := "1024x1024"
	quality := "standard"
	responseFormat := "url"

	request := &openai.ImageGenerationRequest{
		Prompt:         "A beautiful sunset",
		Model:          stringPtr("dall-e-3"),
		N:              &n,
		Size:           &size,
		Quality:        &quality,
		ResponseFormat: &responseFormat,
	}

	_, err = endpoint.GenerateImage(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateImage_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := openai.ImageGenerationResponse{
			Created: 1234567890,
			Data: []openai.ImageData{
				{
					URL:     stringPtr("https://example.com/image.png"),
					B64JSON: nil,
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ImageGenerationRequest{}
	resp, err := endpoint.GenerateImage(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Created != 1234567890 {
		t.Errorf("expected created 1234567890, got %d", resp.Created)
	}
	if len(resp.Data) != 1 {
		t.Errorf("expected 1 image, got %d", len(resp.Data))
	}
	if resp.Data[0].URL == nil || *resp.Data[0].URL != "https://example.com/image.png" {
		t.Errorf("unexpected image URL: %v", resp.Data[0].URL)
	}
}

func TestTranscribeAudio_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/audio/transcriptions") {
			t.Errorf("expected /audio/transcriptions endpoint, got %s", r.URL.Path)
		}

		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected Content-Type: multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			return
		}

		if r.FormValue("model") == "" {
			t.Errorf("model field is required")
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("file field is required: %v", err)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.AudioTranscriptionRequest{
		Model:       "whisper-1",
		File:        "audio.mp3",
		FileContent: []byte("fake audio content"),
	}

	_, err = endpoint.TranscribeAudio(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTranscribeAudio_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := openai.AudioTranscriptionResponse{
			Text: "Hello world",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.AudioTranscriptionRequest{}
	resp, err := endpoint.TranscribeAudio(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %s", resp.Text)
	}
}

func TestTranslateAudio_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/audio/translations") {
			t.Errorf("expected /audio/translations endpoint, got %s", r.URL.Path)
		}

		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			t.Errorf("expected Content-Type: multipart/form-data, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
			return
		}

		if r.FormValue("model") == "" {
			t.Errorf("model field is required")
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("file field is required: %v", err)
			return
		}
		defer file.Close()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.AudioTranslationRequest{
		Model:       "whisper-1",
		File:        "audio.mp3",
		FileContent: []byte("fake audio content"),
	}

	_, err = endpoint.TranslateAudio(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestTranslateAudio_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := openai.AudioTranslationResponse{
			Text: "Hello world",
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.AudioTranslationRequest{}
	resp, err := endpoint.TranslateAudio(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %s", resp.Text)
	}
}

func TestGenerateSpeech_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/audio/speech") {
			t.Errorf("expected /audio/speech endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.TextToSpeechRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if request.Model == "" {
			t.Errorf("model field is required")
		}
		if request.Input == "" {
			t.Errorf("input field is required")
		}

		if request.Voice != "" {
			validVoices := map[string]bool{"alloy": true, "echo": true, "fable": true, "onyx": true, "nova": true, "shimmer": true}
			if !validVoices[request.Voice] {
				t.Errorf("invalid voice: %s", request.Voice)
			}
		}

		if request.ResponseFormat != nil {
			validFormats := map[string]bool{"mp3": true, "opus": true, "aac": true, "flac": true}
			if !validFormats[*request.ResponseFormat] {
				t.Errorf("invalid response_format: %s", *request.ResponseFormat)
			}
		}

		if request.Speed != nil && (*request.Speed < 0.25 || *request.Speed > 4.0) {
			t.Errorf("speed must be between 0.25 and 4.0, got %f", *request.Speed)
		}

		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	voice := "alloy"
	responseFormat := "mp3"
	speed := float32(1.0)

	request := &openai.TextToSpeechRequest{
		Model:          "tts-1",
		Input:          "Hello world",
		Voice:          voice,
		ResponseFormat: &responseFormat,
		Speed:          &speed,
	}

	_, err = endpoint.GenerateSpeech(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGenerateSpeech_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		audioData := []byte("fake audio data")
		w.Header().Set("Content-Type", "audio/mpeg")
		w.WriteHeader(http.StatusOK)
		w.Write(audioData)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.TextToSpeechRequest{}

	resp, err := endpoint.GenerateSpeech(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !bytes.Equal(resp.Data, []byte("fake audio data")) {
		t.Errorf("expected audio data 'fake audio data', got %v", resp.Data)
	}
}

func TestModerateContent_RequestValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		if !strings.HasSuffix(r.URL.Path, "/moderations") {
			t.Errorf("expected /moderations endpoint, got %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type: application/json, got %s", r.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("expected Authorization header with Bearer token, got %s", r.Header.Get("Authorization"))
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
			return
		}

		var request openai.ModerationRequest
		if err := json.Unmarshal(body, &request); err != nil {
			t.Errorf("failed to unmarshal request body: %v", err)
			return
		}

		if len(request.Input) == 0 {
			t.Errorf("input field is required and cannot be empty")
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{}"))
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ModerationRequest{
		Input: []string{"Hello world"},
	}

	_, err = endpoint.ModerateContent(context.Background(), request)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestModerateContent_ResponseValidation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := openai.ModerationResponse{
			ID:    "modr-test",
			Model: "text-moderation-007",
			Results: []openai.ModerationResult{
				{
					Flagged: false,
					Categories: openai.ModerationCategories{
						Sexual:                false,
						Hate:                  false,
						Harassment:            false,
						SelfHarm:              false,
						SexualMinors:          false,
						HateThreatening:       false,
						ViolenceGraphic:       false,
						SelfHarmIntent:        false,
						SelfHarmInstructions:  false,
						HarassmentThreatening: false,
						Violence:              false,
					},
					CategoryScores: openai.ModerationCategoryScores{
						Sexual:                0.0,
						Hate:                  0.0,
						Harassment:            0.0,
						SelfHarm:              0.0,
						SexualMinors:          0.0,
						HateThreatening:       0.0,
						ViolenceGraphic:       0.0,
						SelfHarmIntent:        0.0,
						SelfHarmInstructions:  0.0,
						HarassmentThreatening: 0.0,
						Violence:              0.0,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	endpoint, err := NewEndpoint("test", "us-east-1", server.URL, "test-key")
	if err != nil {
		t.Fatalf("failed to create endpoint: %v", err)
	}

	request := &openai.ModerationRequest{}
	resp, err := endpoint.ModerateContent(context.Background(), request)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "modr-test" {
		t.Errorf("expected ID 'modr-test', got %s", resp.ID)
	}
	if resp.Model != "text-moderation-007" {
		t.Errorf("expected model 'text-moderation-007', got %s", resp.Model)
	}
	if len(resp.Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Flagged {
		t.Errorf("expected flagged false, got true")
	}
}

func stringPtr(s string) *string {
	return &s
}
