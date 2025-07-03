package tests

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yanolja/ogem/openai"
)

func defaultPtr[T any](v T) *T { return &v }

func TestOpenAI_Messages_UserRole(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("Hello, OpenAI!")},
		},
	}
	b, err := json.Marshal(messages)
	assert.NoError(t, err)
	var unmarshaled []openai.Message
	assert.NoError(t, json.Unmarshal(b, &unmarshaled))
	assert.Equal(t, "user", unmarshaled[0].Role)
	assert.Equal(t, "Hello, OpenAI!", *unmarshaled[0].Content.String)
}

func TestOpenAI_Messages_UserAssistantRoles(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("Hi!")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: defaultPtr("Hello! How can I help?")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("Tell me a joke.")},
		},
	}
	b, err := json.Marshal(messages)
	assert.NoError(t, err)
	var unmarshaled []openai.Message
	assert.NoError(t, json.Unmarshal(b, &unmarshaled))
	assert.Equal(t, "user", unmarshaled[2].Role)
	assert.Equal(t, "Tell me a joke.", *unmarshaled[2].Content.String)
}

func TestOpenAI_Messages_FunctionCall(t *testing.T) {
	functionName := "get_weather"
	functionArgs := `{"location":"Seoul"}`
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("What's the weather in Seoul?")},
		},
		{
			Role: "assistant",
			ToolCalls: []openai.ToolCall{
				{
					Id:   "call1",
					Type: "function",
					Function: &openai.FunctionCall{
						Name:      functionName,
						Arguments: functionArgs,
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    &openai.MessageContent{String: defaultPtr(`{"temperature":25}`)},
			ToolCallId: defaultPtr("call1"),
		},
	}
	b, err := json.Marshal(messages)
	assert.NoError(t, err)
	var unmarshaled []openai.Message
	assert.NoError(t, json.Unmarshal(b, &unmarshaled))
	assert.Equal(t, "tool", unmarshaled[2].Role)
	assert.Equal(t, "call1", *unmarshaled[2].ToolCallId)
	assert.Equal(t, `{"temperature":25}`, *unmarshaled[2].Content.String)
}

func TestOpenAI_Messages_ToolCall(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("Use the tool, please.")},
		},
		{
			Role: "assistant",
			ToolCalls: []openai.ToolCall{
				{
					Id:   "tool-1",
					Type: "function",
					Function: &openai.FunctionCall{
						Name:      "tool_func",
						Arguments: `{"foo":"bar"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    &openai.MessageContent{String: defaultPtr(`{"result":"ok"}`)},
			ToolCallId: defaultPtr("tool-1"),
		},
	}
	b, err := json.Marshal(messages)
	assert.NoError(t, err)
	var unmarshaled []openai.Message
	assert.NoError(t, json.Unmarshal(b, &unmarshaled))
	assert.Equal(t, "tool_func", unmarshaled[1].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"foo":"bar"}`, unmarshaled[1].ToolCalls[0].Function.Arguments)
	assert.Equal(t, `{"result":"ok"}`, *unmarshaled[2].Content.String)
}

func TestOpenAI_Messages_ChatHistory(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("Hi!")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: defaultPtr("Hello!")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("How are you?")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: defaultPtr("I'm good, thanks!")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("What's the weather?")},
		},
	}
	b, err := json.Marshal(messages)
	assert.NoError(t, err)
	var unmarshaled []openai.Message
	assert.NoError(t, json.Unmarshal(b, &unmarshaled))

	expectedRoles := []string{"user", "assistant", "user", "assistant", "user"}
	expectedContents := []string{"Hi!", "Hello!", "How are you?", "I'm good, thanks!", "What's the weather?"}
	for i, msg := range unmarshaled {
		assert.Equal(t, expectedRoles[i], msg.Role, "Role at index %d", i)
		assert.Equal(t, expectedContents[i], *msg.Content.String, "Content at index %d", i)
	}
}

func TestOpenAI_Messages_CombinedChatHistory(t *testing.T) {
	history1 := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("What's the capital of France?")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: defaultPtr("Let me check that for you.")},
		},
		{
			Role: "assistant",
			ToolCalls: []openai.ToolCall{
				{
					Id:   "func-1",
					Type: "function",
					Function: &openai.FunctionCall{
						Name:      "get_capital",
						Arguments: `{"country":"France"}`,
					},
				},
			},
		},
	}
	history2 := []openai.Message{
		{
			Role:       "tool",
			Content:    &openai.MessageContent{String: defaultPtr(`{"capital":"Paris"}`)},
			ToolCallId: defaultPtr("func-1"),
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: defaultPtr("Thank you!")},
		},
	}
	combined := append(history1, history2...)
	b, err := json.Marshal(combined)
	assert.NoError(t, err)
	var unmarshaled []openai.Message
	assert.NoError(t, json.Unmarshal(b, &unmarshaled))
	assert.Equal(t, 5, len(unmarshaled))
	assert.Equal(t, "tool", unmarshaled[3].Role)
	assert.Equal(t, `{"capital":"Paris"}`, *unmarshaled[3].Content.String)
	assert.Equal(t, "user", unmarshaled[4].Role)
	assert.Equal(t, "Thank you!", *unmarshaled[4].Content.String)
}
