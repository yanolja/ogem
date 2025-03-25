package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/utils"
)

func TestContent_UnmarshalJSON(t *testing.T) {
	t.Run("valid text content", func(t *testing.T) {
		input := `{"text": "hello world"}`
		expected := Content{
			TextContent:  &TextContent{Text: "hello world"},
			ImageContent: nil,
		}
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("valid image content with detail", func(t *testing.T) {
		input := `{"url": "https://example.com/image.jpg", "detail": "high"}`
		expected := Content{
			TextContent: nil,
			ImageContent: &ImageContent{
				Url:    "https://example.com/image.jpg",
				Detail: "high",
			},
		}
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("empty object", func(t *testing.T) {
		input := `{}`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
		assert.Equal(t, "expected text or image content, got {}", err.Error())
	})

	t.Run("null content", func(t *testing.T) {
		input := `null`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
		assert.Equal(t, "expected text or image content, got null", err.Error())
	})

	t.Run("empty text content", func(t *testing.T) {
		input := `{"text": ""}`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
		assert.Equal(t, "expected text content, got ", err.Error())
	})

	t.Run("image content without detail", func(t *testing.T) {
		input := `{"url": "https://example.com/image.jpg"}`
		expected := Content{
			TextContent: nil,
			ImageContent: &ImageContent{
				Url:    "https://example.com/image.jpg",
				Detail: "",
			},
		}
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("image content with empty detail", func(t *testing.T) {
		input := `{"url": "https://example.com/image.jpg", "detail": ""}`
		expected := Content{
			TextContent: nil,
			ImageContent: &ImageContent{
				Url:    "https://example.com/image.jpg",
				Detail: "",
			},
		}
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("invalid text type", func(t *testing.T) {
		input := `{"text": 123}`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
		assert.Equal(t, "expected text content, got 123", err.Error())
	})

	t.Run("invalid url type", func(t *testing.T) {
		input := `{"url": 123}`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
		assert.Equal(t, "expected url string content, got 123", err.Error())
	})

	t.Run("empty url content", func(t *testing.T) {
		input := `{"url": ""}`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
		assert.Equal(t, "expected url string content, got ", err.Error())
	})

	t.Run("image content with non-string detail", func(t *testing.T) {
		input := `{"url": "https://example.com/image.jpg", "detail": 123}`
		expected := Content{
			TextContent: nil,
			ImageContent: &ImageContent{
				Url:    "https://example.com/image.jpg",
				Detail: "",
			},
		}
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("image content with null detail", func(t *testing.T) {
		input := `{"url": "https://example.com/image.jpg", "detail": null}`
		expected := Content{
			TextContent: nil,
			ImageContent: &ImageContent{
				Url:    "https://example.com/image.jpg",
				Detail: "",
			},
		}
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("malformed json", func(t *testing.T) {
		input := `{"text": "incomplete"`
		var content Content
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
	})
}

func TestMessageContent_UnmarshalJSON(t *testing.T) {
	t.Run("string content", func(t *testing.T) {
		input := `"hello world"`
		expected := MessageContent{
			String: utils.ToPtr("hello world"),
			Parts:  nil,
		}
		var content MessageContent
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("image content", func(t *testing.T) {
		input := `[
			{
				"type": "image", 
				"content": {"url": "https://example.com/image.jpg", "detail": "high"}
			}
		]`
		expected := MessageContent{
			String: nil,
			Parts: []Part{
				{
					Type: "image",
					Content: Content{
						ImageContent: &ImageContent{
							Url:    "https://example.com/image.jpg",
							Detail: "high",
						},
					},
				},
			},
		}
		var content MessageContent
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("text and image parts", func(t *testing.T) {
		input := `[
			{
				"type": "text",
				"content": {"text": "explain this image"}
			},
			{
				"type": "image",
				"content": {"url": "https://example.com/image.jpg", "detail": "high"}
			}
		]`
		expected := MessageContent{
			String: nil,
			Parts: []Part{
				{
					Type: "text",
					Content: Content{
						TextContent:  &TextContent{Text: "explain this image"},
						ImageContent: nil,
					},
				},
				{
					Type: "image",
					Content: Content{
						TextContent: nil,
						ImageContent: &ImageContent{
							Url:    "https://example.com/image.jpg",
							Detail: "high",
						},
					},
				},
			},
		}
		var content MessageContent
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})

	t.Run("invalid content", func(t *testing.T) {
		input := `{"invalid": "format"}`
		var content MessageContent
		err := json.Unmarshal([]byte(input), &content)
		assert.Error(t, err)
	})

	t.Run("empty array", func(t *testing.T) {
		input := `[]`
		expected := MessageContent{
			String: nil,
			Parts:  []Part{},
		}
		var content MessageContent
		err := json.Unmarshal([]byte(input), &content)
		assert.NoError(t, err)
		assert.Equal(t, expected, content)
	})
}
