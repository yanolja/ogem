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

// Reference: https://platform.openai.com/docs/api-reference/chat/create
type ChatCompletionRequest struct {
	// A list of messages comprising the conversation so far. Depending on the
	// [model](https://platform.openai.com/docs/models) you use, different message
	// types (modalities) are supported, like [text], [images], and [audio].
	Messages []Message `json:"messages"`

	Model string `json:"model"`

	// Should be between -2.0 and 2.0
	FrequencyPenalty *float32 `json:"frequency_penalty,omitempty"`

	// A JSON object that maps tokens (specified by their token ID in the
	// tokenizer) to an associated bias value from -100 to 100.
	// Values between -1 and 1 should decrease or increase likelihood of selection;
	// Values like -100 or 100 should result in a ban or exclusive selection of the relevant token.
	LogitBias map[string]float32 `json:"logit_bias,omitempty"`

	// If true, returns the log probabilities of each output token returned in the `content` of `message`.
	Logprobs *bool `json:"logprobs,omitempty"`

	// An integer between 0 and 20 specifying the number of most likely tokens to
	// return at each token position, `logprobs` must be set to `true` if this parameter is used.
	TopLogprobs *int32 `json:"top_logprobs,omitempty"`

	// Deprecated: This value is now deprecated in favor of max_completion_tokens,
	// Not compatible with o1 series models
	MaxTokens *int32 `json:"max_tokens,omitempty"`

	// An upper bound for the number of tokens that can be generated for a completion,
	// including visible output tokens and [reasoning tokens](https://platform.openai.com/docs/guides/reasoning).
	MaxCompletionTokens *int32 `json:"max_completion_tokens,omitempty"`

	// How many chat completion choices to generate for each input message.
	CandidateCount *int32 `json:"n,omitempty"`

	// Number between -2.0 and 2.0. Positive values penalize new tokens based on
	// whether they appear in the text so far, increasing the model's likelihood to talk about new topics.
	PresencePenalty *float32 `json:"presence_penalty,omitempty"`

	// An object specifying the format that the model must output.
	// Setting to `{ "type": "json_schema", "json_schema": {...} }` enables Structured
	// Outputs which ensures the model will match your supplied JSON schema.
	// Setting to `{ "type": "json_object" }` enables the older JSON mode, which ensures the message
	// is valid JSON. Using `json_schema` is preferred for models that support it.
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`

	Seed *int32 `json:"seed,omitempty"`

	// Specifies the latency tier to use for processing the request. This parameter is
	// relevant for customers subscribed to the scale tier service:
	// - If set to 'auto', and the Project is Scale tier enabled, the system will utilize scale tier credits until they are exhausted.
	// - If set to 'auto', and the Project is not Scale tier enabled, the request will be processed using the default service tier
	//   with a lower uptime SLA and no latency guarentee.
	// - If set to 'default', the request will be processed using the default service tier
	//   with a lower uptime SLA and no latency guarentee.
	// - When not set, the default behavior is 'auto'.
	// When this parameter is set, the response body will include the `service_tier` utilized.
	ServiceTier *string `json:"service_tier,omitempty"`

	// Up to 4 sequences where the API will stop generating further tokens. The returned text will not contain the stop sequence.
	StopSequences *StopSequences `json:"stop,omitempty"`

	// If set to true, the model response data will be streamed to the client as it is generated using server-sent events.
	Stream *bool `json:"stream,omitempty"`

	// Options for streaming response. Only set this when you set `stream: true`.
	StreamOptions *StreamOptions `json:"stream_options,omitempty"`

	// Between 0 and 2. Higher values like 0.8 will make the output more random,
	// while lower values like 0.2 will make it more focused and deterministic.
	// Generally recommend altering this or `top_p` but not both.
	Temperature *float32 `json:"temperature,omitempty"`

	// An alternative to sampling with temperature, the model considers the results of the tokens with top_p probability mass.
	// 0.1 means only the tokens comprising the top 10% probability mass are considered.
	// Generally recommend altering this or `temperature` but not both.
	TopP *float32 `json:"top_p,omitempty"`

	// Currently, only functions are supported as a tool.
	// This provides a list of functions the model may generate JSON inputs for.
	// Max of 128 functions are supported.
	Tools []Tool `json:"tools,omitempty"`

	// Controls which tool is called by the model. `none` means the model will
	// not call any tool and instead generates a message. `auto` means the model can
	// pick between generating a message or calling one or more tools. `required` means
	// the model must call one or more tools.
	// `auto` is the default if tools are present.
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`

	// Whether to enable parallel function calling during tool use.
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`

	// A unique identifier representing your end-user, which can help OpenAI to monitor and detect abuse.
	User *string `json:"user,omitempty"`

	// Deprecated in favor of `tool_choice`.
	FunctionCall *LegacyFunctionChoice `json:"function_call,omitempty"`

	// Deprecated in favor of `tools`.
	Functions []LegacyFunction `json:"functions,omitempty"`

	// Required when audio output is requested with `modalities: ["audio"]`.
	Audio *Audio `json:"audio,omitempty"`

	// Maximum: 16 chars for key, 512 chars for value
	Metadata *orderedmap.Map `json:"metadata,omitempty"`

	// Output types that you would like the model to generate. Default: `["text"]`
	// To request that this model generate both text and audio responses, we can use: `["text", "audio"]`
	Modalities *[]string `json:"modalities,omitempty"`

	Prediction *Prediction `json:"prediction,omitempty"`

	// **o-series models only**
	// Constrains effort on reasoning for [reasoning models](https://platform.openai.com/docs/guides/reasoning).
	// Currently supported values are `low`, `medium`, and `high`.
	// Reducing reasoning effort can result in faster responses and fewer tokens used on reasoning in a response.
	ReasoningEffort *string `json:"reasoning_effort,omitempty"`

	// Whether or not to store the output of this chat completion request for use in
	// [model distillation](https://platform.openai.com/docs/guides/distillation)
	// or [evals](https://platform.openai.com/docs/guides/evals) products.
	Store *bool `json:"store,omitempty"`

	WebSearchOptions *WebSearchOptions `json:"web_search_options,omitempty"`
}

type Prediction struct {
	Content Type `json:"content"`
}

type Type string

type WebSearchOptions struct {
	SearchContextSize *string `json:"search_context_size,omitempty"`
	UserLocation      *string `json:"user_location,omitempty"`
}

type UserLocation struct {
	Type        string       `json:"type"`
	Approximate *Approximate `json:"approximate,omitempty"`
}

type Approximate struct {
	City     *string `json:"city,omitempty"`
	Country  *string `json:"country,omitempty"`
	Region   *string `json:"region,omitempty"`
	Timezone *string `json:"timezone,omitempty"` // Use IANA (Internet Assigned Numbers Authority) timezone of the user
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

type Audio struct {
	Format string `json:"format"` // Must be one of wav, mp3, flac, opus, or pcm16
	Voice  string `json:"voice"`  // Supported voices are alloy, ash, ballad, coral, echo, sage, and shimmer
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
	Annotations  *[]Annotation   `json:"annotations,omitempty"`
	Audio        *AudioResponse  `json:"audio,omitempty"`
}

type Annotation struct {
	Type        string       `json:"type"`
	UrlCitation *UrlCitation `json:"url_citation,omitempty"`
}

type UrlCitation struct {
	EndIndex   int    `json:"end_index"`
	StartIndex int    `json:"start_index"`
	Title      string `json:"title"`
	Url        string `json:"url"`
}

type AudioResponse struct {
	Data       *string `json:"data"`
	ExpiresAt  *int64  `json:"expires_at"`
	Id         *string `json:"id"`
	Transcript *string `json:"transcript"`
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
	PromptTokensDetails     PromptTokensDetails     `json:"prompt_tokens_details"`
}

type CompletionTokensDetails struct {
	ReasoningTokens          int32 `json:"reasoning_tokens"`
	AcceptedPredictionTokens int32 `json:"accepted_prediction_tokens"`
	AudioTokens              int32 `json:"audio_tokens"`
	RejectedPredictionTokens int32 `json:"rejected_prediction_tokens"`
}

type PromptTokensDetails struct {
	AudioTokens  int32 `json:"audio_tokens"`
	CachedTokens int32 `json:"cached_tokens"`
}

type StreamOptions struct {
	IncludeUsage *bool `json:"include_usage,omitempty"`
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
