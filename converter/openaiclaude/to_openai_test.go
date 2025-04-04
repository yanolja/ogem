package openaiclaude

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
	t.Run("returns valid text response when input contains single content block", func(t *testing.T) {
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

	// TODO (#106): consider returning multiple blocks instead of concatenation
	t.Run("returns concatenated text when input contains multiple content blocks", func(t *testing.T) {
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

	t.Run("returns error message when claude response is nil", func(t *testing.T) {
		var claudeResponse *anthropic.Message = nil

		result, err := ToOpenAiResponse(claudeResponse)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "claude response is nil")
	})

	t.Run("returns error message when content array is nil", func(t *testing.T) {
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

	t.Run("returns error message when content array is empty", func(t *testing.T) {
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

	t.Run("returns zero usage values when usage is nil", func(t *testing.T) {
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

	t.Run("returns zero usage values when token counts are zero", func(t *testing.T) {
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

	t.Run("returns stop finish reason when stop reason is end_turn", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
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

		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "stop", result.Choices[0].FinishReason)
	})

	t.Run("returns length finish reason when stop reason is max_tokens", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock,
			},
			StopReason: anthropic.MessageStopReasonMaxTokens,
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "length", result.Choices[0].FinishReason)
	})

	t.Run("returns stop finish reason when stop reason is stop_sequence", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock,
			},
			StopReason: anthropic.MessageStopReasonStopSequence,
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "stop", result.Choices[0].FinishReason)
	})

	t.Run("returns content_filter finish reason when stop reason is unknown", func(t *testing.T) {
		contentBlock := unmarshalContentBlock(`{"type": "text", "text": "Test message"}`)
		claudeResponse := &anthropic.Message{
			Content: []anthropic.ContentBlock{
				contentBlock,
			},
			StopReason: "unknown_reason",
			Usage: anthropic.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
		}

		result, err := ToOpenAiResponse(claudeResponse)
		assert.NoError(t, err)
		assert.NotNil(t, result)
		assert.Equal(t, "content_filter", result.Choices[0].FinishReason)
	})
}
