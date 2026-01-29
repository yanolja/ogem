package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yanolja/ogem/openai"
)

// TestModeration_TextClassification tests basic content moderation
func TestModeration_TextClassification(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ModerationRequest{
		Input: []string{"I love programming and building great software!"},
		Model: strPtr("text-moderation-latest"),
	}

	response, err := moderateContent(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Results, 1)
	assert.False(t, response.Results[0].Flagged, "Safe content should not be flagged")
}

// TestModeration_UnsafeContent tests detection of unsafe content
func TestModeration_UnsafeContent(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ModerationRequest{
		Input: []string{"I want to hurt someone"},
		Model: strPtr("text-moderation-stable"),
	}

	response, err := moderateContent(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Results, 1)
	assert.True(t, response.Results[0].Flagged, "Unsafe content should be flagged")
}

// TestModeration_MultipleInputs tests moderating multiple text inputs
func TestModeration_MultipleInputs(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ModerationRequest{
		Input: []string{
			"Hello, how are you?",
			"This is inappropriate content",
			"I love coding!",
		},
		Model: strPtr("text-moderation-latest"),
	}

	response, err := moderateContent(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)
	assert.Len(t, response.Results, 3)
}

// TestModeration_Categories tests category-specific scores
func TestModeration_Categories(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ModerationRequest{
		Input: []string{"Test content for category detection"},
		Model: strPtr("text-moderation-latest"),
	}

	response, err := moderateContent(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)

	result := response.Results[0]

	// Check that all expected categories are present
	categories := result.Categories
	assert.NotNil(t, categories.Sexual)
	assert.NotNil(t, categories.Hate)
	assert.NotNil(t, categories.Harassment)
	assert.NotNil(t, categories.SelfHarm)
	assert.NotNil(t, categories.Violence)

	// Check category scores
	scores := result.CategoryScores
	assert.NotNil(t, scores.Sexual)
	assert.NotNil(t, scores.Hate)
	assert.NotNil(t, scores.Harassment)
	assert.NotNil(t, scores.SelfHarm)
	assert.NotNil(t, scores.Violence)
}

// TestModeration_ThresholdSettings tests custom threshold configuration
func TestModeration_ThresholdSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Test with custom thresholds (this might be Ogem-specific extension)
	request := &openai.ModerationRequest{
		Input: []string{"Borderline content that might need custom thresholds"},
		Model: strPtr("text-moderation-latest"),
	}

	response, err := moderateContent(context.Background(), request)
	require.NoError(t, err)
	require.NotNil(t, response)

	// Verify scores can be compared against custom thresholds
	result := response.Results[0]
	assert.NotNil(t, result.CategoryScores)
}

// TestModeration_EmptyInput tests error handling for empty input
func TestModeration_EmptyInput(t *testing.T) {
	request := &openai.ModerationRequest{
		Input: []string{},
		Model: strPtr("text-moderation-latest"),
	}

	_, err := moderateContent(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "input is required")
}

// TestModeration_InvalidModel tests error handling for invalid model
func TestModeration_InvalidModel(t *testing.T) {
	request := &openai.ModerationRequest{
		Input: []string{"Test content"},
		Model: strPtr("invalid-model"),
	}

	_, err := moderateContent(context.Background(), request)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid model")
}

// TestModeration_Caching tests that moderation responses are cached
func TestModeration_Caching(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	request := &openai.ModerationRequest{
		Input: []string{"Unique content for cache testing 12345"},
		Model: strPtr("text-moderation-latest"),
	}

	// First request
	resp1, err := moderateContent(context.Background(), request)
	require.NoError(t, err)

	// Second identical request should be cached
	resp2, err := moderateContent(context.Background(), request)
	require.NoError(t, err)

	// Both should have identical results
	assert.Equal(t, resp1.Results[0].Flagged, resp2.Results[0].Flagged)
	assert.Equal(t, resp1.Results[0].CategoryScores, resp2.Results[0].CategoryScores)
}

// TestModeration_RateLimiting tests rate limiting for moderation
func TestModeration_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement rate limiting test
	t.Skip("Rate limiting test not yet implemented")
}

// TestModeration_CustomGuardrails tests Ogem-specific guardrail features
func TestModeration_CustomGuardrails(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// TODO: Implement when custom guardrails are ready
	t.Skip("Custom guardrails not yet implemented")
}

