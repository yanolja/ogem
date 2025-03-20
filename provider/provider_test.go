package provider

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils/orderedmap"
)

func TestToGeminiRole(t *testing.T) {
	tests := []struct {
		role string
		expected string
	}{
		{"user", "user"},
		{"assistant", "model"},
		{"tool", "function"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("role: %s", test.role), func(t *testing.T) {
			result := ToGeminiRole(test.role)
			assert.Equal(t, test.expected, result)
		})
	}
}

func TestToGeminiResponseMimeType_SuccessCases(t *testing.T) {
	t.Run("no format specified", func(t *testing.T) {
		mimeType, schema, err := ToGeminiResponseMimeType(&openai.ChatCompletionRequest{})
		assert.NoError(t, err)
		assert.Equal(t, "text/plain", mimeType)
		assert.Nil(t, schema)
	})

	t.Run("plain text format", func(t *testing.T) {
		mimeType, schema, err := ToGeminiResponseMimeType(&openai.ChatCompletionRequest{
			ResponseFormat: &openai.ResponseFormat{
				Type: "plain_text",
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "text/plain", mimeType)
		assert.Nil(t, schema)
	})

	t.Run("json object format", func(t *testing.T) {
		mimeType, schema, err := ToGeminiResponseMimeType(&openai.ChatCompletionRequest{
			ResponseFormat: &openai.ResponseFormat{
				Type: "json_object",
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "application/json", mimeType)
		assert.Nil(t, schema)
	})

	t.Run("json schema format", func(t *testing.T) {
		mimeType, schema, err := ToGeminiResponseMimeType(&openai.ChatCompletionRequest{
			ResponseFormat: &openai.ResponseFormat{
				Type: "json_schema",
				JsonSchema: &openai.JsonSchema{
					Schema: func() *orderedmap.Map {
						schema := orderedmap.New()
						schema.Set("type", "object")
						return schema
					}(),
				},
			},
		})
		assert.NoError(t, err)
		assert.Equal(t, "application/json", mimeType)
		schemaType, ok := schema.Get("type")
		assert.True(t, ok)
		assert.Equal(t, "object", schemaType.(string))
	})
}

func TestToGeminiResponseMimeType_ErrorCases(t *testing.T) {
	tests := []struct {
		name string
		format *openai.ChatCompletionRequest
		expectedError string
	}{
		{
			name: "missing json schema in json_schema response format",
			format: &openai.ChatCompletionRequest{
				ResponseFormat: &openai.ResponseFormat{
					Type: "json_schema",
					JsonSchema: nil,
				},
			},
			expectedError: "missing json_schema in response_format",
		},
		{
			name: "unsupported response format",
			format: &openai.ChatCompletionRequest{
				ResponseFormat: &openai.ResponseFormat{
					Type: "unsupported",
				},
			},
			expectedError: "unsupported response format: unsupported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mimeType, schema, err := ToGeminiResponseMimeType(test.format)
			assert.Error(t, err)
			assert.Equal(t, test.expectedError, err.Error())
			assert.Equal(t, "", mimeType)
			assert.Nil(t, schema)
		})
	}
}
