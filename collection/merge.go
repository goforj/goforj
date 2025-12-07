package collection

import (
	"strconv"
)

/*
Merge merges the given data into the current collection using
Laravel-style semantics.

Behavior depends on the type of `other`:

  • []T (numeric merges)
      Values are appended to the end of the collection.

      Example:
          c := New([]string{"Desk", "Chair"})
          out := c.Merge([]string{"Bookcase", "Door"})
          // ["Desk", "Chair", "Bookcase", "Door"]

  • Collection[T]
      Equivalent to merging a slice: values are appended.

  • map[string]T (associative merges)
      Keys that already exist overwrite the original values.
      Non-existing keys are added.

      Example:
          c := New([]Product{{ID: 1, Price: 100}})
          out := c.Merge(map[string]Product{
              "0":        {ID: 1, Price: 200},
              "discount": {ID: 0, Price: 0},
          })

      // Similar to Laravel:
      // ['product_id' => 1, 'price' => 200, 'discount' => false]

Unsupported merge types will cause a panic.

This method is non-mutating: a new Collection is returned.
*/
func (c Collection[T]) Merge(other any) Collection[T] {
	switch v := other.(type) {

	case []T:
		// Numeric keys → append
		return c.mergeSlice(v)

	case Collection[T]:
		// Merging another collection → treat like slice
		return c.mergeSlice(v.items)

	case map[string]T:
		// Associative keys → overwrite existing keys
		return c.mergeMap(v)

	default:
		panic("collection.Merge: unsupported type")
	}
}

/*
mergeSlice handles Laravel-style numeric merges.

Given a slice ([]T), values are appended to the end of the current items.

This function does not mutate the original collection; it returns a new one.
*/
func (c Collection[T]) mergeSlice(values []T) Collection[T] {
	out := make([]T, len(c.items))
	copy(out, c.items)
	out = append(out, values...)
	return New(out)
}

/*
mergeMap handles Laravel-style associative merges.

Steps:

  1. Convert the current slice items into a temporary map[string]T
     using numeric string keys ("0", "1", ...).

  2. Apply associative merge rules:
        - If a key exists, overwrite it.
        - If a key does not exist, add it.

  3. Convert the map back into a slice.
     Since Go maps do not guarantee order, the resulting slice
     does not have deterministic ordering — which mirrors Laravel's
     associative behavior where key order is not guaranteed.

This function returns a new collection.
*/
func (c Collection[T]) mergeMap(values map[string]T) Collection[T] {
	// Convert slice to temporary map with numeric string keys
	tmp := make(map[string]T)

	for i, v := range c.items {
		tmp[strconv.Itoa(i)] = v
	}

	// Laravel: overwrite matching keys + add new keys
	for k, v := range values {
		tmp[k] = v
	}

	// Convert map back to slice — order not guaranteed
	out := make([]T, 0, len(tmp))
	for _, v := range tmp {
		out = append(out, v)
	}

	return New(out)
}
