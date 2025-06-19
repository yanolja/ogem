package security

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestAuditLogger_NewAuditLogger(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:    true,
		LogLevel:   "INFO",
		OutputPath: "stdout",
		MaxFileSize: 100,
		MaxBackups:  5,
		MaxAge:      30,
		Compress:    true,
	}

	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger)
	
	assert.NotNil(t, auditLogger)
	assert.Equal(t, config, auditLogger.config)
}

func TestAuditLogger_LogRequest(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType:   "REQUEST",
		UserID:      "user123",
		TenantID:    "tenant456",
		Resource:    "/v1/chat/completions",
		Action:      "CREATE",
		IPAddress:   "192.168.1.100",
		UserAgent:   "Mozilla/5.0",
		RequestID:   "req-123",
		Timestamp:   time.Now(),
		Success:     true,
		StatusCode:  200,
		Duration:    100 * time.Millisecond,
		RequestSize: 1024,
		ResponseSize: 2048,
		Metadata: map[string]interface{}{
			"model": "gpt-3.5-turbo",
			"tokens": 150,
		},
	}

	auditLogger.LogRequest(ctx, event)

	// Verify log was written (basic check since zaptest buffer format is complex)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "user123")
	assert.Contains(t, logOutput, "/v1/chat/completions")
}

func TestAuditLogger_LogSecurity(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "WARN",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType:  "SECURITY",
		UserID:     "user123",
		TenantID:   "tenant456",
		Resource:   "/v1/chat/completions",
		Action:     "RATE_LIMIT_EXCEEDED",
		IPAddress:  "192.168.1.100",
		UserAgent:  "Bot/1.0",
		RequestID:  "req-123",
		Timestamp:  time.Now(),
		Success:    false,
		StatusCode: 429,
		RiskLevel:  "HIGH",
		Metadata: map[string]interface{}{
			"reason": "Too many requests",
			"limit":  100,
		},
	}

	auditLogger.LogSecurity(ctx, event)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "SECURITY")
	assert.Contains(t, logOutput, "RATE_LIMIT_EXCEEDED")
	assert.Contains(t, logOutput, "HIGH")
}

func TestAuditLogger_LogError(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "ERROR",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType:    "ERROR",
		UserID:       "user123",
		TenantID:     "tenant456",
		Resource:     "/v1/chat/completions",
		Action:       "CREATE",
		IPAddress:    "192.168.1.100",
		RequestID:    "req-123",
		Timestamp:    time.Now(),
		Success:      false,
		StatusCode:   500,
		ErrorCode:    "INTERNAL_ERROR",
		ErrorMessage: "Database connection failed",
		Metadata: map[string]interface{}{
			"error_details": "Connection timeout after 30s",
		},
	}

	auditLogger.LogError(ctx, event)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "ERROR")
	assert.Contains(t, logOutput, "INTERNAL_ERROR")
	assert.Contains(t, logOutput, "Database connection failed")
}

func TestAuditLogger_LogDataAccess(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType:  "DATA_ACCESS",
		UserID:     "user123",
		TenantID:   "tenant456",
		Resource:   "/v1/models",
		Action:     "LIST",
		IPAddress:  "192.168.1.100",
		RequestID:  "req-123",
		Timestamp:  time.Now(),
		Success:    true,
		StatusCode: 200,
		Metadata: map[string]interface{}{
			"data_type":    "model_list",
			"record_count": 25,
		},
	}

	auditLogger.LogDataAccess(ctx, event)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "DATA_ACCESS")
	assert.Contains(t, logOutput, "LIST")
	assert.Contains(t, logOutput, "model_list")
}

func TestAuditLogger_Disabled(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled: false,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType: "REQUEST",
		UserID:    "user123",
		Action:    "CREATE",
		Timestamp: time.Now(),
	}

	auditLogger.LogRequest(ctx, event)

	// Should not log anything when disabled
	logOutput := logBuffer.String()
	assert.Empty(t, logOutput)
}

func TestAuditEvent_Validation(t *testing.T) {
	tests := []struct {
		name  string
		event *AuditEvent
		valid bool
	}{
		{
			name: "valid event",
			event: &AuditEvent{
				EventType: "REQUEST",
				UserID:    "user123",
				Action:    "CREATE",
				Timestamp: time.Now(),
			},
			valid: true,
		},
		{
			name: "missing event type",
			event: &AuditEvent{
				UserID:    "user123",
				Action:    "CREATE",
				Timestamp: time.Now(),
			},
			valid: false,
		},
		{
			name: "missing action",
			event: &AuditEvent{
				EventType: "REQUEST",
				UserID:    "user123",
				Timestamp: time.Now(),
			},
			valid: false,
		},
		{
			name: "zero timestamp",
			event: &AuditEvent{
				EventType: "REQUEST",
				UserID:    "user123",
				Action:    "CREATE",
			},
			valid: false,
		},
	}

	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
	}

	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			
			// The audit logger should handle invalid events gracefully
			// and not panic
			assert.NotPanics(t, func() {
				auditLogger.LogRequest(ctx, tt.event)
			})
		})
	}
}

func TestAuditLogger_ContextualLogging(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	// Create context with values
	ctx := context.WithValue(context.Background(), "correlation_id", "corr-123")
	ctx = context.WithValue(ctx, "session_id", "sess-456")

	event := &AuditEvent{
		EventType: "REQUEST",
		UserID:    "user123",
		Action:    "CREATE",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"model": "gpt-4",
		},
	}

	auditLogger.LogRequest(ctx, event)

	// Basic verification that logging occurred
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "user123")
}

func TestAuditLogger_HighVolumeLogging(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	
	// Log many events to test performance
	start := time.Now()
	for i := 0; i < 1000; i++ {
		event := &AuditEvent{
			EventType: "REQUEST",
			UserID:    "user123",
			Action:    "CREATE",
			RequestID: "req-" + string(rune(i)),
			Timestamp: time.Now(),
			Metadata: map[string]interface{}{
				"iteration": i,
			},
		}
		auditLogger.LogRequest(ctx, event)
	}
	duration := time.Since(start)

	// Should complete within reasonable time
	assert.Less(t, duration, 5*time.Second)

	// Verify some logs were written
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "user123")
}

func TestAuditLogger_StructuredLogging(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
		Format:   "json",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType:    "REQUEST",
		UserID:       "user123",
		TenantID:     "tenant456",
		Resource:     "/v1/chat/completions",
		Action:       "CREATE",
		IPAddress:    "192.168.1.100",
		UserAgent:    "TestAgent/1.0",
		RequestID:    "req-123",
		Timestamp:    time.Now(),
		Success:      true,
		StatusCode:   200,
		Duration:     150 * time.Millisecond,
		RequestSize:  1024,
		ResponseSize: 2048,
		Metadata: map[string]interface{}{
			"model":       "gpt-3.5-turbo",
			"tokens":      150,
			"temperature": 0.7,
		},
	}

	auditLogger.LogRequest(ctx, event)

	logOutput := logBuffer.String()
	
	// Should contain structured fields
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "user123")
	assert.Contains(t, logOutput, "tenant456")
	assert.Contains(t, logOutput, "/v1/chat/completions")
}

func TestAuditLogger_SensitiveDataFiltering(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:          true,
		LogLevel:         "INFO",
		FilterSensitive:  true,
		SensitiveFields:  []string{"password", "api_key", "secret"},
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	event := &AuditEvent{
		EventType: "REQUEST",
		UserID:    "user123",
		Action:    "CREATE",
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"password":  "secret123",
			"api_key":   "sk-1234567890",
			"username":  "john.doe",
			"secret":    "top-secret-value",
			"public":    "this-is-public",
		},
	}

	auditLogger.LogRequest(ctx, event)

	logOutput := logBuffer.String()
	
	// Sensitive fields should be filtered/masked
	assert.NotContains(t, logOutput, "secret123")
	assert.NotContains(t, logOutput, "sk-1234567890")
	assert.NotContains(t, logOutput, "top-secret-value")
	
	// Public data should still be present
	assert.Contains(t, logOutput, "john.doe")
	assert.Contains(t, logOutput, "this-is-public")
}

func TestAuditLogger_LogRotation(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:     true,
		LogLevel:    "INFO",
		OutputPath:  "/tmp/audit-test.log",
		MaxFileSize: 1, // 1 MB
		MaxBackups:  3,
		MaxAge:      7,
		Compress:    true,
	}

	logger := zaptest.NewLogger(t)
	auditLogger := NewAuditLogger(config, logger)

	// Test that the audit logger was created successfully
	// (actual file rotation testing would require more complex setup)
	assert.NotNil(t, auditLogger)
	assert.Equal(t, config, auditLogger.config)
}

func TestAuditLogger_AsyncLogging(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:     true,
		LogLevel:    "INFO",
		AsyncLogging: true,
		BufferSize:   1000,
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	
	// Log events that should be buffered
	for i := 0; i < 10; i++ {
		event := &AuditEvent{
			EventType: "REQUEST",
			UserID:    "user123",
			Action:    "CREATE",
			RequestID: "req-" + string(rune(i)),
			Timestamp: time.Now(),
		}
		auditLogger.LogRequest(ctx, event)
	}

	// Give async logging time to process
	time.Sleep(100 * time.Millisecond)

	// Verify logs were written
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "user123")
}

func TestAuditLogger_ErrorHandling(t *testing.T) {
	config := &AuditLoggingConfig{
		Enabled:  true,
		LogLevel: "INFO",
	}

	var logBuffer bytes.Buffer
	logger := zaptest.NewLogger(t, zaptest.WrapOptions(zaptest.WriteTo(&logBuffer)))
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()

	// Test with nil event
	assert.NotPanics(t, func() {
		auditLogger.LogRequest(ctx, nil)
	})

	// Test with event containing nil metadata
	event := &AuditEvent{
		EventType: "REQUEST",
		UserID:    "user123",
		Action:    "CREATE",
		Timestamp: time.Now(),
		Metadata:  nil,
	}

	assert.NotPanics(t, func() {
		auditLogger.LogRequest(ctx, event)
	})

	// Test with event containing circular reference in metadata
	circular := make(map[string]interface{})
	circular["self"] = circular
	
	event = &AuditEvent{
		EventType: "REQUEST",
		UserID:    "user123",
		Action:    "CREATE",
		Timestamp: time.Now(),
		Metadata:  circular,
	}

	assert.NotPanics(t, func() {
		auditLogger.LogRequest(ctx, event)
	})
}

func TestAuditEvent_Serialization(t *testing.T) {
	event := &AuditEvent{
		EventType:    "REQUEST",
		UserID:       "user123",
		TenantID:     "tenant456",
		Resource:     "/v1/chat/completions",
		Action:       "CREATE",
		IPAddress:    "192.168.1.100",
		UserAgent:    "TestAgent/1.0",
		RequestID:    "req-123",
		Timestamp:    time.Now(),
		Success:      true,
		StatusCode:   200,
		Duration:     150 * time.Millisecond,
		RequestSize:  1024,
		ResponseSize: 2048,
		RiskLevel:    "LOW",
		ErrorCode:    "",
		ErrorMessage: "",
		Metadata: map[string]interface{}{
			"model":  "gpt-3.5-turbo",
			"tokens": 150,
		},
	}

	// Test JSON serialization
	jsonData, err := json.Marshal(event)
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "REQUEST")
	assert.Contains(t, string(jsonData), "user123")

	// Test JSON deserialization
	var deserializedEvent AuditEvent
	err = json.Unmarshal(jsonData, &deserializedEvent)
	require.NoError(t, err)
	assert.Equal(t, event.EventType, deserializedEvent.EventType)
	assert.Equal(t, event.UserID, deserializedEvent.UserID)
	assert.Equal(t, event.Action, deserializedEvent.Action)
}

func TestAuditLogger_Configuration_Validation(t *testing.T) {
	tests := []struct {
		name   string
		config *AuditLoggingConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &AuditLoggingConfig{
				Enabled:     true,
				LogLevel:    "INFO",
				OutputPath:  "stdout",
				MaxFileSize: 100,
				MaxBackups:  5,
				MaxAge:      30,
			},
			valid: true,
		},
		{
			name: "invalid log level",
			config: &AuditLoggingConfig{
				Enabled:  true,
				LogLevel: "INVALID",
			},
			valid: false,
		},
		{
			name: "negative file size",
			config: &AuditLoggingConfig{
				Enabled:     true,
				LogLevel:    "INFO",
				MaxFileSize: -100,
			},
			valid: false,
		},
	}

	logger := zaptest.NewLogger(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic even with invalid config
			assert.NotPanics(t, func() {
				auditLogger := NewAuditLogger(tt.config, logger)
				assert.NotNil(t, auditLogger)
			})
		})
	}
}