package utils

import (
	"encoding/json"
	"fmt"
)

func Must[T any](obj T, err error) T {
	if err != nil {
		panic(err)
	}
	return obj
}

func MustWithoutOutput(err error) {
	if err != nil {
		panic(err)
	}
}

func ToPtr[T any](v T) *T {
	return &v
}

func JsonToMap(jsonString string) (map[string]any, error) {
	var result map[string]any
	err := json.Unmarshal([]byte(jsonString), &result)
	if err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %v", err)
	}
	return result, nil
}

func MapToJson(jsonMap map[string]any) (string, error) {
	bytes, err := json.Marshal(jsonMap)
	if err != nil {
		return "", fmt.Errorf("failed to serialize map to JSON: %v", err)
	}
	return string(bytes), nil
}
