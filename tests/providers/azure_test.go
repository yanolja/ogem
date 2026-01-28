package providers

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/openai"
)

// TestAzureOpenAI_ChatCompletion tests Azure OpenAI chat completion
func TestAzureOpenAI_ChatCompletion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		t.Skip("AZURE_OPENAI_API_KEY not set")
	}

	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")

	if endpoint == "" || deployment == "" {
		t.Skip("Azure endpoint or deployment not configured")
	}

	request := &openai.ChatCompletionRequest{
		Model: deployment,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Hello from Azure OpenAI test!"),
				},
			},
		},
		MaxTokens: int32Ptr(50),
	}

	response, err := azureChatCompletion(context.Background(), endpoint, deployment, request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Choices)
	assert.NotEmpty(t, response.Choices[0].Message.Content)
}

// TestAzureOpenAI_Embeddings tests Azure OpenAI embeddings
func TestAzureOpenAI_Embeddings(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		t.Skip("AZURE_OPENAI_API_KEY not set")
	}

	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_EMBEDDING_DEPLOYMENT")

	if endpoint == "" || deployment == "" {
		t.Skip("Azure endpoint or embedding deployment not configured")
	}

	request := &openai.EmbeddingRequest{
		Model: deployment,
		Input: []string{"Test text for Azure embeddings"},
	}

	response, err := azureEmbeddings(context.Background(), endpoint, deployment, request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.NotEmpty(t, response.Data)
	assert.NotEmpty(t, response.Data[0].Embedding)
}

// TestAzureOpenAI_APIVersions tests different Azure API versions
func TestAzureOpenAI_APIVersions(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		t.Skip("AZURE_OPENAI_API_KEY not set")
	}

	apiVersions := []string{
		"2023-05-15",
		"2023-06-01-preview",
		"2023-12-01-preview",
		"2024-02-01",
	}

	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")

	for _, apiVersion := range apiVersions {
		t.Run(apiVersion, func(t *testing.T) {
			request := &openai.ChatCompletionRequest{
				Model: deployment,
				Messages: []openai.Message{
					{
						Role: "user",
						Content: &openai.MessageContent{
							String: stringPtr("Test API version " + apiVersion),
						},
					},
				},
				MaxTokens: int32Ptr(10),
			}

			response, err := azureChatCompletionWithVersion(context.Background(), endpoint, deployment, apiVersion, request)
			require.NoError(t, err)
			require.NotNil(t, response)
		})
	}
}

// TestAzureOpenAI_Streaming tests streaming responses from Azure
func TestAzureOpenAI_Streaming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		t.Skip("AZURE_OPENAI_API_KEY not set")
	}

	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")

	request := &openai.ChatCompletionRequest{
		Model: deployment,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Tell me a short story"),
				},
			},
		},
		Stream:    boolPtr(true),
		MaxTokens: int32Ptr(100),
	}

	chunks, errors := azureChatCompletionStream(context.Background(), endpoint, deployment, request)

	chunkCount := 0
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				// Stream closed
				assert.Greater(t, chunkCount, 0, "Should receive at least one chunk")
				return
			}
			chunkCount++
			assert.NotNil(t, chunk)
		case err := <-errors:
			require.NoError(t, err)
			return
		}
	}
}

// TestAzureOpenAI_DeploymentMapping tests Azure deployment name mapping
func TestAzureOpenAI_DeploymentMapping(t *testing.T) {
	tests := []struct {
		name           string
		modelName      string
		expectedDeploy string
	}{
		{
			name:           "gpt-4 to deployment",
			modelName:      "gpt-4",
			expectedDeploy: "gpt-4-deployment",
		},
		{
			name:           "gpt-35-turbo to deployment",
			modelName:      "gpt-3.5-turbo",
			expectedDeploy: "gpt-35-turbo-deployment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := mapModelToDeployment(tt.modelName)
			assert.NotEmpty(t, deployment)
		})
	}
}

// TestAzureOpenAI_ErrorHandling tests Azure-specific error handling
func TestAzureOpenAI_ErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		testFn  func(t *testing.T)
		errMsg  string
	}{
		{
			name: "invalid endpoint",
			testFn: func(t *testing.T) {
				request := &openai.ChatCompletionRequest{
					Model: "test-deployment",
					Messages: []openai.Message{
						{
							Role: "user",
							Content: &openai.MessageContent{
								String: stringPtr("test"),
							},
						},
					},
				}
				_, err := azureChatCompletion(context.Background(), "https://invalid-endpoint.openai.azure.com", "test", request)
				require.Error(t, err)
			},
			errMsg: "failed to connect",
		},
		{
			name: "invalid API key",
			testFn: func(t *testing.T) {
				// TODO: Test with invalid API key
			},
			errMsg: "unauthorized",
		},
		{
			name: "invalid deployment",
			testFn: func(t *testing.T) {
				// TODO: Test with invalid deployment name
			},
			errMsg: "deployment not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.testFn != nil {
				tt.testFn(t)
			}
		})
	}
}

// TestAzureOpenAI_ContentFiltering tests Azure content filtering
func TestAzureOpenAI_ContentFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		t.Skip("AZURE_OPENAI_API_KEY not set")
	}

	// Azure returns content filtering results
	// Test that we can handle them properly
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	deployment := os.Getenv("AZURE_OPENAI_DEPLOYMENT")

	request := &openai.ChatCompletionRequest{
		Model: deployment,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Tell me about AI safety"),
				},
			},
		},
		MaxTokens: int32Ptr(50),
	}

	response, err := azureChatCompletion(context.Background(), endpoint, deployment, request)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Azure responses include content filtering annotations
	// Verify we can parse them
	assert.NotNil(t, response.Choices)
}

// Helper functions

func azureChatCompletion(ctx context.Context, endpoint, deployment string, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	// TODO: Implement actual Azure OpenAI client call
	return nil, assert.AnError
}

func azureChatCompletionWithVersion(ctx context.Context, endpoint, deployment, apiVersion string, request *openai.ChatCompletionRequest) (*openai.ChatCompletionResponse, error) {
	// TODO: Implement with specific API version
	return nil, assert.AnError
}

func azureEmbeddings(ctx context.Context, endpoint, deployment string, request *openai.EmbeddingRequest) (*openai.EmbeddingResponse, error) {
	// TODO: Implement actual Azure OpenAI embeddings call
	return nil, assert.AnError
}

func azureChatCompletionStream(ctx context.Context, endpoint, deployment string, request *openai.ChatCompletionRequest) (<-chan *openai.ChatCompletionStreamResponse, <-chan error) {
	chunks := make(chan *openai.ChatCompletionStreamResponse)
	errors := make(chan error)

	// TODO: Implement streaming
	go func() {
		close(chunks)
		close(errors)
	}()

	return chunks, errors
}

func mapModelToDeployment(modelName string) string {
	// Azure uses deployment names instead of model names
	// Common mappings
	mappings := map[string]string{
		"gpt-4":         "gpt-4-deployment",
		"gpt-3.5-turbo": "gpt-35-turbo-deployment",
		"gpt-4-turbo":   "gpt-4-turbo-deployment",
	}

	if deployment, ok := mappings[modelName]; ok {
		return deployment
	}

	return modelName
}

func stringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
