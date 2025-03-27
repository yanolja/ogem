package openaigeminiconverter

import (
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

func ToOpenAiResponse(geminiResponse *genai.GenerateContentResponse) (*openai.ChatCompletionResponse, error) {
	choices := make([]openai.Choice, len(geminiResponse.Candidates))
	for i, candidate := range geminiResponse.Candidates {
		if candidate.Content == nil {
			return nil, fmt.Errorf("candidate %d does not have content: %+v", i, candidate)
		}
		message, err := toOpenAiMessage(candidate.Content, candidate.Index)
		if err != nil {
			return nil, err
		}
		choices[i] = openai.Choice{
			Index:        candidate.Index,
			Message:      *message,
			FinishReason: toOpenAiFinishReason(candidate.FinishReason),
		}
	}
	return &openai.ChatCompletionResponse{
		Choices: choices,
		Usage: openai.Usage{
			PromptTokens:     geminiResponse.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResponse.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResponse.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

func toOpenAiMessage(content *genai.Content, index int32) (*openai.Message, error) {
	message := &openai.Message{
		Role: "assistant",
	}
	parts := make([]openai.Part, 0, len(content.Parts))
	toolCalls := make([]openai.ToolCall, 0, len(content.Parts))
	for partIndex, part := range content.Parts {
		switch part := part.(type) {
		case genai.Text:
			parts = append(parts, openai.Part{
				Type: "text",
				Content: openai.Content{
					TextContent: &openai.TextContent{
						Text: strings.TrimSpace(fmt.Sprintf("%s", part)),
					},
				},
			})
		case genai.FunctionCall:
			jsonString, err := utils.MapToJson(part.Args)
			if err != nil {
				return nil, err
			}
			toolCalls = append(toolCalls, openai.ToolCall{
				Id:   fmt.Sprintf("tool-%s-%d-%d", part.Name, index, partIndex),
				Type: "function",
				Function: &openai.FunctionCall{
					Name:      part.Name,
					Arguments: jsonString,
				},
			})
		default:
			return nil, fmt.Errorf("unsupported part type: %T", part)
		}
	}
	if len(parts) > 0 {
		if len(parts) == 1 && parts[0].Content.TextContent != nil {
			message.Content = &openai.MessageContent{
				String: &parts[0].Content.TextContent.Text,
			}
			return message, nil
		}
		message.Content = &openai.MessageContent{
			Parts: parts,
		}
		return message, nil
	}
	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
		return message, nil
	}
	return nil, fmt.Errorf("message must have content or tool calls")
}

func toOpenAiPart(part genai.Part) openai.Part {
	switch part := part.(type) {
	case genai.Text:
		return openai.Part{
			Content: openai.Content{
				TextContent: &openai.TextContent{
					Text: strings.TrimSpace(fmt.Sprintf("%s", part)),
				},
			},
		}
	}
	return openai.Part{
		Content: openai.Content{
			TextContent: &openai.TextContent{
				Text: "unsupported content type",
			},
		},
	}
}

func toOpenAiFinishReason(finishReason genai.FinishReason) string {
	switch finishReason {
	case genai.FinishReasonStop:
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "length"
	default:
		// The finish reasons are not 1:1 between OpenAI API and Gemini API.
		// Since the libraries usually handle only 3 finish reasons,
		// we return "content_filter" for the rest of the cases.
		return "content_filter"
	}
}
