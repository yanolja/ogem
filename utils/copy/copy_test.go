package copy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type TestStruct struct {
	String    string
	Int       int
	Float     float64
	Bool      bool
	Slice     []string
	Map       map[string]int
	Ptr       *string
	Interface interface{}
}

func TestCopy(t *testing.T) {
	// Test case 1: Copy primitive types
	t.Run("primitive types", func(t *testing.T) {
		original := TestStruct{
			String: "test",
			Int:    42,
			Float:  3.14,
			Bool:   true,
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	// Test case 2: Copy slice
	t.Run("slice", func(t *testing.T) {
		original := TestStruct{
			Slice: []string{"a", "b", "c"},
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		// Modify the copied slice to verify deep copy
		copied.Slice[0] = "x"
		assert.NotEqual(t, original.Slice[0], "x", "Deep() created a shallow copy of slice")
	})

	// Test case 3: Copy map
	t.Run("map", func(t *testing.T) {
		original := TestStruct{
			Map: map[string]int{
				"a": 1,
				"b": 2,
			},
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		// Modify the copied map to verify deep copy
		copied.Map["a"] = 3
		assert.NotEqual(t, original.Map["a"], 3, "Deep() created a shallow copy of map")
	})

	// Test case 4: Copy pointer
	t.Run("pointer", func(t *testing.T) {
		str := "test"
		original := TestStruct{
			Ptr: &str,
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		// Modify the copied pointer to verify deep copy
		*copied.Ptr = "new"
		assert.NotEqual(t, *original.Ptr, "new", "Deep() created a shallow copy of pointer")
	})

	// Test case 5: Copy interface
	t.Run("interface", func(t *testing.T) {
		original := TestStruct{
			Interface: map[string]interface{}{
				"key": "value",
			},
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		// Modify the copied interface to verify deep copy
		copiedMap := copied.Interface.(map[string]interface{})
		copiedMap["key"] = "new value"
		assert.NotEqual(t, original.Interface.(map[string]interface{})["key"], "new value", "Deep() created a shallow copy of interface")
	})

	// Test case 6: Copy nil values
	t.Run("nil values", func(t *testing.T) {
		original := TestStruct{
			Slice:     nil,
			Map:       nil,
			Ptr:       nil,
			Interface: nil,
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	// Test case 7: Copy empty values
	t.Run("empty values", func(t *testing.T) {
		original := TestStruct{
			Slice:     []string{},
			Map:       map[string]int{},
			Interface: map[string]interface{}{},
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	// Test case 8: Copy primitive type directly
	t.Run("primitive type", func(t *testing.T) {
		original := 42
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	// Test case 9: Copy slice directly
	t.Run("slice directly", func(t *testing.T) {
		original := []string{"a", "b", "c"}
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		copied[0] = "x"
		assert.NotEqual(t, original[0], "x", "Deep() created a shallow copy of slice")
	})
}
