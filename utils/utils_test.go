package utils

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestMust(t *testing.T) {
	tests := []struct {
		name      string
		obj       interface{}
		err       error
		wantPanic bool
	}{
		{
			name:      "success case",
			obj:       "test",
			err:       nil,
			wantPanic: false,
		},
		{
			name:      "panic case",
			obj:       nil,
			err:       fmt.Errorf("test error"),
			wantPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				defer func() {
					if r := recover(); r == nil {
						t.Error("Must() should have panicked but didn't")
					}
				}()
			}
			result := Must(tt.obj, tt.err)
			if !tt.wantPanic && result != tt.obj {
				t.Errorf("Must() = %v, want %v", result, tt.obj)
			}
		})
	}
}

func TestToPtr(t *testing.T) {
	tests := []struct {
		name string
		v    interface{}
	}{
		{
			name: "string value",
			v:    "test",
		},
		{
			name: "int value",
			v:    42,
		},
		{
			name: "bool value",
			v:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToPtr(tt.v)
			if *result != tt.v {
				t.Errorf("ToPtr() = %v, want %v", *result, tt.v)
			}
		})
	}
}

func TestJsonToMap(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		want    map[string]interface{}
		wantErr bool
	}{
		{
			name:    "valid json",
			jsonStr: `{"key": "value", "number": 42}`,
			want: map[string]interface{}{
				"key":    "value",
				"number": float64(42),
			},
			wantErr: false,
		},
		{
			name:    "invalid json",
			jsonStr: `{invalid json}`,
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JsonToMap(tt.jsonStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("JsonToMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
				t.Errorf("JsonToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMapToJson(t *testing.T) {
	tests := []struct {
		name    string
		jsonMap map[string]interface{}
		want    string
		wantErr bool
	}{
		{
			name: "valid map",
			jsonMap: map[string]interface{}{
				"key":    "value",
				"number": 42,
			},
			want:    `{"key":"value","number":42}`,
			wantErr: false,
		},
		{
			name:    "empty map",
			jsonMap: map[string]interface{}{},
			want:    `{}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MapToJson(tt.jsonMap)
			if (err != nil) != tt.wantErr {
				t.Errorf("MapToJson() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Compare the JSON strings after normalizing them
				var gotMap, wantMap map[string]interface{}
				json.Unmarshal([]byte(got), &gotMap)
				json.Unmarshal([]byte(tt.want), &wantMap)
				if !reflect.DeepEqual(gotMap, wantMap) {
					t.Errorf("MapToJson() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}
