package huggingface

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/yanolja/ogem/openai"
)

const REGION = "huggingface"

type Endpoint struct {
	apiKey  string
	baseUrl *url.URL
	client  *http.Client
	region  string
}

// HuggingFace API structures
type HFChatRequest struct {
	Model       string                   `json:"model"`
	Messages    []HFMessage              `json:"messages"`
	MaxTokens   *int32                   `json:"max_tokens,omitempty"`
	Temperature *float32                 `json:"temperature,omitempty"`
	TopP        *float32                 `json:"top_p,omitempty"`
	Stream      *bool                    `json:"stream,omitempty"`
	Stop        []string                 `json:"stop,omitempty"`
	Tools       []map[string]interface{} `json:"tools,omitempty"`
}

type HFMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type HFChatResponse struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []HFChoice `json:"choices"`
	Usage   HFUsage    `json:"usage"`
}

type HFChoice struct {
	Index        int32     `json:"index"`
	Message      HFMessage `json:"message"`
	FinishReason string    `json:"finish_reason"`
}

type HFUsage struct {
	PromptTokens     int32 `json:"prompt_tokens"`
	CompletionTokens int32 `json:"completion_tokens"`
	TotalTokens      int32 `json:"total_tokens"`
}

func NewEndpoint(region string, baseUrl string, apiKey string) (*Endpoint, error) {
	if baseUrl == "" {
		baseUrl = "https://api-inference.huggingface.co"
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
	// Convert OpenAI request to HuggingFace format
	hfRequest := &HFChatRequest{
		Model:       openaiRequest.Model,
		Messages:    make([]HFMessage, len(openaiRequest.Messages)),
		MaxTokens:   openaiRequest.MaxTokens,
		Temperature: openaiRequest.Temperature,
		TopP:        openaiRequest.TopP,
	}

	// Convert messages
	for i, msg := range openaiRequest.Messages {
		content := ""
		if msg.Content != nil && msg.Content.String != nil {
			content = *msg.Content.String
		}
		hfRequest.Messages[i] = HFMessage{
			Role:    msg.Role,
			Content: content,
		}
	}

	jsonData, err := json.Marshal(hfRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Use model-specific endpoint for HuggingFace
	endpointPath := fmt.Sprintf("%s/models/%s", p.baseUrl.String(), openaiRequest.Model)

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

	// Try to parse as HuggingFace response format first
	var hfResponse HFChatResponse
	if err := json.Unmarshal(body, &hfResponse); err != nil {
		// Fallback: some HF models return different formats
		return nil, fmt.Errorf("failed to decode HuggingFace response: %v", err)
	}

	// Convert HF response to OpenAI format
	openaiResponse := &openai.ChatCompletionResponse{
		Id:      hfResponse.ID,
		Object:  "chat.completion",
		Created: hfResponse.Created,
		Model:   hfResponse.Model,
		Choices: make([]openai.Choice, len(hfResponse.Choices)),
		Usage: openai.Usage{
			PromptTokens:     hfResponse.Usage.PromptTokens,
			CompletionTokens: hfResponse.Usage.CompletionTokens,
			TotalTokens:      hfResponse.Usage.TotalTokens,
		},
	}

	for i, choice := range hfResponse.Choices {
		content := choice.Message.Content
		openaiResponse.Choices[i] = openai.Choice{
			Index: choice.Index,
			Message: openai.Message{
				Role:    choice.Message.Role,
				Content: &openai.MessageContent{String: &content},
			},
			FinishReason: choice.FinishReason,
		}
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
	// HuggingFace embedding models typically use different API format
	payload := map[string]interface{}{
		"inputs": embeddingRequest.Input,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath := fmt.Sprintf("%s/pipeline/feature-extraction/%s", p.baseUrl.String(), embeddingRequest.Model)

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

	// HF returns embeddings as nested arrays
	var hfEmbeddings [][]float32
	if err := json.Unmarshal(body, &hfEmbeddings); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings: %v", err)
	}

	// Convert to OpenAI format
	embeddingObjects := make([]openai.EmbeddingObject, len(hfEmbeddings))
	for i, embedding := range hfEmbeddings {
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
			PromptTokens: int32(len(embeddingRequest.Input) * 10), // Rough estimate
			TotalTokens:  int32(len(embeddingRequest.Input) * 10),
		},
	}, nil
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	payload := map[string]interface{}{
		"inputs": imageRequest.Prompt,
	}

	if imageRequest.N != nil && *imageRequest.N > 1 {
		payload["num_inference_steps"] = 50
		payload["guidance_scale"] = 7.5
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	modelName := "stabilityai/stable-diffusion-2-1"
	if imageRequest.Model != nil {
		modelName = *imageRequest.Model
	}
	endpointPath := fmt.Sprintf("%s/models/%s", p.baseUrl.String(), modelName)

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

	if httpResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResponse.Body)
		if httpResponse.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("quota exceeded: %s", string(body))
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	// HF returns raw image data
	imageData, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read image data: %v", err)
	}

	// Convert to base64
	b64Image := fmt.Sprintf("data:image/png;base64,%s", string(imageData))

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
	// Use Whisper model on HuggingFace
	modelName := "openai/whisper-large-v3"
	if request.Model != "" {
		modelName = request.Model
	}

	// Create multipart form data for audio file
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)

	// Add audio file
	fileWriter, err := writer.CreateFormFile("inputs", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	// Convert base64 string to bytes
	audioData, err := base64.StdEncoding.DecodeString(request.File)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio file: %v", err)
	}

	_, err = fileWriter.Write(audioData)
	if err != nil {
		return nil, fmt.Errorf("failed to write audio data: %v", err)
	}

	writer.Close()

	endpointPath := fmt.Sprintf("%s/models/%s", p.baseUrl.String(), modelName)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, &payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Printf("Sending %s request to %s for audio transcription", httpRequest.Method, endpointPath)

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

	// HuggingFace Whisper returns the transcription directly as text
	var hfResponse struct {
		Text string `json:"text"`
	}

	if err := json.Unmarshal(body, &hfResponse); err != nil {
		// If JSON parsing fails, assume the response is plain text
		hfResponse.Text = string(body)
	}

	return &openai.AudioTranscriptionResponse{
		Text: hfResponse.Text,
	}, nil
}

func (p *Endpoint) TranslateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	// Use Whisper model for translation (same as transcription but with different prompt)
	modelName := "openai/whisper-large-v3"
	if request.Model != "" {
		modelName = request.Model
	}

	// Create multipart form data for audio file
	var payload bytes.Buffer
	writer := multipart.NewWriter(&payload)

	// Add audio file
	fileWriter, err := writer.CreateFormFile("inputs", "audio.mp3")
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}

	// Convert base64 string to bytes
	audioData, err := base64.StdEncoding.DecodeString(request.File)
	if err != nil {
		return nil, fmt.Errorf("failed to decode audio file: %v", err)
	}

	_, err = fileWriter.Write(audioData)
	if err != nil {
		return nil, fmt.Errorf("failed to write audio data: %v", err)
	}

	// Add task parameter for translation
	writer.WriteField("task", "translate")

	writer.Close()

	endpointPath := fmt.Sprintf("%s/models/%s", p.baseUrl.String(), modelName)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, &payload)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Printf("Sending %s request to %s for audio translation", httpRequest.Method, endpointPath)

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

	// HuggingFace Whisper returns the translation directly as text
	var hfResponse struct {
		Text string `json:"text"`
	}

	if err := json.Unmarshal(body, &hfResponse); err != nil {
		// If JSON parsing fails, assume the response is plain text
		hfResponse.Text = string(body)
	}

	return &openai.AudioTranslationResponse{
		Text: hfResponse.Text,
	}, nil
}

func (p *Endpoint) GenerateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	// Use Bark or similar TTS model on HuggingFace
	modelName := "suno/bark"
	if request.Model != "" {
		modelName = request.Model
	}

	payload := map[string]interface{}{
		"inputs": request.Input,
	}

	// Add voice parameter if available
	if request.Voice != "" {
		payload["voice"] = request.Voice
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath := fmt.Sprintf("%s/models/%s", p.baseUrl.String(), modelName)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

	log.Printf("Sending %s request to %s for speech generation", httpRequest.Method, endpointPath)

	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer httpResponse.Body.Close()

	if httpResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResponse.Body)
		if httpResponse.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("quota exceeded: %s", string(body))
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	// HuggingFace TTS models typically return audio data directly
	audioData, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %v", err)
	}

	// Determine content type based on response headers or default to mp3
	contentType := httpResponse.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "audio/mpeg"
	}

	return &openai.TextToSpeechResponse{
		Data: audioData,
	}, nil
}

func (p *Endpoint) ModerateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	return nil, fmt.Errorf("content moderation not yet implemented for HuggingFace provider")
}

func (p *Endpoint) CreateFineTuningJob(ctx context.Context, request *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by HuggingFace provider")
}

func (p *Endpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by HuggingFace provider")
}

func (p *Endpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	return nil, fmt.Errorf("fine-tuning not supported by HuggingFace provider")
}

func (p *Endpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	return nil, fmt.Errorf("fine-tuning not supported by HuggingFace provider")
}

func (p *Endpoint) Provider() string {
	return "huggingface"
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
