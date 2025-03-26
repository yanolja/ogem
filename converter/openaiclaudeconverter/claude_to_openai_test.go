package openaiclaudeconverter

import (
	"encoding/json"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

func unmarshalContentBlock(jsonString string) anthropic.ContentBlock {
	var block anthropic.ContentBlock
	utils.Must0(json.Unmarshal([]byte(jsonString), &block))
	return block
}

func TestToOpenAiResponse(t *testing.T) {
	t.Run("valid text response", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "This is a test response"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock,
			},
			StopReason: anthropic.MessageStopReasonEndTurn,
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}
		expected := &openai.ChatCompletionResponse{
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
		}
		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.EqualValues(t, expected, result)
	})

	t.Run("multiple content blocks", func(t *testing.T) {
		contentBlock1 := unmarshalContentBlock(`{"type": "text", "text": "First part"}`)
		contentBlock2 := unmarshalContentBlock(`{"type": "text", "text": "Second part"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock1,
				contentBlock2,
			},
			StopReason: anthropic.MessageStopReasonEndTurn,
			Usage: anthropic.Usage{
				InputTokens:  12,
				OutputTokens: 8,
			},
		}
		expected := &openai.ChatCompletionResponse{
			Choices: []openai.Choice{
				{
					Index: 0,
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: utils.ToPtr("First part Second part"),
						},
					},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{
				PromptTokens:     12,
				CompletionTokens: 8,
				TotalTokens:      20,
			},
		}
		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.EqualValues(t, expected, result)
	})

	t.Run("empty claude response", func(t *testing.T) {
		var claudeResponse *anthropic.Message = nil

		result, err := ToOpenAiResponse(claudeResponse)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "claude response is nil")
	})

	t.Run("nil content array", func(t *testing.T) {
		claudeResponse := &anthropic.Message{
			Content:    nil,
			StopReason: anthropic.MessageStopReasonEndTurn,
			Usage: anthropic.Usage{
				InputTokens:  5,
				OutputTokens: 2,
			},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "claude response content is nil or empty")
	})

	t.Run("empty content array", func(t *testing.T) {
		claudeResponse := &anthropic.Message{
			Content:    []anthropic.ContentBlock{},
			StopReason: anthropic.MessageStopReasonEndTurn,
			Usage: anthropic.Usage{
				InputTokens:  5,
				OutputTokens: 2,
			},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "claude response content is nil or empty")
	})

	t.Run("claude response with nil usage", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock,
			},
			StopReason: anthropic.MessageStopReasonEndTurn,
			Usage:      anthropic.Usage{},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, int32(0), result.Usage.PromptTokens)
		assert.Equal(t, int32(0), result.Usage.CompletionTokens)
		assert.Equal(t, int32(0), result.Usage.TotalTokens)
	})

	t.Run("claude response with zero tokens", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock,
			},
			StopReason: anthropic.MessageStopReasonEndTurn,
			Usage: anthropic.Usage{
				InputTokens:  0,
				OutputTokens: 0,
			},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, int32(0), result.Usage.PromptTokens)
		assert.Equal(t, int32(0), result.Usage.CompletionTokens)
		assert.Equal(t, int32(0), result.Usage.TotalTokens)
	})

	t.Run("stop reason variations", func(t *testing.T) {
		tests := []struct {
			name       string
			stopReason anthropic.MessageStopReason
			expected   string
		}{
			{"end_turn", anthropic.MessageStopReasonEndTurn, "stop"},
			{"max_tokens", anthropic.MessageStopReasonMaxTokens, "length"},
			{"stop_sequence", anthropic.MessageStopReasonStopSequence, "stop"},
			{"unknown stop reason", "unknown_reason", "content_filter"},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
				claudeResponse := &anthropic.Message{
					Content: []anthropic.ContentBlock{
						contentBlock,
					},
					StopReason: tc.stopReason,
					Usage: anthropic.Usage{
						InputTokens:  10,
						OutputTokens: 5,
					},
				}

				result, err := ToOpenAiResponse(claudeResponse)
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tc.expected, result.Choices[0].FinishReason)
			})
		}
	})
}
