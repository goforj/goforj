package collection

import (
	"strconv"
)

func (c Collection[T]) Merge(other any) Collection[T] {
	switch v := other.(type) {

	// Numeric keys → append
	case []T:
		return c.mergeSlice(v)

	case Collection[T]:
		return c.mergeSlice(v.items)

	// Associative keys → overwrite
	case map[string]T:
		return c.mergeMap(v)

	default:
		panic("collection.Merge: unsupported type")
	}
}

func (c Collection[T]) mergeSlice(values []T) Collection[T] {
	out := make([]T, len(c.items))
	copy(out, c.items)
	out = append(out, values...)
	return New(out)
}

func (c Collection[T]) mergeMap(values map[string]T) Collection[T] {
	// Convert slice to temporary map with numeric string keys
	tmp := make(map[string]T)

	for i, v := range c.items {
		tmp[strconv.Itoa(i)] = v
	}

	// Laravel: overwrite matching keys
	for k, v := range values {
		tmp[k] = v
	}

	// Convert map back to slice — Laravel does not guarantee order for associative arrays
	out := make([]T, 0, len(tmp))
	for _, v := range tmp {
		out = append(out, v)
	}

	return New(out)
}
