package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/openai"
)

// TestImageGeneration_DALLE2 tests basic image generation with DALL-E 2
func TestImageGeneration_DALLE2(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ImageGenerationRequest{
		Prompt: "A cute cat playing with a ball of yarn",
		Model:  strPtr("dall-e-2"),
		N:      int32Ptr(1),
		Size:   strPtr("256x256"),
	}

	// TODO: This will fail until image generation is implemented
	response, err := generateImage(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Data, 1)
	assert.NotEmpty(t, response.Data[0].URL)
}

// TestImageGeneration_DALLE3 tests image generation with DALL-E 3
func TestImageGeneration_DALLE3(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ImageGenerationRequest{
		Prompt:  "A futuristic city with flying cars at sunset, digital art",
		Model:   strPtr("dall-e-3"),
		N:       int32Ptr(1),
		Size:    strPtr("1024x1024"),
		Quality: strPtr("standard"),
		Style:   strPtr("vivid"),
	}

	response, err := generateImage(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Data, 1)
	assert.NotEmpty(t, response.Data[0].URL)
	assert.NotEmpty(t, response.Data[0].RevisedPrompt)
}

// TestImageGeneration_MultipleImages tests generating multiple images
func TestImageGeneration_MultipleImages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ImageGenerationRequest{
		Prompt: "A red apple on a wooden table",
		Model:  strPtr("dall-e-2"),
		N:      int32Ptr(3),
		Size:   strPtr("256x256"),
	}

	response, err := generateImage(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Data, 3)
	for _, img := range response.Data {
		assert.NotEmpty(t, img.URL)
	}
}

// TestImageGeneration_Base64Response tests base64-encoded image response
func TestImageGeneration_Base64Response(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ImageGenerationRequest{
		Prompt:         "A simple geometric pattern",
		Model:          strPtr("dall-e-2"),
		N:              int32Ptr(1),
		Size:           strPtr("256x256"),
		ResponseFormat: strPtr("b64_json"),
	}

	response, err := generateImage(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Data, 1)
	assert.NotEmpty(t, response.Data[0].B64JSON)
	assert.Empty(t, response.Data[0].URL)
}

// TestImageGeneration_InvalidInput tests error handling
func TestImageGeneration_InvalidInput(t *testing.T) {
	tests := []struct {
		name    string
		request *openai.ImageGenerationRequest
		errMsg  string
	}{
		{
			name: "empty prompt",
			request: &openai.ImageGenerationRequest{
				Prompt: "",
				Model:  strPtr("dall-e-2"),
			},
			errMsg: "prompt is required",
		},
		{
			name: "invalid size",
			request: &openai.ImageGenerationRequest{
				Prompt: "test",
				Model:  strPtr("dall-e-2"),
				Size:   strPtr("invalid"),
			},
			errMsg: "invalid size",
		},
		{
			name: "too many images for dalle-3",
			request: &openai.ImageGenerationRequest{
				Prompt: "test",
				Model:  strPtr("dall-e-3"),
				N:      int32Ptr(5),
			},
			errMsg: "n must be 1 for dall-e-3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := generateImage(context.Background(), tt.request)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errMsg)
		})
	}
}

// TestImageEdit tests image editing functionality
func TestImageEdit(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement when edit endpoint is ready
	t.Skip("Image edit not yet implemented")
}

// TestImageVariation tests creating variations of an image
func TestImageVariation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement when variation endpoint is ready
	t.Skip("Image variation not yet implemented")
}

// TestImageGeneration_Caching tests that image responses are cached
func TestImageGeneration_Caching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ImageGenerationRequest{
		Prompt: "A unique test image for caching",
		Model:  strPtr("dall-e-2"),
		N:      int32Ptr(1),
		Size:   strPtr("256x256"),
	}

	// First request
	resp1, err := generateImage(context.Background(), request)
	require.NoError(t, err)

	// Second identical request should be cached
	resp2, err := generateImage(context.Background(), request)
	require.NoError(t, err)

	// Both should have the same image URL (from cache)
	assert.Equal(t, resp1.Data[0].URL, resp2.Data[0].URL)
}

// TestImageGeneration_RateLimiting tests rate limiting for image generation
func TestImageGeneration_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement rate limiting test
	t.Skip("Rate limiting test not yet implemented")
}

// Helper functions

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func int32Ptr(i int32) *int32 {
	return &i
}
