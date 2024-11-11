package heap

type MinHeap[T any] struct {
    items []T
    less  func(a, b T) bool
}

func NewMinHeap[T any](less func(a T, b T) bool) *MinHeap[T] {
    return &MinHeap[T]{
        items: make([]T, 0),
        less:  less,
    }
}

func NewMaxHeap[T any](less func(a T, b T) bool) *MinHeap[T] {
	return &MinHeap[T]{
		items: make([]T, 0),
		less:  func(a T, b T) bool { return less(b, a) },
	}
}

func (h *MinHeap[T]) Len() int { return len(h.items) }

func (h *MinHeap[T]) Push(item T) {
    h.items = append(h.items, item)
    h.siftUp(len(h.items) - 1)
}

func (h *MinHeap[T]) Pop() (T, bool) {
    var zero T
    if len(h.items) == 0 {
        return zero, false
    }

    min := h.items[0]
    last := len(h.items) - 1
    h.items[0] = h.items[last]
    h.items = h.items[:last]
    if last > 0 {
        h.siftDown(0)
    }
    return min, true
}

func (h *MinHeap[T]) Peek() (T, bool) {
    var zero T
    if len(h.items) == 0 {
        return zero, false
    }
    return h.items[0], true
}

func (h *MinHeap[T]) Remove(item T) (T, bool) {
    var zero T
    index := h.indexOf(item)
    if index < 0 || index >= len(h.items) {
        return zero, false
    }
    
    removed := h.items[index]
    last := len(h.items) - 1
    if index != last {
        h.items[index] = h.items[last]
        h.items = h.items[:last]
        if index > 0 && h.less(h.items[index], h.items[parent(index)]) {
            h.siftUp(index)
        } else {
            h.siftDown(index)
        }
    } else {
        h.items = h.items[:last]
    }
    return removed, true
}

func (h *MinHeap[T]) Update(item T) bool {
    index := h.indexOf(item)

    if index < 0 || index >= len(h.items) {
        return false
    }
    
    h.items[index] = item
    if index > 0 && h.less(item, h.items[parent(index)]) {
        h.siftUp(index)
    } else {
        h.siftDown(index)
    }
    return true
}

func (h *MinHeap[T]) indexOf(item T) int {
    for i, v := range h.items {
        if !h.less(v, item) && !h.less(item, v) {
            return i
        }
    }
    return -1
}

func (h *MinHeap[T]) siftUp(index int) {
    for index > 0 {
        p := parent(index)
        if !h.less(h.items[index], h.items[p]) {
            break
        }
        h.items[index], h.items[p] = h.items[p], h.items[index]
        index = p
    }
}

func (h *MinHeap[T]) siftDown(index int) {
    for {
        smallest := index
        l, r := leftChild(index), rightChild(index)
        
        if l < len(h.items) && h.less(h.items[l], h.items[smallest]) {
            smallest = l
        }
        if r < len(h.items) && h.less(h.items[r], h.items[smallest]) {
            smallest = r
        }
        
        if smallest == index {
            break
        }
        
        h.items[index], h.items[smallest] = h.items[smallest], h.items[index]
        index = smallest
    }
}

func parent(i int) int     { return (i - 1) / 2 }
func leftChild(i int) int  { return 2*i + 1 }
func rightChild(i int) int { return 2*i + 2 }