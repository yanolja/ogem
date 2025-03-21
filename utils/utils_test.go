package utils

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestMust(t *testing.T) {
	t.Run("Return object when no error", func(t *testing.T) {
		obj := "test"
		err := error(nil)
		got := Must(obj, err)
		assert.Equal(t, obj, got)
	})

	t.Run("Panic when error is not nil", func(t *testing.T) {
		obj := any(nil)
		err := fmt.Errorf("test error")
		assert.PanicsWithValue(t, err, func() {
			_ = Must(obj, err)
		})
	})
}

func TestToPtr(t *testing.T) {
	t.Run("String value", func(t *testing.T) {
		value := "test"
		ptr := ToPtr(value)
		assert.Equal(t, "test", *ptr)
	})

	t.Run("Int value", func(t *testing.T) {
		value := 42
		ptr := ToPtr(value)
		assert.Equal(t, 42, *ptr)
	})

	t.Run("Bool value", func(t *testing.T) {
		value := true
		ptr := ToPtr(value)
		assert.Equal(t, true, *ptr)
	})

	t.Run("Float value", func(t *testing.T) {
		value := 3.14
		ptr := ToPtr(value)
		assert.Equal(t, 3.14, *ptr)
	})

	t.Run("Any value", func(t *testing.T) {
		value := []any{'a', 1, 3.14, "test", []int{1, 2, 3}}
		ptr := ToPtr(value)
		assert.Equal(t, []any{'a', 1, 3.14, "test", []int{1, 2, 3}}, *ptr)
	})
}

func TestJsonToMap(t *testing.T) {
	t.Run("Valid json", func(t *testing.T) {
		jsonStr := `{"key": "value", "number": 42}`
		want := map[string]any{
			"key":    "value",
			"number": float64(42),
		}
		got, err := JsonToMap(jsonStr)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("Invalid json", func(t *testing.T) {
		jsonStr := `{invalid json}`
		got, err := JsonToMap(jsonStr)
		assert.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("Empty json", func(t *testing.T) {
		jsonStr := `{"key": "value", "number": -1, "": 2}`
		want := map[string]any{
			"key":    "value",
			"number": float64(-1),
			"":       float64(2),
		}
		got, err := JsonToMap(jsonStr)
		assert.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestMapToJson(t *testing.T) {
	t.Run("Valid map to JSON", func(t *testing.T) {
		jsonMap := map[string]any{
			"key":    "value",
			"number": 42,
			"":       2,
		}
		want := `{"key":"value","number":42,"":2}`
		got, err := MapToJson(jsonMap)
		assert.NoError(t, err)
		assert.JSONEq(t, want, got)
	})

	t.Run("Empty map to JSON", func(t *testing.T) {
		jsonMap := map[string]any{}
		want := `{}`
		got, err := MapToJson(jsonMap)
		assert.NoError(t, err)
		assert.JSONEq(t, want, got)
	})

	t.Run("Null map to JSON", func(t *testing.T) {
		jsonMap := map[string]any(nil)
		want := `null`
		got, err := MapToJson(jsonMap)
		assert.NoError(t, err)
		assert.JSONEq(t, want, got)
	})
}
