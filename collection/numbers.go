package collection

import "sort"

// Number is a constraint that permits any numeric type.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
	~float32 | ~float64
}

// Sum returns the sum of all numeric items.
func Sum[T Number](c Collection[T]) T {
	items := c.Items()
	var sum T
	for _, v := range items {
		sum += v
	}
	return sum
}

// Min returns the smallest numeric item.
// Second return is false if empty.
func Min[T Number](c Collection[T]) (T, bool) {
	items := c.Items()
	var zero T

	if len(items) == 0 {
		return zero, false
	}

	val := items[0]
	for _, v := range items[1:] {
		if v < val {
			val = v
		}
	}
	return val, true
}

// Max returns the largest numeric item.
// Second return is false if empty.
func Max[T Number](c Collection[T]) (T, bool) {
	items := c.Items()
	var zero T

	if len(items) == 0 {
		return zero, false
	}

	val := items[0]
	for _, v := range items[1:] {
		if v > val {
			val = v
		}
	}
	return val, true
}

// Avg returns the average as float64.
// Even integer averages may be fractional.
func Avg[T Number](c Collection[T]) float64 {
	items := c.Items()
	if len(items) == 0 {
		return 0
	}

	var sum float64
	for _, v := range items {
		sum += float64(v)
	}
	return sum / float64(len(items))
}

// Median returns the median as float64.
// Fractional medians handled correctly.
// False if empty.
func Median[T Number](c Collection[T]) (float64, bool) {
	items := c.Items()
	n := len(items)
	if n == 0 {
		return 0, false
	}

	cp := make([]T, n)
	copy(cp, items)

	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })

	mid := n / 2
	if n%2 == 1 {
		return float64(cp[mid]), true
	}

	a := float64(cp[mid-1])
	b := float64(cp[mid])
	return (a + b) / 2, true
}

// Mode returns the most frequent value(s).
// If tie, returns all values with max freq in first-seen order.
func Mode[T comparable](c Collection[T]) []T {
	items := c.Items()
	if len(items) == 0 {
		return nil
	}

	counts := make(map[T]int)
	order := make([]T, 0, len(items))
	maxCount := 0

	for _, v := range items {
		if _, exists := counts[v]; !exists {
			order = append(order, v)
		}
		counts[v]++

		if counts[v] > maxCount {
			maxCount = counts[v]
		}
	}

	result := make([]T, 0)
	for _, v := range order {
		if counts[v] == maxCount {
			result = append(result, v)
		}
	}

	return result
}
