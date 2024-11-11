package orderedmap

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderedMap(t *testing.T) {
	t.Run("New map", func(t *testing.T) {
		m := New()
		assert.Empty(t, m.Keys())
		assert.Empty(t, m.Entries())
	})

	t.Run("Set and Get", func(t *testing.T) {
		m := New()
		m.Set("a", 1)
		m.Set("b", 2)
		m.Set("c", 3)

		val, exists := m.Get("b")
		assert.True(t, exists)
		assert.Equal(t, 2, val)

		// Test non-existent key
		val, exists = m.Get("d")
		assert.False(t, exists)
		assert.Nil(t, val)

		// Test order preservation
		assert.Equal(t, []string{"a", "b", "c"}, m.Keys())
	})

	t.Run("Set overwrites existing key", func(t *testing.T) {
		m := New()
		m.Set("a", 1)
		m.Set("b", 2)
		m.Set("a", 3) // Overwrite a

		val, exists := m.Get("a")
		assert.True(t, exists)
		assert.Equal(t, 3, val)

		// Order should remain the same
		assert.Equal(t, []string{"a", "b"}, m.Keys())
	})

	t.Run("Entries returns ordered entries", func(t *testing.T) {
		m := New()
		m.Set("a", 1)
		m.Set("b", 2)
		m.Set("c", 3)

		entries := m.Entries()
		expected := []Entry{
			{Key: "a", Value: 1},
			{Key: "b", Value: 2},
			{Key: "c", Value: 3},
		}
		assert.Equal(t, expected, entries)
	})

	t.Run("JSON Marshal/Unmarshal", func(t *testing.T) {
		t.Run("Simple values", func(t *testing.T) {
			m := New()
			// Numbers become float64 when unmarshaled because there are no
			// distinct types for int, float, etc. in JSON.
			m.Set("a", float64(1))
			m.Set("b", "hello")
			m.Set("c", true)

			data, err := json.Marshal(m)
			assert.NoError(t, err)

			var m2 Map
			err = json.Unmarshal(data, &m2)
			assert.NoError(t, err)

			assert.Equal(t, m.Keys(), m2.Keys())
			
			// Check values individually
			v1, _ := m2.Get("a")
			assert.Equal(t, float64(1), v1)
			v2, _ := m2.Get("b")
			assert.Equal(t, "hello", v2)
			v3, _ := m2.Get("c")
			assert.Equal(t, true, v3)
		})

		t.Run("Nested maps", func(t *testing.T) {
			m := New()
			nested := New()
			nested.Set("x", float64(1))
			nested.Set("y", float64(2))
			m.Set("a", nested)
			m.Set("b", "hello")

			data, err := json.Marshal(m)
			assert.NoError(t, err)

			var m2 Map
			err = json.Unmarshal(data, &m2)
			assert.NoError(t, err)

			assert.Equal(t, m.Keys(), m2.Keys())
			
			// Check nested map
			nestedResult, exists := m2.Get("a")
			assert.True(t, exists)
			nestedMap, ok := nestedResult.(*Map)
			assert.True(t, ok)
			assert.Equal(t, []string{"x", "y"}, nestedMap.Keys())
			x, _ := nestedMap.Get("x")
			assert.Equal(t, float64(1), x)
			y, _ := nestedMap.Get("y")
			assert.Equal(t, float64(2), y)
		})

		t.Run("Complex JSON", func(t *testing.T) {
			jsonStr := `{
				"type": "object",
				"properties": {
					"name": {"type": "string"},
					"age": {"type": "integer"},
					"address": {
						"type": "object",
						"properties": {
							"street": {"type": "string"},
							"city": {"type": "string"}
						},
						"required": ["street"]
					}
				},
				"required": ["name", "age"]
			}`

			var m Map
			err := json.Unmarshal([]byte(jsonStr), &m)
			assert.NoError(t, err)

			// Check structure
			assert.Equal(t, []string{"type", "properties", "required"}, m.Keys())
			
			// Check nested properties
			props, exists := m.Get("properties")
			assert.True(t, exists)
			propsMap, ok := props.(*Map)
			assert.True(t, ok)
			assert.Equal(t, []string{"name", "age", "address"}, propsMap.Keys())

			// Verify it can be marshaled back
			data, err := json.Marshal(&m)
			assert.NoError(t, err)

			// Unmarshal into a new map and compare
			var m2 Map
			err = json.Unmarshal(data, &m2)
			assert.NoError(t, err)
			assert.Equal(t, m.Keys(), m2.Keys())
		})
	})

	t.Run("Array handling", func(t *testing.T) {
		jsonStr := `{
			"items": ["a", "b", "c"],
			"numbers": [1, 2, 3]
		}`

		var m Map
		err := json.Unmarshal([]byte(jsonStr), &m)
		assert.NoError(t, err)

		items, exists := m.Get("items")
		assert.True(t, exists)
		itemsArr, ok := items.([]interface{})
		assert.True(t, ok)
		assert.Equal(t, []interface{}{"a", "b", "c"}, itemsArr)

		numbers, exists := m.Get("numbers")
		assert.True(t, exists)
		numbersArr, ok := numbers.([]interface{})
		assert.True(t, ok)
		assert.Equal(t, []interface{}{float64(1), float64(2), float64(3)}, numbersArr)
	})

	t.Run("Edge cases", func(t *testing.T) {
		t.Run("Empty JSON", func(t *testing.T) {
			var m Map
			err := json.Unmarshal([]byte("{}"), &m)
			assert.NoError(t, err)
			assert.Empty(t, m.Keys())
		})

		t.Run("Null values", func(t *testing.T) {
			var m Map
			err := json.Unmarshal([]byte(`{"a": null}`), &m)
			assert.NoError(t, err)
			val, exists := m.Get("a")
			assert.True(t, exists)
			assert.Nil(t, val)
		})

		t.Run("Invalid JSON", func(t *testing.T) {
			var m Map
			err := json.Unmarshal([]byte(`{"a": 1,}`), &m)
			assert.Error(t, err)
		})

		t.Run("Non-string keys", func(t *testing.T) {
			var m Map
			err := json.Unmarshal([]byte(`{1: "a"}`), &m)
			assert.Error(t, err)
		})
	})
}