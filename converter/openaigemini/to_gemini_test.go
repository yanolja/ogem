package openaigemini

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
	"github.com/yanolja/ogem/utils/orderedmap"
)

func TestToGeminiSchema(t *testing.T) {
	t.Run("Simple object schema", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"name": {"type": "string"},
				"age": {"type": "integer"}
			},
			"required": ["name"]
		}`

		expected := &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"name": {Type: genai.TypeString},
				"age":  {Type: genai.TypeInteger},
			},
			Required: []string{"name"},
		}

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err, "Failed to unmarshal input JSON")

		result, err := toGeminiSchema(&schema)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Nested object schema", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"user": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"email": {"type": "string", "format": "email"}
					}
				},
				"items": {
					"type": "array",
					"items": {"type": "string"}
				}
			}
		}`

		expected := &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"user": {
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"name":  {Type: genai.TypeString},
						"email": {Type: genai.TypeString, Format: "email"},
					},
				},
				"items": {
					Type:  genai.TypeArray,
					Items: &genai.Schema{Type: genai.TypeString},
				},
			},
		}

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Schema with enum", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"color": {
					"type": "string",
					"enum": ["red", "green", "blue"]
				}
			}
		}`

		expected := &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"color": {
					Type: genai.TypeString,
					Enum: []string{"red", "green", "blue"},
				},
			},
		}

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Schema with description and nullable", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"description": {
					"type": "string",
					"description": "A brief description",
					"nullable": true
				}
			}
		}`

		expected := &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"description": {
					Type:        genai.TypeString,
					Description: "A brief description",
					Nullable:    true,
				},
			},
		}

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Invalid schema", func(t *testing.T) {
		input := `{"type": "invalid"}`

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.Error(t, err)
		assert.Nil(t, result)
	})

	t.Run("Nil input", func(t *testing.T) {
		result, err := toGeminiSchema(nil)
		assert.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("Simple $ref", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"user": {"$ref": "#/$defs/User"}
			},
			"$defs": {
				"User": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"age": {"type": "integer"}
					}
				}
			}
		}`

		expected := &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"user": {
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"name": {Type: genai.TypeString},
						"age":  {Type: genai.TypeInteger},
					},
				},
			},
		}

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Nested $ref", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"user": {"$ref": "#/$defs/User"}
			},
			"$defs": {
				"User": {
					"type": "object",
					"properties": {
						"name": {"type": "string"},
						"address": {"$ref": "#/$defs/Address"}
					}
				},
				"Address": {
					"type": "object",
					"properties": {
						"street": {"type": "string"},
						"city": {"type": "string"}
					}
				}
			}
		}`

		expected := &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"user": {
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"name": {Type: genai.TypeString},
						"address": {
							Type: genai.TypeObject,
							Properties: map[string]*genai.Schema{
								"street": {Type: genai.TypeString},
								"city":   {Type: genai.TypeString},
							},
						},
					},
				},
			},
		}

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Invalid $ref", func(t *testing.T) {
		input := `{
			"type": "object",
			"properties": {
				"user": {"$ref": "#/$defs/NonExistentUser"}
			},
			"$defs": {
				"User": {
					"type": "object",
					"properties": {
						"name": {"type": "string"}
					}
				}
			}
		}`

		var schema orderedmap.Map
		err := json.Unmarshal([]byte(input), &schema)
		assert.NoError(t, err)

		result, err := toGeminiSchema(&schema)
		assert.Error(t, err)
		assert.Nil(t, result)
	})
}

func TestGetModelFromOpenAiRequest(t *testing.T) {
	client, _ := genai.NewClient(context.Background())

	t.Run("basic configuration", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model:       "gemini-pro",
			Temperature: utils.ToPtr(float32(0.7)),
			TopP:        utils.ToPtr(float32(0.9)),
			MaxTokens:   utils.ToPtr(int32(100)),
		}

		model, err := GetModelFromOpenAiRequest(client, req)
		assert.NoError(t, err)
		assert.NotNil(t, model)
		assert.Equal(t, float32(0.7), *model.Temperature)
		assert.Equal(t, float32(0.9), *model.TopP)
		assert.Equal(t, utils.ToPtr(int32(100)), model.MaxOutputTokens)
	})

	t.Run("with system message", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model: "gemini-pro",
			Messages: []openai.Message{
				{
					Role:    "system",
					Content: &openai.MessageContent{String: utils.ToPtr("You are a helpful assistant")},
				},
			},
		}

		model, err := GetModelFromOpenAiRequest(client, req)
		assert.NoError(t, err)
		assert.NotNil(t, model)
		assert.NotNil(t, model.SystemInstruction)
		assert.Equal(t, "You are a helpful assistant", string(model.SystemInstruction.Parts[0].(genai.Text)))
	})

	t.Run("with stop sequences", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model: "gemini-pro",
			StopSequences: &openai.StopSequences{
				Sequences: []string{"STOP", "END"},
			},
		}

		model, err := GetModelFromOpenAiRequest(client, req)
		assert.NoError(t, err)
		assert.NotNil(t, model)
		assert.Equal(t, []string{"STOP", "END"}, model.StopSequences)
	})

	t.Run("with max completion tokens", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model:               "gemini-pro",
			MaxCompletionTokens: utils.ToPtr(int32(200)),
		}

		model, err := GetModelFromOpenAiRequest(client, req)
		assert.NoError(t, err)
		assert.NotNil(t, model)
		assert.Equal(t, int32(200), *model.MaxOutputTokens)
	})

	t.Run("invalid candidate count", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model:          "gemini-pro",
			CandidateCount: utils.ToPtr(int32(2)),
		}

		model, err := GetModelFromOpenAiRequest(client, req)
		assert.Error(t, err)
		assert.Nil(t, model)
		assert.Contains(t, err.Error(), "unsupported candidate count")
	})

	t.Run("conflicting functions and tools", func(t *testing.T) {
		req := &openai.ChatCompletionRequest{
			Model: "gemini-pro",
			Functions: []openai.LegacyFunction{
				{Name: "test_function"},
			},
			Tools: []openai.Tool{
				{Type: "function"},
			},
		}

		model, err := GetModelFromOpenAiRequest(client, req)
		assert.Error(t, err)
		assert.Nil(t, model)
		assert.Contains(t, err.Error(), "functions and tools are mutually exclusive")
	})
}

func TestToGeminiRequest(t *testing.T) {
	t.Run("empty messages", func(t *testing.T) {
		history, last, err := ToGeminiRequest([]openai.Message{})
		assert.NoError(t, err)
		assert.Nil(t, history)
		assert.Nil(t, last)
	})

	t.Run("basic conversation", func(t *testing.T) {
		messages := []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: utils.ToPtr("Hello")},
			},
			{
				Role:    "assistant",
				Content: &openai.MessageContent{String: utils.ToPtr("Hi there!")},
			},
		}

		history, last, err := ToGeminiRequest(messages)
		assert.NoError(t, err)
		assert.Len(t, history, 1)
		assert.Equal(t, "user", history[0].Role)
		assert.Equal(t, "Hello", string(history[0].Parts[0].(genai.Text)))
		assert.Equal(t, "model", last.Role)
		assert.Equal(t, "Hi there!", string(last.Parts[0].(genai.Text)))
	})

	t.Run("with system message", func(t *testing.T) {
		messages := []openai.Message{
			{
				Role:    "system",
				Content: &openai.MessageContent{String: utils.ToPtr("You are helpful")},
			},
			{
				Role:    "user",
				Content: &openai.MessageContent{String: utils.ToPtr("Hi")},
			},
		}

		history, last, err := ToGeminiRequest(messages)
		assert.NoError(t, err)
		assert.Empty(t, history)
		assert.Equal(t, "user", last.Role)
		assert.Equal(t, "Hi", string(last.Parts[0].(genai.Text)))
	})

	t.Run("with function call", func(t *testing.T) {
		messages := []openai.Message{
			{
				Role: "assistant",
				FunctionCall: &openai.FunctionCall{
					Name:      "test_function",
					Arguments: `{"key": "value"}`,
				},
			},
		}

		history, last, err := ToGeminiRequest(messages)
		assert.NoError(t, err)
		assert.Empty(t, history)
		assert.Equal(t, "model", last.Role)
		functionCall := last.Parts[0].(*genai.FunctionCall)
		assert.Equal(t, "test_function", functionCall.Name)
	})

	t.Run("invalid tool response JSON", func(t *testing.T) {
		messages := []openai.Message{
			{
				Role:       "tool",
				ToolCallId: utils.ToPtr("123"),
				Content:    &openai.MessageContent{String: utils.ToPtr("invalid json")},
			},
		}

		history, last, err := ToGeminiRequest(messages)
		assert.Error(t, err)
		assert.Nil(t, history)
		assert.Nil(t, last)
		assert.Contains(t, err.Error(), "tool response must be a valid JSON object")
	})

	t.Run("missing tool call ID", func(t *testing.T) {
		messages := []openai.Message{
			{
				Role:       "tool",
				Content:    &openai.MessageContent{String: utils.ToPtr(`{"result": "test"}`)},
				ToolCallId: utils.ToPtr("nonexistent"),
			},
		}

		history, last, err := ToGeminiRequest(messages)
		assert.Error(t, err)
		assert.Nil(t, history)
		assert.Nil(t, last)
		assert.Contains(t, err.Error(), "tool call ID")
	})

	t.Run("invalid function call arguments", func(t *testing.T) {
		messages := []openai.Message{
			{
				Role: "assistant",
				FunctionCall: &openai.FunctionCall{
					Name:      "test_function",
					Arguments: "invalid json",
				},
			},
		}

		history, last, err := ToGeminiRequest(messages)
		assert.Error(t, err)
		assert.Nil(t, history)
		assert.Nil(t, last)
	})
}
