package security

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// AuditEventType represents different types of audit events
type AuditEventType string

const (
	AuditEventRequest         AuditEventType = "request"
	AuditEventAuthentication  AuditEventType = "authentication"
	AuditEventAuthorization   AuditEventType = "authorization"
	AuditEventPIIDetection    AuditEventType = "pii_detection"
	AuditEventRateLimit       AuditEventType = "rate_limit"
	AuditEventError           AuditEventType = "error"
	AuditEventConfiguration   AuditEventType = "configuration"
	AuditEventDataAccess      AuditEventType = "data_access"
	AuditEventSecurityEvent   AuditEventType = "security_event"
)

// AuditSeverity represents the severity level of audit events
type AuditSeverity string

const (
	AuditSeverityInfo     AuditSeverity = "info"
	AuditSeverityWarning  AuditSeverity = "warning"
	AuditSeverityError    AuditSeverity = "error"
	AuditSeverityCritical AuditSeverity = "critical"
)

// AuditEvent represents a single audit event
type AuditEvent struct {
	ID            string                 `json:"id"`
	Timestamp     time.Time              `json:"timestamp"`
	Type          AuditEventType         `json:"type"`
	EventType     string                 `json:"event_type"`     // For test compatibility
	Severity      AuditSeverity          `json:"severity"`
	UserID        string                 `json:"user_id,omitempty"`
	TenantID      string                 `json:"tenant_id,omitempty"`
	SessionID     string                 `json:"session_id,omitempty"`
	IPAddress     string                 `json:"ip_address,omitempty"`
	UserAgent     string                 `json:"user_agent,omitempty"`
	Endpoint      string                 `json:"endpoint,omitempty"`
	Method        string                 `json:"method,omitempty"`
	StatusCode    int                    `json:"status_code,omitempty"`
	Message       string                 `json:"message"`
	Details       map[string]interface{} `json:"details,omitempty"`
	RequestID     string                 `json:"request_id,omitempty"`
	Duration      time.Duration          `json:"duration,omitempty"`
	ErrorCode     string                 `json:"error_code,omitempty"`
	ErrorMessage  string                 `json:"error_message,omitempty"`
	Resource      string                 `json:"resource,omitempty"`
	Action        string                 `json:"action,omitempty"`
	Result        string                 `json:"result,omitempty"`
	Success       bool                   `json:"success"`
	RequestSize   int64                  `json:"request_size,omitempty"`
	ResponseSize  int64                  `json:"response_size,omitempty"`
	RiskLevel     string                 `json:"risk_level,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// AuditConfig configures audit logging behavior
type AuditConfig struct {
	// Enable audit logging
	Enabled bool `yaml:"enabled"`
	
	// Log file path (optional, will use structured logging if not set)
	LogFile string `yaml:"log_file,omitempty"`
	
	// Minimum severity level to log
	MinSeverity AuditSeverity `yaml:"min_severity"`
	
	// Include request/response bodies (security risk!)
	IncludeBodies bool `yaml:"include_bodies"`
	
	// Include IP addresses and user agents
	IncludeClientInfo bool `yaml:"include_client_info"`
	
	// Specific event types to audit
	EventTypes []AuditEventType `yaml:"event_types,omitempty"`
	
	// Buffer size for async logging
	BufferSize int `yaml:"buffer_size"`
	
	// Flush interval for batched logging
	FlushInterval time.Duration `yaml:"flush_interval"`
	
	// Retention period for audit logs
	RetentionPeriod time.Duration `yaml:"retention_period"`
	
	// External audit destinations
	Destinations []AuditDestination `yaml:"destinations,omitempty"`
}

// AuditDestination represents an external audit destination
type AuditDestination struct {
	Name     string            `yaml:"name"`
	Type     string            `yaml:"type"` // "syslog", "webhook", "elasticsearch", "file"
	Endpoint string            `yaml:"endpoint"`
	Headers  map[string]string `yaml:"headers,omitempty"`
	Format   string            `yaml:"format"` // "json", "cef", "leef"
}

// AuditLoggingConfig configures audit logging behavior for the audit logger
type AuditLoggingConfig struct {
	// Enable audit logging
	Enabled bool `yaml:"enabled"`
	
	// Log level for audit events
	LogLevel string `yaml:"log_level"`
	
	// Output path for audit logs (e.g., "stdout", "stderr", or file path)
	OutputPath string `yaml:"output_path,omitempty"`
	
	// Maximum file size in MB for log rotation
	MaxFileSize int `yaml:"max_file_size,omitempty"`
	
	// Maximum number of backup files to keep
	MaxBackups int `yaml:"max_backups,omitempty"`
	
	// Maximum age in days to retain old log files
	MaxAge int `yaml:"max_age,omitempty"`
	
	// Compress rotated files
	Compress bool `yaml:"compress"`
	
	// Format of the log output (e.g., "json", "text")
	Format string `yaml:"format,omitempty"`
	
	// Enable async logging
	AsyncLogging bool `yaml:"async_logging"`
	
	// Buffer size for async logging
	BufferSize int `yaml:"buffer_size,omitempty"`
	
	// Filter sensitive data
	FilterSensitive bool `yaml:"filter_sensitive"`
	
	// List of sensitive field names to filter
	SensitiveFields []string `yaml:"sensitive_fields,omitempty"`
}

// AuditLogger handles security audit logging
type AuditLogger struct {
	config      *AuditConfig
	logger      *zap.Logger
	eventChan   chan AuditEvent
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config *AuditConfig, logger *zap.SugaredLogger) *AuditLogger {
	if config == nil {
		config = DefaultAuditConfig()
	}

	bufferSize := config.BufferSize
	if bufferSize == 0 {
		bufferSize = 1000
	}

	auditor := &AuditLogger{
		config:    config,
		logger:    logger.Desugar(),
		eventChan: make(chan AuditEvent, bufferSize),
	}

	// Start async processing if enabled
	if config.Enabled {
		go auditor.processEvents()
	}

	return auditor
}

// DefaultAuditConfig returns default audit configuration
func DefaultAuditConfig() *AuditConfig {
	return &AuditConfig{
		Enabled:           true,
		MinSeverity:       AuditSeverityInfo,
		IncludeBodies:     false,
		IncludeClientInfo: true,
		EventTypes: []AuditEventType{
			AuditEventAuthentication,
			AuditEventAuthorization,
			AuditEventPIIDetection,
			AuditEventRateLimit,
			AuditEventError,
			AuditEventSecurityEvent,
		},
		BufferSize:      1000,
		FlushInterval:   5 * time.Second,
		RetentionPeriod: 90 * 24 * time.Hour, // 90 days
	}
}

// DefaultAuditLoggingConfig returns default audit logging configuration
func DefaultAuditLoggingConfig() *AuditLoggingConfig {
	return &AuditLoggingConfig{
		Enabled:     true,
		LogLevel:    "INFO",
		OutputPath:  "stdout",
		MaxFileSize: 100,
		MaxBackups:  5,
		MaxAge:      30,
		Compress:    false,
		Format:      "json",
		AsyncLogging: false,
		BufferSize:  1000,
		FilterSensitive: false,
	}
}

// LogRequest logs a request audit event
func (a *AuditLogger) LogRequest(ctx context.Context, method, endpoint string, statusCode int, duration time.Duration, metadata map[string]interface{}) {
	if !a.config.Enabled {
		return
	}
	
	event := &AuditEvent{
		Type:       AuditEventRequest,
		EventType:  "REQUEST",
		Severity:   AuditSeverityInfo,
		Method:     method,
		Endpoint:   endpoint,
		StatusCode: statusCode,
		Duration:   duration,
		Timestamp:  time.Now(),
		Metadata:   metadata,
		Success:    statusCode >= 200 && statusCode < 300,
	}
	
	// Extract context values if available
	if requestID, ok := ctx.Value("request_id").(string); ok {
		event.RequestID = requestID
	}
	if clientIP, ok := ctx.Value("client_ip").(string); ok {
		event.IPAddress = clientIP
	}
	if userAgent, ok := ctx.Value("user_agent").(string); ok {
		event.UserAgent = userAgent
	}
	if userID, ok := metadata["user_id"].(string); ok {
		event.UserID = userID
	}
	
	// Log the event
	a.logger.Info("REQUEST",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
		zap.Any("metadata", metadata),
		zap.String("request_id", event.RequestID),
		zap.String("ip_address", event.IPAddress),
		zap.String("user_agent", event.UserAgent),
	)
}

// LogSecurity logs a security audit event
func (a *AuditLogger) LogSecurity(ctx context.Context, event *AuditEvent) {
	if !a.config.Enabled || event == nil {
		return
	}
	
	// Log the event
	a.logger.Warn("SECURITY",
		zap.String("event_type", string(event.EventType)),
		zap.String("user_id", event.UserID),
		zap.String("tenant_id", event.TenantID),
		zap.String("resource", event.Resource),
		zap.String("action", event.Action),
		zap.String("ip_address", event.IPAddress),
		zap.String("user_agent", event.UserAgent),
		zap.String("request_id", event.RequestID),
		zap.Time("timestamp", event.Timestamp),
		zap.Bool("success", event.Success),
		zap.Int("status_code", event.StatusCode),
		zap.String("risk_level", event.RiskLevel),
		zap.Any("metadata", event.Metadata),
	)
}

// LogError logs an error audit event
func (a *AuditLogger) LogError(ctx context.Context, event *AuditEvent) {
	if !a.config.Enabled || event == nil {
		return
	}
	
	// Log the event
	a.logger.Error("ERROR",
		zap.String("event_type", string(event.EventType)),
		zap.String("user_id", event.UserID),
		zap.String("tenant_id", event.TenantID),
		zap.String("resource", event.Resource),
		zap.String("action", event.Action),
		zap.String("ip_address", event.IPAddress),
		zap.String("request_id", event.RequestID),
		zap.Time("timestamp", event.Timestamp),
		zap.Bool("success", event.Success),
		zap.Int("status_code", event.StatusCode),
		zap.String("error_code", event.ErrorCode),
		zap.String("error_message", event.ErrorMessage),
		zap.Any("metadata", event.Metadata),
	)
}

// LogDataAccess logs a data access audit event
func (a *AuditLogger) LogDataAccess(ctx context.Context, event *AuditEvent) {
	if !a.config.Enabled || event == nil {
		return
	}
	
	// Log the event
	a.logger.Info("DATA_ACCESS",
		zap.String("event_type", string(event.EventType)),
		zap.String("user_id", event.UserID),
		zap.String("tenant_id", event.TenantID),
		zap.String("resource", event.Resource),
		zap.String("action", event.Action),
		zap.String("ip_address", event.IPAddress),
		zap.String("request_id", event.RequestID),
		zap.Time("timestamp", event.Timestamp),
		zap.Bool("success", event.Success),
		zap.Int("status_code", event.StatusCode),
		zap.Any("metadata", event.Metadata),
	)
}

// LogPIIDetection logs a PII detection event
func (a *AuditLogger) LogPIIDetection(ctx context.Context, detections []PIIDetection, masked bool) {
	if !a.config.Enabled {
		return
	}
	
	event := &AuditEvent{
		Type:      AuditEventPIIDetection,
		EventType: "PII_DETECTION",
		Severity:  AuditSeverityWarning,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Detected %d PII instances", len(detections)),
		Metadata: map[string]interface{}{
			"detection_count": len(detections),
			"masked":          masked,
		},
	}
	
	// Extract context values if available
	if requestID, ok := ctx.Value("request_id").(string); ok {
		event.RequestID = requestID
	}
	if clientIP, ok := ctx.Value("client_ip").(string); ok {
		event.IPAddress = clientIP
	}
	
	// Log the event
	a.logger.Warn("PII_DETECTION",
		zap.Int("detection_count", len(detections)),
		zap.Bool("masked", masked),
		zap.String("request_id", event.RequestID),
		zap.String("ip_address", event.IPAddress),
	)
}

// LogRateLimit logs a rate limit event
func (a *AuditLogger) LogRateLimit(ctx context.Context, userID, endpoint string, limit, current int, window time.Duration) {
	if !a.config.Enabled {
		return
	}
	
	event := &AuditEvent{
		Type:      AuditEventRateLimit,
		EventType: "RATE_LIMIT",
		Severity:  AuditSeverityWarning,
		UserID:    userID,
		Endpoint:  endpoint,
		Timestamp: time.Now(),
		Message:   fmt.Sprintf("Rate limit hit: %d/%d in %v", current, limit, window),
		Metadata: map[string]interface{}{
			"limit":   limit,
			"current": current,
			"window":  window.String(),
		},
	}
	
	// Extract context values if available
	if requestID, ok := ctx.Value("request_id").(string); ok {
		event.RequestID = requestID
	}
	if clientIP, ok := ctx.Value("client_ip").(string); ok {
		event.IPAddress = clientIP
	}
	
	// Log the event
	a.logger.Warn("RATE_LIMIT",
		zap.String("user_id", userID),
		zap.String("endpoint", endpoint),
		zap.Int("limit", limit),
		zap.Int("current", current),
		zap.Duration("window", window),
		zap.String("request_id", event.RequestID),
		zap.String("ip_address", event.IPAddress),
	)
}

// LogSecurityEvent logs a security event
func (a *AuditLogger) LogSecurityEvent(ctx context.Context, eventType, message string, severity AuditSeverity, metadata map[string]interface{}) {
	if !a.config.Enabled {
		return
	}
	
	event := &AuditEvent{
		Type:      AuditEventSecurityEvent,
		EventType: eventType,
		Severity:  severity,
		Timestamp: time.Now(),
		Message:   message,
		Metadata:  metadata,
	}
	
	// Extract context values if available
	if requestID, ok := ctx.Value("request_id").(string); ok {
		event.RequestID = requestID
	}
	if clientIP, ok := ctx.Value("client_ip").(string); ok {
		event.IPAddress = clientIP
	}
	if userAgent, ok := ctx.Value("user_agent").(string); ok {
		event.UserAgent = userAgent
	}
	
	// Log the event based on severity
	switch severity {
	case AuditSeverityCritical, AuditSeverityError:
		a.logger.Error("SECURITY_EVENT",
			zap.String("event_type", eventType),
			zap.String("severity", string(severity)),
			zap.String("message", message),
			zap.Any("metadata", metadata),
			zap.String("request_id", event.RequestID),
			zap.String("ip_address", event.IPAddress),
		)
	case AuditSeverityWarning:
		a.logger.Warn("SECURITY_EVENT",
			zap.String("event_type", eventType),
			zap.String("severity", string(severity)),
			zap.String("message", message),
			zap.Any("metadata", metadata),
			zap.String("request_id", event.RequestID),
			zap.String("ip_address", event.IPAddress),
		)
	default:
		a.logger.Info("SECURITY_EVENT",
			zap.String("event_type", eventType),
			zap.String("severity", string(severity)),
			zap.String("message", message),
			zap.Any("metadata", metadata),
			zap.String("request_id", event.RequestID),
			zap.String("ip_address", event.IPAddress),
		)
	}
}

// GetAuditStats returns audit statistics
func (a *AuditLogger) GetAuditStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":     a.config.Enabled,
		"buffer_size": a.config.BufferSize,
		"min_severity": string(a.config.MinSeverity),
		"include_bodies": a.config.IncludeBodies,
		"include_client_info": a.config.IncludeClientInfo,
	}
}

// filterSensitiveData filters sensitive data from the event
func (a *AuditLogger) filterSensitiveData(event *AuditEvent) {
	if event.Metadata == nil {
		return
	}
	
	// For now, we don't have sensitive fields configured in AuditConfig
	// This is a placeholder for future implementation
}

// processEvents handles async event processing
func (a *AuditLogger) processEvents() {
	for event := range a.eventChan {
		// Log the event directly for now
		a.logger.Info("Async audit event",
			zap.String("event_type", string(event.EventType)),
			zap.String("user_id", event.UserID),
			zap.Any("event", event),
		)
	}
}