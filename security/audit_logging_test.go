package security

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestAuditLogger_NewAuditLogger(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityInfo,
	}

	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger.Sugar())

	assert.NotNil(t, auditLogger)
	assert.Equal(t, config, auditLogger.config)
}

func TestAuditLogger_LogRequest(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger.Sugar())

	ctx := context.Background()
	details := map[string]interface{}{
		"request_size":  1024,
		"response_size": 2048,
		"model":         "gpt-3.5-turbo",
		"tokens":        150,
	}

	auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, details)

	// Verify log was written (basic check since zaptest buffer format is complex)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "POST")
	assert.Contains(t, logOutput, "/v1/chat/completions")
}

func TestAuditLogger_LogSecurity(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityWarning,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger.Sugar())

	ctx := context.Background()
	details := map[string]interface{}{
		"reason": "Too many requests",
		"limit":  100,
	}

	auditLogger.LogSecurityEvent(ctx, "RATE_LIMIT_EXCEEDED", "Rate limit exceeded", AuditSeverityWarning, details)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "SECURITY_EVENT")
	assert.Contains(t, logOutput, "RATE_LIMIT_EXCEEDED")
}

func TestAuditLogger_LogError(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityError,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger.Sugar())

	ctx := context.Background()
	details := map[string]interface{}{
		"error_details": "Connection timeout after 30s",
	}

	auditLogger.LogError(ctx, "INTERNAL_ERROR", "Database connection failed", details)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "SECURITY_EVENT")
	assert.Contains(t, logOutput, "ERROR")
}

func TestAuditLogger_LogDataAccess(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger.Sugar())

	ctx := context.Background()
	details := map[string]interface{}{
		"data_type":    "model_list",
		"record_count": 25,
	}

	auditLogger.LogSecurityEvent(ctx, "DATA_ACCESS", "Accessed model list", AuditSeverityInfo, details)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "PII_DETECTION")
	assert.Contains(t, logOutput, "detection_count")
}

func TestAuditLogger_Disabled(t *testing.T) {
	config := &AuditConfig{
		Enabled: false,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger.Sugar())

	ctx := context.Background()

	auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, nil)

	// Should not log anything when disabled
	logOutput := logBuffer.String()
	assert.Empty(t, logOutput)
}
