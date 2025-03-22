package array

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMap(t *testing.T) {
	t.Run("Map int to int", func(t *testing.T) {
		array := []int{1, 2, 3}
		mapper := func(i int) int {
			return i * 2
		}
		got := Map(array, mapper)
		assert.Equal(t, []int{2, 4, 6}, got)
	})

	t.Run("Map string to int", func(t *testing.T) {
		array := []string{"1", "2", "3"}
		mapper := func(s string) int {
			return int(s[0] - '0')
		}
		got := Map(array, mapper)
		assert.Equal(t, []int{1, 2, 3}, got)
	})
}

func TestContains(t *testing.T) {
	t.Run("Contains int", func(t *testing.T) {
		array := []int{1, 2, 3}
		assert.False(t, Contains(array, 4))
		assert.True(t, Contains(array, 3))
	})

	t.Run("Contains string", func(t *testing.T) {
		array := []string{"a", "b", "c"}
		assert.False(t, Contains(array, "d"))
		assert.True(t, Contains(array, "a"))
	})

	t.Run("Contains any", func(t *testing.T) {
		array := []any{1, "test", 3.14, []int{1, 2, 3}}
		assert.False(t, Contains(array, 4))
		assert.True(t, Contains(array, "test"))
	})
}

func TestFind(t *testing.T) {
	t.Run("Find int", func(t *testing.T) {
		array := []int{1, 2, 3}
		predicate := func(i int) bool {
			return i > 1
		}
		got, found := Find(array, predicate)
		assert.True(t, found)
		assert.Equal(t, 2, got)
	})

	t.Run("Find string", func(t *testing.T) {
		array := []string{"a", "b", "c"}
		predicate := func(s string) bool {
			return s == "d"
		}
		got, found := Find(array, predicate)
		assert.False(t, found)
		assert.Equal(t, "", got)
	})

	t.Run("Find a value from a mixed-type array", func(t *testing.T) {
		array := []any{1, "test", 3.14, []int{1, 2, 3}}
		predicate := func(a any) bool {
			return a == "test"
		}
		got, found := Find(array, predicate)
		assert.True(t, found)
		assert.Equal(t, "test", got)
	})

}
