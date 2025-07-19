package bedrock

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/yanolja/ogem/openai"
)

const REGION = "bedrock"

type Endpoint struct {
	region        string
	client        *bedrockruntime.Client
	bedrockClient *bedrock.Client
}

func NewEndpoint(region string, accessKey string, secretKey string, sessionToken string) (*Endpoint, error) {
	if region == "" {
		region = "us-east-1" // Default region
	}

	// Load AWS configuration
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %v", err)
	}

	// Override credentials if provided
	if accessKey != "" && secretKey != "" {
		cfg.Credentials = credentials.NewStaticCredentialsProvider(accessKey, secretKey, sessionToken)
	}

	client := bedrockruntime.NewFromConfig(cfg)
	bedrockClient := bedrock.NewFromConfig(cfg)

	endpoint := &Endpoint{
		region:        region,
		client:        client,
		bedrockClient: bedrockClient,
	}

	return endpoint, nil
}

func (p *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	// Convert OpenAI request to Bedrock format based on model
	modelId := mapModelName(openaiRequest.Model)
	payload, err := createBedrockPayload(openaiRequest, modelId)
	if err != nil {
		return nil, fmt.Errorf("failed to create bedrock payload: %v", err)
	}

	// Invoke the model
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelId),
		Body:        payload,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	}

	response, err := p.client.InvokeModel(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke bedrock model: %v", err)
	}

	// Parse response based on model type
	openaiResponse, err := parseBedrockResponse(response.Body, modelId, openaiRequest.Model)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bedrock response: %v", err)
	}

	return openaiResponse, nil
}

func (p *Endpoint) GenerateChatCompletionStream(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	responseCh := make(chan *openai.ChatCompletionStreamResponse)
	errorCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer close(errorCh)

		// Convert OpenAI request to Bedrock streaming format
		modelId := mapModelName(openaiRequest.Model)
		payload, err := createBedrockStreamPayload(openaiRequest, modelId)
		if err != nil {
			errorCh <- fmt.Errorf("failed to create bedrock payload: %v", err)
			return
		}

		// Invoke the model with streaming
		input := &bedrockruntime.InvokeModelWithResponseStreamInput{
			ModelId:     aws.String(modelId),
			Body:        payload,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		}

		response, err := p.client.InvokeModelWithResponseStream(ctx, input)
		if err != nil {
			errorCh <- fmt.Errorf("failed to invoke bedrock streaming: %v", err)
			return
		}

		// Process streaming response
		for event := range response.GetStream().Events() {
			switch e := event.(type) {
			case *types.ResponseStreamMemberChunk:
				chunk, err := parseBedrockStreamChunk(e.Value.Bytes, modelId, openaiRequest.Model)
				if err != nil {
					errorCh <- err
					return
				}
				if chunk != nil {
					select {
					case responseCh <- chunk:
					case <-ctx.Done():
						return
					}
				}
			case *types.InvokeModelWithResponseStreamOutput_InternalServerException:
				errorCh <- fmt.Errorf("bedrock model error: %s", *e.Value.Message)
				return
			default:
				// Unknown event type, ignore
			}
		}

		// Check for stream errors
		if err := response.GetStream().Err(); err != nil {
			errorCh <- fmt.Errorf("bedrock stream error: %v", err)
		}
	}()

	return responseCh, errorCh
}

func (p *Endpoint) GenerateEmbedding(ctx context.Context, embeddingRequest *openai.EmbeddingRequest) (*openai.EmbeddingResponse, error) {
	// Use Amazon Titan Text Embeddings
	modelId := "amazon.titan-embed-text-v1"

	embeddingObjects := make([]openai.EmbeddingObject, len(embeddingRequest.Input))
	totalTokens := int32(0)

	for i, input := range embeddingRequest.Input {
		payload := map[string]interface{}{
			"inputText": input,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal embedding payload: %v", err)
		}

		request := &bedrockruntime.InvokeModelInput{
			ModelId:     aws.String(modelId),
			Body:        payloadBytes,
			ContentType: aws.String("application/json"),
			Accept:      aws.String("application/json"),
		}

		response, err := p.client.InvokeModel(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("failed to invoke embedding model: %v", err)
		}

		var embeddingResponse struct {
			Embedding           []float32 `json:"embedding"`
			InputTextTokenCount int32     `json:"inputTextTokenCount"`
		}

		if err := json.Unmarshal(response.Body, &embeddingResponse); err != nil {
			return nil, fmt.Errorf("failed to parse embedding response: %v", err)
		}

		embeddingObjects[i] = openai.EmbeddingObject{
			Object:    "embedding",
			Embedding: embeddingResponse.Embedding,
			Index:     int32(i),
		}

		totalTokens += embeddingResponse.InputTextTokenCount
	}

	return &openai.EmbeddingResponse{
		Object: "list",
		Data:   embeddingObjects,
		Model:  embeddingRequest.Model,
		Usage: openai.EmbeddingUsage{
			PromptTokens: totalTokens,
			TotalTokens:  totalTokens,
		},
	}, nil
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	// Use Stability AI SDXL on Bedrock
	modelId := "stability.stable-diffusion-xl-v1"

	payload := map[string]interface{}{
		"text_prompts": []map[string]interface{}{
			{"text": imageRequest.Prompt},
		},
		"cfg_scale": 10,
		"seed":      0,
		"steps":     50,
	}

	if imageRequest.Size != nil {
		width, height := parseImageSize(*imageRequest.Size)
		payload["width"] = width
		payload["height"] = height
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal image payload: %v", err)
	}

	request := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(modelId),
		Body:        payloadBytes,
		ContentType: aws.String("application/json"),
		Accept:      aws.String("application/json"),
	}

	response, err := p.client.InvokeModel(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke image model: %v", err)
	}

	var imageResponse struct {
		Artifacts []struct {
			Base64       string `json:"base64"`
			FinishReason string `json:"finishReason"`
		} `json:"artifacts"`
	}

	if err := json.Unmarshal(response.Body, &imageResponse); err != nil {
		return nil, fmt.Errorf("failed to parse image response: %v", err)
	}

	if len(imageResponse.Artifacts) == 0 {
		return nil, fmt.Errorf("no images generated")
	}

	b64Image := fmt.Sprintf("data:image/png;base64,%s", imageResponse.Artifacts[0].Base64)

	return &openai.ImageGenerationResponse{
		Created: time.Now().Unix(),
		Data: []openai.ImageData{
			{
				B64JSON: &b64Image,
			},
		},
	}, nil
}

func (p *Endpoint) TranscribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	// Bedrock doesn't directly support audio transcription - would need Amazon Transcribe integration
	return nil, fmt.Errorf("bedrock audio transcription not supported - use Amazon Transcribe service instead")
}

func (p *Endpoint) TranslateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	// Bedrock doesn't directly support audio translation - would need Amazon Transcribe + Translate
	return nil, fmt.Errorf("bedrock audio translation not supported - use Amazon Transcribe + Translate services instead")
}

func (p *Endpoint) GenerateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	// Bedrock doesn't directly support speech synthesis - would need Amazon Polly integration
	return nil, fmt.Errorf("bedrock speech generation not supported - use Amazon Polly service instead")
}

func (p *Endpoint) ModerateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, fmt.Errorf("content moderation not yet implemented for Bedrock provider")
}

func (p *Endpoint) CreateFineTuningJob(ctx context.Context, request *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Bedrock provider")
}

func (p *Endpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Bedrock provider")
}

func (p *Endpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Bedrock provider")
}

func (p *Endpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by Bedrock provider")
}

func (p *Endpoint) Provider() string {
	return "bedrock"
}

func (p *Endpoint) Region() string {
	return p.region
}

func (p *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()

	// Simple ping by listing available models
	_, err := p.bedrockClient.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return 0, fmt.Errorf("bedrock ping failed: %v", err)
	}

	return time.Since(start), nil
}

func (p *Endpoint) Shutdown() error {
	return nil
}

// Helper functions for Bedrock model mapping and payload creation

func mapModelName(openaiModel string) string {
	// Map OpenAI model names to Bedrock model IDs
	switch {
	case strings.Contains(openaiModel, "claude"):
		if strings.Contains(openaiModel, "3-5-sonnet") {
			return "anthropic.claude-3-5-sonnet-20240620-v1:0"
		} else if strings.Contains(openaiModel, "3-opus") {
			return "anthropic.claude-3-opus-20240229-v1:0"
		} else if strings.Contains(openaiModel, "3-sonnet") {
			return "anthropic.claude-3-sonnet-20240229-v1:0"
		} else if strings.Contains(openaiModel, "3-haiku") {
			return "anthropic.claude-3-haiku-20240307-v1:0"
		}
		return "anthropic.claude-3-sonnet-20240229-v1:0"
	case strings.Contains(openaiModel, "llama"):
		return "meta.llama2-70b-chat-v1"
	case strings.Contains(openaiModel, "titan"):
		return "amazon.titan-text-express-v1"
	default:
		// Default to Claude 3.5 Sonnet
		return "anthropic.claude-3-5-sonnet-20240620-v1:0"
	}
}

func createBedrockPayload(openaiRequest *openai.ChatCompletionRequest, modelId string) ([]byte, error) {
	if strings.Contains(modelId, "anthropic.claude") {
		return createClaudePayload(openaiRequest)
	} else if strings.Contains(modelId, "meta.llama") {
		return createLlamaPayload(openaiRequest)
	} else if strings.Contains(modelId, "amazon.titan") {
		return createTitanPayload(openaiRequest)
	}
	return nil, fmt.Errorf("unsupported model: %s", modelId)
}

func createBedrockStreamPayload(openaiRequest *openai.ChatCompletionRequest, modelId string) ([]byte, error) {
	// Same as regular payload but with streaming enabled
	if strings.Contains(modelId, "anthropic.claude") {
		payload, err := createClaudePayload(openaiRequest)
		if err != nil {
			return nil, err
		}
		// Enable streaming for Claude
		var claudePayload map[string]interface{}
		if err := json.Unmarshal(payload, &claudePayload); err != nil {
			return nil, err
		}
		claudePayload["anthropic_version"] = "bedrock-2023-05-31"
		return json.Marshal(claudePayload)
	}
	return createBedrockPayload(openaiRequest, modelId)
}

func createClaudePayload(openaiRequest *openai.ChatCompletionRequest) ([]byte, error) {
	// Convert OpenAI messages to Claude format
	messages := make([]map[string]interface{}, 0)
	system := ""

	for _, msg := range openaiRequest.Messages {
		if msg.Role == "system" {
			if msg.Content != nil && msg.Content.String != nil {
				system = *msg.Content.String
			}
			continue
		}

		role := msg.Role
		if role == "assistant" {
			role = "assistant"
		} else {
			role = "user"
		}

		content := ""
		if msg.Content != nil && msg.Content.String != nil {
			content = *msg.Content.String
		}

		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}

	payload := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4096,
		"messages":          messages,
	}

	if system != "" {
		payload["system"] = system
	}

	if openaiRequest.MaxTokens != nil {
		payload["max_tokens"] = *openaiRequest.MaxTokens
	}

	if openaiRequest.Temperature != nil {
		payload["temperature"] = *openaiRequest.Temperature
	}

	return json.Marshal(payload)
}

func createLlamaPayload(openaiRequest *openai.ChatCompletionRequest) ([]byte, error) {
	// Convert to Llama format
	prompt := ""
	for _, msg := range openaiRequest.Messages {
		if msg.Content != nil && msg.Content.String != nil {
			role := strings.Title(msg.Role)
			prompt += fmt.Sprintf("<%s>%s</%s>", role, *msg.Content.String, role)
		}
	}

	payload := map[string]interface{}{
		"prompt":      prompt,
		"max_gen_len": 512,
		"temperature": 0.7,
		"top_p":       0.9,
	}

	if openaiRequest.MaxTokens != nil {
		payload["max_gen_len"] = *openaiRequest.MaxTokens
	}

	if openaiRequest.Temperature != nil {
		payload["temperature"] = *openaiRequest.Temperature
	}

	return json.Marshal(payload)
}

func createTitanPayload(openaiRequest *openai.ChatCompletionRequest) ([]byte, error) {
	// Convert to Titan format
	prompt := ""
	for _, msg := range openaiRequest.Messages {
		if msg.Content != nil && msg.Content.String != nil {
			prompt += fmt.Sprintf("%s: %s\n", strings.Title(msg.Role), *msg.Content.String)
		}
	}

	payload := map[string]interface{}{
		"inputText": prompt,
		"textGenerationConfig": map[string]interface{}{
			"maxTokenCount": 512,
			"temperature":   0.7,
			"topP":          0.9,
			"stopSequences": []string{},
		},
	}

	if openaiRequest.MaxTokens != nil {
		payload["textGenerationConfig"].(map[string]interface{})["maxTokenCount"] = *openaiRequest.MaxTokens
	}

	if openaiRequest.Temperature != nil {
		payload["textGenerationConfig"].(map[string]interface{})["temperature"] = *openaiRequest.Temperature
	}

	return json.Marshal(payload)
}

func parseBedrockResponse(responseBody []byte, modelId, originalModel string) (*openai.ChatCompletionResponse, error) {
	if strings.Contains(modelId, "anthropic.claude") {
		return parseClaudeResponse(responseBody, originalModel)
	} else if strings.Contains(modelId, "meta.llama") {
		return parseLlamaResponse(responseBody, originalModel)
	} else if strings.Contains(modelId, "amazon.titan") {
		return parseTitanResponse(responseBody, originalModel)
	}
	return nil, fmt.Errorf("unsupported model: %s", modelId)
}

func parseClaudeResponse(responseBody []byte, originalModel string) (*openai.ChatCompletionResponse, error) {
	var claudeResp struct {
		Content []struct {
			Text string `json:"text"`
			Type string `json:"type"`
		} `json:"content"`
		Usage struct {
			InputTokens  int32 `json:"input_tokens"`
			OutputTokens int32 `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal(responseBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("failed to parse Claude response: %v", err)
	}

	content := ""
	for _, c := range claudeResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	finishReason := "stop"
	if claudeResp.StopReason == "max_tokens" {
		finishReason = "length"
	}

	return &openai.ChatCompletionResponse{
		Id:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   originalModel,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: &openai.MessageContent{String: &content},
				},
				FinishReason: finishReason,
			},
		},
		Usage: openai.Usage{
			PromptTokens:     claudeResp.Usage.InputTokens,
			CompletionTokens: claudeResp.Usage.OutputTokens,
			TotalTokens:      claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		},
	}, nil
}

func parseLlamaResponse(responseBody []byte, originalModel string) (*openai.ChatCompletionResponse, error) {
	var llamaResp struct {
		Generation           string `json:"generation"`
		PromptTokenCount     int32  `json:"prompt_token_count"`
		GenerationTokenCount int32  `json:"generation_token_count"`
		StopReason           string `json:"stop_reason"`
	}

	if err := json.Unmarshal(responseBody, &llamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse Llama response: %v", err)
	}

	finishReason := "stop"
	if llamaResp.StopReason == "length" {
		finishReason = "length"
	}

	return &openai.ChatCompletionResponse{
		Id:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   originalModel,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: &openai.MessageContent{String: &llamaResp.Generation},
				},
				FinishReason: finishReason,
			},
		},
		Usage: openai.Usage{
			PromptTokens:     llamaResp.PromptTokenCount,
			CompletionTokens: llamaResp.GenerationTokenCount,
			TotalTokens:      llamaResp.PromptTokenCount + llamaResp.GenerationTokenCount,
		},
	}, nil
}

func parseTitanResponse(responseBody []byte, originalModel string) (*openai.ChatCompletionResponse, error) {
	var titanResp struct {
		Results []struct {
			OutputText       string `json:"outputText"`
			TokenCount       int32  `json:"tokenCount"`
			CompletionReason string `json:"completionReason"`
		} `json:"results"`
		InputTextTokenCount int32 `json:"inputTextTokenCount"`
	}

	if err := json.Unmarshal(responseBody, &titanResp); err != nil {
		return nil, fmt.Errorf("failed to parse Titan response: %v", err)
	}

	if len(titanResp.Results) == 0 {
		return nil, fmt.Errorf("no results in Titan response")
	}

	result := titanResp.Results[0]
	finishReason := "stop"
	if result.CompletionReason == "LENGTH" {
		finishReason = "length"
	}

	return &openai.ChatCompletionResponse{
		Id:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   originalModel,
		Choices: []openai.Choice{
			{
				Index: 0,
				Message: openai.Message{
					Role:    "assistant",
					Content: &openai.MessageContent{String: &result.OutputText},
				},
				FinishReason: finishReason,
			},
		},
		Usage: openai.Usage{
			PromptTokens:     titanResp.InputTextTokenCount,
			CompletionTokens: result.TokenCount,
			TotalTokens:      titanResp.InputTextTokenCount + result.TokenCount,
		},
	}, nil
}

func parseBedrockStreamChunk(chunkData []byte, modelId, originalModel string) (*openai.ChatCompletionStreamResponse, error) {
	if strings.Contains(modelId, "anthropic.claude") {
		return parseClaudeStreamChunk(chunkData, originalModel)
	}
	// For other models, return nil to skip
	return nil, nil
}

func parseClaudeStreamChunk(chunkData []byte, originalModel string) (*openai.ChatCompletionStreamResponse, error) {
	var chunk struct {
		Type  string `json:"type"`
		Index int32  `json:"index"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
	}

	if err := json.Unmarshal(chunkData, &chunk); err != nil {
		return nil, fmt.Errorf("failed to parse Claude stream chunk: %v", err)
	}

	if chunk.Type == "content_block_delta" && chunk.Delta.Type == "text_delta" {
		return &openai.ChatCompletionStreamResponse{
			Id:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   originalModel,
			Choices: []openai.ChoiceDelta{
				{
					Index: 0,
					Delta: openai.MessageDelta{
						Content: &chunk.Delta.Text,
					},
				},
			},
		}, nil
	}

	return nil, nil
}

func parseImageSize(size string) (int, int) {
	switch size {
	case "256x256":
		return 256, 256
	case "512x512":
		return 512, 512
	case "1024x1024":
		return 1024, 1024
	case "1792x1024":
		return 1792, 1024
	case "1024x1792":
		return 1024, 1792
	default:
		return 1024, 1024
	}
}
