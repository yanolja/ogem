package security

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/openai"
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
	Severity      AuditSeverity          `json:"severity"`
	UserID        string                 `json:"user_id,omitempty"`
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
	Resource      string                 `json:"resource,omitempty"`
	Action        string                 `json:"action,omitempty"`
	Result        string                 `json:"result,omitempty"`
	Metadata      map[string]string      `json:"metadata,omitempty"`
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

// AuditLogger handles security audit logging
type AuditLogger struct {
	config      *AuditConfig
	logger      *zap.SugaredLogger
	eventChan   chan AuditEvent
	destinations []AuditDestination
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config *AuditConfig, logger *zap.SugaredLogger) *AuditLogger {
	if config == nil {
		config = DefaultAuditConfig()
	}

	auditor := &AuditLogger{
		config:       config,
		logger:       logger,
		eventChan:    make(chan AuditEvent, config.BufferSize),
		destinations: config.Destinations,
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

// LogEvent logs an audit event
func (a *AuditLogger) LogEvent(event AuditEvent) {
	if !a.config.Enabled {
		return
	}

	// Check severity filter
	if !a.shouldLogSeverity(event.Severity) {
		return
	}

	// Check event type filter
	if !a.shouldLogEventType(event.Type) {
		return
	}

	// Set timestamp if not provided
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}

	// Generate ID if not provided
	if event.ID == "" {
		event.ID = generateEventID()
	}

	// Send to async processor
	select {
	case a.eventChan <- event:
		// Event queued successfully
	default:
		// Buffer full, log synchronously to avoid losing critical events
		a.processEvent(event)
	}
}

// LogRequest logs a request audit event
func (a *AuditLogger) LogRequest(ctx context.Context, method, endpoint string, statusCode int, duration time.Duration, details map[string]interface{}) {
	event := AuditEvent{
		Type:       AuditEventRequest,
		Severity:   a.getRequestSeverity(statusCode),
		Method:     method,
		Endpoint:   endpoint,
		StatusCode: statusCode,
		Duration:   duration,
		Message:    fmt.Sprintf("%s %s - %d", method, endpoint, statusCode),
		Details:    details,
	}

	// Extract context information
	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogAuthentication logs an authentication audit event
func (a *AuditLogger) LogAuthentication(ctx context.Context, userID, method string, success bool, reason string) {
	severity := AuditSeverityInfo
	result := "success"
	if !success {
		severity = AuditSeverityWarning
		result = "failure"
	}

	event := AuditEvent{
		Type:     AuditEventAuthentication,
		Severity: severity,
		UserID:   userID,
		Method:   method,
		Result:   result,
		Message:  fmt.Sprintf("Authentication %s for user %s via %s", result, userID, method),
		Details: map[string]interface{}{
			"reason": reason,
		},
	}

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogAuthorization logs an authorization audit event
func (a *AuditLogger) LogAuthorization(ctx context.Context, userID, resource, action string, allowed bool, reason string) {
	severity := AuditSeverityInfo
	result := "allowed"
	if !allowed {
		severity = AuditSeverityWarning
		result = "denied"
	}

	event := AuditEvent{
		Type:     AuditEventAuthorization,
		Severity: severity,
		UserID:   userID,
		Resource: resource,
		Action:   action,
		Result:   result,
		Message:  fmt.Sprintf("Authorization %s for user %s on %s:%s", result, userID, resource, action),
		Details: map[string]interface{}{
			"reason": reason,
		},
	}

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogPIIDetection logs a PII detection audit event
func (a *AuditLogger) LogPIIDetection(ctx context.Context, detections []PIIDetection, masked bool) {
	types := make([]string, len(detections))
	for i, d := range detections {
		types[i] = string(d.Type)
	}

	event := AuditEvent{
		Type:     AuditEventPIIDetection,
		Severity: AuditSeverityWarning,
		Message:  fmt.Sprintf("PII detected: %d instances of types %v", len(detections), types),
		Details: map[string]interface{}{
			"detection_count": len(detections),
			"pii_types":      types,
			"masked":         masked,
			"detections":     detections,
		},
	}

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogRateLimit logs a rate limit audit event
func (a *AuditLogger) LogRateLimit(ctx context.Context, userID, endpoint string, limit, current int, windowDuration time.Duration) {
	event := AuditEvent{
		Type:     AuditEventRateLimit,
		Severity: AuditSeverityWarning,
		UserID:   userID,
		Endpoint: endpoint,
		Message:  fmt.Sprintf("Rate limit exceeded for user %s on %s: %d/%d", userID, endpoint, current, limit),
		Details: map[string]interface{}{
			"limit":           limit,
			"current":         current,
			"window_duration": windowDuration,
		},
	}

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogError logs an error audit event
func (a *AuditLogger) LogError(ctx context.Context, errorCode, message string, details map[string]interface{}) {
	event := AuditEvent{
		Type:      AuditEventError,
		Severity:  AuditSeverityError,
		ErrorCode: errorCode,
		Message:   message,
		Details:   details,
	}

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogSecurityEvent logs a security-related audit event
func (a *AuditLogger) LogSecurityEvent(ctx context.Context, eventType, message string, severity AuditSeverity, details map[string]interface{}) {
	event := AuditEvent{
		Type:     AuditEventSecurityEvent,
		Severity: severity,
		Message:  message,
		Details:  details,
	}

	if details == nil {
		event.Details = make(map[string]interface{})
	}
	event.Details["security_event_type"] = eventType

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// LogConfigurationChange logs a configuration change audit event
func (a *AuditLogger) LogConfigurationChange(ctx context.Context, userID, component, action string, oldValue, newValue interface{}) {
	event := AuditEvent{
		Type:     AuditEventConfiguration,
		Severity: AuditSeverityInfo,
		UserID:   userID,
		Resource: component,
		Action:   action,
		Message:  fmt.Sprintf("Configuration changed: %s %s", component, action),
		Details: map[string]interface{}{
			"old_value": oldValue,
			"new_value": newValue,
		},
	}

	a.enrichEventFromContext(ctx, &event)
	a.LogEvent(event)
}

// enrichEventFromContext extracts information from request context
func (a *AuditLogger) enrichEventFromContext(ctx context.Context, event *AuditEvent) {
	if !a.config.IncludeClientInfo {
		return
	}

	// Extract request ID
	if requestID := ctx.Value("request_id"); requestID != nil {
		if id, ok := requestID.(string); ok {
			event.RequestID = id
		}
	}

	// Extract IP address
	if ip := ctx.Value("client_ip"); ip != nil {
		if ipStr, ok := ip.(string); ok {
			event.IPAddress = ipStr
		}
	}

	// Extract user agent
	if ua := ctx.Value("user_agent"); ua != nil {
		if uaStr, ok := ua.(string); ok {
			event.UserAgent = uaStr
		}
	}

	// Extract session ID
	if sessionID := ctx.Value("session_id"); sessionID != nil {
		if id, ok := sessionID.(string); ok {
			event.SessionID = id
		}
	}
}

// processEvents handles async event processing
func (a *AuditLogger) processEvents() {
	ticker := time.NewTicker(a.config.FlushInterval)
	defer ticker.Stop()

	var batch []AuditEvent

	for {
		select {
		case event := <-a.eventChan:
			batch = append(batch, event)
			
			// Flush immediately for critical events
			if event.Severity == AuditSeverityCritical {
				a.flushBatch(batch)
				batch = nil
			}

		case <-ticker.C:
			if len(batch) > 0 {
				a.flushBatch(batch)
				batch = nil
			}
		}
	}
}

// flushBatch processes a batch of audit events
func (a *AuditLogger) flushBatch(events []AuditEvent) {
	for _, event := range events {
		a.processEvent(event)
	}
}

// processEvent processes a single audit event
func (a *AuditLogger) processEvent(event AuditEvent) {
	// Log to structured logger
	a.logToStructuredLogger(event)

	// Send to external destinations
	for _, dest := range a.destinations {
		if err := a.sendToDestination(event, dest); err != nil {
			a.logger.Warnw("Failed to send audit event to destination", 
				"destination", dest.Name, "error", err)
		}
	}
}

// logToStructuredLogger logs event to the structured logger
func (a *AuditLogger) logToStructuredLogger(event AuditEvent) {
	fields := []interface{}{
		"audit_event_id", event.ID,
		"audit_type", event.Type,
		"audit_severity", event.Severity,
		"audit_timestamp", event.Timestamp,
	}

	if event.UserID != "" {
		fields = append(fields, "user_id", event.UserID)
	}
	if event.RequestID != "" {
		fields = append(fields, "request_id", event.RequestID)
	}
	if event.IPAddress != "" {
		fields = append(fields, "ip_address", event.IPAddress)
	}
	if event.Endpoint != "" {
		fields = append(fields, "endpoint", event.Endpoint)
	}
	if event.StatusCode != 0 {
		fields = append(fields, "status_code", event.StatusCode)
	}
	if event.Duration != 0 {
		fields = append(fields, "duration_ms", event.Duration.Milliseconds())
	}

	// Add details as additional fields
	for key, value := range event.Details {
		fields = append(fields, fmt.Sprintf("audit_%s", key), value)
	}

	switch event.Severity {
	case AuditSeverityCritical:
		a.logger.Errorw(event.Message, fields...)
	case AuditSeverityError:
		a.logger.Errorw(event.Message, fields...)
	case AuditSeverityWarning:
		a.logger.Warnw(event.Message, fields...)
	default:
		a.logger.Infow(event.Message, fields...)
	}
}

// sendToDestination sends event to external destination
func (a *AuditLogger) sendToDestination(event AuditEvent, dest AuditDestination) error {
	// This is a simplified implementation - in production you'd implement
	// proper webhook, syslog, Elasticsearch, etc. clients
	
	switch dest.Type {
	case "webhook":
		return a.sendWebhook(event, dest)
	case "file":
		return a.writeToFile(event, dest)
	default:
		return fmt.Errorf("unsupported destination type: %s", dest.Type)
	}
}

// sendWebhook sends event to webhook endpoint
func (a *AuditLogger) sendWebhook(event AuditEvent, dest AuditDestination) error {
	// Simplified webhook implementation
	a.logger.Debugw("Would send audit event to webhook", 
		"destination", dest.Name, "endpoint", dest.Endpoint, "event_id", event.ID)
	return nil
}

// writeToFile writes event to file
func (a *AuditLogger) writeToFile(event AuditEvent, dest AuditDestination) error {
	// Simplified file writing implementation
	a.logger.Debugw("Would write audit event to file", 
		"destination", dest.Name, "file", dest.Endpoint, "event_id", event.ID)
	return nil
}

// Helper functions

func (a *AuditLogger) shouldLogSeverity(severity AuditSeverity) bool {
	severityLevels := map[AuditSeverity]int{
		AuditSeverityInfo:     0,
		AuditSeverityWarning:  1,
		AuditSeverityError:    2,
		AuditSeverityCritical: 3,
	}

	minLevel := severityLevels[a.config.MinSeverity]
	eventLevel := severityLevels[severity]

	return eventLevel >= minLevel
}

func (a *AuditLogger) shouldLogEventType(eventType AuditEventType) bool {
	if len(a.config.EventTypes) == 0 {
		return true // Log all types if none specified
	}

	for _, configType := range a.config.EventTypes {
		if configType == eventType {
			return true
		}
	}

	return false
}

func (a *AuditLogger) getRequestSeverity(statusCode int) AuditSeverity {
	switch {
	case statusCode >= 500:
		return AuditSeverityError
	case statusCode >= 400:
		return AuditSeverityWarning
	default:
		return AuditSeverityInfo
	}
}

func generateEventID() string {
	return fmt.Sprintf("audit_%d", time.Now().UnixNano())
}

// GetAuditStats returns audit logging statistics
func (a *AuditLogger) GetAuditStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":            a.config.Enabled,
		"min_severity":       a.config.MinSeverity,
		"event_types":        a.config.EventTypes,
		"buffer_size":        a.config.BufferSize,
		"destinations_count": len(a.destinations),
		"queue_length":       len(a.eventChan),
	}
}