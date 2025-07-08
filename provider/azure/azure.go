package azure

import (
	"bufio"
	"bytes"
	"context"
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

const REGION = "azure"

var notSupportDimensionModels = []string{
	"text-embedding-ada-002",
}

type Endpoint struct {
	apiKey     string
	baseUrl    *url.URL
	apiVersion string
	client     *http.Client
	region     string
}

func NewEndpoint(region string, baseUrl string, apiKey string, apiVersion string) (*Endpoint, error) {
	parsedBaseUrl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %v", err)
	}

	if apiVersion == "" {
		apiVersion = "2024-02-15-preview" // Default to latest stable version
	}

	endpoint := &Endpoint{
		region:     region,
		apiKey:     apiKey,
		baseUrl:    parsedBaseUrl,
		apiVersion: apiVersion,
		client:     &http.Client{Timeout: 30 * time.Minute},
	}

	return endpoint, nil
}

func (p *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	jsonData, err := json.Marshal(openaiRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Azure uses deployment name instead of model name in URL
	deploymentName := openaiRequest.Model
	endpointPath := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		p.baseUrl.String(), deploymentName, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("api-key", p.apiKey)

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

	var openAiResponse openai.ChatCompletionResponse
	if err := json.Unmarshal(body, &openAiResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &openAiResponse, nil
}

func (p *Endpoint) GenerateChatCompletionStream(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	responseCh := make(chan *openai.ChatCompletionStreamResponse)
	errorCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer close(errorCh)

		// Create a copy of the request and enable streaming
		streamRequest := *openaiRequest
		streamingEnabled := true
		streamRequest.Stream = &streamingEnabled

		jsonData, err := json.Marshal(streamRequest)
		if err != nil {
			errorCh <- fmt.Errorf("failed to marshal request: %v", err)
			return
		}

		deploymentName := openaiRequest.Model
		endpointPath := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
			p.baseUrl.String(), deploymentName, p.apiVersion)

		httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
		if err != nil {
			errorCh <- fmt.Errorf("failed to create request: %v", err)
			return
		}

		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("api-key", p.apiKey)
		httpRequest.Header.Set("Accept", "text/event-stream")

		log.Printf("Sending streaming %s request to %s", httpRequest.Method, endpointPath)

		httpResponse, err := p.client.Do(httpRequest)
		if err != nil {
			errorCh <- fmt.Errorf("failed to send request: %v", err)
			return
		}
		defer httpResponse.Body.Close()

		if httpResponse.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(httpResponse.Body)
			if httpResponse.StatusCode == http.StatusTooManyRequests {
				errorCh <- fmt.Errorf("quota exceeded: %s", string(body))
			} else {
				errorCh <- fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
			}
			return
		}

		scanner := bufio.NewScanner(httpResponse.Body)
		for scanner.Scan() {
			line := scanner.Text()

			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				if data == "[DONE]" {
					break
				}

				var streamResponse openai.ChatCompletionStreamResponse
				if err := json.Unmarshal([]byte(data), &streamResponse); err != nil {
					log.Printf("Failed to parse streaming response: %v, data: %s", err, data)
					continue
				}

				select {
				case responseCh <- &streamResponse:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := scanner.Err(); err != nil {
			errorCh <- fmt.Errorf("error reading stream: %v", err)
		}
	}()

	return responseCh, errorCh
}

func (p *Endpoint) GenerateEmbedding(ctx context.Context, embeddingRequest *openai.EmbeddingRequest) (*openai.EmbeddingResponse, error) {
	supportsDimensions := true
	for _, model := range notSupportDimensionModels {
		if embeddingRequest.Model == model {
			supportsDimensions = false
			break
		}
	}

	requestCopy := *embeddingRequest

	if !supportsDimensions && requestCopy.Dimensions != nil {
		log.Printf("Model %s does not support dimensions parameter, removing it", embeddingRequest.Model)
		requestCopy.Dimensions = nil
	}

	jsonData, err := json.Marshal(requestCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	deploymentName := embeddingRequest.Model
	endpointPath := fmt.Sprintf("%s/openai/deployments/%s/embeddings?api-version=%s",
		p.baseUrl.String(), deploymentName, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("api-key", p.apiKey)

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

	var embeddingResponse openai.EmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &embeddingResponse, nil
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	jsonData, err := json.Marshal(imageRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	deploymentName := "dall-e-3" // Azure typically uses deployment names
	if imageRequest.Model != nil {
		deploymentName = *imageRequest.Model
	}
	endpointPath := fmt.Sprintf("%s/openai/deployments/%s/images/generations?api-version=%s",
		p.baseUrl.String(), deploymentName, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("api-key", p.apiKey)

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

	var imageResponse openai.ImageGenerationResponse
	if err := json.Unmarshal(body, &imageResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &imageResponse, nil
}

func (p *Endpoint) TranscribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", request.File)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = part.Write([]byte(request.File))
	if err != nil {
		return nil, fmt.Errorf("failed to write file content: %v", err)
	}

	writer.WriteField("model", request.Model)
	if request.Language != nil {
		writer.WriteField("language", *request.Language)
	}
	if request.Prompt != nil {
		writer.WriteField("prompt", *request.Prompt)
	}
	if request.ResponseFormat != nil {
		writer.WriteField("response_format", *request.ResponseFormat)
	}
	if request.Temperature != nil {
		writer.WriteField("temperature", fmt.Sprintf("%.2f", *request.Temperature))
	}

	writer.Close()

	deploymentName := request.Model
	endpointPath := fmt.Sprintf("%s/openai/deployments/%s/audio/transcriptions?api-version=%s",
		p.baseUrl.String(), deploymentName, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("api-key", p.apiKey)

	log.Printf("Sending %s request to %s", httpRequest.Method, endpointPath)

	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if httpResponse.StatusCode != http.StatusOK {
		if httpResponse.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("quota exceeded: %s", string(responseBody))
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(responseBody))
	}

	var transcriptionResponse openai.AudioTranscriptionResponse
	if err := json.Unmarshal(responseBody, &transcriptionResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &transcriptionResponse, nil
}

func (p *Endpoint) TranslateAudio(ctx context.Context, request *openai.AudioTranslationRequest) (*openai.AudioTranslationResponse, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", request.File)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}
	_, err = part.Write([]byte(request.File))
	if err != nil {
		return nil, fmt.Errorf("failed to write file content: %v", err)
	}

	writer.WriteField("model", request.Model)
	if request.Prompt != nil {
		writer.WriteField("prompt", *request.Prompt)
	}
	if request.ResponseFormat != nil {
		writer.WriteField("response_format", *request.ResponseFormat)
	}
	if request.Temperature != nil {
		writer.WriteField("temperature", fmt.Sprintf("%.2f", *request.Temperature))
	}

	writer.Close()

	deploymentName := request.Model
	endpointPath := fmt.Sprintf("%s/openai/deployments/%s/audio/translations?api-version=%s",
		p.baseUrl.String(), deploymentName, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("api-key", p.apiKey)

	log.Printf("Sending %s request to %s", httpRequest.Method, endpointPath)

	httpResponse, err := p.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer httpResponse.Body.Close()

	responseBody, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if httpResponse.StatusCode != http.StatusOK {
		if httpResponse.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("quota exceeded: %s", string(responseBody))
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(responseBody))
	}

	var translationResponse openai.AudioTranslationResponse
	if err := json.Unmarshal(responseBody, &translationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &translationResponse, nil
}

func (p *Endpoint) GenerateSpeech(ctx context.Context, request *openai.TextToSpeechRequest) (*openai.TextToSpeechResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	deploymentName := request.Model
	endpointPath := fmt.Sprintf("%s/openai/deployments/%s/audio/speech?api-version=%s",
		p.baseUrl.String(), deploymentName, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("api-key", p.apiKey)

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

	audioData, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read audio data: %v", err)
	}

	return &openai.TextToSpeechResponse{
		Data: audioData,
	}, nil
}

func (p *Endpoint) ModerateContent(ctx context.Context, request *openai.ModerationRequest) (*openai.ModerationResponse, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath := fmt.Sprintf("%s/openai/deployments/text-moderation-latest/moderations?api-version=%s",
		p.baseUrl.String(), p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("api-key", p.apiKey)

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

	var moderationResponse openai.ModerationResponse
	if err := json.Unmarshal(body, &moderationResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &moderationResponse, nil
}

func (p *Endpoint) CreateFineTuningJob(ctx context.Context, request *openai.FineTuningJobRequest) (*openai.FineTuningJob, error) {
	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath := fmt.Sprintf("%s/openai/fine_tuning/jobs?api-version=%s",
		p.baseUrl.String(), p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("api-key", p.apiKey)

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
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	var job openai.FineTuningJob
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &job, nil
}

func (p *Endpoint) GetFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	endpointPath := fmt.Sprintf("%s/openai/fine_tuning/jobs/%s?api-version=%s",
		p.baseUrl.String(), jobID, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", endpointPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("api-key", p.apiKey)

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
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	var job openai.FineTuningJob
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &job, nil
}

func (p *Endpoint) ListFineTuningJobs(ctx context.Context, after *string, limit *int32) (*openai.FineTuningJobList, error) {
	endpointPath := fmt.Sprintf("%s/openai/fine_tuning/jobs?api-version=%s",
		p.baseUrl.String(), p.apiVersion)

	req, err := http.NewRequestWithContext(ctx, "GET", endpointPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	q := req.URL.Query()
	if after != nil {
		q.Add("after", *after)
	}
	if limit != nil {
		q.Add("limit", fmt.Sprintf("%d", *limit))
	}
	req.URL.RawQuery = q.Encode()

	req.Header.Set("api-key", p.apiKey)

	httpResponse, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	if httpResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	var jobList openai.FineTuningJobList
	if err := json.Unmarshal(body, &jobList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &jobList, nil
}

func (p *Endpoint) CancelFineTuningJob(ctx context.Context, jobID string) (*openai.FineTuningJob, error) {
	endpointPath := fmt.Sprintf("%s/openai/fine_tuning/jobs/%s/cancel?api-version=%s",
		p.baseUrl.String(), jobID, p.apiVersion)

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("api-key", p.apiKey)

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
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	var job openai.FineTuningJob
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &job, nil
}

func (p *Endpoint) Provider() string {
	return "azure"
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
	req.Header.Set("api-key", p.apiKey)
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
