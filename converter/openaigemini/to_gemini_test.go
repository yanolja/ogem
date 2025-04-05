
package openaigeminiconverter

import (
	"encoding/json"
	"testing"

	"github.com/google/generative-ai-go/genai"
	"github.com/stretchr/testify/assert"

	"github.com/yanolja/ogem/utils/orderedmap"
)

func TestToGeminiSchema(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedSchema *genai.Schema
		expectedError  bool
	}{
		{
			name: "Simple object schema",
			input: `{
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "integer"}
				},
				"required": ["name"]
			}`,
			expectedSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"name": {Type: genai.TypeString},
					"age":  {Type: genai.TypeInteger},
				},
				Required: []string{"name"},
			},
			expectedError: false,
		},
		{
			name: "Nested object schema",
			input: `{
				"type": "object",
				"properties": {
					"user": {
						"type": "object",
						"properties": {
							"name": {"type": "string"},
							"email": {"type": "string", "format": "email"}
						}
					},
					"items": {
						"type": "array",
						"items": {"type": "string"}
					}
				}
			}`,
			expectedSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"user": {
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"name":  {Type: genai.TypeString},
							"email": {Type: genai.TypeString, Format: "email"},
						},
					},
					"items": {
						Type:  genai.TypeArray,
						Items: &genai.Schema{Type: genai.TypeString},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Schema with enum",
			input: `{
				"type": "object",
				"properties": {
					"color": {
						"type": "string",
						"enum": ["red", "green", "blue"]
					}
				}
			}`,
			expectedSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"color": {
						Type: genai.TypeString,
						Enum: []string{"red", "green", "blue"},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Schema with description and nullable",
			input: `{
				"type": "object",
				"properties": {
					"description": {
						"type": "string",
						"description": "A brief description",
						"nullable": true
					}
				}
			}`,
			expectedSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"description": {
						Type:        genai.TypeString,
						Description: "A brief description",
						Nullable:    true,
					},
				},
			},
			expectedError: false,
		},
		{
			name:          "Invalid schema",
			input:         `{"type": "invalid"}`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema orderedmap.Map
			err := json.Unmarshal([]byte(tt.input), &schema)
			assert.NoError(t, err, "Failed to unmarshal input JSON")

			result, err := toGeminiSchema(&schema)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSchema, result)
			}
		})
	}
}

func TestToGeminiSchema_NilInput(t *testing.T) {
	result, err := toGeminiSchema(nil)
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestToGeminiSchema_WithRef(t *testing.T) {
	tests := []struct {
		name           string
		input          string
		expectedSchema *genai.Schema
		expectedError  bool
	}{
		{
			name: "Simple $ref",
			input: `{
				"type": "object",
				"properties": {
					"user": {"$ref": "#/$defs/User"}
				},
				"$defs": {
					"User": {
						"type": "object",
						"properties": {
							"name": {"type": "string"},
							"age": {"type": "integer"}
						}
					}
				}
			}`,
			expectedSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"user": {
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"name": {Type: genai.TypeString},
							"age":  {Type: genai.TypeInteger},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Nested $ref",
			input: `{
				"type": "object",
				"properties": {
					"user": {"$ref": "#/$defs/User"}
				},
				"$defs": {
					"User": {
						"type": "object",
						"properties": {
							"name": {"type": "string"},
							"address": {"$ref": "#/$defs/Address"}
						}
					},
					"Address": {
						"type": "object",
						"properties": {
							"street": {"type": "string"},
							"city": {"type": "string"}
						}
					}
				}
			}`,
			expectedSchema: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"user": {
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"name": {Type: genai.TypeString},
							"address": {
								Type: genai.TypeObject,
								Properties: map[string]*genai.Schema{
									"street": {Type: genai.TypeString},
									"city":   {Type: genai.TypeString},
								},
							},
						},
					},
				},
			},
			expectedError: false,
		},
		{
			name: "Invalid $ref",
			input: `{
				"type": "object",
				"properties": {
					"user": {"$ref": "#/$defs/NonExistentUser"}
				},
				"$defs": {
					"User": {
						"type": "object",
						"properties": {
							"name": {"type": "string"}
						}
					}
				}
			}`,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema orderedmap.Map
			err := json.Unmarshal([]byte(tt.input), &schema)
			assert.NoError(t, err, "Failed to unmarshal input JSON")

			result, err := toGeminiSchema(&schema)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedSchema, result)
			}
		})
	}
}