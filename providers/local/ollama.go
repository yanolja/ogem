package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yanolja/ogem/providers"
	"github.com/yanolja/ogem/types"
)

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct {
	baseURL    string
	httpClient *http.Client
	config     *OllamaConfig
}

// OllamaConfig holds configuration for Ollama provider
type OllamaConfig struct {
	BaseURL    string        `json:"base_url" yaml:"base_url"`
	Timeout    time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries int           `json:"max_retries" yaml:"max_retries"`
	Models     []string      `json:"models" yaml:"models"`
}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider(config *OllamaConfig) *OllamaProvider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	return &OllamaProvider{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		config: config,
	}
}

// GetName returns the provider name
func (p *OllamaProvider) GetName() string {
	return "ollama"
}

// GetModels returns available models
func (p *OllamaProvider) GetModels(ctx context.Context) ([]types.Model, error) {
	url := fmt.Sprintf("%s/api/tags", p.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API error: %d", resp.StatusCode)
	}

	var ollamaResp struct {
		Models []struct {
			Name       string    `json:"name"`
			ModifiedAt time.Time `json:"modified_at"`
			Size       int64     `json:"size"`
			Digest     string    `json:"digest"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	models := make([]types.Model, len(ollamaResp.Models))
	for i, model := range ollamaResp.Models {
		models[i] = types.Model{
			ID:       model.Name,
			Object:   "model",
			Created:  model.ModifiedAt.Unix(),
			OwnedBy:  "ollama",
			Provider: "ollama",
		}
	}

	return models, nil
}

// ChatCompletion performs chat completion
func (p *OllamaProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	if req.Stream {
		return nil, fmt.Errorf("streaming not supported in non-streaming method")
	}

	ollamaReq := p.convertChatRequest(req)
	
	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(body))
	}

	var ollamaResp OllamaChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return p.convertChatResponse(&ollamaResp, req.Model), nil
}

// ChatCompletionStream performs streaming chat completion
func (p *OllamaProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (<-chan types.ChatCompletionStreamResponse, error) {
	ollamaReq := p.convertChatRequest(req)
	ollamaReq.Stream = true

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/api/chat", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(body))
	}

	responseChan := make(chan types.ChatCompletionStreamResponse, 10)

	go func() {
		defer close(responseChan)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var chunk OllamaChatResponse
			if err := decoder.Decode(&chunk); err != nil {
				if err == io.EOF {
					break
				}
				responseChan <- types.ChatCompletionStreamResponse{
					Error: fmt.Errorf("failed to decode chunk: %w", err),
				}
				return
			}

			streamResp := p.convertChatStreamResponse(&chunk, req.Model)
			responseChan <- types.ChatCompletionStreamResponse{
				Response: streamResp,
			}

			if chunk.Done {
				break
			}
		}
	}()

	return responseChan, nil
}

// CreateEmbedding creates embeddings
func (p *OllamaProvider) CreateEmbedding(ctx context.Context, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
	if len(req.Input) == 0 {
		return nil, fmt.Errorf("input is required")
	}

	var embeddings []types.Embedding
	totalTokens := 0

	for i, input := range req.Input {
		ollamaReq := OllamaEmbeddingRequest{
			Model:  req.Model,
			Prompt: input,
		}

		reqBody, err := json.Marshal(ollamaReq)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request: %w", err)
		}

		url := fmt.Sprintf("%s/api/embeddings", p.baseURL)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := p.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("failed to send request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("ollama API error %d: %s", resp.StatusCode, string(body))
		}

		var ollamaResp OllamaEmbeddingResponse
		if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}

		embeddings = append(embeddings, types.Embedding{
			Object:    "embedding",
			Index:     i,
			Embedding: ollamaResp.Embedding,
		})

		// Estimate tokens (Ollama doesn't provide this)
		totalTokens += len(strings.Fields(input))
	}

	return &types.EmbeddingResponse{
		Object: "list",
		Data:   embeddings,
		Model:  req.Model,
		Usage: types.Usage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}, nil
}

// Health checks provider health
func (p *OllamaProvider) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/api/tags", p.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama health check failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Ollama API types
type OllamaChatRequest struct {
	Model    string           `json:"model"`
	Messages []OllamaMessage  `json:"messages"`
	Stream   bool             `json:"stream,omitempty"`
	Options  *OllamaOptions   `json:"options,omitempty"`
}

type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type OllamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	TopP        float64 `json:"top_p,omitempty"`
	TopK        int     `json:"top_k,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type OllamaChatResponse struct {
	Model     string        `json:"model"`
	CreatedAt time.Time     `json:"created_at"`
	Message   OllamaMessage `json:"message"`
	Done      bool          `json:"done"`
	Context   []int         `json:"context,omitempty"`
}

type OllamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type OllamaEmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

// Helper methods
func (p *OllamaProvider) convertChatRequest(req *types.ChatCompletionRequest) *OllamaChatRequest {
	messages := make([]OllamaMessage, len(req.Messages))
	for i, msg := range req.Messages {
		messages[i] = OllamaMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}

	ollamaReq := &OllamaChatRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   req.Stream,
	}

	if req.Temperature != nil || req.TopP != nil || req.MaxTokens != nil {
		options := &OllamaOptions{}
		if req.Temperature != nil {
			options.Temperature = *req.Temperature
		}
		if req.TopP != nil {
			options.TopP = *req.TopP
		}
		if req.MaxTokens != nil {
			options.NumPredict = *req.MaxTokens
		}
		ollamaReq.Options = options
	}

	return ollamaReq
}

func (p *OllamaProvider) convertChatResponse(resp *OllamaChatResponse, model string) *types.ChatCompletionResponse {
	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: resp.CreatedAt.Unix(),
		Model:   model,
		Choices: []types.Choice{
			{
				Index: 0,
				Message: types.Message{
					Role:    resp.Message.Role,
					Content: resp.Message.Content,
				},
				FinishReason: func() *string {
					if resp.Done {
						reason := "stop"
						return &reason
					}
					return nil
				}(),
			},
		},
		Usage: &types.Usage{
			// Ollama doesn't provide token counts, so we estimate
			PromptTokens:     len(strings.Fields(resp.Message.Content)) / 4,
			CompletionTokens: len(strings.Fields(resp.Message.Content)),
			TotalTokens:      len(strings.Fields(resp.Message.Content)) * 5 / 4,
		},
	}
}

func (p *OllamaProvider) convertChatStreamResponse(resp *OllamaChatResponse, model string) *types.ChatCompletionResponse {
	var finishReason *string
	if resp.Done {
		reason := "stop"
		finishReason = &reason
	}

	return &types.ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion.chunk",
		Created: resp.CreatedAt.Unix(),
		Model:   model,
		Choices: []types.Choice{
			{
				Index: 0,
				Delta: &types.Delta{
					Role:    resp.Message.Role,
					Content: resp.Message.Content,
				},
				FinishReason: finishReason,
			},
		},
	}
}