package scenarios

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadEmbeddedScenarioSpecs(t *testing.T) {
	specs, err := loadScenarioSpecs("")
	if err != nil {
		t.Fatalf("load specs: %v", err)
	}
	if len(specs) == 0 {
		t.Fatal("expected embedded specs")
	}
	found := false
	for _, spec := range specs {
		if spec.ID == "json-api-route" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected json-api-route spec")
	}
}

func TestRenderScenarioMarkdownIncludesVerificationBanner(t *testing.T) {
	specs, err := selectedScenarioSpecs("", []string{"json-api-route"}, false)
	if err != nil {
		t.Fatalf("select spec: %v", err)
	}
	body := renderScenarioMarkdown(specs[0])
	for _, token := range []string{
		"::: info Verified Scenario",
		"forj make:controller users",
		"forj build",
		"/api/v1/users/:id",
	} {
		if !strings.Contains(body, token) {
			t.Fatalf("generated markdown missing %q\n%s", token, body)
		}
	}
}

func TestScenarioGenerateCheckDetectsDrift(t *testing.T) {
	dir := t.TempDir()
	if err := Generate(GenerateOptions{OutDir: dir, IDs: []string{"json-api-route"}}); err != nil {
		t.Fatalf("generate: %v", err)
	}

	path := filepath.Join(dir, "json-api-route.md")
	if err := os.WriteFile(path, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale doc: %v", err)
	}

	if err := Generate(GenerateOptions{OutDir: dir, Check: true, IDs: []string{"json-api-route"}}); err == nil {
		t.Fatal("expected check to detect drift")
	}
}
