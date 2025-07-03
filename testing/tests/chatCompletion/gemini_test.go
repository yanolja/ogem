// Test file for Gemini models - /v1/chat/completions endpoint
//
// Usage:
//   - This test suite covers all manually validated Gemini models for the /v1/chat/completions endpoint.
//   - Models are selected via a two-level map in the test file, not dynamically loaded from config.yaml.
//   - Only models explicitly listed and validated in this file are tested.
//   - To add or remove models, update the model list in this file.
//
// Setup:
//   - Place a .env file in the parent folder of this test file (gemini/.env).
//   - The .env file must contain:
//       OGEM_API_KEY=<your api key>
//       OGEM_BASE_URL=<URL to server>
//
// Example usage:
//   go test -v -run TestChatCompletion_UserContext ./provider/openai/ -provider_region=gemini/gemini
//
// To test all providers at once, run the corresponding test files for each provider.

package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider/studio"
)

func TestGeminiMessages_UserRole(t *testing.T) {
	ctx := context.Background()
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: ptr("Hello, Gemini!")},
		},
	}
	ep := &studio.Endpoint{}
	history, last, err := ep.ToGeminiMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, history, 0)
	assert.NotNil(t, last)
	assert.Equal(t, "user", last.Role)
	assert.Equal(t, "Hello, Gemini!", last.Parts[0].Text)
}

func TestGeminiMessages_UserAssistantRoles(t *testing.T) {
	ctx := context.Background()
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
	ep := &studio.Endpoint{}
	history, last, err := ep.ToGeminiMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "user", last.Role)
	assert.Equal(t, "Tell me a joke.", last.Parts[0].Text)
}

func TestGeminiMessages_FunctionCall(t *testing.T) {
	ctx := context.Background()
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
	ep := &studio.Endpoint{}
	history, last, err := ep.ToGeminiMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.Equal(t, "tool", messages[2].Role)
	assert.NotNil(t, last.Parts[0].FunctionResponse)
	assert.Equal(t, functionName, last.Parts[0].FunctionResponse.Name)
	assert.Equal(t, float64(25), last.Parts[0].FunctionResponse.Response["temperature"])
}

func TestGeminiMessages_ToolCall(t *testing.T) {
	ctx := context.Background()
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
	ep := &studio.Endpoint{}
	history, last, err := ep.ToGeminiMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, history, 2)
	assert.NotNil(t, last.Parts[0].FunctionResponse)
	assert.Equal(t, "tool_func", last.Parts[0].FunctionResponse.Name)
	assert.Equal(t, "ok", last.Parts[0].FunctionResponse.Response["result"])
}

func TestGeminiMessages_ChatHistory(t *testing.T) {
	ctx := context.Background()
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
	ep := &studio.Endpoint{}
	history, last, err := ep.ToGeminiMessagesTest(ctx, messages)
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
	ctx := context.Background()
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

	ep := &studio.Endpoint{}
	history, last, err := ep.ToGeminiMessagesTest(ctx, combined)
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
