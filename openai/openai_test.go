package openai

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/utils"
)

func TestContent_UnmarshalJSON(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        expected Content
        wantErr  bool
        errMsg   string
    }{
        {
            name:  "valid text content",
            input: `{"text": "hello world"}`,
            expected: Content{
                TextContent:  &TextContent{Text: "hello world"},
                ImageContent: nil,
            },
            wantErr: false,
        },
        {
            name:  "valid image content with detail",
            input: `{"url": "https://example.com/image.jpg", "detail": "high"}`,
            expected: Content{
                TextContent: nil,
                ImageContent: &ImageContent{
                    Url:    "https://example.com/image.jpg",
                    Detail: "high",
                },
            },
            wantErr: false,
        },
        {
            name:     "empty object",
            input:    `{}`,
            expected: Content{},
            wantErr:  true,
            errMsg:   "invalid content format: {}",
        },
        {
            name:     "null content",
            input:    `null`,
            expected: Content{},
            wantErr:  true,
            errMsg:   "invalid content format: null",
        },
        {
            name:  "empty text content",
            input: `{"text": ""}`,
            expected: Content{
                TextContent:  &TextContent{Text: ""},
                ImageContent: nil,
            },
            wantErr: true,
            errMsg: "invalid text content: ",
        },
        {
            name:  "image content without detail",
            input: `{"url": "https://example.com/image.jpg"}`,
            expected: Content{
                TextContent: nil,
                ImageContent: &ImageContent{
                    Url:    "https://example.com/image.jpg",
                    Detail: "",
                },
            },
            wantErr: false,
        },
        {
            name:  "image content with empty detail",
            input: `{"url": "https://example.com/image.jpg", "detail": ""}`,
            expected: Content{
                TextContent: nil,
                ImageContent: &ImageContent{
                    Url:    "https://example.com/image.jpg",
                    Detail: "",
                },
            },
            wantErr: false,
        },
        {
            name:     "invalid text type",
            input:    `{"text": 123}`,
            expected: Content{},
            wantErr:  true,
            errMsg:   "invalid text content: 123",
        },
        {
            name:     "invalid url type",
            input:    `{"url": 123}`,
            expected: Content{},
            wantErr:  true,
            errMsg:   "invalid url content: 123",
        },
		{
			name:     "empty url content",
			input:    `{"url": ""}`,
			expected: Content{},
			wantErr:  true,
			errMsg:   "invalid url content: ",
		},
        {
            name:  "image content with non-string detail",
            input: `{"url": "https://example.com/image.jpg", "detail": 123}`,
            expected: Content{
                TextContent: nil,
                ImageContent: &ImageContent{
                    Url:    "https://example.com/image.jpg",
                    Detail: "",
                },
            },
            wantErr: false,
        },
        {
            name:  "image content with null detail",
            input: `{"url": "https://example.com/image.jpg", "detail": null}`,
            expected: Content{
                TextContent: nil,
                ImageContent: &ImageContent{
                    Url:    "https://example.com/image.jpg",
                    Detail: "",
                },
            },
            wantErr: false,
        },
        {
            name:     "malformed JSON",
            input:    `{"text": "incomplete"`,
            expected: Content{},
            wantErr:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            var content Content
            err := json.Unmarshal([]byte(tt.input), &content)

            if tt.wantErr {
                assert.Error(t, err)
                if tt.errMsg != "" {
                    assert.Equal(t, tt.errMsg, err.Error())
                }
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.expected, content)
            }
        })
    }
}

func TestMessageContent_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected MessageContent
		wantErr  bool
	}{
		{
			name:  "string content",
			input: `"hello world"`,
			expected: MessageContent{
				String: utils.ToPtr("hello world"),
				Parts:  nil,
			},
			wantErr: false,
		},
        {
            name: "image content",
            input: `[
                {
                    "type": "image", 
                    "content": {"url": "https://example.com/image.jpg", "detail": "high"}
                }
            ]`,
            expected: MessageContent{
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
            },
            wantErr: false,
        },
		{
			name: "array with text and image parts",
			input: `[
				{
					"type": "text",
					"content": {"text": "explain this image"}
				},
				{
					"type": "image",
					"content": {"url": "https://example.com/image.jpg", "detail": "high"}
				}
			]`,
			expected: MessageContent{
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
			},
			wantErr: false,
		},
		{
			name:     "invalid content",
			input:    `{"invalid": "format"}`,
			expected: MessageContent{},
			wantErr:  true,
		},
		{
			name: "empty array",
			input: `[]`,
			expected: MessageContent{
				String: nil,
				Parts:  []Part{},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var content MessageContent
			err := json.Unmarshal([]byte(tt.input), &content)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, content)
			}
		})
	}
}
