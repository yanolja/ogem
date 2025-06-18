package mistral

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

const REGION = "mistral"

type Endpoint struct {
	apiKey  string
	baseUrl *url.URL
	client  *http.Client
	region  string
}

// Mistral API structures
type MistralChatRequest struct {
	Model            string                 `json:"model"`
	Messages         []MistralMessage       `json:"messages"`
	Temperature      *float32               `json:"temperature,omitempty"`
	TopP             *float32               `json:"top_p,omitempty"`
	MaxTokens        *int32                 `json:"max_tokens,omitempty"`
	Stream           *bool                  `json:"stream,omitempty"`
	SafePrompt       *bool                  `json:"safe_prompt,omitempty"`
	RandomSeed       *int32                 `json:"random_seed,omitempty"`
	Tools            []MistralTool          `json:"tools,omitempty"`
	ToolChoice       interface{}            `json:"tool_choice,omitempty"`
	ResponseFormat   *MistralResponseFormat `json:"response_format,omitempty"`
}

type MistralMessage struct {
	Role      string             `json:"role"`
	Content   interface{}        `json:"content"`
	Name      *string            `json:"name,omitempty"`
	ToolCalls []MistralToolCall  `json:"tool_calls,omitempty"`
	ToolCallID *string           `json:"tool_call_id,omitempty"`
}

type MistralTool struct {
	Type     string              `json:"type"`
	Function MistralFunctionDef  `json:"function"`
}

type MistralFunctionDef struct {
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters"`
}

type MistralToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function MistralFunctionCall `json:"function"`
}

type MistralFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type MistralResponseFormat struct {
	Type string `json:"type"`
}

type MistralChatResponse struct {
	ID      string          `json:"id"`
	Object  string          `json:"object"`
	Created int64           `json:"created"`
	Model   string          `json:"model"`
	Choices []MistralChoice `json:"choices"`
	Usage   MistralUsage    `json:"usage"`
}

type MistralChoice struct {
	Index        int32          `json:"index"`
	Message      MistralMessage `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

type MistralUsage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

// Mistral Embeddings API
type MistralEmbeddingRequest struct {
	Model    string   `json:"model"`
	Input    []string `json:"input"`
	Encoding string   `json:"encoding_format,omitempty"`
}

type MistralEmbeddingResponse struct {
	ID     string                  `json:"id"`
	Object string                  `json:"object"`
	Data   []MistralEmbeddingData  `json:"data"`
	Model  string                  `json:"model"`
	Usage  MistralEmbeddingUsage   `json:"usage"`
}

type MistralEmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int32     `json:"index"`
}

type MistralEmbeddingUsage struct {
	PromptTokens int32 `json:"prompt_tokens"`
	TotalTokens  int32 `json:"total_tokens"`
}

func NewEndpoint(region string, baseUrl string, apiKey string) (*Endpoint, error) {
	if baseUrl == "" {
		baseUrl = "https://api.mistral.ai/v1"
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
	// Convert OpenAI request to Mistral format
	mistralRequest := &MistralChatRequest{
		Model:       openaiRequest.Model,
		Temperature: openaiRequest.Temperature,
		TopP:        openaiRequest.TopP,
		MaxTokens:   openaiRequest.MaxTokens,
	}

	// Convert messages
	mistralRequest.Messages = make([]MistralMessage, len(openaiRequest.Messages))
	for i, msg := range openaiRequest.Messages {
		mistralMsg := MistralMessage{
			Role: msg.Role,
		}

		// Handle content
		if msg.Content != nil {
			if msg.Content.String != nil {
				mistralMsg.Content = *msg.Content.String
			} else {
				// For multipart content, combine text parts
				var textContent strings.Builder
				for _, part := range msg.Content.Parts {
					if part.Content.TextContent != nil {
						textContent.WriteString(part.Content.TextContent.Text)
					}
				}
				mistralMsg.Content = textContent.String()
			}
		}

		// Handle tool calls
		if len(msg.ToolCalls) > 0 {
			mistralMsg.ToolCalls = make([]MistralToolCall, len(msg.ToolCalls))
			for j, toolCall := range msg.ToolCalls {
				mistralMsg.ToolCalls[j] = MistralToolCall{
					ID:   toolCall.Id,
					Type: toolCall.Type,
					Function: MistralFunctionCall{
						Name:      toolCall.Function.Name,
						Arguments: toolCall.Function.Arguments,
					},
				}
			}
		}

		if msg.ToolCallId != nil {
			mistralMsg.ToolCallID = msg.ToolCallId
		}

		mistralRequest.Messages[i] = mistralMsg
	}

	// Convert tools
	if len(openaiRequest.Tools) > 0 {
		mistralRequest.Tools = make([]MistralTool, len(openaiRequest.Tools))
		for i, tool := range openaiRequest.Tools {
			mistralRequest.Tools[i] = MistralTool{
				Type: tool.Type,
				Function: MistralFunctionDef{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			}
		}
	}

	// Convert tool choice
	if openaiRequest.ToolChoice != nil {
		mistralRequest.ToolChoice = openaiRequest.ToolChoice
	}

	// Convert response format
	if openaiRequest.ResponseFormat != nil {
		mistralRequest.ResponseFormat = &MistralResponseFormat{
			Type: openaiRequest.ResponseFormat.Type,
		}
	}

	jsonData, err := json.Marshal(mistralRequest)
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

	var mistralResponse MistralChatResponse
	if err := json.Unmarshal(body, &mistralResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert Mistral response to OpenAI format
	openaiResponse := &openai.ChatCompletionResponse{
		Id:      mistralResponse.ID,
		Object:  "chat.completion",
		Created: mistralResponse.Created,
		Model:   mistralResponse.Model,
		Choices: make([]openai.Choice, len(mistralResponse.Choices)),
		Usage: openai.Usage{
			PromptTokens:     mistralResponse.Usage.PromptTokens,
			CompletionTokens: mistralResponse.Usage.CompletionTokens,
			TotalTokens:      mistralResponse.Usage.TotalTokens,
		},
	}

	for i, choice := range mistralResponse.Choices {
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
	mistralRequest := &MistralEmbeddingRequest{
		Model: embeddingRequest.Model,
		Input: embeddingRequest.Input,
	}

	jsonData, err := json.Marshal(mistralRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "embeddings")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Printf("Sending %s request to %s", httpRequest.Method, endpointPath)

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

	var mistralResponse MistralEmbeddingResponse
	if err := json.Unmarshal(body, &mistralResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert to OpenAI format
	embeddingObjects := make([]openai.EmbeddingObject, len(mistralResponse.Data))
	for i, data := range mistralResponse.Data {
		embeddingObjects[i] = openai.EmbeddingObject{
			Object:    data.Object,
			Embedding: data.Embedding,
			Index:     data.Index,
		}
	}

	return &openai.EmbeddingResponse{
		Object: mistralResponse.Object,
		Data:   embeddingObjects,
		Model:  mistralResponse.Model,
		Usage: openai.EmbeddingUsage{
			PromptTokens: mistralResponse.Usage.PromptTokens,
			TotalTokens:  mistralResponse.Usage.TotalTokens,
		},
	}, nil
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	return nil, fmt.Errorf("image generation not supported by Mistral provider")
}

func (p *Endpoint) TranscribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	return nil, fmt.Errorf("audio transcription not supported by Mistral provider")
}

func (p *Endpoint) TranslateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	return nil, fmt.Errorf("audio translation not supported by Mistral provider")
}

func (p *Endpoint) GenerateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	return nil, fmt.Errorf("speech generation not supported by Mistral provider")
}

func (p *Endpoint) ModerateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, fmt.Errorf("content moderation not supported by Mistral provider")
}

func (p *Endpoint) CreateFineTuningJob(ctx context.Context, request *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Mistral provider")
}

func (p *Endpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Mistral provider")
}

func (p *Endpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Mistral provider")
}

func (p *Endpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Mistral provider")
}

func (p *Endpoint) Provider() string {
	return "mistral"
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