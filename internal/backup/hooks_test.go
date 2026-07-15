package backup

import (
	"context"
	"strings"
	"testing"
)

// TestHookRegistryRejectsUnknownEvents verifies an invalid lifecycle value cannot accidentally run after-create hooks.
func TestHookRegistryRejectsUnknownEvents(t *testing.T) {
	called := false
	registry := HookRegistry{AfterCreate: []Hook{func(context.Context, HookEvent) error {
		called = true
		return nil
	}}}

	err := registry.Run(context.Background(), HookEvent("unknown"))
	if err == nil || !strings.Contains(err.Error(), "unsupported backup hook event") {
		t.Fatalf("HookRegistry.Run error = %v, want unknown-event failure", err)
	}
	if called {
		t.Fatal("unknown event ran after-create hooks")
	}
}
