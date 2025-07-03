package tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/provider/claude"
	"github.com/yanolja/ogem/utils"
)

func TestClaudeMessages_UserRole(t *testing.T) {
	ctx := context.Background()
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("Hello, Claude!")},
		},
	}
	ep, _ := claude.NewEndpoint("test-key")
	claudeMsgs, err := ep.ToClaudeMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, claudeMsgs, 1)
	assert.Equal(t, "user", string(claudeMsgs[0].Role))
	assert.Equal(t, "Hello, Claude!", claudeMsgs[0].Content[0].OfText.Text)
}

func TestClaudeMessages_UserAssistantRoles(t *testing.T) {
	ctx := context.Background()
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("Hi!")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: utils.ToPtr("Hello! How can I help?")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("Tell me a joke.")},
		},
	}
	ep, _ := claude.NewEndpoint("test-key")
	claudeMsgs, err := ep.ToClaudeMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, claudeMsgs, 3)
	assert.Equal(t, "user", string(claudeMsgs[2].Role))
	assert.Equal(t, "Tell me a joke.", claudeMsgs[2].Content[0].OfText.Text)
}

func TestClaudeMessages_ToolCall(t *testing.T) {
	ctx := context.Background()
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("Use the tool, please.")},
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
			Content:    &openai.MessageContent{String: utils.ToPtr(`{"result":"ok"}`)},
			ToolCallId: utils.ToPtr("tool-1"),
		},
	}
	ep, _ := claude.NewEndpoint("test-key")
	claudeMsgs, err := ep.ToClaudeMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, claudeMsgs, 3)
	// Check tool call block
	assert.Equal(t, "tool-1", claudeMsgs[1].Content[0].OfToolUse.ID)
	assert.Equal(t, "tool_func", claudeMsgs[1].Content[0].OfToolUse.Name)
	// Check tool result block
	assert.Equal(t, "tool-1", claudeMsgs[2].Content[0].OfToolResult.ToolUseID)
	assert.Equal(t, `{"result":"ok"}`, claudeMsgs[2].Content[0].OfToolResult.Content[0].OfText.Text)
}

func TestClaudeMessages_ChatHistory(t *testing.T) {
	ctx := context.Background()
	messages := []openai.Message{
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("Hi!")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: utils.ToPtr("Hello!")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("How are you?")},
		},
		{
			Role:    "assistant",
			Content: &openai.MessageContent{String: utils.ToPtr("I'm good, thanks!")},
		},
		{
			Role:    "user",
			Content: &openai.MessageContent{String: utils.ToPtr("What's the weather?")},
		},
	}
	ep, _ := claude.NewEndpoint("test-key")
	claudeMsgs, err := ep.ToClaudeMessagesTest(ctx, messages)
	assert.NoError(t, err)
	assert.Len(t, claudeMsgs, 5)
	roles := []string{"user", "assistant", "user", "assistant", "user"}
	contents := []string{"Hi!", "Hello!", "How are you?", "I'm good, thanks!", "What's the weather?"}
	for i, msg := range claudeMsgs {
		assert.Equal(t, roles[i], string(msg.Role), "Role at index %d", i)
		assert.Equal(t, contents[i], msg.Content[0].OfText.Text, "Content at index %d", i)
	}
}
