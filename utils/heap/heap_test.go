package heap

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

type testItem struct {
	value    int
	priority int
}

func TestHeap(t *testing.T) {
	t.Run("New heap", func(t *testing.T) {
		h := NewMinHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		assert.Equal(t, 0, h.Len())
		_, ok := h.Peek()
		assert.False(t, ok)
	})

	t.Run("Push and Peek single item", func(t *testing.T) {
		h := NewMinHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		item := &testItem{value: 1, priority: 5}
		h.Push(item)
		assert.Equal(t, 1, h.Len())
		peek, ok := h.Peek()
		assert.True(t, ok)
		assert.Equal(t, item, peek)
	})

	t.Run("Push multiple items maintains heap property", func(t *testing.T) {
		h := NewMinHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		h.Push(&testItem{value: 1, priority: 5})
		h.Push(&testItem{value: 2, priority: 3})
		h.Push(&testItem{value: 3, priority: 7})
		peek, ok := h.Peek()
		assert.True(t, ok)
		assert.Equal(t, 2, peek.value) // Should be value 2 (priority 3)
		assert.Equal(t, 3, h.Len())
	})

	t.Run("Pop maintains heap property", func(t *testing.T) {
		h := NewMinHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		h.Push(&testItem{value: 1, priority: 5})
		h.Push(&testItem{value: 2, priority: 3})
		h.Push(&testItem{value: 3, priority: 7})

		item1, ok1 := h.Pop()
		assert.True(t, ok1)
		assert.Equal(t, 2, item1.value) // Priority 3

		item2, ok2 := h.Pop()
		assert.True(t, ok2)
		assert.Equal(t, 1, item2.value) // Priority 5

		item3, ok3 := h.Pop()
		assert.True(t, ok3)
		assert.Equal(t, 3, item3.value) // Priority 7

		assert.Equal(t, 0, h.Len())
	})

	t.Run("Remove item", func(t *testing.T) {
		h := NewMinHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		item1 := &testItem{value: 1, priority: 5}
		item2 := &testItem{value: 2, priority: 3}
		item3 := &testItem{value: 3, priority: 7}

		h.Push(item1)
		h.Push(item2)
		h.Push(item3)

		deletedItem, deleted := h.Remove(item1)
		assert.True(t, deleted)
		assert.Equal(t, item1, deletedItem)
		assert.Equal(t, 2, h.Len())

		peek, _ := h.Peek()
		assert.Equal(t, 2, peek.value) // Should still be the smallest priority
	})

	t.Run("Update item", func(t *testing.T) {
		h := NewMinHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		item1 := &testItem{value: 1, priority: 5}
		item2 := &testItem{value: 2, priority: 3}
		item3 := &testItem{value: 3, priority: 7}

		h.Push(item1)
		h.Push(item2)
		h.Push(item3)

		item2.priority = 8 // Make it larger than everything
		ok := h.Update(item2)
		assert.True(t, ok)

		peek, _ := h.Peek()
		assert.Equal(t, 1, peek.value) // Should now be item1 with priority 5
	})

	t.Run("Max heap", func(t *testing.T) {
		h := NewMaxHeap(func(a, b *testItem) bool {
			return a.priority < b.priority
		})
		h.Push(&testItem{value: 1, priority: 5})
		h.Push(&testItem{value: 2, priority: 3})
		h.Push(&testItem{value: 3, priority: 7})

		peek, _ := h.Peek()
		assert.Equal(t, 3, peek.value) // Should be value 3 (highest priority 7)
	})
}
