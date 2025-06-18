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

// VLLMProvider implements the Provider interface for vLLM
type VLLMProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	config     *VLLMConfig
}

// VLLMConfig holds configuration for vLLM provider
type VLLMConfig struct {
	BaseURL    string        `json:"base_url" yaml:"base_url"`
	APIKey     string        `json:"api_key" yaml:"api_key"`
	Timeout    time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries int           `json:"max_retries" yaml:"max_retries"`
	Models     []string      `json:"models" yaml:"models"`
}

// NewVLLMProvider creates a new vLLM provider
func NewVLLMProvider(config *VLLMConfig) *VLLMProvider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:8000"
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	return &VLLMProvider{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		apiKey:  config.APIKey,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		config: config,
	}
}

// GetName returns the provider name
func (p *VLLMProvider) GetName() string {
	return "vllm"
}

// GetModels returns available models
func (p *VLLMProvider) GetModels(ctx context.Context) ([]types.Model, error) {
	url := fmt.Sprintf("%s/v1/models", p.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if p.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vLLM API error %d: %s", resp.StatusCode, string(body))
	}

	var modelResp types.ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Add provider info to models
	for i := range modelResp.Data {
		modelResp.Data[i].Provider = "vllm"
		if modelResp.Data[i].OwnedBy == "" {
			modelResp.Data[i].OwnedBy = "vllm"
		}
	}

	return modelResp.Data, nil
}

// ChatCompletion performs chat completion
func (p *VLLMProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	if req.Stream {
		return nil, fmt.Errorf("streaming not supported in non-streaming method")
	}

	vllmReq := p.convertChatRequest(req)
	
	reqBody, err := json.Marshal(vllmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vLLM API error %d: %s", resp.StatusCode, string(body))
	}

	var vllmResp types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&vllmResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &vllmResp, nil
}

// ChatCompletionStream performs streaming chat completion
func (p *VLLMProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (<-chan types.ChatCompletionStreamResponse, error) {
	vllmReq := p.convertChatRequest(req)
	vllmReq.Stream = true

	reqBody, err := json.Marshal(vllmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("vLLM API error %d: %s", resp.StatusCode, string(body))
	}

	responseChan := make(chan types.ChatCompletionStreamResponse, 10)

	go func() {
		defer close(responseChan)
		defer resp.Body.Close()

		decoder := providers.NewSSEDecoder(resp.Body)
		for {
			event, err := decoder.Decode()
			if err != nil {
				if err == io.EOF {
					break
				}
				responseChan <- types.ChatCompletionStreamResponse{
					Error: fmt.Errorf("failed to decode SSE: %w", err),
				}
				return
			}

			if event.Data == "[DONE]" {
				break
			}

			var chunk types.ChatCompletionResponse
			if err := json.Unmarshal([]byte(event.Data), &chunk); err != nil {
				responseChan <- types.ChatCompletionStreamResponse{
					Error: fmt.Errorf("failed to decode chunk: %w", err),
				}
				continue
			}

			responseChan <- types.ChatCompletionStreamResponse{
				Response: &chunk,
			}
		}
	}()

	return responseChan, nil
}

// CreateEmbedding creates embeddings (if supported by vLLM deployment)
func (p *VLLMProvider) CreateEmbedding(ctx context.Context, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
	url := fmt.Sprintf("%s/v1/embeddings", p.baseURL)
	
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("embeddings not supported by this vLLM deployment")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vLLM API error %d: %s", resp.StatusCode, string(body))
	}

	var embeddingResp types.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &embeddingResp, nil
}

// Health checks provider health
func (p *VLLMProvider) Health(ctx context.Context) error {
	url := fmt.Sprintf("%s/health", p.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	if p.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("vLLM health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Try alternative health endpoint
		url = fmt.Sprintf("%s/v1/models", p.baseURL)
		req, err = http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return fmt.Errorf("failed to create backup health check request: %w", err)
		}

		if p.apiKey != "" {
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))
		}

		resp, err = p.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("vLLM backup health check failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("vLLM health check failed with status: %d", resp.StatusCode)
		}
	}

	return nil
}

// Helper methods
func (p *VLLMProvider) convertChatRequest(req *types.ChatCompletionRequest) *types.ChatCompletionRequest {
	// vLLM uses the same format as OpenAI, so we can use the request as-is
	// Just ensure stream is set correctly
	vllmReq := *req
	return &vllmReq
}