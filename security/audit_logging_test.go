package security

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest"
)

func TestAuditLogger_NewAuditLogger(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityInfo,
		IncludeBodies: false,
		IncludeClientInfo: true,
		BufferSize:  1000,
		FlushInterval: 5 * time.Second,
		RetentionPeriod: 90 * 24 * time.Hour,
	}

	logger := zaptest.NewLogger(t).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	assert.NotNil(t, auditLogger)
	assert.Equal(t, config, auditLogger.config)
}

func TestAuditLogger_LogRequest(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "client_ip", "192.168.1.100")
	ctx = context.WithValue(ctx, "user_agent", "Mozilla/5.0")
	
	metadata := map[string]interface{}{
		"model":  "gpt-3.5-turbo",
		"tokens": 150,
		"user_id": "user123",
	}

	auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, metadata)

	// Verify log was written (basic check since zaptest buffer format is complex)
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "POST")
	assert.Contains(t, logOutput, "/v1/chat/completions")
}

func TestAuditLogger_LogSecurityEvent(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "client_ip", "192.168.1.100")
	ctx = context.WithValue(ctx, "user_agent", "Bot/1.0")
	
	metadata := map[string]interface{}{
		"reason": "Too many requests",
		"limit":  100,
		"risk_level": "HIGH",
	}

	auditLogger.LogSecurityEvent(ctx, "RATE_LIMIT_EXCEEDED", "Too many requests", AuditSeverityWarning, metadata)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "SECURITY_EVENT")
	assert.Contains(t, logOutput, "RATE_LIMIT_EXCEEDED")
}

func TestAuditLogger_LogError(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "client_ip", "192.168.1.100")
	
	metadata := map[string]interface{}{
		"error_code": "INTERNAL_ERROR",
		"error_details": "Connection timeout after 30s",
	}

	auditLogger.LogSecurityEvent(ctx, "ERROR", "Database connection failed", AuditSeverityError, metadata)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "SECURITY_EVENT")
	assert.Contains(t, logOutput, "ERROR")
}

func TestAuditLogger_LogPIIDetection(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "client_ip", "192.168.1.100")
	
	detections := []PIIDetection{
		{Type: PIITypeEmail, Value: "test@example.com", Position: 0, Length: 16, Confidence: 0.9},
		{Type: PIITypePhone, Value: "123-456-7890", Position: 20, Length: 12, Confidence: 0.8},
	}

	auditLogger.LogPIIDetection(ctx, detections, true)

	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "PII_DETECTION")
	assert.Contains(t, logOutput, "detection_count")
}

func TestAuditLogger_Disabled(t *testing.T) {
	config := &AuditConfig{
		Enabled: false,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()

	auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, nil)

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

	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	logger := zaptest.NewLogger(t).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			// The audit logger should handle invalid events gracefully
			// and not panic
			assert.NotPanics(t, func() {
				if tt.event != nil {
					metadata := map[string]interface{}{
						"event_type": tt.event.EventType,
						"user_id": tt.event.UserID,
						"action": tt.event.Action,
					}
					auditLogger.LogRequest(ctx, "POST", "/test", 200, 0, metadata)
				}
			})
		})
	}
}

func TestAuditLogger_ContextualLogging(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	// Create context with values
	ctx := context.WithValue(context.Background(), "correlation_id", "corr-123")
	ctx = context.WithValue(ctx, "session_id", "sess-456")

	metadata := map[string]interface{}{
		"model": "gpt-4",
		"user_id": "user123",
	}

	auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, metadata)

	// Basic verification that logging occurred
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "POST")
}

func TestAuditLogger_HighVolumeLogging(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()

	// Log many events to test performance
	start := time.Now()
	for i := 0; i < 1000; i++ {
		metadata := map[string]interface{}{
			"iteration": i,
			"user_id": "user123",
		}
		auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, metadata)
	}
	duration := time.Since(start)

	// Should complete within reasonable time
	assert.Less(t, duration, 5*time.Second)

	// Verify some logs were written
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "POST")
}

func TestAuditLogger_StructuredLogging(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "client_ip", "192.168.1.100")
	ctx = context.WithValue(ctx, "user_agent", "TestAgent/1.0")

	metadata := map[string]interface{}{
		"model":       "gpt-3.5-turbo",
		"tokens":      150,
		"temperature": 0.7,
		"user_id":     "user123",
		"tenant_id":   "tenant456",
	}

	auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 150*time.Millisecond, metadata)

	logOutput := logBuffer.String()

	// Should contain structured fields
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "POST")
	assert.Contains(t, logOutput, "/v1/chat/completions")
}

func TestAuditLogger_RateLimit(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "request_id", "req-123")
	ctx = context.WithValue(ctx, "client_ip", "192.168.1.100")

	auditLogger.LogRateLimit(ctx, "user123", "/v1/chat/completions", 100, 150, 1*time.Minute)

	logOutput := logBuffer.String()

	// Should contain rate limit information
	assert.Contains(t, logOutput, "RATE_LIMIT")
	assert.Contains(t, logOutput, "user123")
	assert.Contains(t, logOutput, "/v1/chat/completions")
}

func TestAuditLogger_GetAuditStats(t *testing.T) {
	config := &AuditConfig{
		Enabled:     true,
		MinSeverity: AuditSeverityInfo,
		IncludeBodies: true,
		IncludeClientInfo: true,
		BufferSize: 1000,
	}

	logger := zaptest.NewLogger(t).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	stats := auditLogger.GetAuditStats()

	// Verify stats are returned correctly
	assert.Equal(t, true, stats["enabled"])
	assert.Equal(t, 1000, stats["buffer_size"])
	assert.Equal(t, "info", stats["min_severity"])
	assert.Equal(t, true, stats["include_bodies"])
	assert.Equal(t, true, stats["include_client_info"])
}

func TestAuditLogger_AsyncLogging(t *testing.T) {
	config := &AuditConfig{
		Enabled:      true,
		MinSeverity:  AuditSeverityInfo,
		BufferSize:   1000,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()

	// Log events that should be buffered
	for i := 0; i < 10; i++ {
		metadata := map[string]interface{}{
			"user_id": "user123",
			"iteration": i,
		}
		auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, metadata)
	}

	// Give async logging time to process
	time.Sleep(100 * time.Millisecond)

	// Verify logs were written
	logOutput := logBuffer.String()
	assert.Contains(t, logOutput, "REQUEST")
	assert.Contains(t, logOutput, "POST")
}

func TestAuditLogger_ErrorHandling(t *testing.T) {
	config := &AuditConfig{
		Enabled:  true,
		MinSeverity: AuditSeverityInfo,
	}

	var logBuffer bytes.Buffer
	loggerConfig := zap.NewDevelopmentEncoderConfig()
	encoder := zapcore.NewConsoleEncoder(loggerConfig)
	core := zapcore.NewCore(encoder, zapcore.AddSync(&logBuffer), zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	auditLogger := NewAuditLogger(config, logger)

	ctx := context.Background()

	// Test with nil metadata
	assert.NotPanics(t, func() {
		auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, nil)
	})

	// Test with circular reference in metadata
	circular := make(map[string]interface{})
	circular["self"] = circular

	assert.NotPanics(t, func() {
		auditLogger.LogRequest(ctx, "POST", "/v1/chat/completions", 200, 100*time.Millisecond, circular)
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
		config *AuditConfig
		valid  bool
	}{
		{
			name: "valid config",
			config: &AuditConfig{
				Enabled:     true,
				MinSeverity: AuditSeverityInfo,
				IncludeBodies: false,
				IncludeClientInfo: true,
				BufferSize: 1000,
			},
			valid: true,
		},
		{
			name: "disabled config",
			config: &AuditConfig{
				Enabled:  false,
			},
			valid: true,
		},
		{
			name: "nil config",
			config: nil,
			valid: true,
		},
	}

	logger := zaptest.NewLogger(t).Sugar()

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
