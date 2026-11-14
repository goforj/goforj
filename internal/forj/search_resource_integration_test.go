//go:build integration

package forj

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	components.Feature = true
	components.Notification = true
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
	bootstrapDir := filepath.Join(root, "tools/search-provision")
	if err := os.MkdirAll(bootstrapDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bootstrapDir, "main.go"), []byte(bootstrap), 0600); err != nil {
		t.Fatal(err)
	}
	provision := exec.CommandContext(t.Context(), "go", "run", "./tools/search-provision")
	provision.Dir = root
	provision.Env = environment
	if output, err := provision.CombinedOutput(); err != nil || !strings.Contains(string(output), "Search schemas provisioned") {
		t.Fatalf("standalone provisioning: %v\n%s", err, output)
	}
	before := map[string][]byte{}
	for attempt := 0; attempt < 2; attempt++ {
		command := exec.CommandContext(t.Context(), forj, "generate", "--search")
		command.Dir = root
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("regenerate: %v\n%s", err, output)
		}
		if attempt == 0 {
			// Capture the configured inventory before requiring the next generation to remain byte stable.
			for _, name := range []string{"manager_gen.go", "accessors_gen.go"} {
				path := filepath.Join(root, "internal/searches", name)
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				before[path] = body
			}
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

// TestSearchResourceRuntimeIsolation initializes both real App graphs with disjoint schemas and backend owners.
func TestSearchResourceRuntimeIsolation(t *testing.T) {
	root := t.TempDir()
	repository, err := testkit.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	config := project.Config{ProjectName: "Scoped Search", GoModuleName: "example.com/scopedsearch", Render: project.RenderConfig{Components: project.Components{CLI: true, Search: true}}, Apps: map[string]project.AppConfig{"admin": {Components: project.Components{CLI: true, Search: true}}}}
	if err := writeConventionalAppMarker(root, "admin"); err != nil {
		t.Fatal(err)
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: config, ModuleReplaces: map[string]string{"github.com/goforj/goforj": repository}})
	schema := "import \"github.com/goforj/search\"\n\n// SearchDefinitions owns this App's explicit schema.\nfunc SearchDefinitions() map[string]search.Definition { return map[string]search.Definition{NAME: {Name: NAME, Fields: []search.Field{{Name: \"description\", Type: search.TextField, Searchable: true}}}} }\n"
	for name, path := range map[string]string{"catalog": "app/search.go", "archive": "app/admin/search.go"} {
		pkg := "app"
		if name == "archive" {
			pkg = project.AppPackageName("admin")
		}
		source := "// Package " + pkg + " owns App composition.\npackage " + pkg + "\n" + strings.ReplaceAll(schema, "NAME", strconv.Quote(name))
		if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	environmentFile, err := os.OpenFile(filepath.Join(root, ".env"), os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := fmt.Fprintln(environmentFile, "\nSEARCH_CATALOG_DRIVER=bleve\nSEARCH_CATALOG_PATH=_data/search/catalog\nADMIN_SEARCH_ARCHIVE_DRIVER=bleve\nADMIN_SEARCH_ARCHIVE_PATH=_data/search/admin/archive")
	closeErr := environmentFile.Close()
	if writeErr != nil || closeErr != nil {
		t.Fatalf("configure indexes: %v, %v", writeErr, closeErr)
	}
	forj := testkit.EnsureIntegrationForjBinary(t)
	environment := testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	generate := exec.CommandContext(t.Context(), forj, "generate", "--search")
	generate.Dir = root
	generate.Env = environment
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate scoped accessors: %v\n%s", err, output)
	}
	programDir := filepath.Join(root, "cmd/search-scope-check")
	if err := os.MkdirAll(programDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(programDir, "main.go"), []byte(searchRuntimeIsolationProgram), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "run", "./cmd/search-scope-check")
	command.Dir = root
	command.Env = environment
	if output, err := command.CombinedOutput(); err != nil || !strings.Contains(string(output), "App search indexes isolated") {
		t.Fatalf("real App initialization: %v\n%s", err, output)
	}
}

const searchRuntimeIsolationProgram = `// Package main verifies independently initialized App search resources.
package main
import (
 "context"
 "fmt"
 "log"
 "github.com/goforj/env/v2"
 "github.com/goforj/search"
 "example.com/scopedsearch/app"
 "example.com/scopedsearch/app/admin"
 defaultwire "example.com/scopedsearch/app/wire"
 adminwire "example.com/scopedsearch/app/admin/wire"
 "example.com/scopedsearch/internal/searches"
)
// main proves disjoint App schemas can initialize and close without sharing a Bleve owner.
func main() {
 if err:=env.Load();err!=nil{log.Fatal(err)}
 ctx:=context.Background()
 if err:=searches.Provision(ctx,app.SearchDefinitions());err!=nil{log.Fatal(err)}
 if err:=searches.ProvisionForApp(ctx,"ADMIN",adminapp.SearchDefinitions());err!=nil{log.Fatal(err)}
 defaults,err:=defaultwire.InitializeApplication();if err!=nil{log.Fatal(err)}
 defer defaults.Shutdown(ctx)
 named,err:=adminwire.InitializeApplication();if err!=nil{log.Fatal(err)}
 defer named.Shutdown(ctx)
 if len(defaults.Searches().Instances())!=1 || len(named.Searches().Instances())!=1{log.Fatal("App opened unrelated index")}
 receipt,err:=search.Upsert(ctx,defaults.Searches().Catalog(),"product",map[string]string{"description":"red boot"});if err!=nil{log.Fatal(err)}
 if _,err:=defaults.Searches().Catalog().Wait(ctx,receipt);err!=nil{log.Fatal(err)}
 page,err:=named.Searches().Archive().Search(ctx,search.Query{Text:"red"});if err!=nil || page.Total.Value!=0{log.Fatalf("cross-App index data: %#v %v",page,err)}
 fmt.Println("App search indexes isolated")
}
`

// TestSearchResourceWordAppRuntime renders valid resource-word App names without creating default index settings.
func TestSearchResourceWordAppRuntime(t *testing.T) {
	root := t.TempDir()
	repository, err := testkit.RepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	components := project.Components{CLI: true, Search: true}
	config := project.Config{ProjectName: "Search App names", GoModuleName: "example.com/searchappnames", Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{}}
	for _, name := range []string{"search", "search-worker", "search-search"} {
		config.Apps[name] = project.AppConfig{Components: components}
		if err := writeConventionalAppMarker(root, name); err != nil {
			t.Fatal(err)
		}
	}
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: config, ModuleReplaces: map[string]string{"github.com/goforj/goforj": repository}})
	accessors, err := os.ReadFile(filepath.Join(root, "internal/searches/accessors_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(accessors), "Search()") {
		t.Fatalf("App root generated index accessor: %s", accessors)
	}
	programDir := filepath.Join(root, "cmd/search-app-check")
	if err := os.MkdirAll(programDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(programDir, "main.go"), []byte(searchResourceWordAppProgram), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "run", "./cmd/search-app-check")
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	if output, err := command.CombinedOutput(); err != nil || !strings.Contains(string(output), "Resource-word Apps initialized") {
		t.Fatalf("resource-word App runtime: %v\n%s", err, output)
	}
}

const searchResourceWordAppProgram = `// Package main verifies that App roots cannot invent search indexes.
package main
import (
 "context"
 "fmt"
 "log"
 "github.com/goforj/env/v2"
 defaultwire "example.com/searchappnames/app/wire"
 searchwire "example.com/searchappnames/app/search/wire"
 workerwire "example.com/searchappnames/app/search-worker/wire"
 nestedwire "example.com/searchappnames/app/search-search/wire"
 "example.com/searchappnames/internal/searches"
)
// main provisions empty declarations and initializes the independently rendered App graphs.
func main() {
 if err := env.Load(); err != nil { log.Fatal(err) }
 ctx := context.Background()
 for _, prefix := range []string{"", "SEARCH", "SEARCH_WORKER", "SEARCH_SEARCH"} {
  if err := searches.ProvisionForApp(ctx, prefix, nil); err != nil { log.Fatal(err) }
 }
 defaults, err := defaultwire.InitializeApplication(); if err != nil { log.Fatal(err) }
 defer defaults.Shutdown(ctx)
 named, err := searchwire.InitializeApplication(); if err != nil { log.Fatal(err) }
 defer named.Shutdown(ctx)
 worker, err := workerwire.InitializeApplication(); if err != nil { log.Fatal(err) }
 defer worker.Shutdown(ctx)
 nested, err := nestedwire.InitializeApplication(); if err != nil { log.Fatal(err) }
 defer nested.Shutdown(ctx)
 if len(defaults.Searches().Instances())+len(named.Searches().Instances())+len(worker.Searches().Instances())+len(nested.Searches().Instances()) != 0 { log.Fatal("App environment invented indexes") }
 fmt.Println("Resource-word Apps initialized")
}
`
