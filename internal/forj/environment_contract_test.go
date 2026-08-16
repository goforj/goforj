package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteEnvironmentExampleAtomic verifies publication redacts first and preserves an owner-selected file mode.
func TestWriteEnvironmentExampleAtomic(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env.example")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatalf("write existing example: %v", err)
	}
	source := []byte("APP_KEY=generated\nCACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n")
	if err := WriteEnvironmentExampleAtomic(path, source, 0o644); err != nil {
		t.Fatalf("WriteEnvironmentExampleAtomic() error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example: %v", err)
	}
	want := "APP_KEY=\nCACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\n"
	if string(data) != want {
		t.Fatalf("example = %q, want %q", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat example: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("example mode = %o, want 600", got)
	}
	matches, err := filepath.Glob(filepath.Join(root, ".env.example-*"))
	if err != nil {
		t.Fatalf("glob replacement files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("atomic replacement left temporary files: %v", matches)
	}
}

// TestEnsureGitignoreEnvironmentRulesPreservesOwnerEntries verifies rerender adds the safe contract without replacing custom ignores.
func TestEnsureGitignoreEnvironmentRulesPreservesOwnerEntries(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(path, []byte("vendor/\n.env\n# owner rule\ncustom.tmp\n"), 0o600); err != nil {
		t.Fatalf("write gitignore: %v", err)
	}
	if err := ensureGitignoreEnvironmentRules(path); err != nil {
		t.Fatalf("ensure environment rules: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read gitignore: %v", err)
	}
	text := string(data)
	for _, want := range []string{"vendor/", "# owner rule", "custom.tmp", ".env.host", ".env.local", "!.env.example", "!.env.testing"} {
		if !strings.Contains(text, want) {
			t.Fatalf("updated gitignore omitted %q:\n%s", want, text)
		}
	}
	if strings.Count(text, ".env\n") != 1 {
		t.Fatalf("existing .env rule was duplicated:\n%s", text)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("gitignore mode changed: info=%v err=%v", info, err)
	}
}
