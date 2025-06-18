package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPart_UnmarshalJSON(t *testing.T) {
	t.Run("valid text content", func(t *testing.T) {
		input := `{"type": "text", "content": {"text": "hello world"}}`
		want := Part{
			Type: "text",
			Content: Content{
				TextContent: &TextContent{Text: "hello world"},
			},
		}

		var got Part
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.NotNil(t, got.Content.TextContent)
		assert.Equal(t, want.Content.TextContent.Text, got.Content.TextContent.Text)
		assert.Nil(t, got.Content.ImageContent)
	})

	t.Run("valid image content", func(t *testing.T) {
		input := `{"type": "image", "content": {"url": "http://example.com/image.jpg", "detail": "high"}}`
		want := Part{
			Type: "image",
			Content: Content{
				ImageContent: &ImageContent{Url: "http://example.com/image.jpg", Detail: "high"},
			},
		}

		var got Part
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
		want := Part{
			Type: "text",
			Content: Content{
				TextContent: &TextContent{Text: "hello world"},
			},
		}

		var got Part
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Type, got.Type)
		assert.NotNil(t, got.Content.TextContent)
		assert.Equal(t, want.Content.TextContent.Text, got.Content.TextContent.Text)
		assert.Nil(t, got.Content.ImageContent)
	})

	t.Run("valid image content with extra unknown field", func(t *testing.T) {
		input := `{"type": "image", "content": {"url": "http://example.com/image.jpg", "detail": "high", "text": "this is a test", "unknown": "unknown"}}`
		want := Part{
			Type: "image",
			Content: Content{
				ImageContent: &ImageContent{Url: "http://example.com/image.jpg", Detail: "high"},
			},
		}

		var got Part
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
		var got Part
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"type": "text", "content": }`
		var got Part
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})
}

func TestTextContent_UnmarshalJSON(t *testing.T) {
	t.Run("valid text content", func(t *testing.T) {
		input := `{"text": "hello world"}`
		want := TextContent{Text: "hello world"}

		var got TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Text, got.Text)
	})

	t.Run("valid text content with extra unknown field", func(t *testing.T) {
		input := `{"text": "hello world", "url": "http://example.com/image.jpg", "unknown": "unknown"}`
		want := TextContent{Text: "hello world"}

		var got TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Text, got.Text)
	})

	t.Run("empty text", func(t *testing.T) {
		input := `{"text": ""}`
		var got TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("missing text field", func(t *testing.T) {
		input := `{}`
		var got TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"text": }`
		var got TextContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})
}

func TestImageContent_UnmarshalJSON(t *testing.T) {
	t.Run("valid image content", func(t *testing.T) {
		input := `{"url": "http://example.com/image.jpg", "detail": "high"}`
		want := ImageContent{Url: "http://example.com/image.jpg", Detail: "high"}

		var got ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Url, got.Url)
		assert.Equal(t, want.Detail, got.Detail)
	})

	t.Run("valid image content with extra unknown field", func(t *testing.T) {
		input := `{"url": "http://example.com/image.jpg", "detail": "high", "text": "this is a test", "unknown": "unknown"}`
		want := ImageContent{Url: "http://example.com/image.jpg", Detail: "high"}

		var got ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.NoError(t, err)
		assert.Equal(t, want.Url, got.Url)
		assert.Equal(t, want.Detail, got.Detail)
	})

	t.Run("empty url", func(t *testing.T) {
		input := `{"url": "", "detail": "high"}`
		var got ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("missing url field", func(t *testing.T) {
		input := `{"detail": "high"}`
		var got ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})

	t.Run("invalid json", func(t *testing.T) {
		input := `{"url": "http://example.com/image.jpg", "detail": }`
		var got ImageContent
		err := json.Unmarshal([]byte(input), &got)

		assert.Error(t, err)
	})
}
