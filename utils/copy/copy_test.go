package copy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStruct is a test struct for deep copy, a struct may be shallow copied by copying the pointer to the struct
type TestStruct struct {
	String string
	Int    int
	Float  float64
	Bool   bool
	Slice  []string
	Map    map[string]int
	Ptr    *string
	Any    any
}

func TestCopy(t *testing.T) {

	t.Run("Successfully deep copied primitive types", func(t *testing.T) {
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

	t.Run("Successfully deep copy slice types", func(t *testing.T) {
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

	t.Run("Successfully deep copy map types", func(t *testing.T) {
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

	t.Run("Successfully deep copy pointer types", func(t *testing.T) {
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

	t.Run("Successfully copied any types", func(t *testing.T) {
		original := TestStruct{
			Any: map[string]any{
				"key": "value",
			},
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		// Modify the copied any to verify deep copy
		copiedMap := copied.Any.(map[string]any)
		copiedMap["key"] = "new value"
		assert.NotEqual(t, original.Any.(map[string]any)["key"], "new value", "Deep() created a shallow copy of any")
	})

	t.Run("Fails copied because something return nil", func(t *testing.T) {
		original := TestStruct{
			Slice: nil,
			Map:   nil,
			Ptr:   nil,
			Any:   nil,
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("Fails copied empty values", func(t *testing.T) {
		original := TestStruct{
			Slice: []string{},
			Map:   map[string]int{},
			Any:   map[string]any{},
		}

		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("primitive type", func(t *testing.T) {
		original := 42
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("slice directly", func(t *testing.T) {
		original := []string{"a", "b", "c"}
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)

		copied[0] = "x"
		assert.NotEqual(t, original[0], "x", "Deep() created a shallow copy of slice")
	})
}
