package xai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yanolja/ogem/openai"
)

const REGION = "xai"

type Endpoint struct {
	apiKey  string
	baseUrl *url.URL
	client  *http.Client
	region  string
}

// xAI API structures (similar to OpenAI but with some differences)
type XAIChatRequest struct {
	Model            string                 `json:"model"`
	Messages         []XAIMessage           `json:"messages"`
	Temperature      *float32               `json:"temperature,omitempty"`
	TopP             *float32               `json:"top_p,omitempty"`
	MaxTokens        *int32                 `json:"max_tokens,omitempty"`
	Stream           *bool                  `json:"stream,omitempty"`
	FrequencyPenalty *float32               `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float32               `json:"presence_penalty,omitempty"`
	Tools            []XAITool              `json:"tools,omitempty"`
	ToolChoice       interface{}            `json:"tool_choice,omitempty"`
	ResponseFormat   *XAIResponseFormat     `json:"response_format,omitempty"`
}

type XAIMessage struct {
	Role      string           `json:"role"`
	Content   interface{}      `json:"content"`
	Name      *string          `json:"name,omitempty"`
	ToolCalls []XAIToolCall    `json:"tool_calls,omitempty"`
	ToolCallID *string         `json:"tool_call_id,omitempty"`
}

type XAITool struct {
	Type     string            `json:"type"`
	Function XAIFunctionDef    `json:"function"`
}

type XAIFunctionDef struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type XAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function XAIFunctionCall    `json:"function"`
}

type XAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type XAIResponseFormat struct {
	Type string `json:"type"`
}

type XAIChatResponse struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []XAIChoice `json:"choices"`
	Usage   XAIUsage    `json:"usage"`
}

type XAIChoice struct {
	Index        int32      `json:"index"`
	Message      XAIMessage `json:"message"`
	FinishReason string     `json:"finish_reason"`
}

type XAIUsage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

func NewEndpoint(region string, baseUrl string, apiKey string) (*Endpoint, error) {
	if baseUrl == "" {
		baseUrl = "https://api.x.ai/v1"
	}

	parsedBaseUrl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %v", err)
	}

	endpoint := &Endpoint{
		region:  region,
		apiKey:  apiKey,
		baseUrl: parsedBaseUrl,
		client:  &http.Client{Timeout: 30 * time.Minute},
	}

	return endpoint, nil
}

func (p *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	// Convert OpenAI request to xAI format
	xaiRequest := &XAIChatRequest{
		Model:            openaiRequest.Model,
		Temperature:      openaiRequest.Temperature,
		TopP:             openaiRequest.TopP,
		MaxTokens:        openaiRequest.MaxTokens,
		FrequencyPenalty: openaiRequest.FrequencyPenalty,
		PresencePenalty:  openaiRequest.PresencePenalty,
	}

	// Convert messages
	xaiRequest.Messages = make([]XAIMessage, len(openaiRequest.Messages))
	for i, msg := range openaiRequest.Messages {
		xaiMsg := XAIMessage{
			Role: msg.Role,
		}

		// Handle content
		if msg.Content != nil {
			if msg.Content.String != nil {
				xaiMsg.Content = *msg.Content.String
			} else {
				// For multipart content, combine text parts
				var textContent strings.Builder
				for _, part := range msg.Content.Parts {
					if part.Content.TextContent != nil {
						textContent.WriteString(part.Content.TextContent.Text)
					}
				}
				xaiMsg.Content = textContent.String()
			}
		}

		// Handle tool calls
		if len(msg.ToolCalls) > 0 {
			xaiMsg.ToolCalls = make([]XAIToolCall, len(msg.ToolCalls))
			for j, toolCall := range msg.ToolCalls {
				xaiMsg.ToolCalls[j] = XAIToolCall{
					ID:   toolCall.Id,
					Type: toolCall.Type,
					Function: XAIFunctionCall{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
			}
		}

		if msg.ToolCallId != nil {
			xaiMsg.ToolCallID = msg.ToolCallId
		}

		xaiRequest.Messages[i] = xaiMsg
	}

	// Convert tools
	if len(openaiRequest.Tools) > 0 {
		xaiRequest.Tools = make([]XAITool, len(openaiRequest.Tools))
		for i, tool := range openaiRequest.Tools {
			xaiRequest.Tools[i] = XAITool{
				Type: tool.Type,
				Function: XAIFunctionDef{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			}
		}
	}

	// Convert tool choice
	if openaiRequest.ToolChoice != nil {
		xaiRequest.ToolChoice = openaiRequest.ToolChoice
	}

	// Convert response format
	if openaiRequest.ResponseFormat != nil {
		xaiRequest.ResponseFormat = &XAIResponseFormat{
			Type: openaiRequest.ResponseFormat.Type,
		}
	}

	jsonData, err := json.Marshal(xaiRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "chat", "completions")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Printf("Sending %s request to %s with body: %s", httpRequest.Method, endpointPath, string(jsonData))

	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if httpResponse.StatusCode != http.StatusOK {
		if httpResponse.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("quota exceeded: %s", string(body))
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	var xaiResponse XAIChatResponse
	if err := json.Unmarshal(body, &xaiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert xAI response to OpenAI format
	openaiResponse := &openai.ChatCompletionResponse{
		Id:      xaiResponse.ID,
		Object:  "chat.completion",
		Created: xaiResponse.Created,
		Model:   xaiResponse.Model,
		Choices: make([]openai.Choice, len(xaiResponse.Choices)),
		Usage: openai.Usage{
			PromptTokens:     xaiResponse.Usage.PromptTokens,
			CompletionTokens: xaiResponse.Usage.CompletionTokens,
			TotalTokens:      xaiResponse.Usage.TotalTokens,
		},
	}

	for i, choice := range xaiResponse.Choices {
		content := ""
		if choice.Message.Content != nil {
			if str, ok := choice.Message.Content.(string); ok {
				content = str
			}
		}

		openaiChoice := openai.Choice{
			Index: choice.Index,
			Message: openai.Message{
				Role:    choice.Message.Role,
				Content: &openai.MessageContent{String: &content},
			},
			FinishReason: choice.FinishReason,
		}

		// Convert tool calls
		if len(choice.Message.ToolCalls) > 0 {
			openaiChoice.Message.ToolCalls = make([]openai.ToolCall, len(choice.Message.ToolCalls))
			for j, toolCall := range choice.Message.ToolCalls {
				openaiChoice.Message.ToolCalls[j] = openai.ToolCall{
					Id:   toolCall.ID,
					Type: toolCall.Type,
					Function: &openai.FunctionCall{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
			}
		}

		openaiResponse.Choices[i] = openaiChoice
	}

	return openaiResponse, nil
}

func (p *Endpoint) GenerateChatCompletionStream(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	responseCh := make(chan *openai.ChatCompletionStreamResponse)
	errorCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer close(errorCh)

		// For now, convert to non-streaming
		openaiResponse, err := p.GenerateChatCompletion(ctx, openaiRequest)
		if err != nil {
			errorCh <- err
			return
		}

		// Convert to streaming format
		if len(openaiResponse.Choices) > 0 {
			choice := openaiResponse.Choices[0]
			
			// Send role chunk
			roleChunk := &openai.ChatCompletionStreamResponse{
				Id:      openaiResponse.Id,
				Object:  "chat.completion.chunk",
				Created: openaiResponse.Created,
				Model:   openaiResponse.Model,
				Choices: []openai.ChoiceDelta{
					{
						Index: 0,
						Delta: openai.MessageDelta{
							Role: &choice.Message.Role,
						},
					},
				},
			}
			
			select {
			case responseCh <- roleChunk:
			case <-ctx.Done():
				return
			}

			// Send content chunk
			if choice.Message.Content != nil && choice.Message.Content.String != nil {
				content := *choice.Message.Content.String
				contentChunk := &openai.ChatCompletionStreamResponse{
					Id:      openaiResponse.Id,
					Object:  "chat.completion.chunk", 
					Created: openaiResponse.Created,
					Model:   openaiResponse.Model,
					Choices: []openai.ChoiceDelta{
						{
							Index: 0,
							Delta: openai.MessageDelta{
								Content: &content,
							},
						},
					},
				}
				
				select {
				case responseCh <- contentChunk:
				case <-ctx.Done():
					return
				}
			}

			// Send final chunk
			finalChunk := &openai.ChatCompletionStreamResponse{
				Id:      openaiResponse.Id,
				Object:  "chat.completion.chunk",
				Created: openaiResponse.Created,
				Model:   openaiResponse.Model,
				Choices: []openai.ChoiceDelta{
					{
						Index:        0,
						Delta:        openai.MessageDelta{},
						FinishReason: &choice.FinishReason,
					},
				},
				Usage: &openaiResponse.Usage,
			}
			
			select {
			case responseCh <- finalChunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return responseCh, errorCh
}

func (p *Endpoint) GenerateEmbedding(ctx context.Context, embeddingRequest *openai.EmbeddingRequest) (*openai.EmbeddingResponse, error) {
	return nil, fmt.Errorf("embeddings not supported by xAI provider")
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	return nil, fmt.Errorf("image generation not supported by xAI provider")
}

func (p *Endpoint) TranscribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	return nil, fmt.Errorf("audio transcription not supported by xAI provider")
}

func (p *Endpoint) TranslateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	return nil, fmt.Errorf("audio translation not supported by xAI provider")
}

func (p *Endpoint) GenerateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	return nil, fmt.Errorf("speech generation not supported by xAI provider")
}

func (p *Endpoint) ModerateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, fmt.Errorf("content moderation not supported by xAI provider")
}

func (p *Endpoint) CreateFineTuningJob(ctx context.Context, request *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by xAI provider")
}

func (p *Endpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by xAI provider")
}

func (p *Endpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, fmt.Errorf("fine-tuning not supported by xAI provider")
}

func (p *Endpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by xAI provider")
}

func (p *Endpoint) Provider() string {
	return "xai"
}

func (p *Endpoint) Region() string {
	return p.region
}

func (p *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseUrl.String()+"/models", nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()
	return time.Since(start), nil
}

func (p *Endpoint) Shutdown() error {
	return nil
}