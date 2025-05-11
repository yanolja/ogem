package vclaude

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
)

// Mock anthropic client
type mockAnthropicClient struct {
	mock.Mock
}

func (m *mockAnthropicClient) New(ctx context.Context, params anthropic.MessageNewParams, opts ...option.RequestOption) (*anthropic.Message, error) {
	args := m.Called(ctx, params, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*anthropic.Message), args.Error(1)
}

func TestNewEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		projectId string
		region    string
		wantErr   bool
	}{
		{
			name:      "valid input",
			projectId: "test-project",
			region:    "us-central1",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep, err := NewEndpoint(tt.projectId, tt.region)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, ep)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, ep)
				assert.Equal(t, tt.region, ep.region)
			}
		})
	}
}

func TestEndpoint_Provider(t *testing.T) {
	ep := &Endpoint{region: "us-central1"}
	assert.Equal(t, "vclaude", ep.Provider())
}

func TestEndpoint_Region(t *testing.T) {
	ep := &Endpoint{region: "us-central1"}
	assert.Equal(t, "us-central1", ep.Region())
}

func TestEndpoint_Shutdown(t *testing.T) {
	ep := &Endpoint{}
	assert.NoError(t, ep.Shutdown())
}

func TestEndpoint_Ping(t *testing.T) {
	mockClient := new(mockAnthropicClient)
	ep := &Endpoint{client: mockClient}

	mockClient.On("New", mock.Anything, mock.MatchedBy(func(params anthropic.MessageNewParams) bool {
		return params.MaxTokens == 1 && len(params.Messages) == 1
	}), mock.Anything).Return(&anthropic.Message{}, nil)

	duration, err := ep.Ping(context.Background())
	assert.NoError(t, err)
	assert.True(t, duration >= 0)
	mockClient.AssertExpectations(t)
}

func TestEndpoint_GenerateChatCompletion(t *testing.T) {
	mockClient := new(mockAnthropicClient)
	ep := &Endpoint{
		client: mockClient,
		region: "us-central1",
	}

	tests := []struct {
		name    string
		request *openai.ChatCompletionRequest
		mock    func()
		wantErr bool
	}{
		{
			name: "successful completion",
			request: &openai.ChatCompletionRequest{
				Model: "claude-3-sonnet",
				Messages: []openai.Message{
					{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hello")}},
				},
			},
			mock: func() {
				mockClient.On("New", mock.Anything, mock.MatchedBy(func(params anthropic.MessageNewParams) bool {
					return params.Model == "claude-3-sonnet@20240229" && len(params.Messages) == 1
				}), mock.Anything).Return(&anthropic.Message{
					Content: []anthropic.ContentBlockUnion{
						{Text: "Hello there!"},
					},
				}, nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mock()
			resp, err := ep.GenerateChatCompletion(context.Background(), tt.request)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, resp)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, resp)
			}
			mockClient.AssertExpectations(t)
		})
	}
}

func TestToClaudeParams(t *testing.T) {
	tests := []struct {
		name    string
		request *openai.ChatCompletionRequest
		want    *anthropic.MessageNewParams
		wantErr bool
	}{
		{
			name: "basic request",
			request: &openai.ChatCompletionRequest{
				Model: "claude-3-sonnet",
				Messages: []openai.Message{
					{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hello")}},
				},
			},
			want: &anthropic.MessageNewParams{
				Model: "claude-3-sonnet@20240229",
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
				},
				MaxTokens: 4096,
			},
			wantErr: false,
		},
		{
			name: "with system message",
			request: &openai.ChatCompletionRequest{
				Model: "claude-3-sonnet",
				Messages: []openai.Message{
					{Role: "system", Content: &openai.MessageContent{String: utils.ToPtr("You are helpful.")}},
					{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hello")}},
				},
			},
			want: &anthropic.MessageNewParams{
				Model: "claude-3-sonnet@20240229",
				Messages: []anthropic.MessageParam{
					anthropic.NewUserMessage(anthropic.NewTextBlock("Hello")),
				},
				System:    []anthropic.TextBlockParam{{Text: "You are helpful."}},
				MaxTokens: 4096,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toClaudeParams(tt.request)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, got)
				assert.Equal(t, tt.want.Model, got.Model)
				assert.Equal(t, tt.want.MaxTokens, got.MaxTokens)
				assert.Equal(t, len(tt.want.Messages), len(got.Messages))
			}
		})
	}
}

func TestToClaudeMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []openai.Message
		want     int
		wantErr  bool
	}{
		{
			name:     "empty messages",
			messages: []openai.Message{},
			want:     0,
			wantErr:  true,
		},
		{
			name: "valid messages",
			messages: []openai.Message{
				{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hello")}},
				{Role: "assistant", Content: &openai.MessageContent{String: utils.ToPtr("Hi")}},
			},
			want:    2,
			wantErr: false,
		},
		{
			name: "skip system message",
			messages: []openai.Message{
				{Role: "system", Content: &openai.MessageContent{String: utils.ToPtr("Be helpful")}},
				{Role: "user", Content: &openai.MessageContent{String: utils.ToPtr("Hello")}},
			},
			want:    1,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := toClaudeMessages(tt.messages)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, len(got))
			}
		})
	}
}

func TestStandardizeModelName(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  string
	}{
		{
			name:  "claude-3-sonnet",
			model: "claude-3-sonnet",
			want:  "claude-3-sonnet@20240229",
		},
		{
			name:  "claude-3-opus",
			model: "claude-3-opus",
			want:  "claude-3-opus@20240307",
		},
		{
			name:  "claude-3-haiku",
			model: "claude-3-haiku",
			want:  "claude-3-haiku@20240307",
		},
		{
			name:  "already standardized",
			model: "claude-3-sonnet@20240229",
			want:  "claude-3-sonnet@20240229",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := standardizeModelName(tt.model)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestToOpenAiFinishReason(t *testing.T) {
	tests := []struct {
		name        string
		stopReason  anthropic.MessageStopReason
		wantReason  string
	}{
		{
			name:        "end_turn",
			stopReason:  anthropic.MessageStopReasonEndTurn,
			wantReason:  "stop",
		},
		{
			name:        "max_tokens",
			stopReason:  anthropic.MessageStopReasonMaxTokens,
			wantReason:  "length",
		},
		{
			name:        "stop_sequence",
			stopReason:  anthropic.MessageStopReasonStopSequence,
			wantReason:  "stop",
		},
		{
			name:        "unknown",
			stopReason:  "unknown",
			wantReason:  "stop",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toOpenAiFinishReason(tt.stopReason)
			assert.Equal(t, tt.wantReason, got)
		})
	}
}
