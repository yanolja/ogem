package copy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeep(t *testing.T) {
	t.Run("Deep copied int type", func(t *testing.T) {
		original := 42
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("Deep copied bool type", func(t *testing.T) {
		original := true
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("Deep copied string type", func(t *testing.T) {
		original := "string"
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("Fails copied in case nil", func(t *testing.T) {
		original := error(nil)
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})

	t.Run("Deep copied any types", func(t *testing.T) {
		original := []any{1, 'a', "test", true, 42.0, []any{1, 2, 3}, map[string]any{"key": "value"}}
		copied, err := Deep(original)
		copied[0] = 2
		copied[1] = 'b'
		copied[4] = 'c'
		assert.NoError(t, err)
		assert.NotEqual(t, original, copied)
	})

	t.Run("Successfully deep copy pointer types", func(t *testing.T) {
		str := "test"
		original := &str
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.NotSame(t, original, copied)
		assert.Equal(t, *original, *copied)
		*copied = "new"
		assert.NotEqual(t, *original, *copied)
	})

	t.Run("Fails copying struct{}{}", func(t *testing.T) {
		original := struct{}{}
		copied, err := Deep(original)
		assert.NoError(t, err)
		assert.Equal(t, original, copied)
	})
}
