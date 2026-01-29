package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yanolja/ogem/openai"
	"github.com/yanolja/ogem/utils"
	"go.uber.org/zap"
)

func TestPIIMasker_NewPIIMasker(t *testing.T) {
	tests := []struct {
		name   string
		config *PIIMaskingConfig
	}{
		{
			name: "default config",
			config: &PIIMaskingConfig{
				Enabled:               true,
				EnableBuiltinPatterns: true,
				PreserveFormat:        false,
				AuditPIIDetection:     true,
			},
		},
		{
			name: "custom patterns",
			config: &PIIMaskingConfig{
				Enabled: true,
				CustomPatterns: []PIIPattern{
					{
						Name:        "test-pattern",
						Pattern:     `test-\d+`,
						Replacement: "[TEST]",
						Confidence:  0.9,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			masker, err := NewPIIMasker(tt.config, zap.NewNop().Sugar())
			assert.NoError(t, err)
			assert.NotNil(t, masker)
			assert.Equal(t, tt.config, masker.config)
		})
	}
}

func TestPIIMasker_MaskChatRequest(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
		PreserveFormat:        false,
		AuditPIIDetection:     true,
	}
	masker, err := NewPIIMasker(config, zap.NewNop().Sugar())
	require.NoError(t, err)

	tests := []struct {
		name        string
		request     *openai.ChatCompletionRequest
		shouldMask  bool
		checkString string
	}{
		{
			name: "mask SSN in message",
			request: &openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{
						Role:    "user",
						Content: &openai.MessageContent{String: utils.ToPtr("My SSN is 123-45-6789")},
					},
				},
			},
			shouldMask:  true,
			checkString: "123-45-6789",
		},
		{
			name: "mask email in message",
			request: &openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{
						Role:    "user",
						Content: &openai.MessageContent{String: utils.ToPtr("Contact me at john.doe@example.com")},
					},
				},
			},
			shouldMask:  true,
			checkString: "john.doe@example.com",
		},
		{
			name: "no PII in message",
			request: &openai.ChatCompletionRequest{
				Model: "gpt-4",
				Messages: []openai.Message{
					{
						Role:    "user",
						Content: &openai.MessageContent{String: utils.ToPtr("This is a normal message")},
					},
				},
			},
			shouldMask:  false,
			checkString: "This is a normal message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, detections, err := masker.MaskChatRequest(tt.request)
			require.NoError(t, err)
			require.NotNil(t, result)

			if tt.shouldMask {
				assert.NotEmpty(t, detections)
				if result.Messages[0].Content.String != nil {
					assert.NotContains(t, *result.Messages[0].Content.String, tt.checkString)
				}
			} else {
				assert.Empty(t, detections)
				if result.Messages[0].Content.String != nil {
					assert.Contains(t, *result.Messages[0].Content.String, tt.checkString)
				}
			}
		})
	}
}

func TestPIIMasker_Disabled(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled: false,
	}
	masker, err := NewPIIMasker(config, zap.NewNop().Sugar())
	require.NoError(t, err)

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: utils.ToPtr("My SSN is 123-45-6789")},
			},
		},
	}

	result, detections, err := masker.MaskChatRequest(request)
	require.NoError(t, err)
	assert.Empty(t, detections)
	assert.Equal(t, "My SSN is 123-45-6789", *result.Messages[0].Content.String)
}

func TestPIIMasker_CustomPatterns(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: false,
		MaskingStrategy:       MaskingStrategyReplace,
		CustomPatterns: []PIIPattern{
			{
				Name:        "employee-id",
				Pattern:     `EMP-\d{4}`,
				Replacement: "[EMPLOYEE_ID]",
				Confidence:  0.9,
			},
		},
	}
	masker, err := NewPIIMasker(config, zap.NewNop().Sugar())
	require.NoError(t, err)

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: utils.ToPtr("Employee EMP-1234 completed the task")},
			},
		},
	}

	result, detections, err := masker.MaskChatRequest(request)
	require.NoError(t, err)
	assert.NotEmpty(t, detections)
	assert.NotContains(t, *result.Messages[0].Content.String, "EMP-1234")
	assert.Contains(t, *result.Messages[0].Content.String, "[EMPLOYEE_ID]")
}

func TestPIIMasker_Unmask(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:                 true,
		EnableBuiltinPatterns:   true,
		MaskingStrategy:         MaskingStrategyTokenize,
		EnableReversibleMasking: true,
	}
	masker, err := NewPIIMasker(config, zap.NewNop().Sugar())
	require.NoError(t, err)

	request := &openai.ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []openai.Message{
			{
				Role:    "user",
				Content: &openai.MessageContent{String: utils.ToPtr("My email is john.doe@example.com")},
			},
		},
	}

	maskedRequest, detections, err := masker.MaskChatRequest(request)
	require.NoError(t, err)
	assert.NotEmpty(t, detections)

	maskedText := *maskedRequest.Messages[0].Content.String
	assert.NotContains(t, maskedText, "john.doe@example.com")

	unmaskedText := masker.Unmask(maskedText)
	assert.Equal(t, "My email is john.doe@example.com", unmaskedText)
}

func TestPIIMasker_GetPIIStats(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
	}
	masker, err := NewPIIMasker(config, zap.NewNop().Sugar())
	require.NoError(t, err)

	stats := masker.GetPIIStats()
	assert.NotNil(t, stats)
	assert.Contains(t, stats, "enabled")
	assert.Contains(t, stats, "patterns_count")
	assert.Equal(t, true, stats["enabled"])
}

func TestPIIMasker_EmptyRequest(t *testing.T) {
	config := &PIIMaskingConfig{
		Enabled:               true,
		EnableBuiltinPatterns: true,
	}
	masker, err := NewPIIMasker(config, zap.NewNop().Sugar())
	require.NoError(t, err)

	request := &openai.ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []openai.Message{},
	}

	result, detections, err := masker.MaskChatRequest(request)
	require.NoError(t, err)
	assert.Empty(t, detections)
	assert.NotNil(t, result)
}
