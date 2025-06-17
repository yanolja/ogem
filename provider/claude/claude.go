package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/array"
)

// A unique identifier for the Claude provider
const REGION = "claude"

type anthropicClient interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

type Endpoint struct {
	client anthropicClient
}

func NewEndpoint(apiKey string) (*Endpoint, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Endpoint{client: &client.Messages}, nil
}

func (ep *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	claudeParams, err := toClaudeParams(openaiRequest)
	if err != nil {
		return nil, err
	}

	claudeResponse, err := ep.client.New(ctx, *claudeParams)
	if err != nil {
		return nil, err
	}

	openaiResponse, err := toOpenAiResponse(claudeResponse)
	if err != nil {
		return nil, err
	}

	return openai.FinalizeResponse(ep.Provider(), ep.Region(), openaiRequest.Model, openaiResponse), nil
}

func (ep *Endpoint) GenerateChatCompletionStream(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	responseCh := make(chan *openai.ChatCompletionStreamResponse)
	errorCh := make(chan error, 1)

	go func() {
		defer close(responseCh)
		defer close(errorCh)

		// For now, convert streaming to non-streaming by getting the full response and emitting it as chunks
		openaiResponse, err := ep.GenerateChatCompletion(ctx, openaiRequest)
		if err != nil {
			errorCh <- err
			return
		}

		// Convert the response to streaming format
		if len(openaiResponse.Choices) > 0 {
			choice := openaiResponse.Choices[0]
			
			// Send role chunk
			roleChunk := &openai.ChatCompletionStreamResponse{
				Id:      openaiResponse.Id,
				Object:  "chat.completion.chunk",
				Created: openaiResponse.Created,
				Model:   openaiResponse.Model,
				Choices: []openai.ChoiceDelta{
					{
						Index: 0,
						Delta: openai.MessageDelta{
							Role: &choice.Message.Role,
						},
					},
				},
			}
			
			select {
			case responseCh <- roleChunk:
			case <-ctx.Done():
				return
			}

			// Send content chunk(s)
			if choice.Message.Content != nil && choice.Message.Content.String != nil {
				content := *choice.Message.Content.String
				contentChunk := &openai.ChatCompletionStreamResponse{
					Id:      openaiResponse.Id,
					Object:  "chat.completion.chunk", 
					Created: openaiResponse.Created,
					Model:   openaiResponse.Model,
					Choices: []openai.ChoiceDelta{
						{
							Index: 0,
							Delta: openai.MessageDelta{
								Content: &content,
							},
						},
					},
				}
				
				select {
				case responseCh <- contentChunk:
				case <-ctx.Done():
					return
				}
			}

			// Send final chunk with finish_reason
			finalChunk := &openai.ChatCompletionStreamResponse{
				Id:      openaiResponse.Id,
				Object:  "chat.completion.chunk",
				Created: openaiResponse.Created,
				Model:   openaiResponse.Model,
				Choices: []openai.ChoiceDelta{
					{
						Index:        0,
						Delta:        openai.MessageDelta{},
						FinishReason: &choice.FinishReason,
					},
				},
				Usage: &openaiResponse.Usage,
			}
			
			select {
			case responseCh <- finalChunk:
			case <-ctx.Done():
				return
			}
		}
	}()

	return responseCh, errorCh
}

func (ep *Endpoint) Provider() string {
	return "claude"
}

func (ep *Endpoint) Region() string {
	return REGION
}

func (ep *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	_, err := ep.client.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaude_3_Haiku_20240307,
		MaxTokens: int64(1),
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Ping")),
		},
	})
	if err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (ep *Endpoint) Shutdown() error {
	return nil
}

func toClaudeParams(openaiRequest *openai.ChatCompletionRequest) (*anthropic.MessageNewParams, error) {
	messages, err := toClaudeMessages(openaiRequest.Messages)
	if err != nil {
		return nil, err
	}

	params := &anthropic.MessageNewParams{
		Model:    standardizeModelName(openaiRequest.Model),
		Messages: messages,
	}

	if openaiRequest.MaxTokens != nil {
		params.MaxTokens = int64(*openaiRequest.MaxTokens)
	}
	if openaiRequest.MaxCompletionTokens != nil {
		params.MaxTokens = int64(*openaiRequest.MaxCompletionTokens)
	}
	if params.MaxTokens == 0 {
		if standardizeModelName(openaiRequest.Model) == "claude-3-5-sonnet-20240620" {
			params.MaxTokens = 8192
		} else {
			params.MaxTokens = 4096
		}
	}
	if openaiRequest.StopSequences != nil {
		params.StopSequences = openaiRequest.StopSequences.Sequences
	}
	systemMessage, err := toClaudeSystemMessage(openaiRequest)
	if err != nil {
		return nil, err
	}
	if systemMessage != nil {
		params.System = systemMessage
	}
	if openaiRequest.Temperature != nil {
		params.Temperature = anthropic.Opt(float64(*openaiRequest.Temperature))
	}
	if openaiRequest.TopP != nil {
		params.TopP = anthropic.Opt(float64(*openaiRequest.TopP))
	}
	if openaiRequest.Tools != nil {
		tools, err := toClaudeToolParams(openaiRequest.Tools)
		if err != nil {
			return nil, err
		}
		params.Tools = tools
	}
	if openaiRequest.ToolChoice != nil {
		toolChoice, err := toClaudeToolChoice(openaiRequest.ToolChoice)
		if err != nil {
			return nil, err
		}
		params.ToolChoice = toolChoice
	}
	if openaiRequest.ResponseFormat != nil {
		return nil, fmt.Errorf("response_format is not supported with Claude")
	}

	return params, nil
}

func toClaudeMessages(openaiMessages []openai.Message) ([]anthropic.MessageParam, error) {
	messageCount := len(openaiMessages)
	if messageCount == 0 {
		return nil, fmt.Errorf("at least one message is required")
	}

	toolMap := make(map[string]string)
	claudeMessages := make([]anthropic.MessageParam, 0, len(openaiMessages))
	for index, message := range openaiMessages {
		if message.Role == "system" {
			continue
		}

		if message.FunctionCall != nil {
			toolMap[message.FunctionCall.Name] = fmt.Sprintf("call-%s-%d", message.FunctionCall.Name, index)
		}
		claudeMessage, err := toClaudeMessage(message, toolMap)
		if err != nil {
			return nil, err
		}
		claudeMessages = append(claudeMessages, *claudeMessage)
	}
	return claudeMessages, nil
}

func toClaudeMessage(openaiMessage openai.Message, toolMap map[string]string) (*anthropic.MessageParam, error) {
	blocks, err := toClaudeMessageBlocks(openaiMessage, toolMap)
	if err != nil {
		return nil, err
	}

	switch openaiMessage.Role {
	case "tool":
		fallthrough
	case "function":
		fallthrough
	case "user":
		return utils.ToPtr(anthropic.NewUserMessage(blocks...)), nil
	case "assistant":
		return utils.ToPtr(anthropic.NewAssistantMessage(blocks...)), nil
	}

	return nil, fmt.Errorf("unsupported message role: %s", openaiMessage.Role)
}

func toClaudeSystemMessage(openAiRequest *openai.ChatCompletionRequest) ([]anthropic.TextBlockParam, error) {
	for _, message := range openAiRequest.Messages {
		if message.Role == "system" {
			blocks, err := toClaudeMessageBlocks(message, nil)
			if err != nil {
				return nil, err
			}
			textBlocks := make([]anthropic.TextBlockParam, 0, len(blocks))
			for _, block := range blocks {
				if block.GetType() != nil && *block.GetType() == "text" {
					textBlocks = append(textBlocks, *block.OfRequestTextBlock)
				} else {
					return nil, fmt.Errorf("system message must contain only text blocks with Claude models")
				}
			}
			return textBlocks, nil
		}
	}
	return nil, nil
}

func toClaudeMessageBlocks(message openai.Message, toolMap map[string]string) ([]anthropic.ContentBlockParamUnion, error) {
	if message.Role == "tool" {
		if message.Content == nil || message.Content.String == nil {
			return nil, fmt.Errorf("tool message must contain a string content")
		}
		if message.ToolCallId == nil {
			return nil, fmt.Errorf("tool message must contain the corresponding tool call ID")
		}
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewToolResultBlock(*message.ToolCallId, *message.Content.String, false),
		}, nil
	}
	if message.Role == "function" {
		if message.Content == nil || message.Content.String == nil {
			return nil, fmt.Errorf("function message must contain a string content")
		}
		if message.Name == nil {
			return nil, fmt.Errorf("function message must contain the corresponding function name")
		}
		toolId, exists := toolMap[*message.Name]
		if !exists {
			return nil, fmt.Errorf("function message must contain the corresponding function name")
		}
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewToolResultBlock(toolId, *message.Content.String, false),
		}, nil
	}
	if message.Content != nil {
		if message.Content.String != nil {
			return []anthropic.ContentBlockParamUnion{
				anthropic.NewTextBlock(*message.Content.String),
			}, nil
		}
		return array.Map(message.Content.Parts, func(part openai.Part) anthropic.ContentBlockParamUnion {
			if part.Content.TextContent != nil {
				return anthropic.NewTextBlock(part.Content.TextContent.Text)
			}
			if part.Content.ImageContent != nil {
				// TODO(seungduk): Implement image downloader and pass it from the main to this provider.
				// It should support cache mechanism using Valkey.
				return anthropic.NewTextBlock("image content is not supported yet")
			}
			return anthropic.NewTextBlock("unsupported content type")
		}), nil
	}
	if message.Refusal != nil {
		return []anthropic.ContentBlockParamUnion{
			anthropic.NewTextBlock(*message.Refusal),
		}, nil
	}
	if message.FunctionCall != nil {
		arguments, err := utils.JsonToMap(message.FunctionCall.Arguments)
		if err != nil {
			return nil, fmt.Errorf("failed to parse function arguments: %v", err)
		}
		toolId, exists := toolMap[message.FunctionCall.Name]
		if !exists {
			return nil, fmt.Errorf("function message must contain the corresponding function name")
		}
		return []anthropic.ContentBlockParamUnion{
			{
				OfRequestToolUseBlock: &anthropic.ToolUseBlockParam{
					ID:      toolId,
					Name:    message.FunctionCall.Name,
					Input:   arguments,
				},
			},
		}, nil
	}
	if len(message.ToolCalls) > 0 {
		toolCalls := make([]anthropic.ContentBlockParamUnion, len(message.ToolCalls))
		for index, toolCall := range message.ToolCalls {
			if toolCall.Type != "function" {
				return nil, fmt.Errorf("unsupported tool call type: %s", toolCall.Type)
			}
			arguments, err := utils.JsonToMap(toolCall.Function.Arguments)
			if err != nil {
				return nil, fmt.Errorf("failed to parse tool arguments: %v", err)
			}
			toolCalls[index] = anthropic.ContentBlockParamUnion{
				OfRequestToolUseBlock: &anthropic.ToolUseBlockParam{
					ID:      toolCall.Id,
					Name:    toolCall.Function.Name,
					Input:   arguments,
				},
			}
		}
		return toolCalls, nil
	}
	return nil, fmt.Errorf("message must have content, refusal, function_call, or tool_calls")
}

func toClaudeToolParams(openaiTools []openai.Tool) ([]anthropic.ToolUnionParam, error) {
	claudeTools := make([]anthropic.ToolUnionParam, len(openaiTools))
	for i, tool := range openaiTools {
		if tool.Type != "function" {
			return nil, fmt.Errorf("unsupported tool type: %s", tool.Type)
		}
		var description string
		if tool.Function.Description == nil {
			description = strings.TrimSpace(fmt.Sprintf("Tool %s", tool.Function.Name))
		} else {
			description = *tool.Function.Description
		}
		claudeTools[i] = anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Function.Name,
				Description: anthropic.String(description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: tool.Function.Parameters,
				},
			},
		}
	}
	return claudeTools, nil
}

func toClaudeToolChoice(toolChoice *openai.ToolChoice) (anthropic.ToolChoiceUnionParam, error) {
	if toolChoice == nil {
		return anthropic.ToolChoiceUnionParam{}, nil
	}
	if toolChoice.Value != nil {
		switch *toolChoice.Value {
		case openai.ToolChoiceAuto:
			return anthropic.ToolChoiceUnionParam{
				OfToolChoiceAuto: &anthropic.ToolChoiceAutoParam{},
			}, nil
		case openai.ToolChoiceRequired:
			return anthropic.ToolChoiceUnionParam{
				OfToolChoiceAny: &anthropic.ToolChoiceAnyParam{},
			}, nil
		case openai.ToolChoiceNone:
			return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("claude does not support 'none' tool choice")
		}
	}
	if toolChoice.Struct == nil {
		return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("tool field must be set to either 'auto', 'required', 'none', or an object with a function name")
	}
	if toolChoice.Struct.Type != "function" {
		return anthropic.ToolChoiceUnionParam{}, fmt.Errorf("unsupported tool type: %s", toolChoice.Struct.Type)
	}
	return anthropic.ToolChoiceUnionParam{
		OfToolChoiceTool: &anthropic.ToolChoiceToolParam{
			Name: toolChoice.Struct.Function.Name,
		},
	}, nil
}

func toClaudeToolParamsFromFunctions(openaiFunctions []openai.LegacyFunction) []anthropic.ToolParam {
	claudeTools := make([]anthropic.ToolParam, len(openaiFunctions))
	for i, function := range openaiFunctions {
		var description string
		if function.Description == nil {
			description = strings.TrimSpace(fmt.Sprintf("Function %s", function.Name))
		} else {
			description = *function.Description
		}
		claudeTools[i] = anthropic.ToolParam{
			Name:        function.Name,
			Description: anthropic.String(description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: function.Parameters,
			},
		}
	}
	return claudeTools
}

func toOpenAiResponse(claudeResponse *anthropic.Message) (*openai.ChatCompletionResponse, error) {
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
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "tool_use":
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

func standardizeModelName(model string) string {
	switch strings.TrimRight(model, "0123456789@-") {
	case "claude-3-5-sonnet":
		return "claude-3-5-sonnet-20240620"
	case "claude-3-opus":
		return "claude-3-opus-20240229"
	case "claude-3-sonnet":
		return "claude-3-sonnet-20240229"
	case "claude-3-haiku":
		return "claude-3-haiku-20240307"
	}
	return model
}
