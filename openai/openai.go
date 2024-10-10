package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

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
	Name        string       `json:"name"`
	Description *string      `json:"description,omitempty"`
	Parameters  *OrderedJson `json:"parameters,omitempty"`
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
	Name        string       `json:"name"`
	Description *string      `json:"description,omitempty"`
	Parameters  *OrderedJson `json:"parameters,omitempty"`
	Strict      *bool        `json:"strict,omitempty"`
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
	Description *string      `json:"description,omitempty"`
	Name        string       `json:"name"`
	Schema      *OrderedJson `json:"schema,omitempty"`
	Strict      *bool        `json:"strict,omitempty"`
}

type OrderedJson struct {
	order []string
	data  map[string]any
}

type NestedOrderedMapEntry struct {
	Key   string
	Value any
}

func NewNestedOrderedMap() *OrderedJson {
	return &OrderedJson{
		order: make([]string, 0),
		data:  make(map[string]any),
	}
}

func (n *OrderedJson) MarshalJSON() ([]byte, error) {
	result := "{"
	for i, key := range n.order {
		if i > 0 {
			result += ","
		}
		quotedKey := strconv.Quote(key)
		result += quotedKey + ":"
		if nested, ok := n.data[key].(*OrderedJson); ok {
			nestedJSON, err := nested.MarshalJSON()
			if err != nil {
				return nil, err
			}
			result += string(nestedJSON)
		} else {
			valueJSON, err := json.Marshal(n.data[key])
			if err != nil {
				return nil, err
			}
			result += string(valueJSON)
		}
	}
	result += "}"
	return []byte(result), nil
}

func (n *OrderedJson) UnmarshalJSON(data []byte) error {
	n.order = make([]string, 0)
	n.data = make(map[string]any)

	jsonDecoder := json.NewDecoder(bytes.NewReader(data))

	// Consume the opening brace.
	if _, err := jsonDecoder.Token(); err != nil {
		return err
	}

	for jsonDecoder.More() {
		keyToken, err := jsonDecoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %T", keyToken)
		}

		var value json.RawMessage
		if err := jsonDecoder.Decode(&value); err != nil {
			return err
		}

		var parsed any
		if err := json.Unmarshal(value, &parsed); err != nil {
			return err
		}

		if _, ok := parsed.(map[string]any); ok {
			nested := NewNestedOrderedMap()
			if err := nested.UnmarshalJSON(value); err != nil {
				return err
			}
			n.Set(key, nested)
		} else {
			n.Set(key, parsed)
		}
	}

	// Consume the closing brace.
	if _, err := jsonDecoder.Token(); err != nil {
		return err
	}

	return nil
}

func (n *OrderedJson) Keys() []string {
	return n.order
}

func (n *OrderedJson) Entries() []NestedOrderedMapEntry {
	entries := make([]NestedOrderedMapEntry, 0, len(n.order))
	for _, key := range n.order {
		entries = append(entries, NestedOrderedMapEntry{
			Key:   key,
			Value: n.data[key],
		})
	}
	return entries
}

func (n *OrderedJson) Set(key string, value any) {
	if _, exists := n.data[key]; !exists {
		n.order = append(n.order, key)
	}
	n.data[key] = value
}

func (n *OrderedJson) Get(key string) (any, bool) {
	value, exists := n.data[key]
	return value, exists
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

func FinalizeResponse(provider string, region string, model string, response *ChatCompletionResponse) *ChatCompletionResponse {
	response.Id = "chatcmpl-" + strings.ReplaceAll(uuid.New().String(), "-", "")
	response.Created = time.Now().Unix()
	response.Model = model
	response.ServiceTier = nil
	response.SystemFingerprint = fmt.Sprintf("open-gemini/%s/%s/%s", provider, region, model)
	response.Object = "chat.completion"
	return response
}
