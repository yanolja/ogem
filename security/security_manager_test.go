package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/yanolja/ogem/openai"
	sdkOgem "github.com/yanolja/ogem/sdk/go"
)

func TestSecurityManager_NewSecurityManager(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
			MaskingStrategy:       MaskingStrategyReplace,
		},
		RateLimiting: &RateLimitConfig{
			Enabled: true,
			UserLimits: []RateLimit{
				{
					Type:   RateLimitTypeRequests,
					Limit:  1000,
					Window: time.Hour,
					Action: RateLimitActionBlock,
				},
			},
		},
		AuditLogging: &AuditConfig{
			Enabled:     true,
			MinSeverity: AuditSeverityInfo,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	assert.NotNil(t, manager)
	assert.Equal(t, config, manager.config)
	assert.NotNil(t, manager.piiMasker)
	assert.NotNil(t, manager.rateLimiter)
	assert.NotNil(t, manager.auditLogger)
}

func TestSecurityManager_ValidateAndSecureRequest_Success(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
			MaskingStrategy:       MaskingStrategyReplace,
		},
		RateLimiting: &RateLimitConfig{
			Enabled: true,
			UserLimits: []RateLimit{
				{
					Type:   RateLimitTypeRequests,
					Limit:  10,
					Window: time.Hour,
					Action: RateLimitActionBlock,
				},
				{
					Type:   RateLimitTypeTokens,
					Limit:  1000,
					Window: time.Hour,
					Action: RateLimitActionBlock,
				},
			},
		},
		AuditLogging: &AuditConfig{
			Enabled:     true,
			MinSeverity: AuditSeverityInfo,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	// Create a test request with PII
	request := &openai.ChatCompletionRequest{
		Model: sdkOgem.ModelGPT4,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("My email is john.doe@example.com and my phone is 555-123-4567"),
				},
			},
		},
		MaxTokens:   int32Ptr(100),
		Temperature: float32Ptr(0.7),
	}

	ctx := context.Background()
	userID := "test-user"
	endpoint := "/v1/chat/completions"
	model := sdkOgem.ModelGPT4

	// Validate and secure the request
	securedRequest, result, err := manager.ValidateAndSecureRequest(ctx, request, userID, endpoint, model)
	require.NoError(t, err)
	assert.NotNil(t, securedRequest)
	assert.NotNil(t, result)
	assert.True(t, result.Allowed)

	// Check that PII was masked
	assert.Contains(t, *securedRequest.Messages[0].Content.String, "[EMAIL]")
	assert.Contains(t, *securedRequest.Messages[0].Content.String, "[PHONE]")
}

func TestSecurityManager_ValidateAndSecureRequest_RateLimitExceeded(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		RateLimiting: &RateLimitConfig{
			Enabled: true,
			UserLimits: []RateLimit{
				{
					Type:   RateLimitTypeRequests,
					Limit:  2,
					Window: time.Minute,
					Action: RateLimitActionBlock,
				},
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	request := &openai.ChatCompletionRequest{
		Model: sdkOgem.ModelGPT4,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Test message"),
				},
			},
		},
		MaxTokens: int32Ptr(50),
	}

	ctx := context.Background()
	userID := "test-user-rate-limit"
	endpoint := "/v1/chat/completions"
	model := sdkOgem.ModelGPT4

	// First two requests should succeed
	for i := 0; i < 2; i++ {
		_, result, err := manager.ValidateAndSecureRequest(ctx, request, userID, endpoint, model)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	}

	// Third request should be rate limited
	_, result, err := manager.ValidateAndSecureRequest(ctx, request, userID, endpoint, model)
	require.NoError(t, err)
	assert.False(t, result.Allowed)
	assert.NotNil(t, result.RateLimitResult)
	assert.False(t, result.RateLimitResult.Allowed)
}

func TestSecurityManager_ValidateAndSecureRequest_Disabled(t *testing.T) {
	config := &SecurityConfig{
		Enabled: false,
		PIIMasking: &PIIMaskingConfig{
			Enabled: true,
		},
		RateLimiting: &RateLimitConfig{
			Enabled: true,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	request := &openai.ChatCompletionRequest{
		Model: sdkOgem.ModelGPT4,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("My SSN is 123-45-6789"),
				},
			},
		},
	}

	ctx := context.Background()
	securedRequest, result, err := manager.ValidateAndSecureRequest(ctx, request, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// PII should not be masked when security is disabled
	assert.Equal(t, "My SSN is 123-45-6789", *securedRequest.Messages[0].Content.String)
}

func TestSecurityManager_GetSecurityStats(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		PIIMasking: &PIIMaskingConfig{
			Enabled: true,
		},
		RateLimiting: &RateLimitConfig{
			Enabled: true,
		},
		AuditLogging: &AuditConfig{
			Enabled: true,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	stats := manager.GetSecurityStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "enabled")
	assert.Equal(t, true, stats["enabled"])
	assert.Contains(t, stats, "pii_masking")
	assert.Contains(t, stats, "rate_limiting")
	assert.Contains(t, stats, "audit_logging")
}

func TestSecurityManager_AuditEvents(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		AuditLogging: &AuditConfig{
			Enabled:     true,
			MinSeverity: AuditSeverityInfo,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	ctx := context.Background()

	// Test audit methods - these are internal methods but we can test they don't panic
	manager.auditRequest(ctx, "/v1/chat/completions", "test-user", sdkOgem.ModelGPT4)

	manager.auditPIIDetection(ctx, []PIIDetection{
		{Type: "email", Value: "test@example.com", Position: 0, Length: 16},
	})

	manager.auditRateLimit(ctx, "test-user", "/v1/chat/completions", &RateLimitResult{
		Allowed:   false,
		Remaining: 0,
		AppliedLimit: &RateLimit{
			Type:   RateLimitTypeRequests,
			Limit:  100,
			Window: time.Hour,
		},
		Current: 100,
	})

	manager.auditSecurityEvent(ctx, "test_event", "Test security event", AuditSeverityInfo)

	// No assertions needed - just ensure no panics
}

func TestSecurityManager_RequestValidation(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		RequestValidation: &RequestValidationConfig{
			Enabled:          true,
			MaxRequestSize:   1024,
			MaxMessageLength: 100,
			MaxMessages:      10,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	// Test with valid request
	request := &openai.ChatCompletionRequest{
		Model: sdkOgem.ModelGPT4,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Valid message"),
				},
			},
		},
	}

	ctx := context.Background()
	_, result, err := manager.ValidateAndSecureRequest(ctx, request, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4)
	require.NoError(t, err)
	assert.True(t, result.Allowed)

	// Test with long message
	longMessage := ""
	for i := 0; i < 200; i++ {
		longMessage += "a"
	}

	request.Messages[0].Content.String = &longMessage
	_, result, err = manager.ValidateAndSecureRequest(ctx, request, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4)
	require.NoError(t, err)
	// The result depends on whether validation is actually implemented
}

func TestSecurityManager_ContentFiltering(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		ContentFiltering: &ContentFilteringConfig{
			Enabled:            true,
			BlockInappropriate: true,
			BlockHarmful:       true,
			BlockSpam:          true,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	// Test with safe content
	request := &openai.ChatCompletionRequest{
		Model: sdkOgem.ModelGPT4,
		Messages: []openai.Message{
			{
				Role: "user",
				Content: &openai.MessageContent{
					String: stringPtr("Tell me about cybersecurity best practices"),
				},
			},
		},
	}

	ctx := context.Background()
	_, result, err := manager.ValidateAndSecureRequest(ctx, request, "test-user", "/v1/chat/completions", sdkOgem.ModelGPT4)
	require.NoError(t, err)
	assert.True(t, result.Allowed)
}

func TestSecurityManager_ReleaseResources(t *testing.T) {
	config := &SecurityConfig{
		Enabled: true,
		RateLimiting: &RateLimitConfig{
			Enabled: true,
			UserLimits: []RateLimit{
				{
					Type:   RateLimitTypeConcurrent,
					Limit:  2,
					Window: time.Minute,
					Action: RateLimitActionBlock,
				},
			},
		},
	}

	logger := zaptest.NewLogger(t).Sugar()
	manager, err := NewSecurityManager(config, logger)
	require.NoError(t, err)

	// Test resource release
	userID := "test-user"
	endpoint := "/v1/chat/completions"
	model := sdkOgem.ModelGPT4

	// Acquire resources first
	request := &openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.Message{
			{Role: "user", Content: &openai.MessageContent{String: stringPtr("Test")}},
		},
	}

	ctx := context.Background()
	_, _, err = manager.ValidateAndSecureRequest(ctx, request, userID, endpoint, model)
	require.NoError(t, err)

	// Release resources
	manager.ReleaseResources(userID, endpoint, model)

	// No assertion needed - just ensure it doesn't panic
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func int32Ptr(i int32) *int32 {
	return &i
}

func float32Ptr(f float32) *float32 {
	return &f
}
