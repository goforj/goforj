package cmd

import (
	"testing"

	"github.com/alecthomas/kong"
)

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
