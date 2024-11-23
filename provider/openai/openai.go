package openai

import (
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
	apiKey        string
	client        *http.Client
	batchJobs     map[string]*BatchJob
	batchJobMutex sync.RWMutex

	batchChan       chan *BatchJob
	stopBatchSignal chan struct{}
}

func NewEndpoint(apiKey string) *Endpoint {
	endpoint := &Endpoint{
		apiKey:          apiKey,
		client:          &http.Client{Timeout: 30 * time.Minute},
		batchJobs:       make(map[string]*BatchJob),
		batchChan:       make(chan *BatchJob),
		stopBatchSignal: make(chan struct{}),
	}

	go endpoint.batchManager()
	return endpoint
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

	httpRequest, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/chat/completions", strings.NewReader(string(jsonData)))
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

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/files", body)
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

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/batches", bytes.NewReader(bodyBytes))
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
	req, err := http.NewRequest("GET", "https://api.openai.com/v1/batches/"+batchID, nil)
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
	req, err := http.NewRequest("GET", "https://api.openai.com/v1/files/"+fileID+"/content", nil)
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
		log.Printf("Batch job %s failed: %v", job.Id, err)
		job.Status = "failed"
		job.Error = err
		for _, waiter := range job.Waiters {
			close(waiter)
		}
		job.Waiters = nil
	}
}

func (p *Endpoint) Provider() string {
	return "openai"
}

func (p *Endpoint) Region() string {
	return REGION
}

func (p *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	return 0, nil
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
