// This test file verifies the correctness of the message conversion logic for the Studio provider in the OGEM project.
// It ensures that OpenAI-style messages are accurately transformed into the format expected by Studio, including handling of user/assistant roles, tool calls, tool results, and chat history.
//
// Usage:
// 1. Ensure Go is installed and dependencies are resolved.
// 2. Run tests with: go test ./provider/studio/msg_converting_test.go
//    Or run all Studio provider tests with: go test ./provider/studio/
// 3. All tests should pass, confirming correct message conversion logic.
//
// Coverage:
// - User and assistant message conversion
// - Tool call and tool result handling
// - Chat history and message order preservation

package studio

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yanolja/ogem/openai"
)

func TestGeminiMessages_UserRole(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Hello, Gemini!")},
		},
	}
	history, last, err := toGeminiMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, history, 0)
	assert.NotNil(t, last)
	assert.Equal(t, "user", last.Role)
	assert.Equal(t, "Hello, Gemini!", last.Parts[0].Text)
}

func TestGeminiMessages_UserAssistantRoles(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Hi!")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: ptr("Hello! How can I help?")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Tell me a joke.")},
		},
	}
	history, last, err := toGeminiMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "user", last.Role)
	assert.Equal(t, "Tell me a joke.", last.Parts[0].Text)
}

func TestGeminiMessages_FunctionCall(t *testing.T) {
	functionName := "get_weather"
	functionArgs := `{"location":"Seoul"}`
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("What's the weather in Seoul?")},
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
			Content:    &openai.MessageContent{String: ptr(`{"temperature":25}`)},
			ToolCallId: ptr("call1"),
		},
	}
	history, last, err := toGeminiMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "tool", messages[2].Role)
	assert.NotNil(t, last.Parts[0].FunctionResponse)
	assert.Equal(t, functionName, last.Parts[0].FunctionResponse.Name)
	assert.Equal(t, float64(25), last.Parts[0].FunctionResponse.Response["temperature"])
}

func TestGeminiMessages_ToolCall(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Use the tool, please.")},
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
			Content:    &openai.MessageContent{String: ptr(`{"result":"ok"}`)},
			ToolCallId: ptr("tool-1"),
		},
	}
	history, last, err := toGeminiMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.NotNil(t, last.Parts[0].FunctionResponse)
	assert.Equal(t, "tool_func", last.Parts[0].FunctionResponse.Name)
	assert.Equal(t, "ok", last.Parts[0].FunctionResponse.Response["result"])
}

func TestGeminiMessages_ChatHistory(t *testing.T) {
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Hi!")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: ptr("Hello!")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("How are you?")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: ptr("I'm good, thanks!")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("What's the weather?")},
		},
	}
	history, last, err := toGeminiMessages(messages)
	assert.NoError(t, err)
	assert.Len(t, history, 4)
	assert.Equal(t, "user", last.Role)
	assert.Equal(t, "What's the weather?", last.Parts[0].Text)

	expectedRoles := []string{"user", "model", "user", "model"}
	expectedContents := []string{"Hi!", "Hello!", "How are you?", "I'm good, thanks!"}
	for i, msg := range history {
		assert.Equal(t, expectedRoles[i], msg.Role, "Role at index %d", i)
		assert.Equal(t, expectedContents[i], msg.Parts[0].Text, "Content at index %d", i)
	}
}

func TestGeminiMessages_CombinedChatHistory(t *testing.T) {
	// First part of the conversation
	history1 := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("What's the capital of France?")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: ptr("Let me check that for you.")},
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

	// Second part: tool/function response and user follow-up
	history2 := []openai.Message{
		{
			Role:       "tool",
			Content:    &openai.MessageContent{String: ptr(`{"capital":"Paris"}`)},
			ToolCallId: ptr("func-1"),
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Thank you!")},
		},
	}

	// Combine histories
	combined := append(history1, history2...)

	history, last, err := toGeminiMessages(combined)
	assert.NoError(t, err)
	assert.Len(t, history, 4)

	// Check order and content
	expectedRoles := []string{"user", "model", "model", "function"}
	expectedContents := []string{
		"What's the capital of France?",
		"Let me check that for you.",
		"", // Tool call, no direct text content
		"", // Tool response, content is in FunctionResponse
	}
	for i, msg := range history {
		assert.Equal(t, expectedRoles[i], msg.Role, "Role at index %d", i)
		if msg.Role == "model" && len(msg.Parts) > 0 && msg.Parts[0].Text != "" {
			assert.Equal(t, expectedContents[i], msg.Parts[0].Text, "Content at index %d", i)
		}
	}

	// Check the last message (user follow-up)
	assert.Equal(t, "user", last.Role)
	assert.Equal(t, "Thank you!", last.Parts[0].Text)
}

func ptr[T any](v T) *T {
	return &v
}
