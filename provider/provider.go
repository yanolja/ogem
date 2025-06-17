package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils/orderedmap"
)

type AiEndpoint interface {
	GenerateChatCompletion(ctx context.Context, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error)
	GenerateChatCompletionStream(ctx context.Context, request *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error)
	Ping(ctx context.Context) (time.Duration, error)
	Provider() string
	Region() string
	Shutdown() error
}

func ToGeminiRole(role string) string {
	lowered := strings.ToLower(role)
	switch lowered {
	case "assistant":
		return "model"
	case "tool":
		return "function"
	}
	return lowered
}

func ToGeminiResponseMimeType(openAiRequest *openai.ChatCompletionRequest) (string, *orderedmap.Map, error) {
	format := openAiRequest.ResponseFormat
	if format == nil {
		return "text/plain", nil, nil
	}
	switch format.Type {
	case "json_schema":
		if format.JsonSchema == nil {
			return "", nil, fmt.Errorf("missing json_schema in response_format")
		}
		return "application/json", format.JsonSchema.Schema, nil
	case "json_object":
		return "application/json", nil, nil
	case "plain_text":
		return "text/plain", nil, nil
	}
	return "", nil, fmt.Errorf("unsupported response format: %s", format.Type)
}
