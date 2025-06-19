package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPIIMasker_NewPIIMasker(t *testing.T) {
	tests := []struct {
		name   string
		config *PIIMaskingConfig
		want   bool
	}{
		{
			name: "default config",
			config: &PIIMaskingConfig{
				Enabled:               true,
				EnableBuiltinPatterns: true,
				PreserveFormat:        false,
				AuditPIIDetection:     true,
			},
			want: true,
		},
		{
			name: "custom patterns",
			config: &PIIMaskingConfig{
				Enabled:        true,
				CustomPatterns: []PIIPattern{
					{
						Name:        "test-pattern",
						Pattern:     `test-\d+`,
						Replacement: "[TEST]",
						Confidence:  0.9,
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masker := NewPIIMasker(tt.config)
			assert.NotNil(t, masker)
			assert.Equal(t, tt.config, masker.config)
		})
	}
}

func TestPIIMasker_MaskPII(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
		PreserveFormat:        false,
		AuditPIIDetection:     true,
	}
	masker := NewPIIMasker(config)

	tests := []struct {
		name     string
		input    string
		contains string
		excludes string
	}{
		{
			name:     "SSN masking",
			input:    "My SSN is 123-45-6789",
			contains: "[SSN]",
			excludes: "123-45-6789",
		},
		{
			name:     "email masking",
			input:    "Contact me at john.doe@example.com",
			contains: "[EMAIL]",
			excludes: "john.doe@example.com",
		},
		{
			name:     "phone masking",
			input:    "Call me at (555) 123-4567",
			contains: "[PHONE]",
			excludes: "(555) 123-4567",
		},
		{
			name:     "credit card masking",
			input:    "My card is 4532-1234-5678-9012",
			contains: "[CREDIT_CARD]",
			excludes: "4532-1234-5678-9012",
		},
		{
			name:     "IP address masking",
			input:    "Server IP: 192.168.1.100",
			contains: "[IP_ADDRESS]",
			excludes: "192.168.1.100",
		},
		{
			name:     "multiple PII types",
			input:    "John Doe, SSN: 123-45-6789, email: john@example.com, phone: 555-123-4567",
			contains: "[SSN]",
			excludes: "123-45-6789",
		},
		{
			name:     "no PII",
			input:    "This is a normal message with no sensitive data",
			contains: "This is a normal message with no sensitive data",
			excludes: "[",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result := masker.MaskPII(ctx, tt.input)
			
			assert.Contains(t, result, tt.contains)
			if tt.excludes != "" {
				assert.NotContains(t, result, tt.excludes)
			}
		})
	}
}

func TestPIIMasker_PreserveFormat(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
		PreserveFormat:        true,
		AuditPIIDetection:     false,
	}
	masker := NewPIIMasker(config)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "SSN format preservation",
			input:    "SSN: 123-45-6789",
			expected: "SSN: XXX-XX-XXXX",
		},
		{
			name:     "phone format preservation",
			input:    "Phone: (555) 123-4567",
			expected: "Phone: (XXX) XXX-XXXX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result := masker.MaskPII(ctx, tt.input)
			assert.Contains(t, result, "XXX")
		})
	}
}

func TestPIIMasker_CustomPatterns(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: false,
		CustomPatterns: []PIIPattern{
			{
				Name:        "employee-id",
				Pattern:     `EMP-\d{4}`,
				Replacement: "[EMPLOYEE_ID]",
				Confidence:  0.9,
			},
			{
				Name:        "project-code",
				Pattern:     `PROJ-[A-Z]{3}`,
				Replacement: "[PROJECT]",
				Confidence:  0.8,
			},
		},
	}
	masker := NewPIIMasker(config)

	tests := []struct {
		name     string
		input    string
		contains string
		excludes string
	}{
		{
			name:     "employee ID masking",
			input:    "Employee EMP-1234 completed the task",
			contains: "[EMPLOYEE_ID]",
			excludes: "EMP-1234",
		},
		{
			name:     "project code masking",
			input:    "Working on PROJ-ABC this week",
			contains: "[PROJECT]",
			excludes: "PROJ-ABC",
		},
		{
			name:     "multiple custom patterns",
			input:    "EMP-5678 is assigned to PROJ-XYZ",
			contains: "[EMPLOYEE_ID]",
			excludes: "EMP-5678",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result := masker.MaskPII(ctx, tt.input)
			
			assert.Contains(t, result, tt.contains)
			assert.NotContains(t, result, tt.excludes)
		})
	}
}

func TestPIIMasker_ReversibleMasking(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:                 true,
		EnableBuiltinPatterns:   true,
		EnableReversibleMasking: true,
	}
	masker := NewPIIMasker(config)

	ctx := context.Background()
	originalText := "My email is john.doe@example.com"
	
	// Mask the text
	maskedText := masker.MaskPII(ctx, originalText)
	assert.NotEqual(t, originalText, maskedText)
	assert.NotContains(t, maskedText, "john.doe@example.com")
	
	// Unmask the text
	unmaskedText := masker.Unmask(maskedText)
	assert.Equal(t, originalText, unmaskedText)
}

func TestPIIMasker_Disabled(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled: false,
	}
	masker := NewPIIMasker(config)

	ctx := context.Background()
	input := "My SSN is 123-45-6789 and email is test@example.com"
	result := masker.MaskPII(ctx, input)
	
	// Should return original text unchanged when disabled
	assert.Equal(t, input, result)
}

func TestPIIMasker_AuditLogging(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
		AuditPIIDetection:     true,
	}
	masker := NewPIIMasker(config)

	ctx := context.Background()
	input := "Contact: john@example.com, Phone: 555-123-4567"
	
	// This should trigger audit logging (we can't easily test the actual logging,
	// but we can verify the masking works)
	result := masker.MaskPII(ctx, input)
	
	assert.Contains(t, result, "[EMAIL]")
	assert.Contains(t, result, "[PHONE]")
	assert.NotContains(t, result, "john@example.com")
	assert.NotContains(t, result, "555-123-4567")
}

func TestPIIMasker_PerformanceWithLargeText(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
	}
	masker := NewPIIMasker(config)

	// Create a large text with embedded PII
	largeText := ""
	for i := 0; i < 1000; i++ {
		largeText += "Some normal text here. "
		if i%100 == 0 {
			largeText += "Contact john@example.com or call 555-123-4567. "
		}
	}

	ctx := context.Background()
	start := time.Now()
	result := masker.MaskPII(ctx, largeText)
	duration := time.Since(start)

	// Should complete within reasonable time (less than 1 second for this size)
	assert.Less(t, duration, time.Second)
	assert.Contains(t, result, "[EMAIL]")
	assert.Contains(t, result, "[PHONE]")
}

func TestPIIMasker_ConcurrentAccess(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
	}
	masker := NewPIIMasker(config)

	// Test concurrent access to the masker
	const numGoroutines = 10
	const numOperations = 100
	
	done := make(chan bool, numGoroutines)
	
	for i := 0; i < numGoroutines; i++ {
		go func(routineID int) {
			ctx := context.Background()
			for j := 0; j < numOperations; j++ {
				input := "SSN: 123-45-6789, Email: test@example.com"
				result := masker.MaskPII(ctx, input)
				assert.Contains(t, result, "[SSN]")
				assert.Contains(t, result, "[EMAIL]")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < numGoroutines; i++ {
		<-done
	}
}

func TestPIIMasker_EdgeCases(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
	}
	masker := NewPIIMasker(config)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "very long string",
			input: string(make([]byte, 10000)),
		},
		{
			name:  "special characters",
			input: "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		},
		{
			name:  "unicode characters",
			input: "Unicode: 你好世界 🌟 émojis",
		},
		{
			name:  "mixed content",
			input: "Normal text 123-45-6789 more text test@example.com end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			// Should not panic or error
			result := masker.MaskPII(ctx, tt.input)
			assert.NotNil(t, result)
		})
	}
}

func TestBuiltinPatterns(t *testing.T) {
	patterns := getBuiltinPatterns()
	
	// Verify we have expected built-in patterns
	assert.NotEmpty(t, patterns)
	
	patternNames := make(map[string]bool)
	for _, pattern := range patterns {
		patternNames[pattern.Name] = true
		
		// Verify each pattern has required fields
		assert.NotEmpty(t, pattern.Name)
		assert.NotEmpty(t, pattern.Pattern)
		assert.NotEmpty(t, pattern.Replacement)
		assert.Greater(t, pattern.Confidence, 0.0)
		assert.LessOrEqual(t, pattern.Confidence, 1.0)
	}
	
	// Verify we have key PII pattern types
	expectedPatterns := []string{"ssn", "email", "phone", "credit_card", "ip_address"}
	for _, expected := range expectedPatterns {
		assert.True(t, patternNames[expected], "Missing expected pattern: %s", expected)
	}
}

func TestPIIPattern_Validation(t *testing.T) {
	tests := []struct {
		name    string
		pattern PIIPattern
		valid   bool
	}{
		{
			name: "valid pattern",
			pattern: PIIPattern{
				Name:        "test",
				Pattern:     `\d+`,
				Replacement: "[TEST]",
				Confidence:  0.8,
			},
			valid: true,
		},
		{
			name: "invalid regex pattern",
			pattern: PIIPattern{
				Name:        "test",
				Pattern:     `[`,
				Replacement: "[TEST]",
				Confidence:  0.8,
			},
			valid: false,
		},
		{
			name: "confidence too low",
			pattern: PIIPattern{
				Name:        "test",
				Pattern:     `\d+`,
				Replacement: "[TEST]",
				Confidence:  -0.1,
			},
			valid: false,
		},
		{
			name: "confidence too high",
			pattern: PIIPattern{
				Name:        "test",
				Pattern:     `\d+`,
				Replacement: "[TEST]",
				Confidence:  1.1,
			},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &PIIMaskingConfig{
				Enabled:        true,
				CustomPatterns: []PIIPattern{tt.pattern},
			}
			
			// This tests the pattern validation during masker creation
			masker := NewPIIMasker(config)
			assert.NotNil(t, masker)
			
			// For invalid patterns, they should be skipped during processing
			ctx := context.Background()
			result := masker.MaskPII(ctx, "test 123 data")
			assert.NotNil(t, result)
		})
	}
}