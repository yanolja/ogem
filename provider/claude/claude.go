package claude

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/yanolja/ogem/converter/openaiclaude"
	"github.com/yanolja/ogem/openai"
)

// A unique identifier for the Claude provider
const REGION = "claude"

type anthropicClient interface {
	New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

type Endpoint struct {
	client anthropicClient
}

func NewEndpoint(apiKey string) (*Endpoint, error) {
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	return &Endpoint{client: client.Messages}, nil
}

func (ep *Endpoint) GenerateChatCompletion(ctx context.Context, openaiRequest *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	claudeParams, err := openaiclaude.ToClaudeRequest(openaiRequest)
	if err != nil {
		return nil, err
	}

	claudeResponse, err := ep.client.New(ctx, *claudeParams)
	if err != nil {
		return nil, err
	}

	openaiResponse, err := openaiclaude.ToOpenAiResponse(claudeResponse)
	if err != nil {
		return nil, err
	}

	return openai.FinalizeResponse(ep.Provider(), ep.Region(), openaiRequest.Model, openaiResponse), nil
}

func (ep *Endpoint) Provider() string {
	return "claude"
}

func (ep *Endpoint) Region() string {
	return REGION
}

func (ep *Endpoint) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	_, err := ep.client.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.F(anthropic.ModelClaude_3_Haiku_20240307),
		MaxTokens: anthropic.Int(1),
		Messages: anthropic.F([]anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Ping")),
		}),
	})
	if err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

func (ep *Endpoint) Shutdown() error {
	return nil
}
