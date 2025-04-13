// This file is auto-generated. Do not edit manually.

package vertex

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/orderedmap"
)

type Endpoint struct {
	client *genai.Client
	region string
}

func NewEndpoint(projectId string, region string) (*Endpoint, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project: projectId,
		Location: region,
		Backend: genai.BackendVertexAI,
	})
	if err != nil {
		return nil, err
	}
	return &Endpoint{client: client, region: region}, nil
}

func (ep *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	config, err := toGeminiConfig(openaiRequest)
	if err != nil {
		return nil, err
	}

	history, messageToSend, err := toGeminiMessages(openaiRequest.Messages)
	if err != nil {
		return nil, err
	}

	chat, err := ep.client.Chats.Create(ctx, openaiRequest.Model, config, history)
	if err != nil {
		return nil, err
	}

	parts := make([]genai.Part, len(messageToSend.Parts))
	for i, part := range messageToSend.Parts {
		parts[i] = *part
	}

	geminiResponse, err := chat.SendMessage(ctx, parts...)
	if err != nil {
		return nil, err
	}

	openaiResponse, err := toOpenAiResponse(geminiResponse)
	if err != nil {
		return nil, err
	}

	return openai.FinalizeResponse(ep.Provider(), ep.Region(), openaiRequest.Model, openaiResponse), nil
}

func (ep *Endpoint) Provider() string {
	return "vertex"
}

func (ep *Endpoint) Region() string {
	return ep.region
}

func (ep *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	config := &genai.GenerateContentConfig{MaxOutputTokens: 1}
	content := []*genai.Content{
		{
			Parts: []*genai.Part{{Text: "Ping"}},
			Role:  "user",
		},
	}

	start := time.Now()
	if _, err := ep.client.Chats.Create(ctx, "gemini-2.0-flash-lite-001", config, content); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (ep *Endpoint) Shutdown() error {
	return nil
}

func toGeminiConfig(openaiRequest *openai.ChatCompletionRequest) (*genai.GenerateContentConfig, error) {
	config := &genai.GenerateContentConfig{}
	config.SystemInstruction = toGeminiSystemInstruction(openaiRequest)
	config.Temperature = openaiRequest.Temperature
	config.TopP = openaiRequest.TopP

	if openaiRequest.MaxTokens != nil {
		config.MaxOutputTokens = *openaiRequest.MaxTokens
	} else if openaiRequest.MaxCompletionTokens != nil {
		config.MaxOutputTokens = *openaiRequest.MaxCompletionTokens
	}

	if openaiRequest.CandidateCount != nil && *openaiRequest.CandidateCount != 1 {
		return nil, fmt.Errorf("unsupported candidate count: %d, only 1 is supported", *openaiRequest.CandidateCount)
	} else if openaiRequest.CandidateCount != nil {
		config.CandidateCount = *openaiRequest.CandidateCount
	}

	if openaiRequest.StopSequences != nil {
		config.StopSequences = openaiRequest.StopSequences.Sequences
	}

	config.SafetySettings = []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockThresholdBlockNone},
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockThresholdBlockNone},
	}

	if err := setToolsAndFunctions(config, openaiRequest); err != nil {
		return nil, err
	}

	if err := setResponseFormat(config, openaiRequest); err != nil {
		return nil, err
	}

	return config, nil
}

func setToolsAndFunctions(config *genai.GenerateContentConfig, openaiRequest *openai.ChatCompletionRequest) error {
	hasFunctions := len(openaiRequest.Functions) > 0
	hasTools := len(openaiRequest.Tools) > 0

	if hasFunctions && hasTools {
		return fmt.Errorf("functions and tools are mutually exclusive")
	}

	if hasFunctions {
		tools, err := toGeminiToolsFromFunctions(openaiRequest.Functions)
		if err != nil {
			return err
		}
		config.Tools = tools

		toolConfig, err := toGeminiToolConfigFromFunctions(openaiRequest.FunctionCall)
		if err != nil {
			return err
		}
		config.ToolConfig = toolConfig
	} else if hasTools {
		tools, err := toGeminiTools(openaiRequest.Tools)
		if err != nil {
			return err
		}
		config.Tools = tools

		toolConfig, err := toGeminiToolConfig(openaiRequest.ToolChoice)
		if err != nil {
			return err
		}
		config.ToolConfig = toolConfig
	}

	return nil
}

func setResponseFormat(config *genai.GenerateContentConfig, openaiRequest *openai.ChatCompletionRequest) error {
	mimeType, jsonSchema, err := provider.ToGeminiResponseMimeType(openaiRequest)
	if err != nil {
		return err
	}
	config.ResponseMIMEType = mimeType
	config.ResponseSchema, err = toGeminiSchema(jsonSchema)
	return err
}

func toGeminiMessages(openAiMessages []openai.Message) ([]*genai.Content, *genai.Content, error) {
	messageCount := len(openAiMessages)
	if messageCount == 0 {
		return nil, nil, nil
	}

	toolMap := make(map[string]string)
	geminiMessages := make([]*genai.Content, 0, messageCount)
	for _, message := range openAiMessages {
		if message.Role == "system" {
			continue
		}

		for _, toolCall := range message.ToolCalls {
			toolMap[toolCall.Id] = toolCall.Function.Name
		}
		parts, err := toGeminiParts(message, toolMap)
		if err != nil {
			return nil, nil, err
		}
		geminiMessages = append(geminiMessages, &genai.Content{
			Role:  provider.ToGeminiRole(message.Role),
			Parts: parts,
		})
	}
	lastIndex := len(geminiMessages) - 1
	geminiMessages, last := geminiMessages[:lastIndex], geminiMessages[lastIndex]
	return geminiMessages, last, nil
}

func toGeminiSystemInstruction(openAiRequest *openai.ChatCompletionRequest) *genai.Content {
	for _, message := range openAiRequest.Messages {
		if provider.ToGeminiRole(message.Role) == "system" {
			return &genai.Content{
				Parts: []*genai.Part{{Text: *message.Content.String}},
			}
		}
	}
	return nil
}

func toGeminiParts(message openai.Message, toolMap map[string]string) ([]*genai.Part, error) {
	if message.Role == "tool" {
		response, err := utils.JsonToMap(*message.Content.String)
		if err != nil {
			return nil, fmt.Errorf("tool response must be a valid JSON object: %v", err)
		}
		functionName, exists := toolMap[*message.ToolCallId]
		if !exists {
			return nil, fmt.Errorf("tool call ID %s not found in the previous messages", *message.ToolCallId)
		}
		return []*genai.Part{
			{
				FunctionResponse: &genai.FunctionResponse{
					Name:     functionName,
					Response: response,
				},
			},
		}, nil
	}
	if message.Role == "function" {
		response, err := utils.JsonToMap(*message.Content.String)
		if err != nil {
			return nil, fmt.Errorf("function response must be a valid JSON object: %v", err)
		}
		return []*genai.Part{
			{
				FunctionResponse: &genai.FunctionResponse{
					Name:     *message.Name,
					Response: response,
				},
			},
		}, nil
	}
	if message.Content != nil {
		if message.Content.String != nil {
			return []*genai.Part{{Text: *message.Content.String}}, nil
		}
		parts := make([]*genai.Part, len(message.Content.Parts))
		for i, part := range message.Content.Parts {
			if part.Content.TextContent != nil {
				parts[i] = &genai.Part{Text: part.Content.TextContent.Text}
			} else if part.Content.ImageContent != nil {
				// TODO(seungduk): Implement image downloader and pass it from the main to this provider.
				// It should support cache mechanism using Valkey.
				parts[i] = &genai.Part{Text: "image content is not supported yet"}
			} else {
				parts[i] = &genai.Part{Text: "unsupported content type"}
			}
		}
		return parts, nil
	}
	if message.Refusal != nil {
		return []*genai.Part{{Text: *message.Refusal}}, nil
	}
	if message.FunctionCall != nil {
		arguments, err := utils.JsonToMap(message.FunctionCall.Arguments)
		if err != nil {
			return nil, err
		}
		return []*genai.Part{{
			FunctionCall: &genai.FunctionCall{
				Name: message.FunctionCall.Name,
				Args: arguments,
			},
		}}, nil
	}
	if len(message.ToolCalls) > 0 {
		toolCalls := make([]*genai.Part, len(message.ToolCalls))
		for index, toolCall := range message.ToolCalls {
			if toolCall.Type != "function" {
				return nil, fmt.Errorf("unsupported tool call type: %s", toolCall.Type)
			}
			arguments, err := utils.JsonToMap(toolCall.Function.Arguments)
			if err != nil {
				return nil, err
			}
			toolCalls[index] = &genai.Part{
				FunctionCall: &genai.FunctionCall{
					Name: toolCall.Function.Name,
					Args: arguments,
				},
			}
		}
		return toolCalls, nil
	}
	return nil, fmt.Errorf("message must have content, refusal, function_call, or tool_calls")
}

func toGeminiTools(tools []openai.Tool) ([]*genai.Tool, error) {
	geminiFunctions := make([]*genai.FunctionDeclaration, len(tools))
	for i, tool := range tools {
		if tool.Type != "function" {
			return nil, fmt.Errorf("unsupported tool type: %s", tool.Type)
		}
		var description string
		if tool.Function.Description == nil {
			description = strings.TrimSpace(fmt.Sprintf("Tool %s", tool.Function.Name))
		} else {
			description = *tool.Function.Description
		}
		schema, err := toGeminiSchema(tool.Function.Parameters)
		if err != nil {
			return nil, err
		}
		geminiFunctions[i] = &genai.FunctionDeclaration{
			Name:        tool.Function.Name,
			Description: description,
			Parameters:  schema,
		}
	}
	return []*genai.Tool{{FunctionDeclarations: geminiFunctions}}, nil
}

func toGeminiToolConfig(toolChoice *openai.ToolChoice) (*genai.ToolConfig, error) {
	if toolChoice == nil {
		return nil, nil
	}
	if toolChoice.Value != nil {
		switch *toolChoice.Value {
		case "auto":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAuto,
				},
			}, nil
		case "required":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAny,
				},
			}, nil
		case "none":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeNone,
				},
			}, nil
		}
	}
	if toolChoice.Struct == nil {
		return nil, fmt.Errorf("tool field must be set to either 'auto', 'required', 'none', or an object with a function name")
	}
	if toolChoice.Struct.Type != "function" {
		return nil, fmt.Errorf("unsupported tool type: %s", toolChoice.Struct.Type)
	}
	return &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode:                 genai.FunctionCallingConfigModeAny,
			AllowedFunctionNames: []string{toolChoice.Struct.Function.Name},
		},
	}, nil
}

func toGeminiToolsFromFunctions(functions []openai.LegacyFunction) ([]*genai.Tool, error) {
	geminiFunctions := make([]*genai.FunctionDeclaration, len(functions))
	for i, function := range functions {
		var description string
		if function.Description == nil {
			description = strings.TrimSpace(fmt.Sprintf("Function %s", function.Name))
		} else {
			description = *function.Description
		}
		schema, err := toGeminiSchema(function.Parameters)
		if err != nil {
			return nil, err
		}
		geminiFunctions[i] = &genai.FunctionDeclaration{
			Name:        function.Name,
			Description: description,
			Parameters:  schema,
		}
	}
	return []*genai.Tool{{FunctionDeclarations: geminiFunctions}}, nil
}

func toGeminiToolConfigFromFunctions(functionCall *openai.LegacyFunctionChoice) (*genai.ToolConfig, error) {
	if functionCall == nil {
		return nil, nil
	}
	if functionCall.Value != nil {
		switch *functionCall.Value {
		case "auto":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAuto,
				},
			}, nil
		case "any":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeAny,
				},
			}, nil
		case "none":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingConfigModeNone,
				},
			}, nil
		}
	}
	if functionCall.Function == nil {
		return nil, fmt.Errorf("function_call field must be set to either 'auto', 'any', 'none', or an object with a function name")
	}
	return &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode:                 genai.FunctionCallingConfigModeAny,
			AllowedFunctionNames: []string{functionCall.Function.Name},
		},
	}, nil
}

func toGeminiSchema(schema *orderedmap.Map) (*genai.Schema, error) {
	if schema == nil {
		return nil, nil
	}
	definitions := extractDefinitions(schema)
	return toGeminiSchemaObject(schema, definitions)
}

func toGeminiSchemaObject(schema *orderedmap.Map, definitions map[string]*orderedmap.Map) (*genai.Schema, error) {
	geminiSchema := &genai.Schema{}
	for _, entry := range schema.Entries() {
		switch strings.ToLower(entry.Key) {
		case "$ref":
			refSchema, err := resolveRef(entry.Value.(string), definitions)
			if err != nil {
				return nil, err
			}
			return toGeminiSchemaObject(refSchema, definitions)
		case "type":
			geminiType, err := toGeminiType(entry.Value.(string))
			if err != nil {
				return nil, err
			}
			geminiSchema.Type = geminiType
		case "format":
			geminiSchema.Format = entry.Value.(string)
		case "description":
			geminiSchema.Description = entry.Value.(string)
		case "nullable":
			geminiSchema.Nullable = utils.ToPtr(entry.Value.(bool))
		case "enum":
			openaiEnum := entry.Value.([]any)
			geminiSchema.Enum = make([]string, len(openaiEnum))
			for i, enumValue := range openaiEnum {
				geminiSchema.Enum[i] = enumValue.(string)
			}
		case "items":
			arraySchema, err := toGeminiSchemaObject(entry.Value.(*orderedmap.Map), definitions)
			if err != nil {
				return nil, err
			}
			geminiSchema.Items = arraySchema
		case "properties":
			properties := make(map[string]*genai.Schema)
			orderedProperties := entry.Value.(*orderedmap.Map)
			for _, propEntry := range orderedProperties.Entries() {
				propValue := propEntry.Value.(*orderedmap.Map)
				propSchema, err := toGeminiSchemaObject(propValue, definitions)
				if err != nil {
					return nil, err
				}
				properties[propEntry.Key] = propSchema
			}
			geminiSchema.Properties = properties
		case "required":
			openaiRequired := entry.Value.([]any)
			geminiSchema.Required = make([]string, len(openaiRequired))
			for i, requiredValue := range openaiRequired {
				geminiSchema.Required[i] = requiredValue.(string)
			}
		default:
			// Ignores unknown keys since Gemini doesn't support them.
			continue
		}
	}
	return geminiSchema, nil
}

func extractDefinitions(schema *orderedmap.Map) map[string]*orderedmap.Map {
	definitions := make(map[string]*orderedmap.Map)
	defs, exists := schema.Get("$defs")
	if !exists {
		return definitions
	}
	defsMap, ok := defs.(*orderedmap.Map)
	if !ok {
		return definitions
	}

	for _, entry := range defsMap.Entries() {
		if defSchema, ok := entry.Value.(*orderedmap.Map); ok {
			definitions[entry.Key] = defSchema
		}
	}
	return definitions
}

func resolveRef(ref string, definitions map[string]*orderedmap.Map) (*orderedmap.Map, error) {
	parts := strings.Split(strings.TrimPrefix(ref, "#/$defs/"), "/")
	if len(parts) != 1 {
		return nil, fmt.Errorf("invalid $ref: %s", ref)
	}
	schema, exists := definitions[parts[0]]
	if !exists {
		return nil, fmt.Errorf("failed to resolve $ref: %s", ref)
	}
	return schema, nil
}

func toGeminiType(openAiType string) (genai.Type, error) {
	switch strings.ToLower(openAiType) {
	case "string":
		return genai.TypeString, nil
	case "number":
		return genai.TypeNumber, nil
	case "integer":
		return genai.TypeInteger, nil
	case "boolean":
		return genai.TypeBoolean, nil
	case "array":
		return genai.TypeArray, nil
	case "object":
		return genai.TypeObject, nil
	}
	return genai.TypeUnspecified, fmt.Errorf("unsupported type: %s", openAiType)
}

func toOpenAiResponse(geminiResponse *genai.GenerateContentResponse) (*openai.ChatCompletionResponse, error) {
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
		switch {
		case part.Text != "":
			parts = append(parts, openai.Part{
				Type: "text",
				Content: openai.Content{
					TextContent: &openai.TextContent{
						Text: strings.TrimSpace(fmt.Sprintf("%s", part.Text)),
					},
				},
			})
		case part.FunctionCall != nil:
			jsonString, err := utils.MapToJson(part.FunctionCall.Args)
			if err != nil {
				return nil, err
			}
			toolCalls = append(toolCalls, openai.ToolCall{
				Id:   fmt.Sprintf("tool-%s-%d-%d", part.FunctionCall.Name, index, partIndex),
				Type: "function",
				Function: &openai.FunctionCall{
					Name:      part.FunctionCall.Name,
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
	switch {
	case part.Text != "":
		return openai.Part{
			Content: openai.Content{
				TextContent: &openai.TextContent{
					Text: strings.TrimSpace(fmt.Sprintf("%s", part.Text)),
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
