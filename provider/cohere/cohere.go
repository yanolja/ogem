package cohere

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

const REGION = "cohere"

type Endpoint struct {
	apiKey  string
	baseUrl *url.URL
	client  *http.Client
	region  string
}

// Cohere API structures
type CohereChatRequest struct {
	Model            string                   `json:"model"`
	Message          string                   `json:"message"`
	ChatHistory      []CohereChatMessage      `json:"chat_history,omitempty"`
	ConversationID   string                   `json:"conversation_id,omitempty"`
	PromptTruncation string                   `json:"prompt_truncation,omitempty"`
	Connectors       []CohereConnector        `json:"connectors,omitempty"`
	SearchQueries    []CohereSearchQuery      `json:"search_queries_only,omitempty"`
	Documents        []CohereDocument         `json:"documents,omitempty"`
	Temperature      *float32                 `json:"temperature,omitempty"`
	MaxTokens        *int32                   `json:"max_tokens,omitempty"`
	K                *int32                   `json:"k,omitempty"`
	P                *float32                 `json:"p,omitempty"`
	Seed             *int32                   `json:"seed,omitempty"`
	StopSequences    []string                 `json:"stop_sequences,omitempty"`
	FrequencyPenalty *float32                 `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float32                 `json:"presence_penalty,omitempty"`
	Tools            []CohereTool             `json:"tools,omitempty"`
	ToolResults      []CohereToolResult       `json:"tool_results,omitempty"`
}

type CohereChatMessage struct {
	Role    string `json:"role"`
	Message string `json:"message"`
}

type CohereConnector struct {
	ID string `json:"id"`
}

type CohereSearchQuery struct {
	Text         string `json:"text"`
	GenerationID string `json:"generation_id"`
}

type CohereDocument struct {
	Title   string                 `json:"title"`
	Snippet string                 `json:"snippet"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

type CohereTool struct {
	Name                 string                         `json:"name"`
	Description          string                         `json:"description"`
	ParameterDefinitions map[string]CohereParameterDef `json:"parameter_definitions"`
}

type CohereParameterDef struct {
	Description string `json:"description"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
}

type CohereToolResult struct {
	Call    CohereToolCall         `json:"call"`
	Outputs []CohereToolCallOutput `json:"outputs"`
}

type CohereToolCall struct {
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters"`
}

type CohereToolCallOutput struct {
	Text string `json:"text"`
}

type CohereChatResponse struct {
	ResponseID     string                `json:"response_id"`
	Text           string                `json:"text"`
	GenerationID   string                `json:"generation_id"`
	ChatHistory    []CohereChatMessage   `json:"chat_history"`
	FinishReason   string                `json:"finish_reason"`
	Meta           CohereResponseMeta    `json:"meta"`
	Citations      []CohereCitation      `json:"citations,omitempty"`
	Documents      []CohereDocument      `json:"documents,omitempty"`
	SearchQueries  []CohereSearchQuery   `json:"search_queries,omitempty"`
	SearchResults  []CohereSearchResult  `json:"search_results,omitempty"`
	ToolCalls      []CohereToolCall      `json:"tool_calls,omitempty"`
}

type CohereResponseMeta struct {
	APIVersion struct {
		Version string `json:"version"`
	} `json:"api_version"`
	BilledUnits struct {
		InputTokens  int32 `json:"input_tokens"`
		OutputTokens int32 `json:"output_tokens"`
	} `json:"billed_units"`
	Tokens struct {
		InputTokens  int32 `json:"input_tokens"`
		OutputTokens int32 `json:"output_tokens"`
	} `json:"tokens"`
}

type CohereCitation struct {
	Start       int32    `json:"start"`
	End         int32    `json:"end"`
	Text        string   `json:"text"`
	DocumentIDs []string `json:"document_ids"`
}

type CohereSearchResult struct {
	SearchQuery CohereSearchQuery `json:"search_query"`
	Connectors  []CohereConnector `json:"connectors"`
	DocumentIDs []string          `json:"document_ids"`
}

// Cohere Embed API structures
type CohereEmbedRequest struct {
	Texts         []string `json:"texts"`
	Model         string   `json:"model"`
	InputType     string   `json:"input_type"`
	EmbeddingType string   `json:"embedding_types,omitempty"`
	Truncate      string   `json:"truncate,omitempty"`
}

type CohereEmbedResponse struct {
	ID          string      `json:"id"`
	Embeddings  [][]float32 `json:"embeddings"`
	Texts       []string    `json:"texts"`
	Meta        CohereEmbedMeta `json:"meta"`
	ResponseType string     `json:"response_type"`
}

type CohereEmbedMeta struct {
	APIVersion struct {
		Version string `json:"version"`
	} `json:"api_version"`
	BilledUnits struct {
		InputTokens int32 `json:"input_tokens"`
	} `json:"billed_units"`
}

func NewEndpoint(region string, baseUrl string, apiKey string) (*Endpoint, error) {
	if baseUrl == "" {
		baseUrl = "https://api.cohere.ai/v1"
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
	// Convert OpenAI request to Cohere format
	cohereRequest := &CohereChatRequest{
		Model:       openaiRequest.Model,
		Temperature: openaiRequest.Temperature,
		MaxTokens:   openaiRequest.MaxTokens,
	}

	// Extract the last message as the main message and convert others to chat history
	if len(openaiRequest.Messages) > 0 {
		lastMessage := openaiRequest.Messages[len(openaiRequest.Messages)-1]
		if lastMessage.Content != nil && lastMessage.Content.String != nil {
			cohereRequest.Message = *lastMessage.Content.String
		}

		// Convert previous messages to chat history
		if len(openaiRequest.Messages) > 1 {
			chatHistory := make([]CohereChatMessage, 0, len(openaiRequest.Messages)-1)
			for _, msg := range openaiRequest.Messages[:len(openaiRequest.Messages)-1] {
				if msg.Role != "system" && msg.Content != nil && msg.Content.String != nil {
					role := msg.Role
					if role == "assistant" {
						role = "CHATBOT"
					} else if role == "user" {
						role = "USER"
					}
					chatHistory = append(chatHistory, CohereChatMessage{
						Role:    role,
						Message: *msg.Content.String,
					})
				}
			}
			cohereRequest.ChatHistory = chatHistory
		}
	}

	if openaiRequest.StopSequences != nil {
		cohereRequest.StopSequences = openaiRequest.StopSequences.Sequences
	}

	jsonData, err := json.Marshal(cohereRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "chat")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
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

	var cohereResponse CohereChatResponse
	if err := json.Unmarshal(body, &cohereResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert Cohere response to OpenAI format
	openaiResponse := &openai.ChatCompletionResponse{
		Id:      cohereResponse.ResponseID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   openaiRequest.Model,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: &openai.MessageContent{String: &cohereResponse.Text},
				},
				FinishReason: convertCohereFinishReason(cohereResponse.FinishReason),
			},
		},
		Usage: openai.Usage{
			PromptTokens:     cohereResponse.Meta.BilledUnits.InputTokens,
			CompletionTokens: cohereResponse.Meta.BilledUnits.OutputTokens,
			TotalTokens:      cohereResponse.Meta.BilledUnits.InputTokens + cohereResponse.Meta.BilledUnits.OutputTokens,
		},
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
	cohereRequest := &CohereEmbedRequest{
		Texts:     embeddingRequest.Input,
		Model:     embeddingRequest.Model,
		InputType: "search_document", // Default input type
		Truncate:  "END",
	}

	jsonData, err := json.Marshal(cohereRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "embed")
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

	var cohereResponse CohereEmbedResponse
	if err := json.Unmarshal(body, &cohereResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	// Convert to OpenAI format
	embeddingObjects := make([]openai.EmbeddingObject, len(cohereResponse.Embeddings))
	for i, embedding := range cohereResponse.Embeddings {
		embeddingObjects[i] = openai.EmbeddingObject{
			Object:    "embedding",
			Embedding: embedding,
			Index:     int32(i),
		}
	}

	return &openai.EmbeddingResponse{
		Object: "list",
		Data:   embeddingObjects,
		Model:  embeddingRequest.Model,
		Usage: openai.EmbeddingUsage{
			PromptTokens: cohereResponse.Meta.BilledUnits.InputTokens,
			TotalTokens:  cohereResponse.Meta.BilledUnits.InputTokens,
		},
	}, nil
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	return nil, fmt.Errorf("image generation not supported by Cohere provider")
}

func (p *Endpoint) TranscribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	return nil, fmt.Errorf("audio transcription not supported by Cohere provider")
}

func (p *Endpoint) TranslateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	return nil, fmt.Errorf("audio translation not supported by Cohere provider")
}

func (p *Endpoint) GenerateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	return nil, fmt.Errorf("speech generation not supported by Cohere provider")
}

func (p *Endpoint) ModerateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, fmt.Errorf("content moderation not supported by Cohere provider")
}

func (p *Endpoint) CreateFineTuningJob(ctx context.Context, request *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Cohere provider")
}

func (p *Endpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Cohere provider")
}

func (p *Endpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Cohere provider")
}

func (p *Endpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Cohere provider")
}

func (p *Endpoint) Provider() string {
	return "cohere"
}

func (p *Endpoint) Region() string {
	return p.region
}

func (p *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, "GET", p.baseUrl.String(), nil)
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

func convertCohereFinishReason(cohereReason string) string {
	switch strings.ToUpper(cohereReason) {
	case "COMPLETE":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "ERROR":
		return "content_filter"
	default:
		return "stop"
	}
}