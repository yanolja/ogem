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
	var block anthropic.ContentBlock
	utils.Must0(json.Unmarshal([]byte(`{
		"type": "text",
		"text": "Pong"
	}`), &block))
	return &anthropic.Message{
		ID:      "msg_abc123",
		Content: []anthropic.ContentBlock{block},
		Model:   "claude-3-haiku-20240307",
		Role:    "assistant",
	}, nil
}

func mockNewRequestReturnsError(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	return nil, fmt.Errorf("claude api call error")
}

func newMockEndpoint(client anthropicClient) *Endpoint {
	return &Endpoint{client: client}
}

func TestNewEndpoint(t *testing.T) {
	endpoint, err := NewEndpoint("test-api-key")
	assert.NoError(t, err)
	assert.NotNil(t, endpoint)
}

func TestEndpoint_GenerateChatCompletion_SuccessCases(t *testing.T) {
	ctx := context.Background()
	client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsValidResponse}
	endpoint := newMockEndpoint(client)
	openaiRequest := &openai.ChatCompletionRequest{
		Model: "claude-3-haiku",
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
	assert.Contains(t, resp.Model, "claude-3-haiku")
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "Pong", *resp.Choices[0].Message.Content.String)
}

func TestEndpoint_GenerateChatCompletion_ErrorCases(t *testing.T) {
	tests := []struct {
		name          string
		openaiRequest *openai.ChatCompletionRequest
		resultFunc    func(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error)
	}{
		{
			name: "toClaudeParams error: empty messages",
			openaiRequest: &openai.ChatCompletionRequest{
				Model:    "claude-3-haiku",
				Messages: []openai.Message{},
			},
			resultFunc: nil,
		},
		{
			name: "claude api call error",
			openaiRequest: &openai.ChatCompletionRequest{
				Model: "claude-3-haiku",
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: utils.ToPtr("Ping"),
						},
					},
				},
			},
			resultFunc: mockNewRequestReturnsError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			client := &mockAnthropicClient{resultFunc: test.resultFunc}
			endpoint := newMockEndpoint(client)

			resp, err := endpoint.GenerateChatCompletion(ctx, test.openaiRequest)

			assert.Error(t, err)
			assert.Nil(t, resp)
		})
	}
}

func TestEndpoint_Provider(t *testing.T) {
	endpoint := newMockEndpoint(&mockAnthropicClient{})
	assert.Equal(t, "claude", endpoint.Provider())
}

func TestEndpoint_Region(t *testing.T) {
	endpoint := newMockEndpoint(&mockAnthropicClient{})
	assert.Equal(t, REGION, endpoint.Region())
}

func TestEndpoint_Ping_SuccessCases(t *testing.T) {
	ctx := context.Background()
	client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsValidResponse}
	endpoint := newMockEndpoint(client)

	duration, err := endpoint.Ping(ctx)

	assert.NoError(t, err)
	assert.Greater(t, duration, time.Duration(0))
}

func TestEndpoint_Ping_ErrorCases(t *testing.T) {
	ctx := context.Background()
	client := &mockAnthropicClient{resultFunc: mockNewRequestReturnsError}
	endpoint := newMockEndpoint(client)

	duration, err := endpoint.Ping(ctx)

	assert.Error(t, err)
	assert.Equal(t, "claude api call error", err.Error())
	assert.Equal(t, time.Duration(0), duration)
}

func TestEndpoint_Shutdown(t *testing.T) {
	endpoint := newMockEndpoint(&mockAnthropicClient{})
	assert.NoError(t, endpoint.Shutdown())
}
