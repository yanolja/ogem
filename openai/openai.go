package openai

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/yanolja/ogem/utils/orderedmap"
)

// TODO(seungduk): Auto-generate this file from the OpenAI API reference.

type ChatCompletionRequest struct {
	Messages            []Message             `json:"messages"`
	Model               string                `json:"model"`
	FrequencyPenalty    *float32              `json:"frequency_penalty,omitempty"`
	LogitBias           map[string]float32    `json:"logit_bias,omitempty"`
	Logprobs            *bool                 `json:"logprobs,omitempty"`
	TopLogprobs         *int32                `json:"top_logprobs,omitempty"`
	MaxTokens           *int32                `json:"max_tokens,omitempty"`
	MaxCompletionTokens *int32                `json:"max_completion_tokens,omitempty"`
	CandidateCount      *int32                `json:"n,omitempty"`
	PresencePenalty     *float32              `json:"presence_penalty,omitempty"`
	ResponseFormat      *ResponseFormat       `json:"response_format,omitempty"`
	Seed                *int32                `json:"seed,omitempty"`
	ServiceTier         *string               `json:"service_tier,omitempty"`
	StopSequences       *StopSequences        `json:"stop,omitempty"`
	Stream              *bool                 `json:"stream,omitempty"`
	StreamOptions       *StreamOptions        `json:"stream_options,omitempty"`
	Temperature         *float32              `json:"temperature,omitempty"`
	TopP                *float32              `json:"top_p,omitempty"`
	Tools               []Tool                `json:"tools,omitempty"`
	ToolChoice          *ToolChoice           `json:"tool_choice,omitempty"`
	ParallelToolCalls   *bool                 `json:"parallel_tool_calls,omitempty"`
	User                *string               `json:"user,omitempty"`
	FunctionCall        *LegacyFunctionChoice `json:"function_call,omitempty"`
	Functions           []LegacyFunction      `json:"functions,omitempty"`
}

type StopSequences struct {
	Sequences []string `json:"tokens"`
}

func (ss *StopSequences) MarshalJSON() ([]byte, error) {
	return json.Marshal(ss.Sequences)
}

func (ss *StopSequences) UnmarshalJSON(data []byte) error {
	var sequences []string
	if err := json.Unmarshal(data, &sequences); err == nil {
		ss.Sequences = sequences
		return nil
	}

	var sequence string
	if err := json.Unmarshal(data, &sequence); err == nil {
		ss.Sequences = []string{sequence}
		return nil
	}
	return json.Unmarshal(data, &ss.Sequences)
}

type Tool struct {
	Type     string       `json:"type"`
	Function FunctionTool `json:"function"`
}

type ToolChoice struct {
	Value  *ToolChoiceValue
	Struct *ToolChoiceStruct
}

type ToolChoiceValue string

const (
	ToolChoiceUnspecified ToolChoiceValue = ""
	ToolChoiceNone        ToolChoiceValue = "none"
	ToolChoiceAuto        ToolChoiceValue = "auto"
	ToolChoiceRequired    ToolChoiceValue = "required"
)

type ToolChoiceStruct struct {
	Type     string    `json:"type"`
	Function *Function `json:"function,omitempty"`
}

func (tc *ToolChoice) MarshalJSON() ([]byte, error) {
	if tc.Value != nil {
		return json.Marshal(tc.Value)
	}
	if tc.Struct != nil {
		return json.Marshal(tc.Struct)
	}
	return nil, nil
}

func (tc *ToolChoice) UnmarshalJSON(data []byte) error {
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		choiceValue := ToolChoiceValue(stringValue)
		tc.Value = &choiceValue
		return nil
	}

	var objectValue ToolChoiceStruct
	if err := json.Unmarshal(data, &objectValue); err == nil {
		tc.Struct = &objectValue
		return nil
	}
	return json.Unmarshal(data, &tc.Value)
}

type LegacyFunction struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Parameters  *orderedmap.Map `json:"parameters,omitempty"`
}

type Function struct {
	Name string `json:"name"`
}

type LegacyFunctionChoice struct {
	Value    *string
	Function *Function
}

func (fc *LegacyFunctionChoice) MarshalJSON() ([]byte, error) {
	if fc.Value != nil {
		return json.Marshal(fc.Value)
	}
	if fc.Function != nil {
		return json.Marshal(fc.Function)
	}
	return nil, nil
}

func (fc *LegacyFunctionChoice) UnmarshalJSON(data []byte) error {
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		fc.Value = &stringValue
		return nil
	}

	var objectValue Function
	if err := json.Unmarshal(data, &objectValue); err == nil {
		fc.Function = &objectValue
		return nil
	}
	return json.Unmarshal(data, &fc.Value)
}

type FunctionTool struct {
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Parameters  *orderedmap.Map `json:"parameters,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

type Message struct {
	Role string `json:"role"`
	// When the role is "tool" or "function", the content must be a JSON string.
	Content      *MessageContent `json:"content"`
	Name         *string         `json:"name,omitempty"`
	Refusal      *string         `json:"refusal,omitempty"`
	ToolCalls    []ToolCall      `json:"tool_calls,omitempty"`
	ToolCallId   *string         `json:"tool_call_id,omitempty"`
	FunctionCall *FunctionCall   `json:"function_call,omitempty"`
}

type MessageContent struct {
	String *string
	Parts  []Part
}

func (sop *MessageContent) MarshalJSON() ([]byte, error) {
	if sop.String != nil {
		return json.Marshal(sop.String)
	}
	return json.Marshal(sop.Parts)
}

func (sop *MessageContent) UnmarshalJSON(data []byte) error {
	var stringValue string
	if err := json.Unmarshal(data, &stringValue); err == nil {
		sop.String = &stringValue
		return nil
	}
	var parts []Part
	if err := json.Unmarshal(data, &parts); err == nil {
		sop.Parts = parts
		return nil
	}
	return fmt.Errorf("expected string or parts, got %s", data)
}

type Part struct {
	Type    string  `json:"type"`
	Content Content `json:"content"`
}

type Content struct {
	TextContent  *TextContent
	ImageContent *ImageContent
}

func (p *Content) MarshalJSON() ([]byte, error) {
	if p.TextContent != nil {
		return json.Marshal(p.TextContent)
	}
	return json.Marshal(p.ImageContent)
}

func (p *Content) UnmarshalJSON(data []byte) error {
	var text TextContent
	if err := json.Unmarshal(data, &text); err == nil {
		p.TextContent = &text
		return nil
	}
	var image ImageContent
	if err := json.Unmarshal(data, &image); err == nil {
		p.ImageContent = &image
		return nil
	}
	return fmt.Errorf("expected text or image content, got %s", data)
}

type TextContent struct {
	Text string `json:"text"`
}

type ImageContent struct {
	Url    string `json:"url"`
	Detail string `json:"detail"`
}

type ToolCall struct {
	Id       string        `json:"id"`
	Type     string        `json:"type"`
	Function *FunctionCall `json:"function,omitempty"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ResponseFormat struct {
	Type       string      `json:"type"`
	JsonSchema *JsonSchema `json:"json_schema,omitempty"`
}

type JsonSchema struct {
	Description *string         `json:"description,omitempty"`
	Name        string          `json:"name"`
	Schema      *orderedmap.Map `json:"schema,omitempty"`
	Strict      *bool           `json:"strict,omitempty"`
}

type ChatCompletionResponse struct {
	Id                string   `json:"id"`
	Choices           []Choice `json:"choices"`
	Created           int64    `json:"created"`
	Model             string   `json:"model"`
	ServiceTier       *string  `json:"service_tier,omitempty"`
	SystemFingerprint string   `json:"system_fingerprint"`
	Object            string   `json:"object"`
	Usage             Usage    `json:"usage"`
}

type Choice struct {
	Index        int32     `json:"index"`
	Message      Message   `json:"message"`
	Logprobs     *Logprobs `json:"logprobs"`
	FinishReason string    `json:"finish_reason"`
}

type Logprobs struct {
	Content []Logprob `json:"content,omitempty"`
	Refusal []Logprob `json:"refusal,omitempty"`
}

type Logprob struct {
	Token       string       `json:"token"`
	Logprob     float32      `json:"logprob"`
	Bytes       []byte       `json:"bytes,omitempty"`
	TopLogprobs []TopLogprob `json:"top_logprobs,omitempty"`
}

type TopLogprob struct {
	Token   string  `json:"token"`
	Logprob float32 `json:"logprob"`
	Bytes   []byte  `json:"bytes,omitempty"`
}

type Usage struct {
	PromptTokens            int32                   `json:"prompt_tokens"`
	CompletionTokens        int32                   `json:"completion_tokens"`
	TotalTokens             int32                   `json:"total_tokens"`
	CompletionTokensDetails CompletionTokensDetails `json:"completion_tokens_details"`
}

type CompletionTokensDetails struct {
	ReasoningTokens int32 `json:"reasoning_tokens"`
}

type StreamOptions struct {
	IncludeUsage *bool `json:"include_usage,omitempty"`
}

type ChatCompletionStreamResponse struct {
	Id                string               `json:"id"`
	Object            string               `json:"object"`
	Created           int64                `json:"created"`
	Model             string               `json:"model"`
	ServiceTier       *string              `json:"service_tier,omitempty"`
	SystemFingerprint string               `json:"system_fingerprint"`
	Choices           []ChoiceDelta        `json:"choices"`
	Usage             *Usage               `json:"usage,omitempty"`
}

type ChoiceDelta struct {
	Index        int32         `json:"index"`
	Delta        MessageDelta  `json:"delta"`
	Logprobs     *Logprobs     `json:"logprobs,omitempty"`
	FinishReason *string       `json:"finish_reason"`
}

type MessageDelta struct {
	Role         *string         `json:"role,omitempty"`
	Content      *string         `json:"content,omitempty"`
	Refusal      *string         `json:"refusal,omitempty"`
	ToolCalls    []ToolCallDelta `json:"tool_calls,omitempty"`
	FunctionCall *FunctionCall   `json:"function_call,omitempty"`
}

type ToolCallDelta struct {
	Index    *int32        `json:"index,omitempty"`
	Id       *string       `json:"id,omitempty"`
	Type     *string       `json:"type,omitempty"`
	Function *FunctionCall `json:"function,omitempty"`
}

func FinalizeResponse(provider string, region string, model string, response *ChatCompletionResponse) *ChatCompletionResponse {
	response.Id = "chatcmpl-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	response.Created = time.Now().Unix()
	response.Model = model
	response.ServiceTier = nil
	response.SystemFingerprint = fmt.Sprintf("open-gemini/%s/%s/%s", provider, region, model)
	response.Object = "chat.completion"
	return response
}

func FinalizeStreamResponse(provider string, region string, model string, response *ChatCompletionStreamResponse) *ChatCompletionStreamResponse {
	if response.Id == "" {
		response.Id = "chatcmpl-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	}
	if response.Created == 0 {
		response.Created = time.Now().Unix()
	}
	response.Model = model
	response.ServiceTier = nil
	response.SystemFingerprint = fmt.Sprintf("open-gemini/%s/%s/%s", provider, region, model)
	response.Object = "chat.completion.chunk"
	return response
}

type EmbeddingRequest struct {
	Input          []string  `json:"input"`
	Model          string    `json:"model"`
	EncodingFormat *string   `json:"encoding_format,omitempty"`
	Dimensions     *int32    `json:"dimensions,omitempty"`
	User           *string   `json:"user,omitempty"`
}

type EmbeddingResponse struct {
	Object string            `json:"object"`
	Data   []EmbeddingObject `json:"data"`
	Model  string            `json:"model"`
	Usage  EmbeddingUsage    `json:"usage"`
}

type EmbeddingObject struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int32     `json:"index"`
}

type EmbeddingUsage struct {
	PromptTokens int32 `json:"prompt_tokens"`
	TotalTokens  int32 `json:"total_tokens"`
}

type ImageGenerationRequest struct {
	Prompt         string  `json:"prompt"`
	Model          *string `json:"model,omitempty"`
	N              *int32  `json:"n,omitempty"`
	Quality        *string `json:"quality,omitempty"`
	ResponseFormat *string `json:"response_format,omitempty"`
	Size           *string `json:"size,omitempty"`
	Style          *string `json:"style,omitempty"`
	User           *string `json:"user,omitempty"`
}

type ImageGenerationResponse struct {
	Created int64       `json:"created"`
	Data    []ImageData `json:"data"`
}

type ImageData struct {
	URL           *string `json:"url,omitempty"`
	B64JSON       *string `json:"b64_json,omitempty"`
	RevisedPrompt *string `json:"revised_prompt,omitempty"`
}

type AudioTranscriptionRequest struct {
	File           string  `json:"file"`
	Model          string  `json:"model"`
	Language       *string `json:"language,omitempty"`
	Prompt         *string `json:"prompt,omitempty"`
	ResponseFormat *string `json:"response_format,omitempty"`
	Temperature    *float32 `json:"temperature,omitempty"`
}

type AudioTranscriptionResponse struct {
	Text string `json:"text"`
}

type AudioTranslationRequest struct {
	File           string  `json:"file"`
	Model          string  `json:"model"`
	Prompt         *string `json:"prompt,omitempty"`
	ResponseFormat *string `json:"response_format,omitempty"`
	Temperature    *float32 `json:"temperature,omitempty"`
}

type AudioTranslationResponse struct {
	Text string `json:"text"`
}

type TextToSpeechRequest struct {
	Model          string  `json:"model"`
	Input          string  `json:"input"`
	Voice          string  `json:"voice"`
	ResponseFormat *string `json:"response_format,omitempty"`
	Speed          *float32 `json:"speed,omitempty"`
}

type TextToSpeechResponse struct {
	Data []byte `json:"-"` // Raw audio data
}

type ModerationRequest struct {
	Input []string `json:"input"`
	Model *string  `json:"model,omitempty"`
}

type ModerationResponse struct {
	ID      string             `json:"id"`
	Model   string             `json:"model"`
	Results []ModerationResult `json:"results"`
}

type ModerationResult struct {
	Categories     ModerationCategories     `json:"categories"`
	CategoryScores ModerationCategoryScores `json:"category_scores"`
	Flagged        bool                     `json:"flagged"`
}

type ModerationCategories struct {
	Sexual                bool `json:"sexual"`
	Hate                  bool `json:"hate"`
	Harassment            bool `json:"harassment"`
	SelfHarm              bool `json:"self-harm"`
	SexualMinors          bool `json:"sexual/minors"`
	HateThreatening       bool `json:"hate/threatening"`
	ViolenceGraphic       bool `json:"violence/graphic"`
	SelfHarmIntent        bool `json:"self-harm/intent"`
	SelfHarmInstructions  bool `json:"self-harm/instructions"`
	HarassmentThreatening bool `json:"harassment/threatening"`
	Violence              bool `json:"violence"`
}

type ModerationCategoryScores struct {
	Sexual                float32 `json:"sexual"`
	Hate                  float32 `json:"hate"`
	Harassment            float32 `json:"harassment"`
	SelfHarm              float32 `json:"self-harm"`
	SexualMinors          float32 `json:"sexual/minors"`
	HateThreatening       float32 `json:"hate/threatening"`
	ViolenceGraphic       float32 `json:"violence/graphic"`
	SelfHarmIntent        float32 `json:"self-harm/intent"`
	SelfHarmInstructions  float32 `json:"self-harm/instructions"`
	HarassmentThreatening float32 `json:"harassment/threatening"`
	Violence              float32 `json:"violence"`
}

// Fine-tuning types
type FineTuningJobRequest struct {
	TrainingFile                  string                         `json:"training_file"`
	ValidationFile                *string                        `json:"validation_file,omitempty"`
	Model                         string                         `json:"model"`
	Hyperparameters              *FineTuningHyperparameters     `json:"hyperparameters,omitempty"`
	Suffix                        *string                        `json:"suffix,omitempty"`
	Integrations                  []FineTuningIntegration        `json:"integrations,omitempty"`
	Seed                          *int32                         `json:"seed,omitempty"`
}

type FineTuningHyperparameters struct {
	BatchSize              *string `json:"batch_size,omitempty"`
	LearningRateMultiplier *string `json:"learning_rate_multiplier,omitempty"`
	NEpochs                *string `json:"n_epochs,omitempty"`
}

type FineTuningIntegration struct {
	Type           string                       `json:"type"`
	Wandb          *FineTuningWandbIntegration  `json:"wandb,omitempty"`
}

type FineTuningWandbIntegration struct {
	Project *string   `json:"project,omitempty"`
	Name    *string   `json:"name,omitempty"`
	Entity  *string   `json:"entity,omitempty"`
	Tags    []string  `json:"tags,omitempty"`
}

type FineTuningJob struct {
	ID               string                     `json:"id"`
	Object           string                     `json:"object"`
	CreatedAt        int64                      `json:"created_at"`
	FinishedAt       *int64                     `json:"finished_at"`
	Model            string                     `json:"model"`
	FineTunedModel   *string                    `json:"fine_tuned_model"`
	OrganizationID   string                     `json:"organization_id"`
	Status           string                     `json:"status"`
	Hyperparameters  FineTuningHyperparameters  `json:"hyperparameters"`
	TrainingFile     string                     `json:"training_file"`
	ValidationFile   *string                    `json:"validation_file"`
	ResultFiles      []string                   `json:"result_files"`
	TrainedTokens    *int32                     `json:"trained_tokens"`
	Integrations     []FineTuningIntegration    `json:"integrations,omitempty"`
	Seed             *int32                     `json:"seed"`
	EstimatedFinish  *int64                     `json:"estimated_finish"`
	Error            *FineTuningError           `json:"error,omitempty"`
}

type FineTuningError struct {
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	Param      *string `json:"param,omitempty"`
}

type FineTuningJobList struct {
	Object  string          `json:"object"`
	Data    []FineTuningJob `json:"data"`
	HasMore bool            `json:"has_more"`
}

type FineTuningJobEvent struct {
	ID        string                  `json:"id"`
	Object    string                  `json:"object"`
	CreatedAt int64                   `json:"created_at"`
	Level     string                  `json:"level"`
	Message   string                  `json:"message"`
	Data      *FineTuningJobEventData `json:"data,omitempty"`
	Type      string                  `json:"type"`
}

type FineTuningJobEventData struct {
	Step    *int32   `json:"step,omitempty"`
	Metrics *map[string]interface{} `json:"metrics,omitempty"`
}

type FineTuningJobEventList struct {
	Object  string               `json:"object"`
	Data    []FineTuningJobEvent `json:"data"`
	HasMore bool                 `json:"has_more"`
}

type FineTuningJobCheckpoint struct {
	ID              string                       `json:"id"`
	Object          string                       `json:"object"`
	CreatedAt       int64                        `json:"created_at"`
	FineTunedModel  string                       `json:"fine_tuned_model"`
	FineTuningJobID string                       `json:"fine_tuning_job_id"`
	Metrics         FineTuningJobCheckpointMetrics `json:"metrics"`
	StepNumber      int32                        `json:"step_number"`
}

type FineTuningJobCheckpointMetrics struct {
	Step                   *int32   `json:"step,omitempty"`
	TrainLoss              *float32 `json:"train_loss,omitempty"`
	TrainMeanTokenAccuracy *float32 `json:"train_mean_token_accuracy,omitempty"`
	ValidLoss              *float32 `json:"valid_loss,omitempty"`
	ValidMeanTokenAccuracy *float32 `json:"valid_mean_token_accuracy,omitempty"`
	FullValidLoss          *float32 `json:"full_valid_loss,omitempty"`
	FullValidMeanTokenAccuracy *float32 `json:"full_valid_mean_token_accuracy,omitempty"`
}

type FineTuningJobCheckpointList struct {
	Object  string                    `json:"object"`
	Data    []FineTuningJobCheckpoint `json:"data"`
	HasMore bool                      `json:"has_more"`
	FirstID *string                   `json:"first_id,omitempty"`
	LastID  *string                   `json:"last_id,omitempty"`
}
