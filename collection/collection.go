package collection

import (
	"github.com/goforj/godump"
)

// Collection is a strongly-typed, fluent wrapper around a slice of T.
type Collection[T any] struct {
	items []T
}

// Number is a constraint that permits any numeric type.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
	~float32 | ~float64
}

// New wraps a slice in a Collection.
// A shallow copy is made so that further operations don't mutate the original slice.
func New[T any](items []T) Collection[T] {
	out := make([]T, len(items))
	copy(out, items)
	return Collection[T]{items: out}
}

// FromSlice is an alias for New for readability.
func FromSlice[T any](items []T) Collection[T] {
	return New(items)
}

// Items returns a copy of the underlying slice.
// This avoids callers mutating internal state accidentally.
func (c Collection[T]) Items() []T {
	out := make([]T, len(c.items))
	copy(out, c.items)
	return out
}

// IsEmpty returns true if the collection has no items.
func (c Collection[T]) IsEmpty() bool {
	return len(c.items) == 0
}

//
// ─── SAME-TYPE FLUENT OPERATIONS (METHODS) ─────────────────────────────────────
//

// Any returns true if at least one item satisfies fn.
func (c Collection[T]) Any(fn func(T) bool) bool {
	for _, v := range c.items {
		if fn(v) {
			return true
		}
	}
	return false
}

// All returns the underlying slice of items.
func (c Collection[T]) All() []T {
	out := make([]T, len(c.items))
	copy(out, c.items)
	return out
}

// Append returns a new collection with the given values appended.
func (c Collection[T]) Append(values ...T) Collection[T] {
	out := make([]T, 0, len(c.items)+len(values))
	out = append(out, c.items...)
	out = append(out, values...)
	return Collection[T]{items: out}
}

// Prepend returns a new collection with the given values prepended.
func (c Collection[T]) Prepend(values ...T) Collection[T] {
	out := make([]T, 0, len(c.items)+len(values))
	out = append(out, values...)
	out = append(out, c.items...)
	return Collection[T]{items: out}
}

//
// ─── TYPE-CHANGING OPERATIONS (FREE FUNCTIONS) ─────────────────────────────────
//

// MapTo maps a Collection[T] to a Collection[R] using fn(T) R.
//
// This cannot be a method because methods can't introduce a new type parameter R.
func MapTo[T any, R any](c Collection[T], fn func(T) R) Collection[R] {
	items := c.Items()
	out := make([]R, len(items))
	for i, v := range items {
		out[i] = fn(v)
	}
	return Collection[R]{items: out}
}

// Pluck is an alias for MapTo with a more semantic name when projecting fields.
func Pluck[T any, R any](c Collection[T], fn func(T) R) Collection[R] {
	return MapTo(c, fn)
}

// Reduce reduces a collection of T into a single value of type R.
//
// Example:
//   sum := Reduce(nums, 0, func(acc, n int) int { return acc + n })
func Reduce[T any, R any](c Collection[T], initial R, fn func(R, T) R) R {
	acc := initial
	for _, v := range c.Items() {
		acc = fn(acc, v)
	}
	return acc
}

// Before returns all items before the first element for which pred returns true.
// If no element matches, the entire collection is returned.
func (c Collection[T]) Before(pred func(T) bool) Collection[T] {
	idx := len(c.items)
	for i, v := range c.items {
		if pred(v) {
			idx = i
			break
		}
	}

	out := make([]T, idx)
	copy(out, c.items[:idx])
	return Collection[T]{items: out}
}

// After returns all items after the first element for which pred returns true.
// If no element matches, an empty collection is returned.
//
// Example:
//   c := collection.New([]int{1,2,3,4,5})
//   c.After(func(v int) bool { return v == 3 }) → [4,5]
func (c Collection[T]) After(pred func(T) bool) Collection[T] {
	idx := -1
	for i, v := range c.items {
		if pred(v) {
			idx = i
			break
		}
	}

	// If no match found → empty collection
	if idx == -1 || idx+1 >= len(c.items) {
		return Collection[T]{items: []T{}}
	}

	out := make([]T, len(c.items)-(idx+1))
	copy(out, c.items[idx+1:])
	return Collection[T]{items: out}
}

// AvgBy calculates the average of values extracted by fn from the collection items.
//
// Example:
//   avgAge := AvgBy(users, func(u User) float64 { return float64(u.Age) })
func AvgBy[T any](c Collection[T], fn func(T) float64) float64 {
	items := c.Items()

	if len(items) == 0 {
		return 0
	}

	var sum float64
	for _, v := range items {
		sum += fn(v)
	}

	return sum / float64(len(items))
}

// SumBy returns the sum of a numeric projection from each item.
//
// Example (structs):
//   type Row struct{ Foo int }
//   rows := New([]Row{{10}, {20}})
//   total := SumBy(rows, func(r Row) int { return r.Foo }) // 30
func SumBy[T any, N Number](c Collection[T], fn func(T) N) N {
	items := c.Items()
	var sum N
	for _, v := range items {
		sum += fn(v)
	}
	return sum
}

// Contains returns true if any item satisfies the predicate.
func (c Collection[T]) Contains(pred func(T) bool) bool {
	for _, v := range c.items {
		if pred(v) {
			return true
		}
	}
	return false
}

// Count returns the total number of items in the collection.
func (c Collection[T]) Count() int {
	return len(c.items)
}

// CountBy returns a map of keys extracted by fn to their occurrence counts.
// K must be comparable.
// Example:
//   counts := CountBy(users, func(u User) string { return u.Role })
//  // counts == map[string]int{"admin": 3, "user": 5}
func CountBy[T any, K comparable](c Collection[T], fn func(T) K) map[K]int {
	items := c.Items()
	result := make(map[K]int, len(items))

	for _, v := range items {
		key := fn(v)
		result[key]++
	}

	return result
}

// CountByValue returns a map of item values to their occurrence counts.
// T must be comparable.
// Example:
//   counts := CountByValue(collection.New([]string{"a", "b", "a"}))
//  // counts == map[string]int{"a": 2, "b": 1}
func CountByValue[T comparable](c Collection[T]) map[T]int {
	items := c.Items()
	result := make(map[T]int, len(items))

	for _, v := range items {
		result[v]++
	}

	return result
}

// Dump pretty-prints the collection contents using goforj/godump
// and returns the collection so it can be used mid-chain.
//
// Example:
//   users.
//     Filter(func(u User) bool { return u.Age >= 35 }).
//     Dump().
//     Sort(func(a, b User) bool { return a.Age < b.Age })
func (c Collection[T]) Dump() Collection[T] {
	godump.Dump(c.Items()) // or c.items if you don't care about copying
	return c
}

// Dd pretty-prints the collection contents using goforj/godump
// and then exits the program (just like Laravel's dd()).
//
// Example:
//   users.
//     Filter(func(u User) bool { return u.Age >= 35 }).
//     Dd()
func (c Collection[T]) Dd() {
	godump.Dd(c.Items())
}
