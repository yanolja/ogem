package claude

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

type mockMessageService struct{}

func (m *mockMessageService) New(ctx context.Context, params anthropic.MessageNewParams) (*anthropic.Message, error) {
	var block anthropic.ContentBlock
	if err := json.Unmarshal([]byte(`{
		"type": "text",
		"text": "Pong"
	}`), &block); err != nil {
		panic(err)
	}
	return &anthropic.Message{
		ID: "msg_abc123",
		Content: []anthropic.ContentBlock{block},
		Model: "claude-3-haiku-20240307",
		Role: "assistant",
	}, nil
}

type mockAnthropicClient struct {
	mockService MessageService
}

func (m *mockAnthropicClient) Messages() MessageService {
	return m.mockService
}

func TestEndpoint_Ping(t *testing.T) {
	ctx := context.Background()
	client := &mockAnthropicClient{mockService: &mockMessageService{}}
	endpoint := NewEndpointWithClient(client)

	duration, err := endpoint.Ping(ctx)

	assert.NoError(t, err)
	assert.Greater(t, duration, time.Duration(0))
}

func TestEndpoint_GenerateChatCompletion(t *testing.T) {
	ctx := context.Background()

	client := &mockAnthropicClient{mockService: &mockMessageService{}}
	endpoint := NewEndpointWithClient(client)

	openaiRequest := &openai.ChatCompletionRequest{
		Model: "claude-3-haiku",
		Messages: []openai.Message{
			{
				Role:    "assistant",
				Content: &openai.MessageContent{
					String: utils.ToPtr("Ping"),
				},
			},
		},
	}

	// Call GenerateChatCompletion
	resp, err := endpoint.GenerateChatCompletion(ctx, openaiRequest)

	// Assertions
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Contains(t, resp.Model, "claude-3-haiku")
	assert.Len(t, resp.Choices, 1)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "Pong", *resp.Choices[0].Message.Content.String)
}
