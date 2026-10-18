//go:build integration

package forj

import (
	"context"
	"fmt"
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
		wantDSN := "synthetic-service-dsn"
		if phase == "local" {
			wantDSN = ""
		}
		probe := fmt.Sprintf(`package database

import ("testing"; "strings")

// TestDriverSwitchRuntimeDSN checks the actual generated runtime fallback after a provider switch.
func TestDriverSwitchRuntimeDSN(t *testing.T) {
    t.Setenv("DB_DSN", %q)
    t.Setenv("DB_REPORTS_DSN", %q)
    if got := envOr("reports", "DSN"); got != %q { t.Fatal("generated runtime selected the wrong DSN") }
    t.Setenv("DB_EXISTING_DSN", %q)
    t.Setenv("DB_EXISTING_SQLITE_DATABASE", %q)
    if got := strings.TrimSpace(envOr("existing", "DSN")); got != "" { t.Fatal("whitespace DSN unexpectedly inherited a service connection") }
    if got, err := buildDSN("existing", "sqlite"); err != nil || got != "existing.db" { t.Fatal("existing SQLite target changed") }
}
`, fresh.Current["DB_DSN"], fresh.Current["DB_REPORTS_DSN"], wantDSN, fresh.Current["DB_EXISTING_DSN"], fresh.Current["DB_EXISTING_SQLITE_DATABASE"])
		if err := os.WriteFile(filepath.Join(root, "internal/database/driver_switch_test.go"), []byte(probe), 0600); err != nil {
			t.Fatal(err)
		}
		command = exec.Command("go", "test", "./internal/database", "-run", "^TestDriverSwitchRuntimeDSN$", "-count=1")
		command.Dir = root
		command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%s runtime DSN: %v\n%s", phase, err, output)
		}
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
