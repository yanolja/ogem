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

// LMStudioProvider implements the Provider interface for LM Studio
type LMStudioProvider struct {
	baseURL    string
	httpClient *http.Client
	config     *LMStudioConfig
}

// LMStudioConfig holds configuration for LM Studio provider
type LMStudioConfig struct {
	BaseURL    string        `json:"base_url" yaml:"base_url"`
	Timeout    time.Duration `json:"timeout" yaml:"timeout"`
	MaxRetries int           `json:"max_retries" yaml:"max_retries"`
	Models     []string      `json:"models" yaml:"models"`
}

// NewLMStudioProvider creates a new LM Studio provider
func NewLMStudioProvider(config *LMStudioConfig) *LMStudioProvider {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:1234"
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	return &LMStudioProvider{
		baseURL: strings.TrimSuffix(config.BaseURL, "/"),
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		config: config,
	}
}

// GetName returns the provider name
func (p *LMStudioProvider) GetName() string {
	return "lmstudio"
}

// GetModels returns available models
func (p *LMStudioProvider) GetModels(ctx context.Context) ([]types.Model, error) {
	url := fmt.Sprintf("%s/v1/models", p.baseURL)
	
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
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio API error %d: %s", resp.StatusCode, string(body))
	}

	var modelResp types.ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Add provider info to models
	for i := range modelResp.Data {
		modelResp.Data[i].Provider = "lmstudio"
		if modelResp.Data[i].OwnedBy == "" {
			modelResp.Data[i].OwnedBy = "lmstudio"
		}
	}

	return modelResp.Data, nil
}

// ChatCompletion performs chat completion
func (p *LMStudioProvider) ChatCompletion(ctx context.Context, req *types.ChatCompletionRequest) (*types.ChatCompletionResponse, error) {
	if req.Stream {
		return nil, fmt.Errorf("streaming not supported in non-streaming method")
	}

	lmReq := p.convertChatRequest(req)
	
	reqBody, err := json.Marshal(lmReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/chat/completions", p.baseURL)
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
		return nil, fmt.Errorf("LM Studio API error %d: %s", resp.StatusCode, string(body))
	}

	var lmResp types.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&lmResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &lmResp, nil
}

// ChatCompletionStream performs streaming chat completion
func (p *LMStudioProvider) ChatCompletionStream(ctx context.Context, req *types.ChatCompletionRequest) (<-chan types.ChatCompletionStreamResponse, error) {
	lmReq := p.convertChatRequest(req)
	lmReq.Stream = true

	reqBody, err := json.Marshal(lmReq)
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("LM Studio API error %d: %s", resp.StatusCode, string(body))
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

// CreateEmbedding creates embeddings (if supported by LM Studio deployment)
func (p *LMStudioProvider) CreateEmbedding(ctx context.Context, req *types.EmbeddingRequest) (*types.EmbeddingResponse, error) {
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

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("embeddings not supported by this LM Studio deployment")
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio API error %d: %s", resp.StatusCode, string(body))
	}

	var embeddingResp types.EmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&embeddingResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &embeddingResp, nil
}

// Health checks provider health
func (p *LMStudioProvider) Health(ctx context.Context) error {
	// LM Studio doesn't have a dedicated health endpoint, so we check models
	url := fmt.Sprintf("%s/v1/models", p.baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("LM Studio health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("LM Studio health check failed with status: %d", resp.StatusCode)
	}

	// Check if any models are loaded
	var modelResp types.ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelResp); err != nil {
		return fmt.Errorf("failed to decode models response: %w", err)
	}

	if len(modelResp.Data) == 0 {
		return fmt.Errorf("no models loaded in LM Studio")
	}

	return nil
}

// Helper methods
func (p *LMStudioProvider) convertChatRequest(req *types.ChatCompletionRequest) *types.ChatCompletionRequest {
	// LM Studio uses the same format as OpenAI, so we can use the request as-is
	// Just ensure stream is set correctly and handle any LM Studio specific quirks
	lmReq := *req
	
	// LM Studio sometimes has issues with certain parameters, so we filter them
	// Remove unsupported parameters that might cause issues
	lmReq.LogitBias = nil
	lmReq.Functions = nil
	lmReq.FunctionCall = nil
	lmReq.Tools = nil
	lmReq.ToolChoice = nil
	lmReq.ResponseFormat = nil
	lmReq.Seed = nil
	lmReq.LogProbs = nil
	lmReq.TopLogProbs = nil

	return &lmReq
}