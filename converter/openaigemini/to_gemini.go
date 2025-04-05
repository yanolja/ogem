package openaigemini

import (
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/array"
	"github.com/yanolja/ogem/utils/orderedmap"
)

func GetModelFromOpenAiRequest(client *genai.Client, openAiRequest *openai.ChatCompletionRequest) (*genai.GenerativeModel, error) {
	model := client.GenerativeModel(openAiRequest.Model)

	model.SystemInstruction = toGeminiSystemInstruction(openAiRequest)
	model.Temperature = openAiRequest.Temperature
	model.TopP = openAiRequest.TopP

	if openAiRequest.MaxTokens != nil {
		model.MaxOutputTokens = openAiRequest.MaxTokens
	} else if openAiRequest.MaxCompletionTokens != nil {
		model.MaxOutputTokens = openAiRequest.MaxCompletionTokens
	}

	if openAiRequest.CandidateCount != nil && *openAiRequest.CandidateCount != 1 {
		return nil, fmt.Errorf("unsupported candidate count: %d, only 1 is supported", *openAiRequest.CandidateCount)
	}
	model.CandidateCount = openAiRequest.CandidateCount

	if openAiRequest.StopSequences != nil {
		model.StopSequences = openAiRequest.StopSequences.Sequences
	}

	model.SafetySettings = []*genai.SafetySetting{
		{Category: genai.HarmCategoryHarassment, Threshold: genai.HarmBlockNone},
		{Category: genai.HarmCategoryHateSpeech, Threshold: genai.HarmBlockNone},
		{Category: genai.HarmCategorySexuallyExplicit, Threshold: genai.HarmBlockNone},
		{Category: genai.HarmCategoryDangerousContent, Threshold: genai.HarmBlockNone},
	}

	if err := setToolsAndFunctions(model, openAiRequest); err != nil {
		return nil, err
	}

	if err := setResponseFormat(model, openAiRequest); err != nil {
		return nil, err
	}

	return model, nil
}

func ToGeminiRequest(openAiMessages []openai.Message) ([]*genai.Content, *genai.Content, error) {
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

func setToolsAndFunctions(model *genai.GenerativeModel, openAiRequest *openai.ChatCompletionRequest) error {
	hasFunctions := len(openAiRequest.Functions) > 0
	hasTools := len(openAiRequest.Tools) > 0

	if hasFunctions && hasTools {
		return fmt.Errorf("functions and tools are mutually exclusive")
	}

	if hasFunctions {
		tools, err := toGeminiToolsFromFunctions(openAiRequest.Functions)
		if err != nil {
			return err
		}
		model.Tools = tools

		config, err := toGeminiToolConfigFromFunctions(openAiRequest.FunctionCall)
		if err != nil {
			return err
		}
		model.ToolConfig = config
	} else if hasTools {
		tools, err := toGeminiTools(openAiRequest.Tools)
		if err != nil {
			return err
		}
		model.Tools = tools

		config, err := toGeminiToolConfig(openAiRequest.ToolChoice)
		if err != nil {
			return err
		}
		model.ToolConfig = config
	}

	return nil
}

func setResponseFormat(model *genai.GenerativeModel, openAiRequest *openai.ChatCompletionRequest) error {
	mimeType, jsonSchema, err := provider.ToGeminiResponseMimeType(openAiRequest)
	if err != nil {
		return err
	}
	model.ResponseMIMEType = mimeType
	model.ResponseSchema, err = toGeminiSchema(jsonSchema)
	return err
}

func toGeminiSystemInstruction(openAiRequest *openai.ChatCompletionRequest) *genai.Content {
	for _, message := range openAiRequest.Messages {
		if provider.ToGeminiRole(message.Role) == "system" {
			return &genai.Content{
				Parts: []genai.Part{genai.Text(*message.Content.String)},
			}
		}
	}
	return nil
}

func toGeminiParts(message openai.Message, toolMap map[string]string) ([]genai.Part, error) {
	if message.Role == "tool" {
		response, err := utils.JsonToMap(*message.Content.String)
		if err != nil {
			return nil, fmt.Errorf("tool response must be a valid JSON object: %v", err)
		}
		functionName, exists := toolMap[*message.ToolCallId]
		if !exists {
			return nil, fmt.Errorf("tool call ID %s not found in the previous messages", *message.ToolCallId)
		}
		return []genai.Part{&genai.FunctionResponse{
			Name:     functionName,
			Response: response,
		}}, nil
	}
	if message.Role == "function" {
		response, err := utils.JsonToMap(*message.Content.String)
		if err != nil {
			return nil, fmt.Errorf("function response must be a valid JSON object: %v", err)
		}
		return []genai.Part{&genai.FunctionResponse{
			Name:     *message.Name,
			Response: response,
		}}, nil
	}
	if message.Content != nil {
		if message.Content.String != nil {
			return []genai.Part{genai.Text(*message.Content.String)}, nil
		}
		return array.Map(message.Content.Parts, func(part openai.Part) genai.Part {
			if part.Content.TextContent != nil {
				return genai.Text(part.Content.TextContent.Text)
			}
			if part.Content.ImageContent != nil {
				// TODO(seungduk): Implement image downloader and pass it from the main to this provider.
				// It should support cache mechanism using Valkey.
				return genai.Text("image content is not supported yet")
			}
			return genai.Text("unsupported content type")
		}), nil
	}
	if message.Refusal != nil {
		return []genai.Part{genai.Text(*message.Refusal)}, nil
	}
	if message.FunctionCall != nil {
		arguments, err := utils.JsonToMap(message.FunctionCall.Arguments)
		if err != nil {
			return nil, err
		}
		return []genai.Part{&genai.FunctionCall{
			Name: message.FunctionCall.Name,
			Args: arguments,
		}}, nil
	}
	if len(message.ToolCalls) > 0 {
		toolCalls := make([]genai.Part, len(message.ToolCalls))
		for index, toolCall := range message.ToolCalls {
			if toolCall.Type != "function" {
				return nil, fmt.Errorf("unsupported tool call type: %s", toolCall.Type)
			}
			arguments, err := utils.JsonToMap(toolCall.Function.Arguments)
			if err != nil {
				return nil, err
			}
			toolCalls[index] = &genai.FunctionCall{
				Name: toolCall.Function.Name,
				Args: arguments,
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
					Mode: genai.FunctionCallingAuto,
				},
			}, nil
		case "required":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingAny,
				},
			}, nil
		case "none":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingNone,
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
			Mode:                 genai.FunctionCallingAny,
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
					Mode: genai.FunctionCallingAuto,
				},
			}, nil
		case "any":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingAny,
				},
			}, nil
		case "none":
			return &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					Mode: genai.FunctionCallingNone,
				},
			}, nil
		}
	}
	if functionCall.Function == nil {
		return nil, fmt.Errorf("function_call field must be set to either 'auto', 'any', 'none', or an object with a function name")
	}
	return &genai.ToolConfig{
		FunctionCallingConfig: &genai.FunctionCallingConfig{
			Mode:                 genai.FunctionCallingAny,
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
			geminiSchema.Nullable = entry.Value.(bool)
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
