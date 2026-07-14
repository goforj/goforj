package forj

import (
	"strings"
	"testing"
)

// TestEnvDefaultsHelperStartsInFixtureDirectory prevents generated package initialization from loading the project environment before the helper scenario runs.
func TestEnvDefaultsHelperStartsInFixtureDirectory(t *testing.T) {
	content, err := templatesFS.ReadFile("internal/cmd/env_defaults_test.go.tmpl")
	if err != nil {
		t.Fatalf("read env defaults test template: %v", err)
	}
	if !strings.Contains(string(content), "cmd.Dir = dir") {
		t.Fatal("env defaults helper must start in its isolated fixture directory")
	}
	if !strings.Contains(string(content), `writeTestEnvFile(t, filepath.Join(dir, ".env"), "\n")`) {
		t.Fatal("APP_ENV defaults fixture must stop ancestor .env discovery")
	}
}
