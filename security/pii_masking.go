package security

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/yanolja/ogem/openai"
)

// PIIMaskingConfig configures PII detection and masking behavior
type PIIMaskingConfig struct {
	// Enable PII masking
	Enabled bool `yaml:"enabled"`
	
	// Masking strategy
	MaskingStrategy PIIMaskingStrategy `yaml:"masking_strategy"`
	
	// Custom PII patterns to detect
	CustomPatterns []PIIPattern `yaml:"custom_patterns,omitempty"`
	
	// Enable built-in PII detection patterns
	EnableBuiltinPatterns bool `yaml:"enable_builtin_patterns"`
	
	// Preserve format when masking (e.g., XXX-XX-1234 for SSN)
	PreserveFormat bool `yaml:"preserve_format"`
	
	// Store mapping for reversible masking 
	// WARNING: This creates security risks by retaining original PII in memory.
	// Only enable in development or when absolutely required by compliance needs.
	EnableReversibleMasking bool `yaml:"enable_reversible_masking"`
	
	// Audit PII detection events
	AuditPIIDetection bool `yaml:"audit_pii_detection"`
}

// PIIMaskingStrategy defines how PII should be masked
type PIIMaskingStrategy string

const (
	// MaskingStrategyRedact completely removes PII
	MaskingStrategyRedact PIIMaskingStrategy = "redact"
	
	// MaskingStrategyReplace replaces with placeholder
	MaskingStrategyReplace PIIMaskingStrategy = "replace"
	
	// MaskingStrategyHash replaces with hash
	MaskingStrategyHash PIIMaskingStrategy = "hash"
	
	// MaskingStrategyPartial shows only part of the data
	MaskingStrategyPartial PIIMaskingStrategy = "partial"
	
	// MaskingStrategyTokenize replaces with consistent tokens
	MaskingStrategyTokenize PIIMaskingStrategy = "tokenize"
)

// PIIType represents different types of personally identifiable information
type PIIType string

const (
	PIITypeEmail           PIIType = "email"
	PIITypePhone           PIIType = "phone"
	PIITypeSSN             PIIType = "ssn"
	PIITypeCreditCard      PIIType = "credit_card"
	PIITypeIPAddress       PIIType = "ip_address"
	PIITypeName            PIIType = "name"
	PIITypeAddress         PIIType = "address"
	PIITypeDateOfBirth     PIIType = "date_of_birth"
	PIITypePassport        PIIType = "passport"
	PIITypeDriversLicense  PIIType = "drivers_license"
	PIITypeBankAccount     PIIType = "bank_account"
	PIITypeCustom          PIIType = "custom"
)

// PIIPattern defines a pattern for detecting specific PII types
type PIIPattern struct {
	Type        PIIType `yaml:"type"`
	Name        string  `yaml:"name"`
	Pattern     string  `yaml:"pattern"`
	Replacement string  `yaml:"replacement"`
	Confidence  float64 `yaml:"confidence"` // 0.0 to 1.0
}

// PIIDetection represents a detected PII instance
type PIIDetection struct {
	Type       PIIType `json:"type"`
	Value      string  `json:"value"`
	Position   int     `json:"position"`
	Length     int     `json:"length"`
	Confidence float64 `json:"confidence"`
	Pattern    string  `json:"pattern"`
}

// PIIMasker handles PII detection and masking
type PIIMasker struct {
	config    *PIIMaskingConfig
	patterns  []compiledPattern
	tokenMap  map[string]string // For reversible masking
	logger    *zap.SugaredLogger
}

type compiledPattern struct {
	PIIPattern
	regex *regexp.Regexp
}

// NewPIIMasker creates a new PII masking instance
func NewPIIMasker(config *PIIMaskingConfig, logger *zap.SugaredLogger) (*PIIMasker, error) {
	if config == nil {
		config = DefaultPIIMaskingConfig()
	}

	masker := &PIIMasker{
		config:   config,
		patterns: make([]compiledPattern, 0),
		tokenMap: make(map[string]string),
		logger:   logger,
	}

	// Compile built-in patterns
	if config.EnableBuiltinPatterns {
		builtinPatterns := getBuiltinPIIPatterns()
		for _, pattern := range builtinPatterns {
			if err := masker.addPattern(pattern); err != nil {
				logger.Warnw("Failed to compile built-in PII pattern", "pattern", pattern.Name, "error", err)
			}
		}
	}

	// Compile custom patterns
	for _, pattern := range config.CustomPatterns {
		if err := masker.addPattern(pattern); err != nil {
			logger.Warnw("Failed to compile custom PII pattern", "pattern", pattern.Name, "error", err)
		}
	}

	return masker, nil
}

// DefaultPIIMaskingConfig returns default PII masking configuration
func DefaultPIIMaskingConfig() *PIIMaskingConfig {
	return &PIIMaskingConfig{
		Enabled:                 true,
		MaskingStrategy:         MaskingStrategyReplace,
		EnableBuiltinPatterns:   true,
		PreserveFormat:          true,
		EnableReversibleMasking: false,
		AuditPIIDetection:      true,
	}
}

// addPattern compiles and adds a PII detection pattern
func (m *PIIMasker) addPattern(pattern PIIPattern) error {
	regex, err := regexp.Compile(pattern.Pattern)
	if err != nil {
		return err
	}

	m.patterns = append(m.patterns, compiledPattern{
		PIIPattern: pattern,
		regex:      regex,
	})

	return nil
}

// MaskChatRequest masks PII in a chat completion request
func (m *PIIMasker) MaskChatRequest(request *openai.ChatCompletionRequest) (*openai.ChatCompletionRequest, []PIIDetection, error) {
	if !m.config.Enabled {
		return request, nil, nil
	}

	// Create a copy of the request to avoid modifying the original
	maskedRequest := *request
	maskedRequest.Messages = make([]openai.Message, len(request.Messages))
	copy(maskedRequest.Messages, request.Messages)

	var allDetections []PIIDetection

	// Mask PII in messages
	for i, message := range maskedRequest.Messages {
		if message.Content != nil {
			maskedContent, detections, err := m.maskMessageContent(message.Content)
			if err != nil {
				return nil, nil, err
			}
			maskedRequest.Messages[i].Content = maskedContent
			allDetections = append(allDetections, detections...)
		}
	}

	// Audit PII detection if enabled
	if m.config.AuditPIIDetection && len(allDetections) > 0 {
		m.logger.Infow("PII detected in chat request", 
			"detections_count", len(allDetections),
			"types", m.getDetectedTypes(allDetections))
	}

	return &maskedRequest, allDetections, nil
}

// maskMessageContent masks PII in message content
func (m *PIIMasker) maskMessageContent(content *openai.MessageContent) (*openai.MessageContent, []PIIDetection, error) {
	var detections []PIIDetection
	maskedContent := &openai.MessageContent{}

	if content.String != nil {
		maskedText, textDetections, err := m.maskText(*content.String)
		if err != nil {
			return nil, nil, err
		}
		maskedContent.String = &maskedText
		detections = append(detections, textDetections...)
	}

	if content.Parts != nil {
		maskedParts := make([]openai.Part, len(content.Parts))
		for i, part := range content.Parts {
			maskedPart := part
			if part.Content.TextContent != nil {
				maskedText, textDetections, err := m.maskText(part.Content.TextContent.Text)
				if err != nil {
					return nil, nil, err
				}
				maskedPart.Content.TextContent = &openai.TextContent{Text: maskedText}
				detections = append(detections, textDetections...)
			}
			maskedParts[i] = maskedPart
		}
		maskedContent.Parts = maskedParts
	}

	return maskedContent, detections, nil
}

// maskText performs PII detection and masking on text
func (m *PIIMasker) maskText(text string) (string, []PIIDetection, error) {
	var detections []PIIDetection
	maskedText := text

	// Apply each pattern
	for _, pattern := range m.patterns {
		matches := pattern.regex.FindAllStringSubmatch(text, -1)
		indices := pattern.regex.FindAllStringIndex(text, -1)

		for i, match := range matches {
			if len(match) > 0 {
				detection := PIIDetection{
					Type:       pattern.Type,
					Value:      match[0],
					Position:   indices[i][0],
					Length:     len(match[0]),
					Confidence: pattern.Confidence,
					Pattern:    pattern.Name,
				}
				detections = append(detections, detection)

				// Apply masking
				replacement := m.generateReplacement(detection, pattern)
				maskedText = strings.Replace(maskedText, match[0], replacement, 1)
			}
		}
	}

	return maskedText, detections, nil
}

// generateReplacement generates appropriate replacement text based on strategy
func (m *PIIMasker) generateReplacement(detection PIIDetection, pattern compiledPattern) string {
	switch m.config.MaskingStrategy {
	case MaskingStrategyRedact:
		return "[REDACTED]"
	
	case MaskingStrategyReplace:
		if pattern.Replacement != "" {
			return pattern.Replacement
		}
		return "[" + strings.ToUpper(string(detection.Type)) + "]"
	
	case MaskingStrategyHash:
		hash := sha256.Sum256([]byte(detection.Value))
		hashStr := hex.EncodeToString(hash[:])[:8]
		return "[" + strings.ToUpper(string(detection.Type)) + "_" + hashStr + "]"
	
	case MaskingStrategyPartial:
		return m.generatePartialMask(detection.Value, detection.Type)
	
	case MaskingStrategyTokenize:
		return m.generateToken(detection.Value, detection.Type)
	
	default:
		return "[MASKED]"
	}
}

// generatePartialMask shows only part of the original value
func (m *PIIMasker) generatePartialMask(value string, piiType PIIType) string {
	switch piiType {
	case PIITypeEmail:
		parts := strings.Split(value, "@")
		if len(parts) == 2 {
			username := parts[0]
			domain := parts[1]
			if len(username) > 2 {
				return username[:2] + "***@" + domain
			}
		}
		return "***@***"
	
	case PIITypePhone:
		if len(value) >= 4 {
			return "***-***-" + value[len(value)-4:]
		}
		return "***-***-****"
	
	case PIITypeSSN:
		if len(value) >= 4 {
			return "***-**-" + value[len(value)-4:]
		}
		return "***-**-****"
	
	case PIITypeCreditCard:
		if len(value) >= 4 {
			return "**** **** **** " + value[len(value)-4:]
		}
		return "**** **** **** ****"
	
	default:
		if len(value) > 4 {
			return value[:2] + "***" + value[len(value)-2:]
		}
		return "***"
	}
}

// generateToken generates a consistent token for the value
func (m *PIIMasker) generateToken(value string, piiType PIIType) string {
	if m.config.EnableReversibleMasking {
		// Generate consistent token based on hash
		hash := sha256.Sum256([]byte(value))
		token := strings.ToUpper(string(piiType)) + "_" + hex.EncodeToString(hash[:])[:8]
		
		// Store mapping for potential reversal (security risk!)
		m.tokenMap[token] = value
		
		return "[" + token + "]"
	}
	
	// Generate non-reversible token
	hash := sha256.Sum256([]byte(value + string(piiType)))
	token := strings.ToUpper(string(piiType)) + "_" + hex.EncodeToString(hash[:])[:8]
	return "[" + token + "]"
}

// getDetectedTypes extracts unique PII types from detections
func (m *PIIMasker) getDetectedTypes(detections []PIIDetection) []string {
	typeSet := make(map[PIIType]bool)
	for _, detection := range detections {
		typeSet[detection.Type] = true
	}
	
	types := make([]string, 0, len(typeSet))
	for piiType := range typeSet {
		types = append(types, string(piiType))
	}
	
	return types
}

// getBuiltinPIIPatterns returns built-in PII detection patterns
func getBuiltinPIIPatterns() []PIIPattern {
	return []PIIPattern{
		{
			Type:        PIITypeEmail,
			Name:        "email_standard",
			Pattern:     `\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`,
			Replacement: "[EMAIL]",
			Confidence:  0.95,
		},
		{
			Type:        PIITypePhone,
			Name:        "phone_us",
			Pattern:     `(\+1[-.\s]?)?\(?([0-9]{3})\)?[-.\s]?([0-9]{3})[-.\s]?([0-9]{4})`,
			Replacement: "[PHONE]",
			Confidence:  0.9,
		},
		{
			Type:        PIITypeSSN,
			Name:        "ssn_us",
			Pattern:     `\b\d{3}-\d{2}-\d{4}\b`,
			Replacement: "[SSN]",
			Confidence:  0.95,
		},
		{
			Type:        PIITypeCreditCard,
			Name:        "credit_card_visa",
			Pattern:     `\b4[0-9]{12}(?:[0-9]{3})?\b`,
			Replacement: "[CREDIT_CARD]",
			Confidence:  0.85,
		},
		{
			Type:        PIITypeCreditCard,
			Name:        "credit_card_mastercard",
			Pattern:     `\b5[1-5][0-9]{14}\b`,
			Replacement: "[CREDIT_CARD]",
			Confidence:  0.85,
		},
		{
			Type:        PIITypeCreditCard,
			Name:        "credit_card_amex",
			Pattern:     `\b3[47][0-9]{13}\b`,
			Replacement: "[CREDIT_CARD]",
			Confidence:  0.85,
		},
		{
			Type:        PIITypeIPAddress,
			Name:        "ipv4",
			Pattern:     `\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`,
			Replacement: "[IP_ADDRESS]",
			Confidence:  0.8,
		},
		{
			Type:        PIITypeDateOfBirth,
			Name:        "date_us",
			Pattern:     `\b(0?[1-9]|1[0-2])/(0?[1-9]|[12]\d|3[01])/(19|20)\d{2}\b`,
			Replacement: "[DATE_OF_BIRTH]",
			Confidence:  0.7,
		},
		{
			Type:        PIITypePassport,
			Name:        "passport_us",
			Pattern:     `\b[A-Z0-9]{6,9}\b`,
			Replacement: "[PASSPORT]",
			Confidence:  0.6,
		},
		{
			Type:        PIITypeBankAccount,
			Name:        "bank_account",
			Pattern:     `\b\d{8,17}\b`,
			Replacement: "[BANK_ACCOUNT]",
			Confidence:  0.6,
		},
	}
}

// Unmask attempts to reverse PII masking (only works if reversible masking is enabled)
func (m *PIIMasker) Unmask(text string) string {
	if !m.config.EnableReversibleMasking {
		return text
	}
	
	result := text
	for token, originalValue := range m.tokenMap {
		result = strings.ReplaceAll(result, "["+token+"]", originalValue)
	}
	
	return result
}

// GetPIIStats returns statistics about PII detection
func (m *PIIMasker) GetPIIStats() map[string]interface{} {
	return map[string]interface{}{
		"enabled":                m.config.Enabled,
		"masking_strategy":       m.config.MaskingStrategy,
		"patterns_count":         len(m.patterns),
		"reversible_enabled":     m.config.EnableReversibleMasking,
		"stored_tokens":          len(m.tokenMap),
		"builtin_patterns":       m.config.EnableBuiltinPatterns,
		"audit_enabled":          m.config.AuditPIIDetection,
	}
}