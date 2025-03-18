package claude

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/orderedmap"
)

func TestToClaudeToolParams_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    []openai.Tool
		expected []anthropic.ToolParam
	}{
		{
			name: "converts valid function tool with description and parameters to Claude format",
			input: []openai.Tool{
				{
					Type: "function",
					Function: openai.FunctionTool{
						Name:        "get_weather",
						Description: utils.ToPtr("Get the weather in a specific location"),
						Parameters: func() *orderedmap.Map {
							params := orderedmap.New()
							params.Set("location", map[string]any{
								"type":        "string",
								"description": "The location to get the weather for",
							})
							return params
						}(),
					},
				},
			},
			expected: []anthropic.ToolParam{
				{
					Name:        anthropic.F("get_weather"),
					Description: anthropic.F("Get the weather in a specific location"),
					InputSchema: anthropic.F(any(func() *orderedmap.Map {
						params := orderedmap.New()
						params.Set("location", map[string]any{
							"type":        "string",
							"description": "The location to get the weather for",
						})
						return params
					}())),
				},
			},
		},
		{
			name: "converts valid function tool without description to Claude format",
			input: []openai.Tool{
				{
					Type: "function",
					Function: openai.FunctionTool{
						Name:        "get_weather",
						Description: nil,
						Parameters: func() *orderedmap.Map {
							params := orderedmap.New()
							params.Set("location", map[string]any{
								"type":        "string",
								"description": "The location to get the weather for",
							})
							return params
						}(),
					},
				},
			},
			expected: []anthropic.ToolParam{
				{
					Name:        anthropic.F("get_weather"),
					Description: anthropic.F("Tool get_weather"),
					InputSchema: anthropic.F(any(func() *orderedmap.Map {
						params := orderedmap.New()
						params.Set("location", map[string]any{
							"type":        "string",
							"description": "The location to get the weather for",
						})
						return params
					}())),
				},
			},
		},
		{
			name: "converts valid function tool without parameters to Claude format",
			input: []openai.Tool{
				{
					Type: "function",
					Function: openai.FunctionTool{
						Name:        "get_weather",
						Description: utils.ToPtr("Get the weather in a specific location"),
						Parameters:  nil,
					},
				},
			},
			expected: []anthropic.ToolParam{
				{
					Name:        anthropic.F("get_weather"),
					Description: anthropic.F("Get the weather in a specific location"),
					InputSchema: anthropic.F(any(func() *orderedmap.Map {
						return nil
					}())),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeToolParams(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeToolParams_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       []openai.Tool
		expectedErr string
	}{
		{
			name: "returns error for unsupported tool type",
			input: []openai.Tool{
				{
					Type: "invalid",
				},
			},
			expectedErr: "unsupported tool type: invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeToolParams(tt.input)
			assert.Nil(t, result)
			assert.EqualError(t, err, tt.expectedErr)
		})
	}
}

func TestToClaudeMessageBlocks_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    openai.Message
		toolMap  map[string]string
		expected []anthropic.MessageParamContentUnion
	}{
		{
			name: "converts valid openai tool message to claude message blocks",
			input: openai.Message{
				Role: "tool",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
				ToolCallId: utils.ToPtr("tool-123"),
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewToolResultBlock("tool-123", "Hello, world!", false),
			},
		},
		{
			name: "converts valid openai function message to claude message blocks",
			input: openai.Message{
				Role: "function",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
				Name: utils.ToPtr("get_weather"),
			},
			toolMap: map[string]string{
				"get_weather": "tool-123",
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewToolResultBlock("tool-123", "Hello, world!", false),
			},
		},
		{
			name: "converts content message with text to claude message blocks",
			input: openai.Message{
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewTextBlock("Hello, world!"),
			},
		},
		{
			name: "converts content message with string parts to claude message blocks",
			input: openai.Message{
				Content: &openai.MessageContent{
					Parts: []openai.Part{
						{
							Type: "text",
							Content: openai.Content{
								TextContent: &openai.TextContent{
									Text: "Hello, world!",
								},
							},
						},
						{
							Type: "text",
							Content: openai.Content{
								TextContent: &openai.TextContent{
									Text: "This message is from ogem.",
								},
							},
						},
					},
				},
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewTextBlock("Hello, world!"),
				anthropic.NewTextBlock("This message is from ogem."),
			},
		},
		{
			name: "converts content message with image to claude message blocks",
			input: openai.Message{
				Content: &openai.MessageContent{
					Parts: []openai.Part{
						{
							Type: "text",
							Content: openai.Content{
								ImageContent: &openai.ImageContent{
									Url: "https://example.com/image.jpg",
								},
							},
						},
						{
							Type: "image",
							Content: openai.Content{
								ImageContent: &openai.ImageContent{
									Url: "https://example.com/image.jpg",
								},
							},
						},
					},
				},
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewTextBlock("image content is not supported yet"),
				anthropic.NewTextBlock("image content is not supported yet"),
			},
		},
		{
			name: "converts content message with unsupported content type to claude message blocks",
			input: openai.Message{
				Content: &openai.MessageContent{
					Parts: []openai.Part{
						{
							Type:    "unsupported",
							Content: openai.Content{},
						},
					},
				},
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewTextBlock("unsupported content type"),
			},
		},
		{
			name: "converts refusal message to claude message blocks",
			input: openai.Message{
				Refusal: utils.ToPtr("I'm sorry, I can't do that."),
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewTextBlock("I'm sorry, I can't do that."),
			},
		},
		{
			name: "converts function message to claude message blocks",
			input: openai.Message{
				FunctionCall: &openai.FunctionCall{
					Name: "get_weather",
					Arguments: `{
						"location": "Seoul",
						"unit": "celsius"
					}`,
				},
			},
			toolMap: map[string]string{
				"get_weather": "tool-123",
			},
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewToolUseBlockParam("tool-123", "get_weather", any(map[string]any{
					"location": "Seoul",
					"unit":     "celsius",
				})),
			},
		},
		{
			name: "converts tool call message to claude message blocks",
			input: openai.Message{
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
			expected: []anthropic.MessageParamContentUnion{
				anthropic.NewToolUseBlockParam("tool-123", "get_weather", any(map[string]any{
					"location": "Seoul",
					"unit":     "celsius",
				})),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeMessageBlocks(tt.input, tt.toolMap)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeMessageBlocks_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       openai.Message
		toolMap     map[string]string
		expectedErr string
	}{
		{
			name: "returns error for tool message with nil content",
			input: openai.Message{
				Role:    "tool",
				Content: nil,
			},
			expectedErr: "tool message must contain a string content",
		},
		{
			name: "returns error for tool message with empty string content",
			input: openai.Message{
				Role: "tool",
				Content: &openai.MessageContent{
					String: nil,
				},
			},
			expectedErr: "tool message must contain a string content",
		},
		{
			name: "returns error for tool message with nil tool call ID",
			input: openai.Message{
				Role: "tool",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
				ToolCallId: nil,
			},
			expectedErr: "tool message must contain the corresponding tool call ID",
		},
		{
			name: "returns error for function message with nil content",
			input: openai.Message{
				Role:    "function",
				Content: nil,
			},
			expectedErr: "function message must contain a string content",
		},
		{
			name: "returns error for function message with empty string content",
			input: openai.Message{
				Role: "function",
				Content: &openai.MessageContent{
					String: nil,
				},
			},
			expectedErr: "function message must contain a string content",
		},
		{
			name: "returns error for function message with nil function name",
			input: openai.Message{
				Role: "function",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
				Name: nil,
			},
			expectedErr: "function message must contain the corresponding function name",
		},
		{
			name: "returns error for function message that does not exist in the tool map",
			input: openai.Message{
				Role: "function",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Hello, world!"),
				},
				Name: utils.ToPtr("get_weather"),
			},
			toolMap:     map[string]string{},
			expectedErr: "function message must contain the corresponding function name",
		},
		{
			name: "returns error for not being able to parse function arguments",
			input: openai.Message{
				FunctionCall: &openai.FunctionCall{
					Arguments: `{
						"location": "Seoul",
						error: should not parse
					}`,
				},
			},
			expectedErr: "failed to parse function arguments:",
		},
		{
			name: "returns error for function call not existing function name in the tool map",
			input: openai.Message{
				FunctionCall: &openai.FunctionCall{
					Name: "get_weather",
					Arguments: `{
						"location": "Seoul",
						"unit": "celsius"
					}`,
				},
			},
			toolMap:     map[string]string{},
			expectedErr: "function message must contain the corresponding function name",
		},
		{
			name: "returns error for tool call type not function",
			input: openai.Message{
				ToolCalls: []openai.ToolCall{
					{
						Type: "invalid",
					},
				},
			},
			expectedErr: "unsupported tool call type: invalid",
		},
		{
			name: "returns error for not being able to parse tool call arguments",
			input: openai.Message{
				ToolCalls: []openai.ToolCall{
					{
						Type: "function",
						Function: &openai.FunctionCall{
							Arguments: `{
								"location": "Seoul",
								error: should not parse
							}`,
						},
					},
				},
			},
			expectedErr: "failed to parse tool arguments:",
		},
		{
			name:        "returns error for not having content, refusal, function_call, or tool_calls",
			input:       openai.Message{},
			expectedErr: "message must have content, refusal, function_call, or tool_calls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeMessageBlocks(tt.input, tt.toolMap)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}

func TestToClaudeSystemMessage_SuccessCases(t *testing.T) {
	tests := []struct {
		name     string
		input    *openai.ChatCompletionRequest
		expected []anthropic.TextBlockParam
	}{
		{
			name: "returns text blocks for system message",
			input: &openai.ChatCompletionRequest{
				Messages: []openai.Message{
					{
						Role: "system",
						Content: &openai.MessageContent{
							String: utils.ToPtr("Hello, world!"),
						},
					},
				},
			},
			expected: []anthropic.TextBlockParam{
				anthropic.NewTextBlock("Hello, world!"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeSystemMessage(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToClaudeSystemMessage_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		input       *openai.ChatCompletionRequest
		expectedErr string
	}{
		{
			name: "returns error for not being a text block	",
			input: &openai.ChatCompletionRequest{
				Messages: []openai.Message{
					{
						Role: "system",
						ToolCalls: []openai.ToolCall{
							{
								Type: "function",
								Id:   "tool-id-1",
								Function: &openai.FunctionCall{
									Name:      "get_weather",
									Arguments: `{"location": "Hanoi"}`,
								},
							},
						},
					},
				},
			},
			expectedErr: "system message must contain only text blocks with Claude models",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toClaudeSystemMessage(tt.input)
			assert.Nil(t, result)
			assert.Contains(t, err.Error(), tt.expectedErr)
		})
	}
}
