package claude

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

func TestNewEndpoint(t *testing.T) {
	ep, err := NewEndpoint("test-api-key")
	assert.NoError(t, err)
	assert.NotNil(t, ep)
	assert.NotNil(t, ep.client)
}

func TestStandardizeModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-3-5-sonnet", "claude-3-5-sonnet-20240620"},
		{"claude-3-opus", "claude-3-opus-20240229"},
		{"claude-3-sonnet", "claude-3-sonnet-20240229"},
		{"claude-3-haiku", "claude-3-haiku-20240307"},
		{"custom-model", "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := standardizeModelName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToOpenAiFinishReason(t *testing.T) {
	tests := []struct {
		input    anthropic.MessageStopReason
		expected string
	}{
		{anthropic.MessageStopReasonMaxTokens, "length"},
		{anthropic.MessageStopReasonEndTurn, "stop"},
		{anthropic.MessageStopReasonStopSequence, "stop"},
		{anthropic.MessageStopReasonToolUse, "stop"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := toOpenAiFinishReason(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToOpenAiMessage(t *testing.T) {
	tests := []struct {
		name     string
		input    *anthropic.Message
		expected *openai.Message
		wantErr  bool
	}{
		{
			name: "success: text block only",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					{
						Type: anthropic.ContentBlockTypeText,
						Text: "Hello, world!",
					},
				},
			},
			expected: &openai.Message{
				Role: "assistant",
				Content: &openai.MessageContent{
					String: stringPtr("Hello, world!"),
				},
			},
			wantErr: false,
		},
		{
			name: "success: tool use block only",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					{
						Type: anthropic.ContentBlockTypeToolUse,
						ID:   "tool-123",
						Name: "get_weather",
						Input: []byte(`{
							"location": "Seoul",
							"unit": "celsius"
						}`),
					},
				},
			},
			expected: &openai.Message{
				Role: "assistant",
				ToolCalls: []openai.ToolCall{
					{
						Id:   "tool-123",
						Type: "function",
						Function: &openai.FunctionCall{
							Name: "get_weather",
							Arguments: `{
							"location": "Seoul",
							"unit": "celsius"
						}`,
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "success: mixed blocks",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					{
						Type: anthropic.ContentBlockTypeText,
						Text: "The weather today is: ",
					},
					{
						Type: anthropic.ContentBlockTypeToolUse,
						ID:   "tool-123",
						Name: "get_weather",
						Input: []byte(`{
							"location": "Seoul",
							"unit": "celsius"
						}`),
					},
					{
						Type: anthropic.ContentBlockTypeText,
						Text: " Please wait for the result.",
					},
				},
			},
			expected: &openai.Message{
				Role: "assistant",
				Content: &openai.MessageContent{
					String: stringPtr("The weather today is:  Please wait for the result."),
				},
				ToolCalls: []openai.ToolCall{
					{
						Id:   "tool-123",
						Type: "function",
						Function: &openai.FunctionCall{
							Name: "get_weather",
							Arguments: `{
							"location": "Seoul",
							"unit": "celsius"
						}`,
						},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toOpenAiMessage(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function
func stringPtr(s string) *string {
	return &s
}
