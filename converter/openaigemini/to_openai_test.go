package openaigemini

import (
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

func TestToOpenAiResponse(t *testing.T) {
	t.Run("successful conversion with text content", func(t *testing.T) {
		geminiResp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Index: 0,
					Content: &genai.Content{
						Parts: []genai.Part{
							genai.Text("Hello, world!"),
						},
					},
					FinishReason: genai.FinishReasonStop,
				},
			},
			UsageMetadata: &genai.UsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
				TotalTokenCount:      15,
			},
		}

		expected := &openai.ChatCompletionResponse{
			Choices: []openai.Choice{
				{
					Index: 0,
					Message: openai.Message{
						Role: "assistant",
						Content: &openai.MessageContent{
							String: utils.ToPtr("Hello, world!"),
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

		result, err := ToOpenAiResponse(geminiResp)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("error with nil content", func(t *testing.T) {
		geminiResp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Index:   0,
					Content: nil,
				},
			},
		}

		result, err := ToOpenAiResponse(geminiResp)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "does not have content")
	})

	t.Run("evaluate error handling when error returned from toOpenAiMessage", func(t *testing.T) {
		geminiResp := &genai.GenerateContentResponse{
			Candidates: []*genai.Candidate{
				{
					Index: 0,
					Content: &genai.Content{},
					FinishReason: genai.FinishReasonStop,
				},
			},
		}

		result, err := ToOpenAiResponse(geminiResp)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "message must have content or tool calls")
	})
}
