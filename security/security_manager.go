package security

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/openai"
)

// SecurityConfig represents comprehensive security configuration
type SecurityConfig struct {
	// Enable security features
	Enabled bool `yaml:"enabled"`
	
	// PII masking configuration
	PIIMasking *PIIMaskingConfig `yaml:"pii_masking,omitempty"`
	
	// Audit logging configuration
	AuditLogging *AuditConfig `yaml:"audit_logging,omitempty"`
	
	// Rate limiting configuration
	RateLimiting *RateLimitConfig `yaml:"rate_limiting,omitempty"`
	
	// Request validation configuration
	RequestValidation *RequestValidationConfig `yaml:"request_validation,omitempty"`
	
	// Content filtering configuration
	ContentFiltering *ContentFilteringConfig `yaml:"content_filtering,omitempty"`
	
	// Encryption configuration
	Encryption *EncryptionConfig `yaml:"encryption,omitempty"`
	
	// Session management configuration
	SessionManagement *SessionConfig `yaml:"session_management,omitempty"`
}

// RequestValidationConfig configures request validation
type RequestValidationConfig struct {
	// Enable request validation
	Enabled bool `yaml:"enabled"`
	
	// Maximum request size in bytes
	MaxRequestSize int64 `yaml:"max_request_size"`
	
	// Maximum message length
	MaxMessageLength int `yaml:"max_message_length"`
	
	// Maximum number of messages per request
	MaxMessages int `yaml:"max_messages"`
	
	// Allowed content types
	AllowedContentTypes []string `yaml:"allowed_content_types"`
	
	// Required headers
	RequiredHeaders []string `yaml:"required_headers,omitempty"`
	
	// Forbidden patterns in requests
	ForbiddenPatterns []string `yaml:"forbidden_patterns,omitempty"`
	
	// Validate JSON schema
	ValidateSchema bool `yaml:"validate_schema"`
}

// ContentFilteringConfig configures content filtering
type ContentFilteringConfig struct {
	// Enable content filtering
	Enabled bool `yaml:"enabled"`
	
	// Block inappropriate content
	BlockInappropriate bool `yaml:"block_inappropriate"`
	
	// Block potentially harmful content
	BlockHarmful bool `yaml:"block_harmful"`
	
	// Block spam/promotional content
	BlockSpam bool `yaml:"block_spam"`
	
	// Custom filtering rules
	CustomRules []ContentFilterRule `yaml:"custom_rules,omitempty"`
	
	// Content moderation API configuration
	ModerationAPI *ModerationAPIConfig `yaml:"moderation_api,omitempty"`
}

// ContentFilterRule represents a custom content filtering rule
type ContentFilterRule struct {
	Name        string  `yaml:"name"`
	Pattern     string  `yaml:"pattern"`
	Action      string  `yaml:"action"` // "block", "warn", "flag"
	Severity    string  `yaml:"severity"`
	Description string  `yaml:"description"`
	Confidence  float64 `yaml:"confidence"`
}

// ModerationAPIConfig configures external content moderation
type ModerationAPIConfig struct {
	// Enable external moderation
	Enabled bool `yaml:"enabled"`
	
	// API endpoint
	Endpoint string `yaml:"endpoint"`
	
	// API key
	APIKey string `yaml:"api_key"`
	
	// Confidence threshold
	ConfidenceThreshold float64 `yaml:"confidence_threshold"`
	
	// Categories to check
	Categories []string `yaml:"categories"`
}

// EncryptionConfig configures encryption settings
type EncryptionConfig struct {
	// Enable encryption for sensitive data
	Enabled bool `yaml:"enabled"`
	
	// Encryption algorithm
	Algorithm string `yaml:"algorithm"` // "AES-256-GCM", "ChaCha20-Poly1305"
	
	// Key rotation interval
	KeyRotationInterval time.Duration `yaml:"key_rotation_interval"`
	
	// Encrypt request/response bodies
	EncryptBodies bool `yaml:"encrypt_bodies"`
	
	// Encrypt logs
	EncryptLogs bool `yaml:"encrypt_logs"`
	
	// Key management service configuration
	KMS *KMSConfig `yaml:"kms,omitempty"`
}

// KMSConfig configures key management service
type KMSConfig struct {
	// KMS provider
	Provider string `yaml:"provider"` // "aws", "gcp", "azure", "vault"
	
	// KMS endpoint
	Endpoint string `yaml:"endpoint"`
	
	// Key ID
	KeyID string `yaml:"key_id"`
	
	// Region
	Region string `yaml:"region,omitempty"`
}

// SessionConfig configures session management
type SessionConfig struct {
	// Enable session management
	Enabled bool `yaml:"enabled"`
	
	// Session timeout
	Timeout time.Duration `yaml:"timeout"`
	
	// Session inactivity timeout
	InactivityTimeout time.Duration `yaml:"inactivity_timeout"`
	
	// Maximum concurrent sessions per user
	MaxConcurrentSessions int `yaml:"max_concurrent_sessions"`
	
	// Session storage type
	StorageType string `yaml:"storage_type"` // "memory", "redis", "database"
	
	// Session token configuration
	TokenConfig *SessionTokenConfig `yaml:"token_config,omitempty"`
}

// SessionTokenConfig configures session tokens
type SessionTokenConfig struct {
	// Token length
	Length int `yaml:"length"`
	
	// Include IP address in token validation
	ValidateIP bool `yaml:"validate_ip"`
	
	// Include user agent in token validation
	ValidateUserAgent bool `yaml:"validate_user_agent"`
	
	// Token refresh interval
	RefreshInterval time.Duration `yaml:"refresh_interval"`
}

// SecurityManager provides comprehensive security management
type SecurityManager struct {
	config         *SecurityConfig
	piiMasker      *PIIMasker
	auditLogger    *AuditLogger
	rateLimiter    *AdvancedRateLimiter
	logger         *zap.SugaredLogger
}

// NewSecurityManager creates a new security manager
func NewSecurityManager(config *SecurityConfig, logger *zap.SugaredLogger) (*SecurityManager, error) {
	if config == nil {
		config = DefaultSecurityConfig()
	}

	manager := &SecurityManager{
		config: config,
		logger: logger,
	}

	// Initialize PII masking
	if config.PIIMasking != nil && config.PIIMasking.Enabled {
		piiMasker, err := NewPIIMasker(config.PIIMasking, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize PII masker: %v", err)
		}
		manager.piiMasker = piiMasker
	}

	// Initialize audit logging
	if config.AuditLogging != nil && config.AuditLogging.Enabled {
		manager.auditLogger = NewAuditLogger(config.AuditLogging, logger)
	}

	// Initialize rate limiting
	if config.RateLimiting != nil && config.RateLimiting.Enabled {
		manager.rateLimiter = NewAdvancedRateLimiter(config.RateLimiting, logger)
	}

	return manager, nil
}

// DefaultSecurityConfig returns default security configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		Enabled:    true,
		PIIMasking: DefaultPIIMaskingConfig(),
		AuditLogging: DefaultAuditConfig(),
		RateLimiting: DefaultRateLimitConfig(),
		RequestValidation: &RequestValidationConfig{
			Enabled:             true,
			MaxRequestSize:      10 * 1024 * 1024, // 10MB
			MaxMessageLength:    100000,           // 100k characters
			MaxMessages:         50,
			AllowedContentTypes: []string{"application/json"},
			ValidateSchema:      true,
		},
		ContentFiltering: &ContentFilteringConfig{
			Enabled:            true,
			BlockInappropriate: true,
			BlockHarmful:       true,
			BlockSpam:          false,
		},
		Encryption: &EncryptionConfig{
			Enabled:             false, // Disabled by default for performance
			Algorithm:           "AES-256-GCM",
			KeyRotationInterval: 24 * time.Hour,
			EncryptBodies:       false,
			EncryptLogs:         false,
		},
		SessionManagement: &SessionConfig{
			Enabled:               true,
			Timeout:               24 * time.Hour,
			InactivityTimeout:     2 * time.Hour,
			MaxConcurrentSessions: 5,
			StorageType:           "memory",
			TokenConfig: &SessionTokenConfig{
				Length:            32,
				ValidateIP:        false,
				ValidateUserAgent: false,
				RefreshInterval:   time.Hour,
			},
		},
	}
}

// ValidateAndSecureRequest validates and secures an incoming request
func (s *SecurityManager) ValidateAndSecureRequest(ctx context.Context, request *openai.ChatCompletionRequest, userID, endpoint, model string) (*openai.ChatCompletionRequest, *RequestSecurityResult, error) {
	if !s.config.Enabled {
		return request, &RequestSecurityResult{Allowed: true}, nil
	}

	result := &RequestSecurityResult{
		Allowed:     true,
		Validations: make(map[string]bool),
		Detections:  make(map[string]interface{}),
	}

	// Request validation
	if s.config.RequestValidation != nil && s.config.RequestValidation.Enabled {
		if err := s.validateRequest(request); err != nil {
			result.Allowed = false
			result.ValidationErrors = append(result.ValidationErrors, err.Error())
			s.auditSecurityEvent(ctx, "request_validation_failed", err.Error(), AuditSeverityWarning)
			return request, result, nil
		}
		result.Validations["request_validation"] = true
	}

	// Content filtering
	if s.config.ContentFiltering != nil && s.config.ContentFiltering.Enabled {
		contentResult, err := s.filterContent(ctx, request)
		if err != nil {
			result.ValidationErrors = append(result.ValidationErrors, err.Error())
		}
		if !contentResult.Allowed {
			result.Allowed = false
			result.Detections["content_filtering"] = contentResult
			s.auditSecurityEvent(ctx, "content_blocked", contentResult.Reason, AuditSeverityWarning)
			return request, result, nil
		}
		result.Validations["content_filtering"] = true
	}

	// Rate limiting
	if s.rateLimiter != nil {
		tokenCount := s.estimateTokens(request)
		cost := s.estimateCost(request, model)
		
		rateLimitResult, err := s.rateLimiter.CheckRateLimit(ctx, userID, endpoint, model, tokenCount, cost)
		if err != nil {
			result.ValidationErrors = append(result.ValidationErrors, err.Error())
		}
		if !rateLimitResult.Allowed {
			result.Allowed = false
			result.RateLimitResult = rateLimitResult
			s.auditRateLimit(ctx, userID, endpoint, rateLimitResult)
			return request, result, nil
		}
		result.Validations["rate_limiting"] = true
	}

	// PII masking
	var maskedRequest *openai.ChatCompletionRequest = request
	if s.piiMasker != nil {
		var piiDetections []PIIDetection
		var err error
		maskedRequest, piiDetections, err = s.piiMasker.MaskChatRequest(request)
		if err != nil {
			result.ValidationErrors = append(result.ValidationErrors, err.Error())
		}
		if len(piiDetections) > 0 {
			result.Detections["pii_detections"] = piiDetections
			s.auditPIIDetection(ctx, piiDetections)
		}
		result.Validations["pii_masking"] = true
	}

	// Audit the request
	s.auditRequest(ctx, endpoint, userID, model)

	return maskedRequest, result, nil
}

// RequestSecurityResult represents the result of security validation
type RequestSecurityResult struct {
	Allowed           bool                  `json:"allowed"`
	Validations       map[string]bool       `json:"validations"`
	ValidationErrors  []string              `json:"validation_errors,omitempty"`
	Detections        map[string]interface{} `json:"detections,omitempty"`
	RateLimitResult   *RateLimitResult      `json:"rate_limit_result,omitempty"`
}

// ContentFilterResult represents content filtering result
type ContentFilterResult struct {
	Allowed    bool   `json:"allowed"`
	Reason     string `json:"reason,omitempty"`
	Category   string `json:"category,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// validateRequest performs request validation
func (s *SecurityManager) validateRequest(request *openai.ChatCompletionRequest) error {
	config := s.config.RequestValidation

	// Validate message count
	if len(request.Messages) > config.MaxMessages {
		return fmt.Errorf("too many messages: %d > %d", len(request.Messages), config.MaxMessages)
	}

	// Validate message length
	for i, message := range request.Messages {
		if message.Content != nil {
			totalLength := 0
			if message.Content.String != nil {
				totalLength += len(*message.Content.String)
			}
			if message.Content.Parts != nil {
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						totalLength += len(part.Content.TextContent.Text)
					}
				}
			}
			
			if totalLength > config.MaxMessageLength {
				return fmt.Errorf("message %d too long: %d > %d characters", i, totalLength, config.MaxMessageLength)
			}
		}
	}

	// Check forbidden patterns
	for _, pattern := range config.ForbiddenPatterns {
		for _, message := range request.Messages {
			if message.Content != nil && message.Content.String != nil {
				if strings.Contains(strings.ToLower(*message.Content.String), strings.ToLower(pattern)) {
					return fmt.Errorf("forbidden pattern detected: %s", pattern)
				}
			}
		}
	}

	return nil
}

// filterContent performs content filtering
func (s *SecurityManager) filterContent(ctx context.Context, request *openai.ChatCompletionRequest) (*ContentFilterResult, error) {
	config := s.config.ContentFiltering

	// Extract text content
	var textContent []string
	for _, message := range request.Messages {
		if message.Content != nil {
			if message.Content.String != nil {
				textContent = append(textContent, *message.Content.String)
			}
			if message.Content.Parts != nil {
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						textContent = append(textContent, part.Content.TextContent.Text)
					}
				}
			}
		}
	}

	fullText := strings.Join(textContent, " ")

	// Apply custom rules
	for _, rule := range config.CustomRules {
		if strings.Contains(strings.ToLower(fullText), strings.ToLower(rule.Pattern)) {
			if rule.Action == "block" {
				return &ContentFilterResult{
					Allowed:    false,
					Reason:     rule.Description,
					Category:   rule.Name,
					Confidence: rule.Confidence,
				}, nil
			}
		}
	}

	// Check for inappropriate content (simplified implementation)
	inappropriateTerms := []string{"explicit", "harmful", "illegal"}
	if config.BlockInappropriate {
		for _, term := range inappropriateTerms {
			if strings.Contains(strings.ToLower(fullText), term) {
				return &ContentFilterResult{
					Allowed:    false,
					Reason:     "Inappropriate content detected",
					Category:   "inappropriate",
					Confidence: 0.8,
				}, nil
			}
		}
	}

	return &ContentFilterResult{Allowed: true}, nil
}

// estimateTokens estimates token count for a request
func (s *SecurityManager) estimateTokens(request *openai.ChatCompletionRequest) int64 {
	totalChars := 0
	for _, message := range request.Messages {
		if message.Content != nil {
			if message.Content.String != nil {
				totalChars += len(*message.Content.String)
			}
			if message.Content.Parts != nil {
				for _, part := range message.Content.Parts {
					if part.Content.TextContent != nil {
						totalChars += len(part.Content.TextContent.Text)
					}
				}
			}
		}
	}
	return int64(totalChars / 4) // Rough estimation: 4 chars per token
}

// estimateCost estimates cost for a request
func (s *SecurityManager) estimateCost(request *openai.ChatCompletionRequest, model string) float64 {
	tokens := s.estimateTokens(request)
	// Simplified cost estimation - $0.001 per 1000 tokens
	return float64(tokens) * 0.001 / 1000.0
}

// Audit helper methods

func (s *SecurityManager) auditRequest(ctx context.Context, endpoint, userID, model string) {
	if s.auditLogger != nil {
		s.auditLogger.LogRequest(ctx, "POST", endpoint, 200, 0, map[string]interface{}{
			"user_id": userID,
			"model":   model,
		})
	}
}

func (s *SecurityManager) auditPIIDetection(ctx context.Context, detections []PIIDetection) {
	if s.auditLogger != nil {
		s.auditLogger.LogPIIDetection(ctx, detections, true)
	}
}

func (s *SecurityManager) auditRateLimit(ctx context.Context, userID, endpoint string, result *RateLimitResult) {
	if s.auditLogger != nil {
		s.auditLogger.LogRateLimit(ctx, userID, endpoint, int(result.AppliedLimit.Limit), int(result.Current), result.AppliedLimit.Window)
	}
}

func (s *SecurityManager) auditSecurityEvent(ctx context.Context, eventType, message string, severity AuditSeverity) {
	if s.auditLogger != nil {
		s.auditLogger.LogSecurityEvent(ctx, eventType, message, severity, nil)
	}
}

// CreateSecurityMiddleware creates HTTP middleware for security
func (s *SecurityManager) CreateSecurityMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add request ID for tracking
			requestID := fmt.Sprintf("req_%d", time.Now().UnixNano())
			ctx := context.WithValue(r.Context(), "request_id", requestID)
			
			// Add client IP
			clientIP := s.extractClientIP(r)
			ctx = context.WithValue(ctx, "client_ip", clientIP)
			
			// Add user agent
			ctx = context.WithValue(ctx, "user_agent", r.UserAgent())
			
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// extractClientIP extracts client IP from request
func (s *SecurityManager) extractClientIP(r *http.Request) string {
	// Check for forwarded IP
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}
	
	// Check for real IP
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}
	
	// Fall back to remote address
	return strings.Split(r.RemoteAddr, ":")[0]
}

// ReleaseResources releases security resources (e.g., concurrent limits)
func (s *SecurityManager) ReleaseResources(userID, endpoint, model string) {
	if s.rateLimiter != nil {
		s.rateLimiter.ReleaseResource(userID, endpoint, model)
	}
}

// GetSecurityStats returns security statistics
func (s *SecurityManager) GetSecurityStats() map[string]interface{} {
	stats := map[string]interface{}{
		"enabled": s.config.Enabled,
	}

	if s.piiMasker != nil {
		stats["pii_masking"] = s.piiMasker.GetPIIStats()
	}

	if s.auditLogger != nil {
		stats["audit_logging"] = s.auditLogger.GetAuditStats()
	}

	if s.rateLimiter != nil {
		stats["rate_limiting"] = s.rateLimiter.GetRateLimitStats()
	}

	return stats
}