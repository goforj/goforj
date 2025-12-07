package collection_test

import (
	"github.com/goforj/godump"
	"testing"

	"github.com/goforj/goforj/collection"
)

type User struct {
	ID   int
	Name string
	Age  int
}

func TestFluentChainWithStructs(t *testing.T) {
	users := collection.New([]User{
		{1, "Chris", 34},
		{2, "Van", 42},
		{3, "Shawn", 39},
	})

	// Fluent chain across SAME type:
	filteredAndSorted := users.
		Filter(func(u User) bool { return u.Age >= 35 }).
		Sort(func(a, b User) bool { return a.Age < b.Age })

	// Type change happens at the edge using MapTo/Pluck.
	names := collection.Pluck(filteredAndSorted, func(u User) string {
		return u.Name
	}).Items()

	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d (%v)", len(names), names)
	}
	if names[0] != "Shawn" || names[1] != "Van" {
		t.Fatalf("unexpected order: %#v", names)
	}
}

func TestReduceInts(t *testing.T) {
	nums := collection.New([]int{1, 2, 3, 4})

	sum := collection.Reduce(nums, 0, func(acc, n int) int {
		return acc + n
	})

	if sum != 10 {
		t.Fatalf("expected 10, got %d", sum)
	}
}

func TestUnique(t *testing.T) {
	nums := collection.New([]int{1, 2, 2, 3, 3, 3, 4})

	unique := nums.Unique(func(a, b int) bool { return a == b }).Items()

	if len(unique) != 4 {
		t.Fatalf("expected 4 unique items, got %d (%v)", len(unique), unique)
	}
}

func TestFluentChainWithStructsDump(t *testing.T) {
	users := collection.New([]User{
		{1, "Chris", 34},
		{2, "Van", 42},
		{3, "Shawn", 39},
	})

	// Fluent chain across SAME type:
	list := users.
		Filter(func(u User) bool { return u.Age >= 35 }).
		Sort(func(a, b User) bool { return a.Age < b.Age })

	godump.Dump(list.All())
}
