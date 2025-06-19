package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestSecurityManager_NewSecurityManager(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 1000,
			DefaultTokensPerHour:   100000,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	assert.NotNil(t, manager)
	assert.Equal(t, config, manager.config)
	assert.NotNil(t, manager.piiMasker)
	assert.NotNil(t, manager.rateLimiter)
	assert.NotNil(t, manager.auditLogger)
}

func TestSecurityManager_ValidateAndSecureRequest_Success(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 10,
			DefaultTokensPerHour:   1000,
			SlidingWindowSize:      time.Hour,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	request := &SecurityRequest{
		UserID:       "user123",
		TenantID:     "tenant456",
		IPAddress:    "192.168.1.100",
		UserAgent:    "TestAgent/1.0",
		RequestID:    "req-123",
		Content:      "Hello, this is a test message",
		ContentType:  "text/plain",
		RequestSize:  100,
		TokenCount:   25,
		Endpoint:     "/v1/chat/completions",
		Method:       "POST",
	}

	response, err := manager.ValidateAndSecureRequest(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Allowed)
	assert.Equal(t, "Hello, this is a test message", response.SecuredContent)
	assert.Empty(t, response.DenialReason)
}

func TestSecurityManager_ValidateAndSecureRequest_PIIMasking(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled: false,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: false,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	request := &SecurityRequest{
		UserID:      "user123",
		TenantID:    "tenant456",
		Content:     "My email is john.doe@example.com and my SSN is 123-45-6789",
		ContentType: "text/plain",
		Endpoint:    "/v1/chat/completions",
		Method:      "POST",
	}

	response, err := manager.ValidateAndSecureRequest(ctx, request)
	require.NoError(t, err)
	assert.NotNil(t, response)
	assert.True(t, response.Allowed)
	assert.Contains(t, response.SecuredContent, "[EMAIL]")
	assert.Contains(t, response.SecuredContent, "[SSN]")
	assert.NotContains(t, response.SecuredContent, "john.doe@example.com")
	assert.NotContains(t, response.SecuredContent, "123-45-6789")
}

func TestSecurityManager_ValidateAndSecureRequest_RateLimited(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: false,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 1,
			DefaultTokensPerHour:   100,
			SlidingWindowSize:      time.Hour,
			BurstMultiplier:        1.0,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	request := &SecurityRequest{
		UserID:      "user123",
		TenantID:    "tenant456",
		Content:     "Test message",
		ContentType: "text/plain",
		TokenCount:  25,
		Endpoint:    "/v1/chat/completions",
		Method:      "POST",
	}

	// First request should succeed
	response, err := manager.ValidateAndSecureRequest(ctx, request)
	require.NoError(t, err)
	assert.True(t, response.Allowed)

	// Second request should be rate limited
	response, err = manager.ValidateAndSecureRequest(ctx, request)
	require.NoError(t, err)
	assert.False(t, response.Allowed)
	assert.Contains(t, response.DenialReason, "rate limit")
}

func TestSecurityManager_ValidateAndSecureRequest_TokenLimited(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: false,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 10,
			DefaultTokensPerHour:   100,
			SlidingWindowSize:      time.Hour,
			BurstMultiplier:        1.0,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: false,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	request := &SecurityRequest{
		UserID:      "user123",
		TenantID:    "tenant456",
		Content:     "Test message",
		ContentType: "text/plain",
		TokenCount:  150, // Exceeds limit of 100
		Endpoint:    "/v1/chat/completions",
		Method:      "POST",
	}

	response, err := manager.ValidateAndSecureRequest(ctx, request)
	require.NoError(t, err)
	assert.False(t, response.Allowed)
	assert.Contains(t, response.DenialReason, "rate limit")
}

func TestSecurityManager_LogSecurityEvent(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: false,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled: false,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType:  "SECURITY",
		UserID:     "user123",
		TenantID:   "tenant456",
		Resource:   "/v1/chat/completions",
		Action:     "SUSPICIOUS_ACTIVITY",
		IPAddress:  "192.168.1.100",
		RequestID:  "req-123",
		Timestamp:  time.Now(),
		Success:    false,
		RiskLevel:  "HIGH",
		Metadata: map[string]interface{}{
			"reason": "Multiple failed attempts",
		},
	}

	// Should not panic
	assert.NotPanics(t, func() {
		manager.LogSecurityEvent(ctx, event)
	})
}

func TestSecurityManager_GetRateLimitUsage(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: false,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 10,
			DefaultTokensPerHour:   1000,
			SlidingWindowSize:      time.Hour,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: false,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	userKey := "user123"

	// Make some requests
	request := &SecurityRequest{
		UserID:     userKey,
		TenantID:   "tenant456",
		Content:    "Test message",
		TokenCount: 25,
		Endpoint:   "/v1/chat/completions",
		Method:     "POST",
	}

	manager.ValidateAndSecureRequest(ctx, request)
	manager.ValidateAndSecureRequest(ctx, request)

	usage := manager.GetRateLimitUsage(userKey)
	assert.Equal(t, 2, usage.RequestCount)
	assert.Equal(t, 50, usage.TokenCount)
}

func TestSecurityManager_ResetRateLimitUsage(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: false,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 10,
			DefaultTokensPerHour:   1000,
			SlidingWindowSize:      time.Hour,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: false,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	userKey := "user123"

	// Make some requests
	request := &SecurityRequest{
		UserID:     userKey,
		TenantID:   "tenant456",
		Content:    "Test message",
		TokenCount: 25,
		Endpoint:   "/v1/chat/completions",
		Method:     "POST",
	}

	manager.ValidateAndSecureRequest(ctx, request)

	// Verify usage
	usage := manager.GetRateLimitUsage(userKey)
	assert.Equal(t, 1, usage.RequestCount)

	// Reset usage
	manager.ResetRateLimitUsage(userKey)

	// Verify usage is reset
	usage = manager.GetRateLimitUsage(userKey)
	assert.Equal(t, 0, usage.RequestCount)
}

func TestSecurityManager_GetSecurityMetrics(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 10,
			DefaultTokensPerHour:   1000,
			SlidingWindowSize:      time.Hour,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	metrics := manager.GetSecurityMetrics()
	assert.NotNil(t, metrics)
	assert.True(t, metrics.PIIMaskingEnabled)
	assert.True(t, metrics.RateLimitingEnabled)
	assert.True(t, metrics.AuditLoggingEnabled)
	assert.Equal(t, int64(0), metrics.TotalRequests)
	assert.Equal(t, int64(0), metrics.BlockedRequests)
	assert.Equal(t, int64(0), metrics.PIIDetections)
}

func TestSecurityManager_UpdateSecurityMetrics(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 2,
			DefaultTokensPerHour:   1000,
			SlidingWindowSize:      time.Hour,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()

	// Make requests that should trigger metrics updates
	request1 := &SecurityRequest{
		UserID:      "user123",
		TenantID:    "tenant456",
		Content:     "My email is test@example.com",
		ContentType: "text/plain",
		TokenCount:  25,
		Endpoint:    "/v1/chat/completions",
		Method:      "POST",
	}

	// This should be allowed but trigger PII detection
	response, err := manager.ValidateAndSecureRequest(ctx, request1)
	require.NoError(t, err)
	assert.True(t, response.Allowed)

	// Make another request
	response, err = manager.ValidateAndSecureRequest(ctx, request1)
	require.NoError(t, err)
	assert.True(t, response.Allowed)

	// Third request should be rate limited
	response, err = manager.ValidateAndSecureRequest(ctx, request1)
	require.NoError(t, err)
	assert.False(t, response.Allowed)

	metrics := manager.GetSecurityMetrics()
	assert.Equal(t, int64(3), metrics.TotalRequests)
	assert.Equal(t, int64(1), metrics.BlockedRequests)
	assert.Greater(t, metrics.PIIDetections, int64(0))
}

func TestSecurityManager_ConcurrentRequests(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled:               true,
			EnableBuiltinPatterns: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled:                true,
			DefaultRequestsPerHour: 100,
			DefaultTokensPerHour:   10000,
			SlidingWindowSize:      time.Hour,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled:  true,
			LogLevel: "INFO",
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	const numGoroutines = 10
	const requestsPerGoroutine = 5

	results := make(chan bool, numGoroutines*requestsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			for j := 0; j < requestsPerGoroutines; j++ {
				request := &SecurityRequest{
					UserID:      "user123",
					TenantID:    "tenant456",
					Content:     "Test message",
					ContentType: "text/plain",
					TokenCount:  25,
					Endpoint:    "/v1/chat/completions",
					Method:      "POST",
				}

				response, err := manager.ValidateAndSecureRequest(ctx, request)
				if err != nil {
					results <- false
				} else {
					results <- response.Allowed
				}
			}
		}(i)
	}

	// Collect results
	allowedCount := 0
	for i := 0; i < numGoroutines*requestsPerGoroutine; i++ {
		if <-results {
			allowedCount++
		}
	}

	// Should handle concurrent requests without errors
	assert.Greater(t, allowedCount, 0)
	assert.LessOrEqual(t, allowedCount, numGoroutines*requestsPerGoroutine)
}

func TestSecurityManager_EdgeCases(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled: true,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: true,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()

	tests := []struct {
		name    string
		request *SecurityRequest
	}{
		{
			name:    "nil request",
			request: nil,
		},
		{
			name: "empty content",
			request: &SecurityRequest{
				UserID:   "user123",
				TenantID: "tenant456",
				Content:  "",
				Endpoint: "/v1/chat/completions",
				Method:   "POST",
			},
		},
		{
			name: "very long content",
			request: &SecurityRequest{
				UserID:   "user123",
				TenantID: "tenant456",
				Content:  string(make([]byte, 100000)),
				Endpoint: "/v1/chat/completions",
				Method:   "POST",
			},
		},
		{
			name: "special characters",
			request: &SecurityRequest{
				UserID:   "user123",
				TenantID: "tenant456",
				Content:  "!@#$%^&*()_+-=[]{}|;':\",./<>?",
				Endpoint: "/v1/chat/completions",
				Method:   "POST",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should handle edge cases gracefully without panicking
			assert.NotPanics(t, func() {
				_, err := manager.ValidateAndSecureRequest(ctx, tt.request)
				// Error is acceptable for nil request
				if tt.request == nil {
					assert.Error(t, err)
				}
			})
		})
	}
}

func TestSecurityManager_AllDisabled(t *testing.T) {
	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: false,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled: false,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: false,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()
	request := &SecurityRequest{
		UserID:      "user123",
		TenantID:    "tenant456",
		Content:     "My email is test@example.com and SSN is 123-45-6789",
		ContentType: "text/plain",
		TokenCount:  25,
		Endpoint:    "/v1/chat/completions",
		Method:      "POST",
	}

	response, err := manager.ValidateAndSecureRequest(ctx, request)
	require.NoError(t, err)
	assert.True(t, response.Allowed)
	// Content should be unchanged when PII masking is disabled
	assert.Equal(t, request.Content, response.SecuredContent)
}

func TestSecurityRequest_Validation(t *testing.T) {
	tests := []struct {
		name    string
		request *SecurityRequest
		valid   bool
	}{
		{
			name: "valid request",
			request: &SecurityRequest{
				UserID:   "user123",
				TenantID: "tenant456",
				Content:  "Test content",
				Endpoint: "/v1/chat/completions",
				Method:   "POST",
			},
			valid: true,
		},
		{
			name: "missing user ID",
			request: &SecurityRequest{
				TenantID: "tenant456",
				Content:  "Test content",
				Endpoint: "/v1/chat/completions",
				Method:   "POST",
			},
			valid: false,
		},
		{
			name: "missing endpoint",
			request: &SecurityRequest{
				UserID:   "user123",
				TenantID: "tenant456",
				Content:  "Test content",
				Method:   "POST",
			},
			valid: false,
		},
		{
			name: "missing method",
			request: &SecurityRequest{
				UserID:   "user123",
				TenantID: "tenant456",
				Content:  "Test content",
				Endpoint: "/v1/chat/completions",
			},
			valid: false,
		},
	}

	config := &SecurityManagerConfig{
		PIIMasking: &PIIMaskingConfig{
			Enabled: true,
		},
		RateLimiting: &RateLimitingConfig{
			Enabled: true,
		},
		AuditLogging: &AuditLoggingConfig{
			Enabled: true,
		},
	}

	logger := zaptest.NewLogger(t)
	manager := NewSecurityManager(config, logger)

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := manager.ValidateAndSecureRequest(ctx, tt.request)
			
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}