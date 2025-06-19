package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

type mockAnthropicClient struct {
	resultFunc func(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
}

func (m *mockAnthropicClient) New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	return m.resultFunc(ctx, params, opts...)
}

func mockNewRequestReturnsValidResponse(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	var block anthropic.ContentBlockUnion
	utils.Must0(json.Unmarshal([]byte(`{
		"type": "text",
		"text": "Pong"
	}`), &block))
	return &anthropic.Message{
		ID:      "msg_abc123",
		Content: []anthropic.ContentBlockUnion{block},
		Model:   "claude-3.5-haiku-20241022",
		Role:    "assistant",
	}, nil
}

func mockNewRequestReturnsError(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	return nil, fmt.Errorf("claude api call error")
}

func newMockEndpoint(client anthropicClient) *Endpoint {
	return &Endpoint{client: client}
}

func TestEndpoint(t *testing.T) {
	t.Run("new endpoint should succeed", func(t *testing.T) {
		endpoint, err := NewEndpoint("test-api-key")
		assert.NoError(t, err)
		assert.NotNil(t, endpoint)
	})

	t.Run("generate chat completion should return valid response", func(t *testing.T) {
		ctx := context.Background()
		client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsValidResponse}
		endpoint := newMockEndpoint(client)
		openaiRequest := &openai.ChatCompletionRequest{
			Model: "claude-3.5-haiku-20241022",
			Messages: []openai.Message{
				{
					Role: "assistant",
					Content: &openai.MessageContent{
						String: utils.ToPtr("Ping"),
					},
				},
			},
		}

		resp, err := endpoint.GenerateChatCompletion(ctx, openaiRequest)

		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Contains(t, resp.Model, "claude-3.5-haiku")
		assert.Len(t, resp.Choices, 1)
		assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
		assert.Equal(t, "Pong", *resp.Choices[0].Message.Content.String)
	})

	t.Run("evaluate handling error from toClaudeParams", func(t *testing.T) {
		ctx := context.Background()
		// resultFunc is not needed because generateChatCompletion method
		// should return an error before sending the request to the claude api
		client := &mockAnthropicClient{resultFunc: nil}
		endpoint := newMockEndpoint(client)
		openaiRequest := &openai.ChatCompletionRequest{
			Model:    "claude-3-haiku",
			Messages: []openai.Message{},
		}

		resp, err := endpoint.GenerateChatCompletion(ctx, openaiRequest)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, "at least one message is required", err.Error())
	})

	t.Run("evaluate handling error from claude api call", func(t *testing.T) {
		ctx := context.Background()
		client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsError}
		endpoint := newMockEndpoint(client)
		openaiRequest := &openai.ChatCompletionRequest{
			Model: "claude-3.5-haiku-20241022",
			Messages: []openai.Message{
				{
					Role: "user",
					Content: &openai.MessageContent{
						String: utils.ToPtr("Ping"),
					},
				},
			},
		}

		resp, err := endpoint.GenerateChatCompletion(ctx, openaiRequest)

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, "claude api call error", err.Error())
	})

	t.Run("provider should return claude provider", func(t *testing.T) {
		endpoint := newMockEndpoint(&mockAnthropicClient{})
		assert.Equal(t, "claude", endpoint.Provider())
	})

	t.Run("region should return correct region", func(t *testing.T) {
		endpoint := newMockEndpoint(&mockAnthropicClient{})
		assert.Equal(t, REGION, endpoint.Region())
	})

	t.Run("ping should succeed with valid response", func(t *testing.T) {
		ctx := context.Background()
		client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsValidResponse}
		endpoint := newMockEndpoint(client)

		duration, err := endpoint.Ping(ctx)

		assert.NoError(t, err)
		assert.Greater(t, duration, time.Duration(0))
	})

	t.Run("ping should fail on api error", func(t *testing.T) {
		ctx := context.Background()
		client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsError}
		endpoint := newMockEndpoint(client)

		duration, err := endpoint.Ping(ctx)

		assert.Error(t, err)
		assert.Equal(t, "claude api call error", err.Error())
		assert.Equal(t, time.Duration(0), duration)
	})

	t.Run("shutdown should succeed", func(t *testing.T) {
		endpoint := newMockEndpoint(&mockAnthropicClient{})
		assert.NoError(t, endpoint.Shutdown())
	})
}
