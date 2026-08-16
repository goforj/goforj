package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvSetCmdReadsOneHiddenValue verifies the command does not accept or echo secret material through its argument surface.
func TestEnvSetCmdReadsOneHiddenValue(t *testing.T) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("CUSTOM_TOKEN=\n"), 0o644); err != nil {
		t.Fatalf("write example: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	calls := 0
	cmd := EnvSetCmd{Key: "CUSTOM_TOKEN", readSecret: func(prompt string) (string, error) {
		calls++
		if prompt != "Value for CUSTOM_TOKEN" {
			t.Fatalf("prompt = %q", prompt)
		}
		return "private-value", nil
	}}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("hidden input calls = %d, want 1", calls)
	}
	local, err := os.ReadFile(filepath.Join(root, ".env"))
	if err != nil || !strings.Contains(string(local), "CUSTOM_TOKEN=private-value") {
		t.Fatalf("local environment = %q, %v", local, err)
	}
	example, err := os.ReadFile(filepath.Join(root, ".env.example"))
	if err != nil || strings.Contains(string(example), "private-value") {
		t.Fatalf("safe example exposed local value: %q, %v", example, err)
	}
}

// TestEnvSetCmdRejectsInvalidAndEmptyInput verifies each validation failure stops before local mutation.
func TestEnvSetCmdRejectsInvalidAndEmptyInput(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "invalid key", key: "BAD-KEY", value: "secret"},
		{name: "empty value", key: "GOOD_KEY", value: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original, err := os.Getwd()
			if err != nil {
				t.Fatalf("get working directory: %v", err)
			}
			t.Cleanup(func() { _ = os.Chdir(original) })
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, ".env.example"), []byte("GOOD_KEY=\n"), 0o644); err != nil {
				t.Fatalf("write example: %v", err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatalf("change working directory: %v", err)
			}
			cmd := EnvSetCmd{Key: test.key, readSecret: func(string) (string, error) { return test.value, nil }}
			if err := cmd.Run(); err == nil {
				t.Fatal("Run() unexpectedly succeeded")
			}
		})
	}
}
