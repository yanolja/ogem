package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
)

func TestPart_UnmarshalJSON(t *testing.T) {
	t.Run("valid text content", func(t *testing.T) {
		input := `{"type": "text", "content": {"text": "hello world"}}`
		want := openai.Part{
			Type: "text",
			Content: openai.Content{
				TextContent: &openai.TextContent{Text: "hello world"},
			},
		}

		var got openai.Part
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.NotNil(t, got.Content.TextContent)
		assert.Equal(t, want.Content.TextContent.Text, got.Content.TextContent.Text)
		assert.Nil(t, got.Content.ImageContent)
	})

	t.Run("valid image content", func(t *testing.T) {
		input := `{"type": "image", "content": {"url": "http://example.com/image.jpg", "detail": "high"}}`
		want := openai.Part{
			Type: "image",
			Content: openai.Content{
				ImageContent: &openai.ImageContent{Url: "http://example.com/image.jpg", Detail: "high"},
			},
		}

		var got openai.Part
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.NotNil(t, got.Content.ImageContent)
		assert.Equal(t, want.Content.ImageContent.Url, got.Content.ImageContent.Url)
		assert.Equal(t, want.Content.ImageContent.Detail, got.Content.ImageContent.Detail)
		assert.Nil(t, got.Content.TextContent)
	})

	t.Run("valid text content with extra unknown field", func(t *testing.T) {
		input := `{"type": "text", "content": {"text": "hello world", "url": "http://example.com/image.jpg", "unknown": "unknown"}}`
		want := openai.Part{
			Type: "text",
			Content: openai.Content{
				TextContent: &openai.TextContent{Text: "hello world"},
			},
		}

		var got openai.Part
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.NotNil(t, got.Content.TextContent)
		assert.Equal(t, want.Content.TextContent.Text, got.Content.TextContent.Text)
		assert.Nil(t, got.Content.ImageContent)
	})

	t.Run("valid image content with extra unknown field", func(t *testing.T) {
		input := `{"type": "image", "content": {"url": "http://example.com/image.jpg", "detail": "high", "text": "this is a test", "unknown": "unknown"}}`
		want := openai.Part{
			Type: "image",
			Content: openai.Content{
				ImageContent: &openai.ImageContent{Url: "http://example.com/image.jpg", Detail: "high"},
			},
		}

		var got openai.Part
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.NotNil(t, got.Content.ImageContent)
		assert.Equal(t, want.Content.ImageContent.Url, got.Content.ImageContent.Url)
		assert.Equal(t, want.Content.ImageContent.Detail, got.Content.ImageContent.Detail)
		assert.Nil(t, got.Content.TextContent)
	})

	t.Run("invalid type", func(t *testing.T) {
		input := `{"type": "unknown", "content": {}}`
		var got openai.Part
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"type": "text", "content": }`
		var got openai.Part
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})
}

func TestTextContent_UnmarshalJSON(t *testing.T) {
	t.Run("valid text content", func(t *testing.T) {
		input := `{"text": "hello world"}`
		want := openai.TextContent{Text: "hello world"}

		var got openai.TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Text, got.Text)
	})

	t.Run("valid text content with extra unknown field", func(t *testing.T) {
		input := `{"text": "hello world", "url": "http://example.com/image.jpg", "unknown": "unknown"}`
		want := openai.TextContent{Text: "hello world"}

		var got openai.TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Text, got.Text)
	})

	t.Run("empty text", func(t *testing.T) {
		input := `{"text": ""}`
		var got openai.TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("missing text field", func(t *testing.T) {
		input := `{}`
		var got openai.TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"text": }`
		var got openai.TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})
}

func TestImageContent_UnmarshalJSON(t *testing.T) {
	t.Run("valid image content", func(t *testing.T) {
		input := `{"url": "http://example.com/image.jpg", "detail": "high"}`
		want := openai.ImageContent{Url: "http://example.com/image.jpg", Detail: "high"}

		var got openai.ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Url, got.Url)
		assert.Equal(t, want.Detail, got.Detail)
	})

	t.Run("valid image content with extra unknown field", func(t *testing.T) {
		input := `{"url": "http://example.com/image.jpg", "detail": "high", "text": "this is a test", "unknown": "unknown"}`
		want := openai.ImageContent{Url: "http://example.com/image.jpg", Detail: "high"}

		var got openai.ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Url, got.Url)
		assert.Equal(t, want.Detail, got.Detail)
	})

	t.Run("empty url", func(t *testing.T) {
		input := `{"url": "", "detail": "high"}`
		var got openai.ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("missing url field", func(t *testing.T) {
		input := `{"detail": "high"}`
		var got openai.ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"url": "http://example.com/image.jpg", "detail": }`
		var got openai.ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})
}

func TestChatCompletionRequest_MarshalJSON(t *testing.T) {
	t.Run("user and assistant messages", func(t *testing.T) {
		req := openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{Role: "user", Content: &openai.MessageContent{String: ptr("Hello, OpenAI!")}},
				{Role: "assistant", Content: &openai.MessageContent{String: ptr("Hi! How can I help?")}},
			},
		}
		data, err := json.Marshal(req)
		assert.NoError(t, err)
		var got map[string]any
		json.Unmarshal(data, &got)
		assert.Equal(t, "gpt-3.5-turbo", got["model"])
		msgs, ok := got["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 2)
		m0 := msgs[0].(map[string]any)
		assert.Equal(t, "user", m0["role"])
		assert.Equal(t, "Hello, OpenAI!", m0["content"])
		m1 := msgs[1].(map[string]any)
		assert.Equal(t, "assistant", m1["role"])
		assert.Equal(t, "Hi! How can I help?", m1["content"])
	})

	t.Run("tool call and tool response", func(t *testing.T) {
		callID := "call-1"
		functionName := "get_weather"
		functionArgs := `{"location":"Seoul"}`
		toolResult := `{"temperature":25}`
		req := openai.ChatCompletionRequest{
			Model: "gpt-3.5-turbo",
			Messages: []openai.Message{
				{Role: "user", Content: &openai.MessageContent{String: ptr("What's the weather in Seoul?")}},
				{Role: "assistant", ToolCalls: []openai.ToolCall{{Id: callID, Type: "function", Function: &openai.FunctionCall{Name: functionName, Arguments: functionArgs}}}},
				{Role: "tool", Content: &openai.MessageContent{String: ptr(toolResult)}, ToolCallId: &callID},
			},
		}
		data, err := json.Marshal(req)
		assert.NoError(t, err)
		var got map[string]any
		json.Unmarshal(data, &got)
		msgs, ok := got["messages"].([]any)
		assert.True(t, ok)
		assert.Len(t, msgs, 3)
		m1 := msgs[1].(map[string]any)
		assert.Equal(t, "assistant", m1["role"])
		calls, ok := m1["tool_calls"].([]any)
		assert.True(t, ok)
		assert.Len(t, calls, 1)
		call := calls[0].(map[string]any)
		assert.Equal(t, callID, call["id"])
		assert.Equal(t, "function", call["type"])
		fn := call["function"].(map[string]any)
		assert.Equal(t, functionName, fn["name"])
		assert.Equal(t, functionArgs, fn["arguments"])
		m2 := msgs[2].(map[string]any)
		assert.Equal(t, "tool", m2["role"])
		assert.Equal(t, toolResult, m2["content"])
		assert.Equal(t, callID, m2["tool_call_id"])
	})
}

func ptr[T any](v T) *T { return &v }
