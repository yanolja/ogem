package claude

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEndpoint(t *testing.T) {
	ep, err := NewEndpoint("test-api-key")
	assert.NoError(t, err)
	assert.NotNil(t, ep)
	assert.NotNil(t, ep.client)
}

func TestStandardizeModelName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"claude-3-5-sonnet", "claude-3-5-sonnet-20240620"},
		{"claude-3-opus", "claude-3-opus-20240229"},
		{"claude-3-sonnet", "claude-3-sonnet-20240229"},
		{"claude-3-haiku", "claude-3-haiku-20240307"},
		{"custom-model", "custom-model"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := standardizeModelName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
