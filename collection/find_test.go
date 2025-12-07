package collection

import (
	"reflect"
	"testing"
)

func TestFind_FindsMatchingValue(t *testing.T) {
	c := New([]int{1, 2, 3, 4, 5})

	v, ok := c.Find(func(n int) bool { return n%2 == 0 })

	if !ok {
		t.Fatalf("Find should return ok=true when match exists")
	}

	if v != 2 {
		t.Fatalf("Find returned wrong value. expected 2, got %v", v)
	}
}

func TestFind_NoMatchReturnsZeroValue(t *testing.T) {
	c := New([]int{1, 3, 5})

	v, ok := c.Find(func(n int) bool { return n%2 == 0 })

	if ok {
		t.Fatalf("Find should return ok=false when no match exists")
	}

	if v != 0 {
		t.Fatalf("Find should return zero-value when no match exists; got %v", v)
	}
}

func TestFind_AliasToFirst(t *testing.T) {
	c := New([]int{1, 2, 3, 4, 5})

	v1, ok1 := c.First(func(n int) bool { return n > 3 })
	v2, ok2 := c.Find(func(n int) bool { return n > 3 })

	if v1 != v2 || ok1 != ok2 {
		t.Fatalf("Find should behave exactly the same as First(fn)")
	}
}

func TestFind_WorksWithStructs(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	c := New([]User{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
		{ID: 3, Name: "C"},
	})

	expected := User{ID: 2, Name: "B"}

	v, ok := c.Find(func(u User) bool {
		return u.ID == 2
	})

	if !ok {
		t.Fatalf("Find should locate struct matching predicate")
	}

	if !reflect.DeepEqual(v, expected) {
		t.Fatalf("Find returned wrong struct. expected %#v, got %#v", expected, v)
	}
}

func TestFind_NoStructMatch(t *testing.T) {
	type User struct {
		ID   int
		Name string
	}

	c := New([]User{
		{ID: 1, Name: "A"},
		{ID: 2, Name: "B"},
	})

	v, ok := c.Find(func(u User) bool {
		return u.ID == 999
	})

	if ok {
		t.Fatalf("Find should return ok=false when no struct matches")
	}

	if (v != User{}) {
		t.Fatalf("Find should return zero-value struct when no match")
	}
}
