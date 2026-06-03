package cmd

import (
	"slices"
	"testing"

	"github.com/alecthomas/kong"
)

func TestSortedKeysReturnsAlphabeticalNamespaces(t *testing.T) {
	sections := map[string][]*kong.Node{
		"make":      nil,
		"migrate":   nil,
		"app":       nil,
		"auth":      nil,
		"benchmark": nil,
	}

	got := sortedKeys(sections)
	want := []string{"app", "auth", "benchmark", "make", "migrate"}
	if !slices.Equal(got, want) {
		t.Fatalf("sortedKeys() = %v, want %v", got, want)
	}
}

func TestCommandVisibleInHelp(t *testing.T) {
	tests := []struct {
		name           string
		node           *kong.Node
		maintainerMode bool
		want           bool
	}{
		{
			name: "regular command visible",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "build:api-index",
				Tag:  &kong.Tag{Hidden: false},
			},
			maintainerMode: false,
			want:           true,
		},
		{
			name: "hidden test command invisible by default",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "test:integration",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: false,
			want:           false,
		},
		{
			name: "hidden test command visible in maintainer mode",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "test:integration",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: true,
			want:           true,
		},
		{
			name: "hidden scenario command visible in maintainer mode",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "scenario:generate",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: true,
			want:           true,
		},
		{
			name: "hidden scenario command invisible by default",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "scenario:test",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: false,
			want:           false,
		},
		{
			name: "other hidden command remains hidden in maintainer mode",
			node: &kong.Node{
				Type: kong.CommandNode,
				Name: "build:binary",
				Tag:  &kong.Tag{Hidden: true},
			},
			maintainerMode: true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := commandVisibleInHelp(tt.node, tt.maintainerMode)
			if got != tt.want {
				t.Fatalf("commandVisibleInHelp() = %v, want %v", got, tt.want)
			}
		})
	}
}
