package array

func Map[T1, T2 any](array []T1, mapper func(T1) T2) []T2 {
	result := make([]T2, len(array))
	for i, elem := range array {
		result[i] = mapper(elem)
	}
	return result
}

func Contains[T comparable](array []T, target T) bool {
	for _, elem := range array {
		if elem == target {
			return true
		}
	}
	return false
}

// Returns the first element in the array that satisfies the predicate.
// If no element satisfies the predicate, the second return value is false.
func Find[T any](array []T, predicate func(T) bool) (T, bool) {
	for _, elem := range array {
		if predicate(elem) {
			return elem, true
		}
	}
	var zero T
	return zero, false
}
