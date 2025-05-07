package schema

import (
	"testing"
)

func TestValidateSchema(t *testing.T) {
	tests := []struct {
		name    string
		schema  []byte
		wantErr bool
	}{
		{
			name: "valid OpenAPI schema",
			schema: []byte(`{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API",
					"version": "1.0.0"
				},
				"paths": {}
			}`),
			wantErr: false,
		},
		{
			name: "invalid OpenAPI schema - missing version",
			schema: []byte(`{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API"
				},
				"paths": {}
			}`),
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			schema:  []byte(`{"invalid": json`),
			wantErr: true,
		},
		{
			name:    "empty schema",
			schema:  []byte{},
			wantErr: true,
		},
	}

	monitor := &Monitor{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := monitor.validateSchema(tt.schema)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSchema() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCalculateSchemaHash(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{
			name: "simple string",
			data: []byte("test data"),
			want: "916f0027a575074ce72a331777c3478d6513f786a591bd892da1a577bf2335f9",
		},
		{
			name: "empty data",
			data: []byte{},
			want: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // SHA-256 of empty string
		},
		{
			name: "OpenAPI schema - normalized",
			data: []byte(`{"info":{"title":"Test API","version":"1.0.0"},"openapi":"3.0.0"}`),
			want: "65b2e6596adb1111c95c38723de4cf0e84f72a38791c44ba1041f25aefc89ff2",
		},
		{
			name: "OpenAPI schema - with whitespace",
			data: []byte(`{
				"openapi": "3.0.0",
				"info": {
					"title": "Test API",
					"version": "1.0.0"
				}
			}`),
			want: "65b2e6596adb1111c95c38723de4cf0e84f72a38791c44ba1041f25aefc89ff2", // Same as normalized
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateSchemaHash(tt.data); got != tt.want {
				t.Errorf("calculateSchemaHash() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestHashConsistency ensures that the same input always produces the same hash
func TestHashConsistency(t *testing.T) {
	data := []byte("test data")
	hash1 := calculateSchemaHash(data)
	hash2 := calculateSchemaHash(data)

	if hash1 != hash2 {
		t.Errorf("Hash inconsistency: hash1 = %v, hash2 = %v", hash1, hash2)
	}
}

// TestHashDifferentInputs ensures that different inputs produce different hashes
func TestHashDifferentInputs(t *testing.T) {
	data1 := []byte("test data 1")
	data2 := []byte("test data 2")
	hash1 := calculateSchemaHash(data1)
	hash2 := calculateSchemaHash(data2)

	if hash1 == hash2 {
		t.Error("Different inputs produced the same hash")
	}
}
