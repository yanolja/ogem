package openai

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/yanolja/ogem/openai"
)

const REGION = "openai"

const (
	BatchJobStatusPending   BatchJobStatus = "pending"
	BatchJobStatusCompleted BatchJobStatus = "completed"
	BatchJobStatusFailed    BatchJobStatus = "failed"
)

const (
	ChatCompletionMethod BatchJobMethod = "POST"
)

type (
	BatchJobStatus string
	BatchJobMethod string
)

type BatchJob struct {
	Id           string                         `json:"custom_id"`
	Method       BatchJobMethod                 `json:"method"`
	Url          string                         `json:"url"`
	Body         *openai.ChatCompletionRequest  `json:"body"`
	Status       BatchJobStatus                 `json:"-"`
	Result       *openai.ChatCompletionResponse `json:"-"`
	Error        error                          `json:"-"`
	Waiters      []chan struct{}                `json:"-"`
	BatchId      string                         `json:"-"`
	InputFileId  string                         `json:"-"`
	OutputFileId string                         `json:"-"`
}

type Endpoint struct {
	apiKey       string
	baseUrl      *url.URL
	client       *http.Client
	providerName string
	region       string

	batchJobs       map[string]*BatchJob
	batchJobMutex   sync.RWMutex
	batchChan       chan *BatchJob
	stopBatchSignal chan struct{}
}

func NewEndpoint(providerName string, region string, baseUrl string, apiKey string) (*Endpoint, error) {
	parsedBaseUrl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint: %v", err)
	}
	
	// Validate URL has a scheme
	if parsedBaseUrl.Scheme == "" || parsedBaseUrl.Host == "" {
		return nil, fmt.Errorf("invalid endpoint: URL must have a scheme and host")
	}
	
	endpoint := &Endpoint{
		providerName:    providerName,
		region:          region,
		apiKey:          apiKey,
		baseUrl:         parsedBaseUrl,
		client:          &http.Client{Timeout: 30 * time.Minute},
		batchJobs:       make(map[string]*BatchJob),
		batchChan:       make(chan *BatchJob),
		stopBatchSignal: make(chan struct{}),
	}

	go endpoint.batchManager()
	return endpoint, nil
}

func (p *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	if batchModel, found := strings.CutSuffix(openaiRequest.Model, "@batch"); found {
		openaiRequest.Model = batchModel
		return p.GenerateBatchChatCompletion(ctx, openaiRequest)
	}

	jsonData, err := json.Marshal(openaiRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "chat/completions")
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
			// Must include `quota` keyword in the error message to disable the provider for a while.
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

		endpointPath, err := url.JoinPath(p.baseUrl.String(), "chat/completions")
		if err != nil {
			errorCh <- fmt.Errorf("failed to build endpoint path: %v", err)
			return
		}

		httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
		if err != nil {
			errorCh <- fmt.Errorf("failed to create request: %v", err)
			return
		}

		httpRequest.Header.Set("Content-Type", "application/json")
		httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)
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
			
			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}

			// Parse SSE format: "data: {...}"
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				
				// Handle termination signal
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
	jsonData, err := json.Marshal(embeddingRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "embeddings")
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

	var embeddingResponse openai.EmbeddingResponse
	if err := json.Unmarshal(body, &embeddingResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &embeddingResponse, nil
}

func (p *Endpoint) GenerateBatchChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	jobId, err := p.createOrGetBatchJob(ctx, openaiRequest)
	if err != nil {
		return nil, err
	}

	job, err := p.waitForBatchJob(ctx, jobId)
	if err != nil {
		return nil, err
	}

	return job.Result, job.Error
}

func (p *Endpoint) createOrGetBatchJob(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (string, error) {
	jobId := generateJobId(openaiRequest)

	p.batchJobMutex.Lock()
	if _, exists := p.batchJobs[jobId]; exists {
		log.Printf("Found existing batch job %v", jobId)
		p.batchJobMutex.Unlock()
		return jobId, nil
	}

	job := &BatchJob{
		Id:      jobId,
		Method:  "POST",
		Url:     "/v1/chat/completions",
		Body:    openaiRequest,
		Status:  BatchJobStatusPending,
		Waiters: []chan struct{}{},
	}

	log.Printf("Creating batch job %v", job)

	p.batchJobs[jobId] = job
	p.batchJobMutex.Unlock()

	// Send the job to the batch manager
	p.batchChan <- job

	return jobId, nil
}

func (p *Endpoint) waitForBatchJob(ctx context.Context, jobId string) (*BatchJob, error) {
	p.batchJobMutex.Lock()
	job, exists := p.batchJobs[jobId]
	if !exists {
		p.batchJobMutex.Unlock()
		return nil, fmt.Errorf("batch job not found")
	}

	if job.Status == BatchJobStatusCompleted || job.Status == BatchJobStatusFailed {
		p.batchJobMutex.Unlock()
		return job, job.Error
	}

	// Job is pending; wait for it to complete
	waiter := make(chan struct{})
	job.Waiters = append(job.Waiters, waiter)
	p.batchJobMutex.Unlock()

	select {
	case <-ctx.Done():
		log.Printf("Context cancelled while waiting for batch job %v", jobId)
		return nil, ctx.Err()
	case <-waiter:
		log.Printf("Batch job %v completed", jobId)
		// Job completed; return the result
		p.batchJobMutex.Lock()
		defer p.batchJobMutex.Unlock()
		return job, job.Error
	}
}

func (p *Endpoint) batchManager() {
	var batch []*BatchJob
	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	for {
		select {
		case job := <-p.batchChan:
			log.Printf("Received batch job %v", job)
			batch = append(batch, job)
			if len(batch) == 50000 {
				p.sendBatch(batch)
				continue
			}

			// Reset the timer
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(10 * time.Second)
		case <-timer.C:
			log.Printf("Sending batch of %d jobs", len(batch))
			// Send the batch using OpenAI Batch API
			p.sendBatch(batch)
			batch = nil
		case <-p.stopBatchSignal:
			log.Printf("Stopping batch manager")
			if len(batch) > 0 {
				p.sendBatch(batch)
			}
			if !timer.Stop() {
				<-timer.C
			}
			return
		}
	}
}

func (p *Endpoint) sendBatch(batch []*BatchJob) {
	if len(batch) == 0 {
		return
	}

	log.Printf("Sending batch of %d jobs", len(batch))

	// Step 1: Create JSONL content in memory
	buffer, fileName, err := p.createBatchFileInMemory(batch)
	if err != nil {
		p.handleBatchError(batch, err)
		return
	}

	log.Printf("Uploading batch file %s", fileName)

	// Step 2: Upload the file to OpenAI
	inputFileId, err := p.uploadFileFromBuffer(buffer, fileName, "batch")
	if err != nil {
		p.handleBatchError(batch, err)
		return
	}

	log.Printf("Uploaded batch file %s with ID %s", fileName, inputFileId)

	// Step 3: Create a batch job
	batchId, err := p.createBatchJob(inputFileId)
	if err != nil {
		p.handleBatchError(batch, err)
		return
	}

	log.Printf("Created batch job %s", batchId)

	// Assign BatchId and InputFileId to each job
	for _, job := range batch {
		job.BatchId = batchId
		job.InputFileId = inputFileId
	}

	log.Printf("Assigned batch ID %s to %d jobs", batchId, len(batch))

	// Step 4: Monitor the batch job until completion
	go p.monitorBatchJob(batchId, batch)
}

func (p *Endpoint) createBatchFileInMemory(batch []*BatchJob) (*bytes.Buffer, string, error) {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	for _, job := range batch {
		if err := encoder.Encode(job); err != nil {
			return nil, "", err
		}
	}
	// Since we need to specify a filename when uploading, we'll just use a placeholder
	fileName := fmt.Sprintf("ogem_openai_batch_%d.jsonl", time.Now().UnixNano())
	return buffer, fileName, nil
}

func (p *Endpoint) uploadFileFromBuffer(buffer *bytes.Buffer, fileName string, purpose string) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add 'purpose' field
	_ = writer.WriteField("purpose", purpose)

	// Add 'file' field
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(part, buffer)
	if err != nil {
		return "", err
	}
	writer.Close()

	req, err := http.NewRequest("POST", p.baseUrl.String()+"/v1/files", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("file upload failed: %s", respBody)
	}

	var fileResponse struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&fileResponse); err != nil {
		return "", err
	}

	return fileResponse.ID, nil
}

func (p *Endpoint) createBatchJob(inputFileId string) (string, error) {
	requestBody := map[string]any{
		"input_file_id":     inputFileId,
		"endpoint":          "/v1/chat/completions",
		"completion_window": "24h",
	}
	bodyBytes, _ := json.Marshal(requestBody)

	req, err := http.NewRequest("POST", p.baseUrl.String()+"/v1/batches", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("batch creation failed: %s", respBody)
	}

	var batchResponse struct {
		Id string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&batchResponse); err != nil {
		return "", err
	}

	log.Printf("Created batch job %s", batchResponse.Id)
	return batchResponse.Id, nil
}

func (p *Endpoint) monitorBatchJob(batchId string, batch []*BatchJob) {
	initialDelay := 10 * time.Second
	maxDelay := 24 * time.Hour
	delay := initialDelay

	for {
		select {
		case <-time.After(delay):
			status, outputFileId, err := p.checkBatchStatus(batchId)
			log.Printf("Batch %s status: %s", batchId, status)
			if err != nil {
				p.handleBatchError(batch, err)
				return
			}

			if status == BatchJobStatusCompleted {
				log.Printf("Batch %s completed", batchId)
				// Retrieve results
				err = p.retrieveBatchResults(batch, outputFileId)
				if err != nil {
					p.handleBatchError(batch, err)
				}
				return
			} else if status == BatchJobStatusFailed {
				p.handleBatchError(batch, fmt.Errorf("batch %s failed", batchId))
				return
			}

			// Exponential backoff
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

func (p *Endpoint) checkBatchStatus(batchID string) (status BatchJobStatus, outputFileId string, err error) {
	req, err := http.NewRequest("GET", p.baseUrl.String()+"/v1/batches/"+batchID, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("batch status check failed: %s", respBody)
	}

	var batchStatusResponse struct {
		Status       BatchJobStatus `json:"status"`
		OutputFileId string         `json:"output_file_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&batchStatusResponse); err != nil {
		return "", "", err
	}

	return batchStatusResponse.Status, batchStatusResponse.OutputFileId, nil
}

func (p *Endpoint) retrieveBatchResults(batch []*BatchJob, outputFileID string) error {
	// Step 1: Download the output file content into memory
	content, err := p.downloadFileContent(outputFileID)
	if err != nil {
		return err
	}

	// Step 2: Read the results and distribute them to jobs
	reader := bytes.NewReader(content)
	decoder := json.NewDecoder(reader)
	index := 0
	for decoder.More() {
		var response openai.ChatCompletionResponse
		if err := decoder.Decode(&response); err != nil {
			return err
		}

		job := batch[index]
		p.batchJobMutex.Lock()
		job.Status = "completed"
		job.Result = &response
		for _, waiter := range job.Waiters {
			close(waiter)
		}
		job.Waiters = nil
		p.batchJobMutex.Unlock()

		index++
	}

	return nil
}

func (p *Endpoint) downloadFileContent(fileID string) ([]byte, error) {
	req, err := http.NewRequest("GET", p.baseUrl.String()+"/v1/files/"+fileID+"/content", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("file download failed: %s", respBody)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (p *Endpoint) handleBatchError(batch []*BatchJob, err error) {
	p.batchJobMutex.Lock()
	defer p.batchJobMutex.Unlock()

	for _, job := range batch {
		if job == nil {
			continue
		}
		log.Printf("Batch job %s failed: %v", job.Id, err)
		job.Status = BatchJobStatusFailed
		job.Error = err
		for _, waiter := range job.Waiters {
			close(waiter)
		}
		job.Waiters = nil
	}
}

func (p *Endpoint) GenerateImage(ctx context.Context, imageRequest *openai.ImageGenerationRequest) (*openai.ImageGenerationResponse, error) {
	jsonData, err := json.Marshal(imageRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "images/generations")
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

	var imageResponse openai.ImageGenerationResponse
	if err := json.Unmarshal(body, &imageResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %v", err)
	}

	return &imageResponse, nil
}

func (p *Endpoint) TranscribeAudio(ctx context.Context, request *openai.AudioTranscriptionRequest) (*openai.AudioTranscriptionResponse, error) {
	// Create multipart form data for file upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file field
	part, err := writer.CreateFormFile("file", request.File)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}
	// Note: In a real implementation, you'd read the file content
	// For now, we'll just write the filename as content
	_, err = part.Write([]byte(request.File))
	if err != nil {
		return nil, fmt.Errorf("failed to write file content: %v", err)
	}

	// Add other fields
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

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "audio/transcriptions")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

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
	// Create multipart form data for file upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file field
	part, err := writer.CreateFormFile("file", request.File)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %v", err)
	}
	// Note: In a real implementation, you'd read the file content
	_, err = part.Write([]byte(request.File))
	if err != nil {
		return nil, fmt.Errorf("failed to write file content: %v", err)
	}

	// Add other fields
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

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "audio/translations")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", writer.FormDataContentType())
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

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

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "audio/speech")
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

	if httpResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResponse.Body)
		if httpResponse.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("quota exceeded: %s", string(body))
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", httpResponse.StatusCode, string(body))
	}

	// Read the raw audio data
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

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "moderations")
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

	endpointPath, err := url.JoinPath(p.baseUrl.String(), "fine_tuning/jobs")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

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
	endpointPath, err := url.JoinPath(p.baseUrl.String(), "fine_tuning/jobs", jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "GET", endpointPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

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
	endpointPath, err := url.JoinPath(p.baseUrl.String(), "fine_tuning/jobs")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

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

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

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
	endpointPath, err := url.JoinPath(p.baseUrl.String(), "fine_tuning/jobs", jobID, "cancel")
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint path: %v", err)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", endpointPath, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	httpRequest.Header.Set("Authorization", "Bearer "+p.apiKey)

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
	return p.providerName
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
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer resp.Body.Close()
	return time.Since(start), nil
}

func (p *Endpoint) Shutdown() error {
	close(p.stopBatchSignal)
	close(p.batchChan)
	return nil
}

func generateJobId(openAiRequest *openai.ChatCompletionRequest) string {
	h := sha256.New()
	json.NewEncoder(h).Encode(openAiRequest)
	return "ogem-" + hex.EncodeToString(h.Sum(nil))
}
