package collection

// First returns the first item for which fn returns true.
// If none match, ok=false and the zero value of T is returned.
// Example usage:
//
//	c := collection.New([]int{1, 2, 3, 4, 5})
//	value, ok := c.First(func(v int) bool { return v%2 == 0 }) // finds first even number
//
//	// value: 2, ok: true
func (c Collection[T]) First(fn func(T) bool) (value T, ok bool) {
	for _, v := range c.items {
		if fn(v) {
			return v, true
		}
	}
	var zero T
	return zero, false
}
