//go:build integration

package forj

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/localproviders"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestLocalProvidersRenderedComposition builds the complete feature composition before and after restoring service settings.
func TestLocalProvidersRenderedComposition(t *testing.T) {
	root := t.TempDir()
	if err := writeConventionalAppMarker(root, "admin"); err != nil {
		t.Fatal(err)
	}
	components := project.Components{CLI: true, DemoApp: true, Mail: true, Auth: true, OAuth: true, WebAPI: true, WebUI: true, Metrics: true, Observability: true, Grafana: true, Docker: true, DatabaseMySQL: true, Scheduler: true, Cache: true, Events: true, Storage: true, Jobs: true}
	modCache, buildCache := testkit.GoCachePaths()
	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{EnvOverrides: map[string]string{"GOMODCACHE": modCache, "GOCACHE": buildCache, "GOWORK": "off"}, Config: project.Config{
		ProjectName: "Local Providers", GoModuleName: "example.com/localproviders", Render: project.RenderConfig{Components: components},
		Apps: map[string]project.AppConfig{"admin": {Components: components}},
	}})
	for key, value := range map[string]string{"CACHE_DRIVER": "redis", "CACHE_SUPPORTED_DRIVERS": "memory,redis", "QUEUE_SUPPORTED_DRIVERS": "workerpool,redis", "QUEUE_DRIVER": "redis", "DB_DRIVER": "mysql", "DB_SUPPORTED_DRIVERS": "mysql,sqlite", "DB_DSN": "synthetic-service-dsn", "DB_REPORTS_DRIVER": "mysql", "DB_EXISTING_DRIVER": "sqlite", "DB_EXISTING_DSN": "' '", "DB_EXISTING_SQLITE_DATABASE": "existing.db", "ADMIN_DB_DRIVER": "mysql", "ADMIN_CACHE_DRIVER": "redis", "ADMIN_QUEUE_DRIVER": "redis", "COMPOSE_PROFILES": "mysql,redis,grafana"} {
		setRenderedEnvValue(t, root, key, value)
	}
	session, err := localproviders.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	values, err := session.Portable()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Apply(values); err != nil {
		t.Fatal(err)
	}
	if values["COMPOSE_PROFILES"] != "grafana" || values["ADMIN_DB_DRIVER"] != "sqlite" || values["ADMIN_QUEUE_DRIVER"] != "workerpool" {
		t.Fatal("full composition did not switch to local providers")
	}
	for _, phase := range []string{"local", "picked-named-service", "picked-services", "restored"} {
		if phase == "picked-named-service" {
			fresh, err := localproviders.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			values, err := fresh.Drivers(map[string]string{"DB_REPORTS_DRIVER": "mysql"})
			if err != nil {
				t.Fatal(err)
			}
			if values["DB_DRIVER"] != "sqlite" || values["ADMIN_DB_DRIVER"] != "sqlite" || values["DB_REPORTS_DSN"] != "synthetic-service-dsn" {
				t.Fatal("named service did not recover its inherited connection independently")
			}
			if err := fresh.Apply(values); err != nil {
				t.Fatal(err)
			}
		}
		if phase == "picked-services" {
			fresh, err := localproviders.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			choices := map[string]string{"DB_DRIVER": "mysql", "ADMIN_DB_DRIVER": "mysql", "CACHE_DRIVER": "redis", "ADMIN_CACHE_DRIVER": "redis", "QUEUE_DRIVER": "redis", "ADMIN_QUEUE_DRIVER": "redis"}
			before, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fresh.Drivers(choices); err == nil {
				t.Fatal("partial selection accepted incompatible App DSN inheritance")
			}
			after, err := os.ReadFile(filepath.Join(root, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("rejected selection changed the active environment")
			}
			choices["ADMIN_DB_REPORTS_DRIVER"] = "mysql"
			values, err := fresh.Drivers(choices)
			if err != nil {
				t.Fatal(err)
			}
			if values["DB_DSN"] != "synthetic-service-dsn" {
				t.Fatal("picker did not recover service DSN")
			}
			if err := fresh.Apply(values); err != nil {
				t.Fatal(err)
			}
		}
		if phase == "restored" {
			fresh, err := localproviders.Open(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := fresh.Restore(); err != nil {
				t.Fatal(err)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		command := exec.CommandContext(ctx, testkit.EnsureIntegrationForjBinary(t), "build")
		command.Dir = root
		command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
		output, err := command.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%s forj build: %v\n%s", phase, err, output)
		}
		buildMultiAppRuntimeBinaries(t, root, []multiAppRuntimeSpec{{name: "app", packagePath: "./cmd/app"}, {name: "admin", packagePath: "./cmd/admin"}})
		fresh, err := localproviders.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		assertRenderedProviderTargets(t, root, phase, fresh.Current)
	}
	command := exec.Command("go", "list", "-m", "-f", "{{if .Replace}}{{.Path}}{{end}}", "all")
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect published modules: %v\n%s", err, output)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("render used local replacements: %s", output)
	}
}

// assertRenderedProviderTargets exercises the generated App overlay and database resolver together for both Apps.
func assertRenderedProviderTargets(t *testing.T, root, phase string, values map[string]string) {
	t.Helper()
	expected := map[string]map[string]string{}
	for _, app := range []string{"app", "admin"} {
		prefix := ""
		if app == "admin" {
			prefix = "ADMIN_"
		}
		rootTarget, reportsTarget := "synthetic-service-dsn", "synthetic-service-dsn"
		rootDriver, reportsDriver := "mysql", "mysql"
		if phase == "local" || phase == "picked-named-service" {
			rootDriver = "sqlite"
			rootTarget = values[prefix+"DB_SQLITE_DATABASE"]
			if rootTarget == "" {
				rootTarget = values["DB_SQLITE_DATABASE"]
			}
		}
		if phase == "local" || phase == "picked-named-service" && app == "admin" {
			reportsDriver = "sqlite"
			reportsTarget = values[prefix+"DB_REPORTS_SQLITE_DATABASE"]
			if reportsTarget == "" {
				reportsTarget = values["DB_REPORTS_SQLITE_DATABASE"]
			}
		}
		expected[app] = map[string]string{"default": rootTarget, "reports": reportsTarget, "existing": "existing.db", "default_driver": rootDriver, "reports_driver": reportsDriver, "existing_driver": "sqlite"}
	}
	data, err := json.Marshal(struct {
		Values   map[string]string
		Expected map[string]map[string]string
	}{values, expected})
	if err != nil {
		t.Fatal(err)
	}
	valuesPath, resultPath := filepath.Join(root, ".provider-probe-values.json"), filepath.Join(root, ".provider-probe-result.json")
	if err := os.WriteFile(valuesPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	overlayProbe := `package cmd

import ("encoding/json"; "os"; "strings"; "testing")

// TestDriverSwitchAppOverlay exports the actual generated App environment for the database resolver probe.
func TestDriverSwitchAppOverlay(t *testing.T) {
 raw,err := os.ReadFile(os.Getenv("FORJ_PROVIDER_PROBE_VALUES")); if err != nil { t.Fatal(err) }
 var input struct { Values map[string]string; Expected map[string]map[string]string }
 if err := json.Unmarshal(raw,&input); err != nil { t.Fatal(err) }
 for _,entry := range os.Environ() { key,_,_ := strings.Cut(entry,"="); if strings.HasPrefix(key,"DB_") || strings.HasPrefix(key,"ADMIN_DB_") { t.Setenv(key,"") } }
 for key,value := range input.Values { t.Setenv(key,value) }
 app := os.Getenv("FORJ_PROVIDER_PROBE_APP")
 t.Setenv("FORJ_APP",app)
 if err := ApplyAppEnvOverlay(); err != nil { t.Fatal(err) }
 settings := map[string]string{}
 for _,entry := range os.Environ() { key,value,_ := strings.Cut(entry,"="); if strings.HasPrefix(key,"DB_") { settings[key] = value } }
 data,err := json.Marshal(struct { Settings map[string]string; Expected map[string]string }{settings,input.Expected[app]}); if err != nil { t.Fatal(err) }
 if err := os.WriteFile(os.Getenv("FORJ_PROVIDER_PROBE_RESULT"),data,0600); err != nil { t.Fatal(err) }
}
`
	databaseProbe := `package database

import ("encoding/json"; "os"; "strings"; "testing")

// TestDriverSwitchRuntimeDSN checks actual generated lookup after the actual App overlay has run.
func TestDriverSwitchRuntimeDSN(t *testing.T) {
 raw,err := os.ReadFile(os.Getenv("FORJ_PROVIDER_PROBE_RESULT")); if err != nil { t.Fatal(err) }
 var input struct { Settings map[string]string; Expected map[string]string }
 if err := json.Unmarshal(raw,&input); err != nil { t.Fatal(err) }
 for _,entry := range os.Environ() { key,_,_ := strings.Cut(entry,"="); if strings.HasPrefix(key,"DB_") { t.Setenv(key,"") } }
 for key,value := range input.Settings { t.Setenv(key,value) }
 for _,name := range []string{"default","reports","existing"} {
  driver := strings.ToLower(strings.TrimSpace(envOr(name,"DRIVER"))); if driver == "" { driver = "sqlite" }
  if driver != input.Expected[name+"_driver"] { t.Fatalf("%s selected the wrong driver",name) }
  target := strings.TrimSpace(envOr(name,"DSN"))
  if target == "" { target,err = buildDSN(name,driver); if err != nil { t.Fatal(err) } }
  if target != input.Expected[name] { t.Fatalf("%s selected the wrong database target",name) }
 }
}
`
	for path, source := range map[string]string{"internal/cmd/provider_overlay_test.go": overlayProbe, "internal/database/driver_switch_test.go": databaseProbe} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, app := range []string{"app", "admin"} {
		for _, probe := range []struct{ path, name string }{{"./internal/cmd", "TestDriverSwitchAppOverlay"}, {"./internal/database", "TestDriverSwitchRuntimeDSN"}} {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			command := exec.CommandContext(ctx, "go", "test", probe.path, "-run", "^"+probe.name+"$", "-count=1")
			command.Dir = root
			command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off", "FORJ_PROVIDER_PROBE_VALUES": valuesPath, "FORJ_PROVIDER_PROBE_RESULT": resultPath, "FORJ_PROVIDER_PROBE_APP": app})
			output, err := command.CombinedOutput()
			cancel()
			if err != nil {
				t.Fatalf("%s/%s %s: %v\n%s", phase, app, probe.name, err, output)
			}
		}
	}
}
