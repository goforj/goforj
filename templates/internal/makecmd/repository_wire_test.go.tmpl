package makecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateRepositorySetAddsRepo(t *testing.T) {
	src := `package wire

import "github.com/goforj/wire"

var repositorySet = wire.NewSet()
`
	dir := t.TempDir()
	path := filepath.Join(dir, "inject_repositories_app.go")
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	changed, err := updateRepositorySet(path, "example.com/test/internal/models", "models", "NewUserRepo")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected repositorySet to be updated")
	}

	changed, err = updateRepositorySet(path, "example.com/test/internal/models", "models", "NewUserRepo")
	if err != nil {
		t.Fatalf("second update failed: %v", err)
	}
	if changed {
		t.Fatalf("expected idempotent update to be a no-op")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if string(contents) == src {
		t.Fatalf("expected repositorySet to be updated")
	}
	if !containsRepoCtor(string(contents), "models.NewUserRepo") {
		t.Fatalf("expected models.NewUserRepo to be present")
	}
	if !strings.Contains(string(contents), "repositorySetPlaceholder") {
		t.Fatalf("expected placeholder to be present")
	}
}

func TestUpdateRepositorySetCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wire", "inject_repositories_app.go")

	changed, err := updateRepositorySet(path, "example.com/test/internal/models", "models", "NewUserRepo")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected repositorySet to be created")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !containsRepoCtor(string(contents), "models.NewUserRepo") {
		t.Fatalf("expected models.NewUserRepo to be present")
	}
	if !strings.Contains(string(contents), "repositorySetPlaceholder") {
		t.Fatalf("expected placeholder to be present")
	}
}

func containsRepoCtor(src, ctor string) bool {
	return strings.Contains(src, ctor)
}
