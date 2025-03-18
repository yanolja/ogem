package claude

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

func TestToClaudeToolChoice_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    *openai.ToolChoice
		expected anthropic.ToolChoiceUnionParam
	}{
		{
			name:     "returns nil when tool choice is nil",
			input:    nil,
			expected: nil,
		},
		{
			name: "converts 'auto' tool choice to Claude format",
			input: &openai.ToolChoice{
				Value: (*openai.ToolChoiceValue)(utils.ToPtr("auto")),
			},
			expected: anthropic.ToolChoiceAutoParam{
				Type: anthropic.F(anthropic.ToolChoiceAutoTypeAuto),
			},
		},
		{
			name: "converts 'required' tool choice to 'any' for Claude",
			input: &openai.ToolChoice{
				Value: (*openai.ToolChoiceValue)(utils.ToPtr("required")),
			},
			expected: anthropic.ToolChoiceAnyParam{
				Type: anthropic.F(anthropic.ToolChoiceAnyTypeAny),
			},
		},
		{
			name: "converts function tool choice with valid struct to Claude format",
			input: &openai.ToolChoice{
				Value: (*openai.ToolChoiceValue)(utils.ToPtr("function")),
				Struct: &openai.ToolChoiceStruct{
					Type: "function",
					Function: &openai.Function{
						Name: "get_weather",
					},
				},
			},
			expected: anthropic.ToolChoiceParam{
				Type: anthropic.F(anthropic.ToolChoiceTypeTool),
				Name: anthropic.F("get_weather"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeToolChoice(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeToolChoice_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       *openai.ToolChoice
		expectedErr string
	}{
		{
			name: "returns error for unsupported 'none' tool choice",
			input: &openai.ToolChoice{
				Value: (*openai.ToolChoiceValue)(utils.ToPtr("none")),
			},
			expectedErr: "claude does not support 'none' tool choice",
		},
		{
			name: "returns error for invalid tool choice value",
			input: &openai.ToolChoice{
				Value: (*openai.ToolChoiceValue)(utils.ToPtr("invalid")),
			},
			expectedErr: "tool field must be set to either 'auto', 'required', 'none', or an object with a function name",
		},
		{
			name: "returns error when 'function' tool choice is missing struct",
			input: &openai.ToolChoice{
				Struct: nil,
			},
			expectedErr: "tool field must be set to either 'auto', 'required', 'none', or an object with a function name",
		},
		{
			name: "function value but struct type mismatch",
			input: &openai.ToolChoice{
				Struct: &openai.ToolChoiceStruct{
					Type: "not_function",
					Function: &openai.Function{
						Name: "get_weather",
					},
				},
			},
			expectedErr: "unsupported tool type: not_function",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeToolChoice(tt.input)
			assert.Nil(t, result)
			assert.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestToClaudeMessage_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    openai.Message
		toolMap  map[string]string
		expected *anthropic.MessageParam
	}{
		{
			name: "returns user message for user role",
			input: openai.Message{
				Role: "user",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
			},
			expected: &anthropic.MessageParam{
				Role: anthropic.F(anthropic.MessageParamRoleUser),
				Content: anthropic.F([]anthropic.MessageParamContentUnion{
					anthropic.NewTextBlock("Hello, world!"),
				}),
			},
		},
		{
			name: "returns assistant message for assistant role",
			input: openai.Message{
				Role: "assistant",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
			},
			expected: &anthropic.MessageParam{
				Role: anthropic.F(anthropic.MessageParamRoleAssistant),
				Content: anthropic.F([]anthropic.MessageParamContentUnion{
					anthropic.NewTextBlock("Hello, world!"),
				}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeMessage(tt.input, tt.toolMap)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeMessage_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       openai.Message
		toolMap     map[string]string
		expectedErr string
	}{
		{
			name: "returns error for unsupported message role",
			input: openai.Message{
				Role: "system",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
			},
			expectedErr: "unsupported message role: system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeMessage(tt.input, tt.toolMap)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestToClaudeMessages_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    []openai.Message
		toolMap  map[string]string
		expected []anthropic.MessageParam
	}{
		{
			name: "returns user message for user role",
			input: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: utils.ToPtr("Hello, world!"),
					},
				},
			},
			expected: []anthropic.MessageParam{
				{
					Role: anthropic.F(anthropic.MessageParamRoleUser),
					Content: anthropic.F([]anthropic.MessageParamContentUnion{
						anthropic.NewTextBlock("Hello, world!"),
					}),
				},
			},
		},
		{
			name: "returns empty message params if message is from system role",
			input: []openai.Message{
				{
					Role: "system",
					Content: &openai.MessageContent{
						String: utils.ToPtr("Hello, world!"),
					},
				},
			},
			expected: []anthropic.MessageParam{},
		},
		{
			name: "returns function message",
			input: []openai.Message{
				{
					Role: "user",
					FunctionCall: &openai.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location": "Hanoi"}`,
					},
				},
			},
			expected: []anthropic.MessageParam{
				{
					Role: anthropic.F(anthropic.MessageParamRoleUser),
					Content: anthropic.F([]anthropic.MessageParamContentUnion{
						anthropic.NewToolUseBlockParam(
							"call-get_weather-0",
							"get_weather",
							any(map[string]any{
								"location": "Hanoi",
							}),
						),
					}),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeMessages(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeMessages_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       []openai.Message
		expectedErr string
	}{
		{
			name:        "returns error for zero messages",
			input:       []openai.Message{},
			expectedErr: "at least one message is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeMessages(tt.input)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestToClaudeParams_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    *openai.ChatCompletionRequest
		expected *anthropic.MessageNewParams
	}{
		{
			name: "basic request with model and message",
			input: &openai.ChatCompletionRequest{
				Model: "claude-3-sonnet",
				Messages: []openai.Message{
					{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hi there")}},
				},
				MaxTokens:   utils.ToPtr(int32(256)),
				Temperature: utils.ToPtr(float32(0.8)),
			},
			expected: &anthropic.MessageNewParams{
				Model:       anthropic.F("claude-3-sonnet-20240229"),
				MaxTokens:   anthropic.Int(256),
				Temperature: anthropic.F(float64(float32(0.8))),
				Messages: anthropic.F([]anthropic.MessageParam{
					{
						Role: anthropic.F(anthropic.MessageParamRoleUser),
						Content: anthropic.F([]anthropic.MessageParamContentUnion{
							anthropic.NewTextBlock("Hi there"),
						}),
					},
				}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeParams(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeParams_ErrorCases(t *testing.T) {
	tests := []struct {
		name    string
		input   *openai.ChatCompletionRequest
		wantErr string
	}{
		{
			name: "unsupported response format",
			input: &openai.ChatCompletionRequest{
				Model: "claude-3-sonnet",
				Messages: []openai.Message{
					{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hi there")}},
				},
				ResponseFormat: &openai.ResponseFormat{Type: "json"},
			},
			wantErr: "response_format is not supported with Claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := toClaudeParams(tt.input)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}
