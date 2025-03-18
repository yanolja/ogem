package claude

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

func TestToOpenAiMessage_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    *anthropic.Message
		expected *openai.Message
	}{
		{
			name: "text block only",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "text",
						"text": "Hello, world!"
					}`),
				},
			},
			expected: &openai.Message{
				Role: "assistant",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
			},
		},
		{
			name: "tool use block only",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "tool_use",
						"id": "tool-123",
						"name": "get_weather",
						"input": {
							"location": "Seoul",
							"unit": "celsius"
						}
					}`),
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
		},
		{
			name: "mixed blocks",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "text",
						"text": "The weather today is: "
					}`),
					mustUnmarshalContentBlock(`{
						"type": "tool_use",
						"id": "tool-123",
						"name": "get_weather",
						"input": {
							"location": "Seoul",
							"unit": "celsius"
						}
					}`),
					mustUnmarshalContentBlock(`{
						"type": "text",
						"text": " Please wait for the result."
					}`),
				},
			},
			expected: &openai.Message{
				Role: "assistant",
				Content: &openai.MessageContent{
					String: utils.ToPtr("The weather today is:  Please wait for the result."),
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
		},
		{
			name: "empty message content",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{},
			},
			expected: &openai.Message{
				Role: "assistant",
			},
		},
		{
			name: "multiple tool use blocks",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "tool_use",
						"id": "tool-123",
						"name": "get_weather",
						"input": {
							"location": "Seoul",
							"unit": "celsius"
						}
					}`),
					mustUnmarshalContentBlock(`{
						"type": "tool_use",
						"id": "tool-456",
						"name": "get_time",
						"input": {
							"timezone": "Asia/Seoul"
						}
					}`),
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
					{
						Id:   "tool-456",
						Type: "function",
						Function: &openai.FunctionCall{
							Name: "get_time",
							Arguments: `{
							"timezone": "Asia/Seoul"
						}`,
						},
					},
				},
			},
		},
		{
			name: "multiple text blocks concatenated",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "text",
						"text": "Hello, "
					}`),
					mustUnmarshalContentBlock(`{
						"type": "text",
						"text": "world!"
					}`),
				},
			},
			expected: &openai.Message{
				Role: "assistant",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toOpenAiMessage(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToOpenAiMessage_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       *anthropic.Message
		expectedErr string
	}{
		{
			name: "unsupported block type",
			input: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "image",
						"url": "https://example.com/image.jpg"
					}`),
				},
			},
			expectedErr: "unsupported block type: image",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toOpenAiMessage(tt.input)
			assert.Error(t, err)
			assert.Nil(t, result)
		})
	}
}

func TestToOpenAiResponse(t *testing.T) {
	tests := []struct {
		name           string
		claudeResponse *anthropic.Message
		expected       *openai.ChatCompletionResponse
	}{
		{
			name: "valid text response",
			claudeResponse: &anthropic.Message{
				Content: []anthropic.ContentBlock{
					mustUnmarshalContentBlock(`{
						"type": "text",
						"text": "This is a test response"
					}`),
				},
				StopReason: anthropic.MessageStopReasonEndTurn,
				Usage: anthropic.Usage{
					InputTokens:  10,
					OutputTokens: 5,
				},
			},
			expected: &openai.ChatCompletionResponse{
				Choices: []openai.Choice{
					{
						Index: 0,
						Message: openai.Message{
							Role: "assistant",
							Content: &openai.MessageContent{
								String: utils.ToPtr("This is a test response"),
							},
						},
						FinishReason: "stop",
					},
				},
				Usage: openai.Usage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toOpenAiResponse(tt.claudeResponse)
			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.EqualValues(t, tt.expected, result)
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

// Helper function
func mustUnmarshalContentBlock(raw string) anthropic.ContentBlock {
	var block anthropic.ContentBlock
	if err := json.Unmarshal([]byte(raw), &block); err != nil {
		panic(err)
	}
	return block
}
