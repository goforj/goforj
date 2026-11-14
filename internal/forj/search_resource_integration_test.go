//go:build integration

package forj

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestSearchResourceFullComposition renders the complete supported component envelope plus Search and mixed App participation.
func TestSearchResourceFullComposition(t *testing.T) {
	root := t.TempDir()
	repository, err := testkit.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	components := project.DefaultSelectedComponents()
	components.DatabaseMySQL = false
	components.DatabaseSQLite = true
	components.Search = true
	components.DemoApp = true
	config := project.Config{ProjectName: "Search composition", GoModuleName: "example.com/searchcomposition", Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{
		"catalog": {Components: project.Components{CLI: true, WebAPI: true, Search: true}},
		"plain":   {Components: project.Components{CLI: true, WebAPI: true}},
	}}
	for _, name := range []string{"catalog", "plain"} {
		if err := writeConventionalAppMarker(root, name); err != nil {
			t.Fatal(err)
		}
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: config, ModuleReplaces: map[string]string{"github.com/goforj/goforj": repository}})
	forj := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	for _, arguments := range [][]string{{"go", "test", "./...", "-run", "^$"}, {"go", "test", "./app/wire", "./app/catalog/wire", "-run", "^TestSearchShutdownReturnsBackendFailure$", "-count=1"}, {forj, "catalog", "build"}, {forj, "plain", "build"}} {
		command := exec.CommandContext(t.Context(), arguments[0], arguments[1:]...)
		command.Dir = root
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", arguments, err, output)
		}
	}
	for _, path := range []string{"app/search.go", "app/catalog/search.go", "internal/searches/manager_gen.go", "internal/searches/accessors_gen.go"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "app/plain/search.go")); !os.IsNotExist(err) {
		t.Fatal("disabled App got Search schema code")
	}
	plain, err := os.ReadFile(filepath.Join(root, "app/plain/wire/app.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "searches.Manager") {
		t.Fatal("disabled App got Search dependency")
	}

	schema := "package app\n\nimport \"github.com/goforj/search\"\n\n// SearchDefinitions owns this App's catalog projection.\nfunc SearchDefinitions() map[string]search.Definition { return map[string]search.Definition{\"catalog\": {Name: \"catalog\", Fields: []search.Field{{Name: \"description\", Type: search.TextField, Searchable: true}}}} }\n"
	if err := os.WriteFile(filepath.Join(root, "app/search.go"), []byte(schema), 0600); err != nil {
		t.Fatal(err)
	}
	bootstrap := strings.ReplaceAll(searchProvisionBootstrap, "your/module", config.GoModuleName)
	documentation, err := os.ReadFile(filepath.Join(root, "internal/searches/README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(documentation), strings.TrimSpace(bootstrap)) {
		t.Fatal("documented bootstrap differs from executed fixture")
	}
	bootstrapDir := filepath.Join(root, "cmd/search-provision")
	if err := os.MkdirAll(bootstrapDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bootstrapDir, "main.go"), []byte(bootstrap), 0600); err != nil {
		t.Fatal(err)
	}
	provision := exec.CommandContext(t.Context(), "go", "run", "./cmd/search-provision")
	provision.Dir = root
	provision.Env = environment
	if output, err := provision.CombinedOutput(); err != nil || !strings.Contains(string(output), "Search schemas provisioned") {
		t.Fatalf("standalone provisioning: %v\n%s", err, output)
	}
	before := map[string][]byte{}
	for _, name := range []string{"manager_gen.go", "accessors_gen.go"} {
		path := filepath.Join(root, "internal/searches", name)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		before[path] = body
	}
	for attempt := 0; attempt < 2; attempt++ {
		command := exec.CommandContext(t.Context(), forj, "generate", "--search")
		command.Dir = root
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("regenerate: %v\n%s", err, output)
		}
	}
	for path, want := range before {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("generation changed %s: %v", path, err)
		}
	}
}

const searchProvisionBootstrap = `package main

import (
 "context"
 "fmt"
 "log"
 "time"

 "github.com/goforj/env/v2"
 "your/module/app"
 "your/module/internal/searches"
)

// main provisions declared schemas without initializing the App runtime or its index manager.
func main() {
 if err := env.Load(); err != nil { log.Fatal(err) }
 ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
 defer cancel()
 if err := searches.Provision(ctx, app.SearchDefinitions()); err != nil { log.Fatal(err) }
 fmt.Println("Search schemas provisioned")
}
`
