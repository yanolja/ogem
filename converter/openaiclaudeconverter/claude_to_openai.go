package openaiclaudeconverter

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/yanolja/ogem/openai"
)

func ToOpenAiResponse(claudeResponse *anthropic.Message) (*openai.ChatCompletionResponse, error) {
	choices := make([]openai.Choice, 1)
	message, err := toOpenAiMessage(claudeResponse)
	if err != nil {
		return nil, err
	}
	choices[0] = openai.Choice{
		Index:        0,
		Message:      *message,
		FinishReason: toOpenAiFinishReason(claudeResponse.StopReason),
	}

	return &openai.ChatCompletionResponse{
		Choices: choices,
		Usage: openai.Usage{
			PromptTokens:     int32(claudeResponse.Usage.InputTokens),
			CompletionTokens: int32(claudeResponse.Usage.OutputTokens),
			TotalTokens:      int32(claudeResponse.Usage.InputTokens + claudeResponse.Usage.OutputTokens),
		},
	}, nil
}

func toOpenAiMessage(claudeMessage *anthropic.Message) (*openai.Message, error) {
	message := &openai.Message{
		Role: "assistant",
	}

	content := strings.Builder{}
	toolCalls := make([]openai.ToolCall, 0)

	for _, block := range claudeMessage.Content {
		switch block := block.AsUnion().(type) {
		case anthropic.TextBlock:
			content.WriteString(block.Text)
		case anthropic.ToolUseBlock:
			toolCalls = append(toolCalls, openai.ToolCall{
				Id:   block.ID,
				Type: "function",
				Function: &openai.FunctionCall{
					Name:      block.Name,
					Arguments: string(block.Input),
				},
			})
		}
	}

	if content.Len() > 0 {
		contentStr := content.String()
		message.Content = &openai.MessageContent{String: &contentStr}
	}

	if len(toolCalls) > 0 {
		message.ToolCalls = toolCalls
	}

	return message, nil
}

func toOpenAiFinishReason(claudeStopReason anthropic.MessageStopReason) string {
	switch claudeStopReason {
	case anthropic.MessageStopReasonMaxTokens:
		return "length"
	case anthropic.MessageStopReasonEndTurn:
		fallthrough
	case anthropic.MessageStopReasonStopSequence:
		fallthrough
	case anthropic.MessageStopReasonToolUse:
		return "stop"
	}
	// Never happens because Claude only returns "length" or "stop".
	return "content_filter"
}