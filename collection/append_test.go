package collection

import (
	"reflect"
	"testing"
)

func TestAppend_Basic(t *testing.T) {
	c := New([]int{1, 2})

	out := c.Append(3, 4)

	expected := []int{1, 2, 3, 4}
	if !reflect.DeepEqual(out.items, expected) {
		t.Fatalf("expected %v, got %v", expected, out.items)
	}
}

func TestAppend_EmptyCollection(t *testing.T) {
	c := New([]int{})

	out := c.Append(5, 6)

	expected := []int{5, 6}
	if !reflect.DeepEqual(out.items, expected) {
		t.Fatalf("expected %v, got %v", expected, out.items)
	}
}

func TestAppend_NoValues(t *testing.T) {
	c := New([]int{10, 20, 30})

	out := c.Append() // no-op

	expected := []int{10, 20, 30}
	if !reflect.DeepEqual(out.items, expected) {
		t.Fatalf("expected %v, got %v", expected, out.items)
	}
}

func TestAppend_Structs(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	c := New([]User{
		{1, "Chris"},
		{2, "Van"},
	})

	out := c.Append(
		User{3, "Shawn"},
		User{4, "Matt"},
	)

	expected := []User{
		{1, "Chris"},
		{2, "Van"},
		{3, "Shawn"},
		{4, "Matt"},
	}

	if !reflect.DeepEqual(out.items, expected) {
		t.Fatalf("expected %v, got %v", expected, out.items)
	}
}

func TestAppend_NoMutation(t *testing.T) {
	orig := []int{1, 2, 3}
	c := New(orig)

	out := c.Append(4, 5)

	// original must be unchanged
	if !reflect.DeepEqual(c.items, orig) {
		t.Fatalf("Append mutated the original collection: %v", c.items)
	}

	// appended version must be correct
	expected := []int{1, 2, 3, 4, 5}
	if !reflect.DeepEqual(out.items, expected) {
		t.Fatalf("expected %v, got %v", expected, out.items)
	}
}
