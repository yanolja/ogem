// Package ogem provides a Go SDK for the Ogem AI proxy server
package ogem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client manages authenticated requests to an Ogem AI proxy server instance
type Client struct {
	baseURL    string
	apiKey     string
	tenantID   string
	httpClient *http.Client
	userAgent  string
	debug      bool
}

// Config specifies connection parameters and behavior for Ogem client instances
type Config struct {
	BaseURL   string        // Ogem server base URL
	APIKey    string        // API key for authentication
	TenantID  string        // Tenant ID for multi-tenancy
	Timeout   time.Duration // Request timeout
	UserAgent string        // Custom user agent
	Debug     bool          // Enable debug logging
}

// NewClient creates a new Ogem client with the provided configuration
func NewClient(config Config) (*Client, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Set defaults
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.UserAgent == "" {
		config.UserAgent = "ogem-go-sdk/1.0.0"
	}

	// Normalize base URL
	baseURL := strings.TrimSuffix(config.BaseURL, "/")

	client := &Client{
		baseURL:  baseURL,
		apiKey:   config.APIKey,
		tenantID: config.TenantID,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		userAgent: config.UserAgent,
		debug:     config.Debug,
	}

	return client, nil
}

// ChatCompletion creates a chat completion request
func (c *Client) ChatCompletion(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionResponse, error) {
	endpoint := "/v1/chat/completions"

	var response ChatCompletionResponse
	if err := c.makeRequest(ctx, "POST", endpoint, req, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// ChatCompletionStream creates a streaming chat completion request
func (c *Client) ChatCompletionStream(ctx context.Context, req ChatCompletionRequest) (*ChatCompletionStream, error) {
	req.Stream = true
	endpoint := "/v1/chat/completions"

	httpReq, err := c.createRequest(ctx, "POST", endpoint, req)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Cache-Control", "no-cache")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, c.handleHTTPError(resp)
	}

	return &ChatCompletionStream{
		reader: resp.Body,
	}, nil
}

// Embeddings creates embeddings for the given input
func (c *Client) Embeddings(ctx context.Context, req EmbeddingsRequest) (*EmbeddingsResponse, error) {
	endpoint := "/v1/embeddings"

	var response EmbeddingsResponse
	if err := c.makeRequest(ctx, "POST", endpoint, req, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Models lists available models
func (c *Client) Models(ctx context.Context) (*ModelsResponse, error) {
	endpoint := "/v1/models"

	var response ModelsResponse
	if err := c.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// GetModel retrieves information about a specific model
func (c *Client) GetModel(ctx context.Context, modelID string) (*Model, error) {
	endpoint := fmt.Sprintf("/v1/models/%s", modelID)

	var response Model
	if err := c.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Health checks the health status of the Ogem server
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	endpoint := "/health"

	var response HealthResponse
	if err := c.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Stats retrieves server statistics (requires appropriate permissions)
func (c *Client) Stats(ctx context.Context) (*StatsResponse, error) {
	endpoint := "/stats"

	var response StatsResponse
	if err := c.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// CacheStats retrieves cache statistics (requires appropriate permissions)
func (c *Client) CacheStats(ctx context.Context) (*CacheStatsResponse, error) {
	endpoint := "/cache/stats"

	var response CacheStatsResponse
	if err := c.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// TenantUsage retrieves tenant usage metrics (requires appropriate permissions)
func (c *Client) TenantUsage(ctx context.Context, tenantID string) (*TenantUsageResponse, error) {
	if tenantID == "" {
		tenantID = c.tenantID
	}
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	endpoint := fmt.Sprintf("/tenants/%s/usage", tenantID)

	var response TenantUsageResponse
	if err := c.makeRequest(ctx, "GET", endpoint, nil, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// SetTenantID updates the tenant ID for subsequent requests
func (c *Client) SetTenantID(tenantID string) {
	c.tenantID = tenantID
}

// SetDebug enables or disables debug logging
func (c *Client) SetDebug(debug bool) {
	c.debug = debug
}

// makeRequest is a helper method for making HTTP requests
func (c *Client) makeRequest(ctx context.Context, method, endpoint string, body interface{}, response interface{}) error {
	req, err := c.createRequest(ctx, method, endpoint, body)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if c.debug {
		fmt.Printf("DEBUG: %s %s -> %d\n", method, endpoint, resp.StatusCode)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.handleHTTPError(resp)
	}

	if response != nil {
		return json.NewDecoder(resp.Body).Decode(response)
	}

	return nil
}

// createRequest creates an HTTP request with proper headers
func (c *Client) createRequest(ctx context.Context, method, endpoint string, body interface{}) (*http.Request, error) {
	url := c.baseURL + endpoint

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, err
	}

	// Set headers
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("User-Agent", c.userAgent)

	// Set tenant ID header if provided
	if c.tenantID != "" {
		req.Header.Set("X-Tenant-ID", c.tenantID)
	}

	return req, nil
}

// handleHTTPError handles HTTP error responses
func (c *Client) handleHTTPError(resp *http.Response) error {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("HTTP %d: failed to read error response", resp.StatusCode)
	}

	var errorResp ErrorResponse
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return &APIError{
		StatusCode: resp.StatusCode,
		Type:       errorResp.Error.Type,
		Message:    errorResp.Error.Message,
		Code:       errorResp.Error.Code,
	}
}

// APIError represents an API error
type APIError struct {
	StatusCode int    `json:"status_code"`
	Type       string `json:"type"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("ogem API error (%d): %s [%s] - %s", e.StatusCode, e.Type, e.Code, e.Message)
	}
	return fmt.Sprintf("ogem API error (%d): %s - %s", e.StatusCode, e.Type, e.Message)
}

// ErrorResponse represents an error response from the API
type ErrorResponse struct {
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Code    string `json:"code,omitempty"`
	} `json:"error"`
}

// ChatCompletionRequest represents a chat completion request
type ChatCompletionRequest struct {
	Model            string          `json:"model"`
	Messages         []Message       `json:"messages"`
	MaxTokens        *int            `json:"max_tokens,omitempty"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	N                *int            `json:"n,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Stop             []string        `json:"stop,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	LogitBias        map[string]int  `json:"logit_bias,omitempty"`
	User             string          `json:"user,omitempty"`
	Functions        []Function      `json:"functions,omitempty"`
	FunctionCall     interface{}     `json:"function_call,omitempty"`
	Tools            []Tool          `json:"tools,omitempty"`
	ToolChoice       interface{}     `json:"tool_choice,omitempty"`
	ResponseFormat   *ResponseFormat `json:"response_format,omitempty"`
	Seed             *int            `json:"seed,omitempty"`
	LogProbs         *bool           `json:"logprobs,omitempty"`
	TopLogProbs      *int            `json:"top_logprobs,omitempty"`
}

// Message represents a chat message
type Message struct {
	Role         string        `json:"role"`
	Content      interface{}   `json:"content"` // Can be string or []MessagePart
	Name         string        `json:"name,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
}

// MessagePart represents a part of a multimodal message
type MessagePart struct {
	Type     string           `json:"type"`
	Text     string           `json:"text,omitempty"`
	ImageURL *MessageImageURL `json:"image_url,omitempty"`
}

// MessageImageURL represents an image URL in a message
type MessageImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// Function represents a function definition
type Function struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// FunctionCall represents a function call
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool represents a tool definition
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}

// ToolCall represents a tool call
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// ResponseFormat specifies the format of the response
type ResponseFormat struct {
	Type string `json:"type"`
}

// ChatCompletionResponse represents a chat completion response
type ChatCompletionResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
	Usage             Usage    `json:"usage"`
}

// Choice represents a completion choice
type Choice struct {
	Index        int       `json:"index"`
	Message      Message   `json:"message"`
	Delta        *Message  `json:"delta,omitempty"`
	LogProbs     *LogProbs `json:"logprobs,omitempty"`
	FinishReason *string   `json:"finish_reason"`
}

// LogProbs represents log probabilities
type LogProbs struct {
	Tokens        []string             `json:"tokens"`
	TokenLogProbs []float64            `json:"token_logprobs"`
	TopLogProbs   []map[string]float64 `json:"top_logprobs"`
}

// Usage represents token usage information
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatCompletionStream represents a streaming chat completion response
type ChatCompletionStream struct {
	reader io.ReadCloser
}

// Recv receives the next chunk from the stream
func (s *ChatCompletionStream) Recv() (*ChatCompletionStreamResponse, error) {
	if s.reader == nil {
		return nil, fmt.Errorf("stream is closed")
	}

	// Read line by line looking for SSE data
	buffer := make([]byte, 4096)
	var line []byte

	for {
		n, err := s.reader.Read(buffer)
		if err == io.EOF {
			return nil, io.EOF
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read from stream: %w", err)
		}

		data := buffer[:n]
		for _, b := range data {
			if b == '\n' {
				lineStr := strings.TrimSpace(string(line))

				// Skip empty lines and comments
				if lineStr == "" || strings.HasPrefix(lineStr, ":") {
					line = line[:0]
					continue
				}

				// Parse SSE data line
				if strings.HasPrefix(lineStr, "data: ") {
					dataContent := strings.TrimPrefix(lineStr, "data: ")

					// Check for end of stream
					if dataContent == "[DONE]" {
						return nil, io.EOF
					}

					// Parse JSON chunk
					var chunk ChatCompletionStreamResponse
					if err := json.Unmarshal([]byte(dataContent), &chunk); err != nil {
						line = line[:0]
						continue // Skip malformed chunks
					}

					line = line[:0]
					return &chunk, nil
				}

				line = line[:0]
			} else {
				line = append(line, b)
			}
		}
	}
}

// Close closes the stream
func (s *ChatCompletionStream) Close() error {
	return s.reader.Close()
}

// ChatCompletionStreamResponse represents a streaming response chunk
type ChatCompletionStreamResponse struct {
	ID                string   `json:"id"`
	Object            string   `json:"object"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	SystemFingerprint string   `json:"system_fingerprint,omitempty"`
	Choices           []Choice `json:"choices"`
}

// EmbeddingsRequest represents an embeddings request
type EmbeddingsRequest struct {
	Model          string      `json:"model"`
	Input          interface{} `json:"input"` // Can be string, []string, []int, or [][]int
	EncodingFormat string      `json:"encoding_format,omitempty"`
	Dimensions     *int        `json:"dimensions,omitempty"`
	User           string      `json:"user,omitempty"`
}

// EmbeddingsResponse represents an embeddings response
type EmbeddingsResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  Usage       `json:"usage"`
}

// Embedding represents a single embedding
type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// ModelsResponse represents a models list response
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

// Model represents a model
type Model struct {
	ID         string `json:"id"`
	Object     string `json:"object"`
	Created    int64  `json:"created"`
	OwnedBy    string `json:"owned_by"`
	Permission []struct {
		ID                 string  `json:"id"`
		Object             string  `json:"object"`
		Created            int64   `json:"created"`
		AllowCreateEngine  bool    `json:"allow_create_engine"`
		AllowSampling      bool    `json:"allow_sampling"`
		AllowLogProbs      bool    `json:"allow_logprobs"`
		AllowSearchIndices bool    `json:"allow_search_indices"`
		AllowView          bool    `json:"allow_view"`
		AllowFineTuning    bool    `json:"allow_fine_tuning"`
		Organization       string  `json:"organization"`
		Group              *string `json:"group"`
		IsBlocking         bool    `json:"is_blocking"`
	} `json:"permission"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status    string                 `json:"status"`
	Version   string                 `json:"version"`
	Uptime    string                 `json:"uptime"`
	Timestamp string                 `json:"timestamp"`
	Services  map[string]interface{} `json:"services,omitempty"`
}

// StatsResponse represents server statistics
type StatsResponse struct {
	Requests    RequestStats           `json:"requests"`
	Performance PerformanceStats       `json:"performance"`
	Providers   map[string]interface{} `json:"providers"`
	Cache       map[string]interface{} `json:"cache,omitempty"`
	Tenants     map[string]interface{} `json:"tenants,omitempty"`
	GeneratedAt string                 `json:"generated_at"`
}

// RequestStats represents request statistics
type RequestStats struct {
	Total       int64   `json:"total"`
	Successful  int64   `json:"successful"`
	Failed      int64   `json:"failed"`
	SuccessRate float64 `json:"success_rate"`
}

// PerformanceStats represents performance statistics
type PerformanceStats struct {
	AverageLatency string  `json:"average_latency"`
	ThroughputRPM  float64 `json:"throughput_rpm"`
	ErrorRate      float64 `json:"error_rate"`
}

// CacheStatsResponse represents cache statistics
type CacheStatsResponse struct {
	Hits          int64                  `json:"hits"`
	Misses        int64                  `json:"misses"`
	HitRate       float64                `json:"hit_rate"`
	TotalEntries  int64                  `json:"total_entries"`
	MemoryUsageMB float64                `json:"memory_usage_mb"`
	ExactHits     int64                  `json:"exact_hits"`
	SemanticHits  int64                  `json:"semantic_hits"`
	TokenHits     int64                  `json:"token_hits"`
	TenantStats   map[string]interface{} `json:"tenant_stats,omitempty"`
	LastUpdated   string                 `json:"last_updated"`
}

// TenantUsageResponse represents tenant usage metrics
type TenantUsageResponse struct {
	TenantID          string  `json:"tenant_id"`
	RequestsThisHour  int64   `json:"requests_this_hour"`
	RequestsThisDay   int64   `json:"requests_this_day"`
	RequestsThisMonth int64   `json:"requests_this_month"`
	TokensThisHour    int64   `json:"tokens_this_hour"`
	TokensThisDay     int64   `json:"tokens_this_day"`
	TokensThisMonth   int64   `json:"tokens_this_month"`
	CostThisHour      float64 `json:"cost_this_hour"`
	CostThisDay       float64 `json:"cost_this_day"`
	CostThisMonth     float64 `json:"cost_this_month"`
	StorageUsedGB     int64   `json:"storage_used_gb"`
	FilesCount        int64   `json:"files_count"`
	ActiveUsers       int     `json:"active_users"`
	TeamsCount        int     `json:"teams_count"`
	ProjectsCount     int     `json:"projects_count"`
	LastUpdated       string  `json:"last_updated"`
}

// Helper functions for creating requests

// NewChatCompletionRequest creates a new chat completion request with sensible defaults
func NewChatCompletionRequest(model string, messages []Message) ChatCompletionRequest {
	return ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}
}

// NewUserMessage creates a new user message
func NewUserMessage(content string) Message {
	return Message{
		Role:    "user",
		Content: content,
	}
}

// NewSystemMessage creates a new system message
func NewSystemMessage(content string) Message {
	return Message{
		Role:    "system",
		Content: content,
	}
}

// NewAssistantMessage creates a new assistant message
func NewAssistantMessage(content string) Message {
	return Message{
		Role:    "assistant",
		Content: content,
	}
}

// NewEmbeddingsRequest creates a new embeddings request
func NewEmbeddingsRequest(model string, input interface{}) EmbeddingsRequest {
	return EmbeddingsRequest{
		Model: model,
		Input: input,
	}
}

// WithMaxTokens sets the max tokens for a chat completion request
func (r ChatCompletionRequest) WithMaxTokens(maxTokens int) ChatCompletionRequest {
	r.MaxTokens = &maxTokens
	return r
}

// WithTemperature sets the temperature for a chat completion request
func (r ChatCompletionRequest) WithTemperature(temperature float64) ChatCompletionRequest {
	r.Temperature = &temperature
	return r
}

// WithTopP sets the top_p for a chat completion request
func (r ChatCompletionRequest) WithTopP(topP float64) ChatCompletionRequest {
	r.TopP = &topP
	return r
}

// WithStream enables streaming for a chat completion request
func (r ChatCompletionRequest) WithStream(stream bool) ChatCompletionRequest {
	r.Stream = stream
	return r
}

// WithUser sets the user identifier for a chat completion request
func (r ChatCompletionRequest) WithUser(user string) ChatCompletionRequest {
	r.User = user
	return r
}

// WithTools sets the tools for a chat completion request
func (r ChatCompletionRequest) WithTools(tools []Tool) ChatCompletionRequest {
	r.Tools = tools
	return r
}

// WithResponseFormat sets the response format for a chat completion request
func (r ChatCompletionRequest) WithResponseFormat(format ResponseFormat) ChatCompletionRequest {
	r.ResponseFormat = &format
	return r
}

// Model constants (latest models)
const (
	// OpenAI Models
	ModelGPT4o     = "gpt-4o"
	ModelGPT4oMini = "gpt-4o-mini"

	// GPT-4.1 Series
	ModelGPT41     = "gpt-4.1"
	ModelGPT41Mini = "gpt-4.1-mini"
	ModelGPT41Nano = "gpt-4.1-nano"

	// o4 Reasoning Models
	ModelO4Mini = "o4-mini"

	// o3 Reasoning Models
	ModelO3     = "o3"
	ModelO3Mini = "o3-mini"

	// o1 Series
	ModelO1        = "o1"
	ModelO1Mini    = "o1-mini"
	ModelO1Preview = "o1-preview"

	// Legacy GPT Models
	ModelGPT4             = "gpt-4"
	ModelGPT35Turbo       = "gpt-3.5-turbo"
	ModelGPT4Turbo        = "gpt-4-turbo"
	ModelGPT4TurboPreview = "gpt-4-turbo-preview"

	// Claude Models
	ModelClaude35Sonnet = "claude-3-5-sonnet-20241022"
	ModelClaude35Haiku  = "claude-3.5-haiku-20241022"

	// Gemini Models
	ModelGemini25Pro       = "gemini-2.5-pro"
	ModelGemini25Flash     = "gemini-2.5-flash"
	ModelGemini25FlashLite = "gemini-2.5-flash-lite"

	// Embedding Models
	ModelEmbedding3Small = "text-embedding-3-small"
	ModelEmbedding3Large = "text-embedding-3-large"

	// Deprecated Models (use alternatives above)
	// ModelGPT4, ModelGPT35Turbo, ModelClaude3*, ModelGeminiPro
)

// Response format constants
const (
	ResponseFormatText = "text"
	ResponseFormatJSON = "json_object"
)

// Message role constants
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
	RoleFunction  = "function"
)
