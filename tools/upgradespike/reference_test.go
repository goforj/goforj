package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestMain lets a copied Go test executable act as the renderer in reference protocol tests.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "render" {
		executable, err := os.Executable()
		if err != nil {
			os.Exit(4)
		}
		mode, _ := os.ReadFile(executable + ".mode")
		switch string(mode) {
		case "dependency":
			_ = os.WriteFile("node_modules/tracked.js", []byte("changed"), 0o644)
		case "bytes":
			_ = os.WriteFile("public.go", []byte("changed"), 0o644)
		case "mode":
			_ = os.Chmod("public.go", 0o700)
		case "fail":
			os.Exit(9)
		case "invalid":
			fmt.Print("invalid JSON")
			os.Exit(0)
		case "empty-object", "null", "missing-files", "null-files", "wrong-scope":
			responses := map[string]string{"empty-object": `{}`, "null": `null`, "missing-files": `{"scope":"framework"}`, "null-files": `{"scope":"framework","files":null}`, "wrong-scope": `{"scope":"other","files":[]}`}
			fmt.Print(responses[string(mode)])
			os.Exit(0)
		}
		fmt.Print(`{"scope":"framework","files":[{"ownership":"framework-template","action":"unchanged"}]}`)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// referenceFixture creates a committed application and renderer without shell fixtures.
func referenceFixture(t *testing.T) (string, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("reference workspaces intentionally use /tmp")
	}
	source, err := os.MkdirTemp("/tmp", "goforj-reference-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(source) })
	for name, value := range map[string]string{"public.go": "original", ".ENV.production": "private", "nested/.env.testing": "private", "nested/go.mod": "module nested", "node_modules/tracked.js": "tracked"} {
		target := filepath.Join(source, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(".ENV.production", filepath.Join(source, "private-link")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"git", "init", "-q"}, {"git", "add", "-f", "--", "."}, {"git", "-c", "user.name=Upgrade Test", "-c", "user.email=upgrade@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "chore: fixture"}} {
		if code, err := runCommand(context.Background(), source, args, trialEnvironment(), os.Stderr, os.Stderr); err != nil || code != 0 {
			t.Fatalf("git fixture: %d %v", code, err)
		}
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	renderer := filepath.Join(t.TempDir(), "renderer")
	if err := copyFile(executable, renderer, 0o700); err != nil {
		t.Fatal(err)
	}
	return source, renderer
}

// TestReference verifies read-only plans and rejects byte changes, mode changes, and invalid renderer output.
func TestReference(t *testing.T) {
	source, renderer := referenceFixture(t)
	for _, mode := range []string{"", "bytes", "mode", "dependency", "fail", "invalid", "empty-object", "null", "missing-files", "null-files", "wrong-scope"} {
		t.Run(mode, func(t *testing.T) {
			if err := os.WriteFile(renderer+".mode", []byte(mode), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := runReference(context.Background(), renderer, []string{source}, renderer)
			if mode != "" {
				if err == nil {
					t.Fatalf("accepted %s mutation/failure", mode)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			summary := result.(map[string]any)
			workspace := summary["workspace"].(string)
			t.Cleanup(func() { _ = os.RemoveAll(workspace) })
			files, err := inventory(filepath.Join(workspace, "0-"+filepath.Base(source)))
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != 2 || files["nested/go.mod"].SHA256 == "" {
				t.Fatalf("unexpected export: %+v", files)
			}
			if !strings.HasPrefix(workspace, "/tmp/") {
				t.Fatalf("unsafe workspace: %s", workspace)
			}
			if _, err := os.Stat(filepath.Join(workspace, "summary.json")); err != nil {
				t.Fatal(err)
			}
		})
	}
	if _, err := runReference(context.Background(), renderer, []string{filepath.Join(source, "missing")}, ""); err == nil {
		t.Fatal("missing project accepted")
	}
}
