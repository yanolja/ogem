package vertex

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils/orderedmap"
)

func TestNewEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		projectID string
		region    string
		wantErr   bool
		skipErr   bool // Skip if credentials error
	}{
		{
			name:      "valid configuration",
			projectID: "test-project",
			region:    "us-central1",
			wantErr:   false,
			skipErr:   true,
		},
		{
			name:      "empty project ID",
			projectID: "",
			region:    "us-central1",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint, err := NewEndpoint(tt.projectID, tt.region)

			// Skip test if Google credentials are not available
			if err != nil && strings.Contains(err.Error(), "Google Cloud credentials not found") && tt.skipErr {
				t.Skip("Skipping test: Google Cloud credentials not available")
			}

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, endpoint)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, endpoint)
				assert.Equal(t, tt.region, endpoint.Region())
				assert.Equal(t, "vertex", endpoint.Provider())
			}
		})
	}
}

func TestToGeminiConfig(t *testing.T) {
	tests := []struct {
		name          string
		openaiRequest *openai.ChatCompletionRequest
		wantErr       bool
		validate      func(*testing.T, *genai.GenerateContentConfig)
	}{
		{
			name: "basic configuration",
			openaiRequest: &openai.ChatCompletionRequest{
				Model:       "gemini-2.5-pro",
				Temperature: float32Ptr(0.7),
				TopP:        float32Ptr(0.9),
				MaxTokens:   int32Ptr(100),
			},
			wantErr: false,
			validate: func(t *testing.T, config *genai.GenerateContentConfig) {
				assert.Equal(t, float32(0.7), *config.Temperature)
				assert.Equal(t, float32(0.9), *config.TopP)
				assert.Equal(t, int32(100), config.MaxOutputTokens)
			},
		},
		{
			name: "with stop sequences",
			openaiRequest: &openai.ChatCompletionRequest{
				Model: "gemini-2.5-pro",
				StopSequences: &openai.StopSequences{
					Sequences: []string{"stop1", "stop2"},
				},
			},
			wantErr: false,
			validate: func(t *testing.T, config *genai.GenerateContentConfig) {
				assert.Equal(t, []string{"stop1", "stop2"}, config.StopSequences)
			},
		},
		{
			name: "invalid candidate count",
			openaiRequest: &openai.ChatCompletionRequest{
				Model:          "gemini-2.5-pro",
				CandidateCount: int32Ptr(2),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := toGeminiConfig(tt.openaiRequest)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, config)
			if tt.validate != nil {
				tt.validate(t, config)
			}
		})
	}
}

func TestToGeminiMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []openai.Message
		wantErr  bool
		validate func(*testing.T, []*genai.Content, *genai.Content)
	}{
		{
			name: "simple conversation",
			messages: []openai.Message{
				{Role: "user", Content: &openai.MessageContent{String: stringPtr("Hello")}},
				{Role: "assistant", Content: &openai.MessageContent{String: stringPtr("Hi there")}},
				{Role: "user", Content: &openai.MessageContent{String: stringPtr("How are you?")}},
			},
			wantErr: false,
			validate: func(t *testing.T, history []*genai.Content, last *genai.Content) {
				assert.Len(t, history, 2)
				assert.Equal(t, "user", history[0].Role)
				assert.Equal(t, "Hello", history[0].Parts[0].Text)
				assert.Equal(t, "How are you?", last.Parts[0].Text)
			},
		},
		{
			name:     "empty messages",
			messages: []openai.Message{},
			wantErr:  false,
			validate: func(t *testing.T, history []*genai.Content, last *genai.Content) {
				assert.Nil(t, history)
				assert.Nil(t, last)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoint := &Endpoint{}
			history, last, err := endpoint.toGeminiMessages(context.Background(), tt.messages)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			if tt.validate != nil {
				tt.validate(t, history, last)
			}
		})
	}
}

func TestToOpenAiResponse(t *testing.T) {
	tests := []struct {
		name           string
		geminiResponse *genai.GenerateContentResponse
		wantErr        bool
		validate       func(*testing.T, *openai.ChatCompletionResponse)
	}{
		{
			name: "successful response",
			geminiResponse: &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{
					Content: &genai.Content{
						Role:  "model",
						Parts: []*genai.Part{{Text: "Hello, how can I help you?"}},
					},
					FinishReason: genai.FinishReasonStop,
				}},
				UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:     10,
					CandidatesTokenCount: 5,
					TotalTokenCount:      15,
				},
			},
			wantErr: false,
			validate: func(t *testing.T, response *openai.ChatCompletionResponse) {
				assert.Len(t, response.Choices, 1)
				assert.Equal(t, "Hello, how can I help you?", *response.Choices[0].Message.Content.String)
				assert.Equal(t, "stop", response.Choices[0].FinishReason)
				assert.Equal(t, int32(10), response.Usage.PromptTokens)
				assert.Equal(t, int32(5), response.Usage.CompletionTokens)
				assert.Equal(t, int32(15), response.Usage.TotalTokens)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response, err := toOpenAiResponse(tt.geminiResponse)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, response)
			if tt.validate != nil {
				tt.validate(t, response)
			}
		})
	}
}

func TestToGeminiSchema(t *testing.T) {
	tests := []struct {
		name     string
		schema   *orderedmap.Map
		wantErr  bool
		validate func(*testing.T, *genai.Schema)
	}{
		{
			name: "simple object schema",
			schema: func() *orderedmap.Map {
				schema := orderedmap.New()
				props := orderedmap.New()
				nameProp := orderedmap.New()
				nameProp.Set("type", "string")
				ageProp := orderedmap.New()
				ageProp.Set("type", "integer")
				props.Set("name", nameProp)
				props.Set("age", ageProp)
				schema.Set("type", "object")
				schema.Set("properties", props)
				return schema
			}(),
			wantErr: false,
			validate: func(t *testing.T, schema *genai.Schema) {
				assert.Equal(t, genai.TypeObject, schema.Type)
				assert.Len(t, schema.Properties, 2)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema, err := toGeminiSchema(tt.schema)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, schema)
			if tt.validate != nil {
				tt.validate(t, schema)
			}
		})
	}
}

func TestEndpoint_Ping(t *testing.T) {
	// Skip this test as it requires a real client
	t.Skip("Skipping ping test as it requires a real client")
}

func TestEndpoint_Shutdown(t *testing.T) {
	endpoint := &Endpoint{
		client: nil,
		region: "us-central1",
	}

	err := endpoint.Shutdown()
	assert.NoError(t, err)
}

// Helper function to create int pointer
func float32Ptr(f float32) *float32 {
	return &f
}

func int32Ptr(i int32) *int32 {
	return &i
}

func stringPtr(s string) *string {
	return &s
}
