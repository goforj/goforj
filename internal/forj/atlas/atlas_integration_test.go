//go:build integration

package atlas

import (
	"os"
	"path/filepath"
	"testing"

	atlasmcp "github.com/goforj/atlas/mcp"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestMain releases suite-scoped integration tools after the Atlas smoke test.
func TestMain(m *testing.M) {
	code := m.Run()
	testkit.CleanupIntegrationHarness()
	os.Exit(code)
}

func TestAtlasMCPServerUsesRenderedProjectInventory(t *testing.T) {
	root, err := os.MkdirTemp("", "atlas-mcp-smoke-*")
	if err != nil {
		t.Fatalf("create temp project: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})

	testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "AtlasSmoke",
			GoModuleName: "example.com/atlas-smoke",
			UpdatedAt:    "2026-06-23 00:00:00 UTC",
			Render: project.RenderConfig{
				StarterKit: project.StarterKitVue,
				Components: project.Components{
					CLI:            true,
					Cache:          true,
					Events:         true,
					WebAPI:         true,
					WebUI:          true,
					Metrics:        true,
					DatabaseSQLite: true,
					Scheduler:      true,
					Storage:        true,
					Jobs:           true,
				},
			},
		},
		EnvOverrides: map[string]string{
			"QUEUE_REPORTS_DRIVER":  "workerpool",
			"CACHE_SESSIONS_DRIVER": "memory",
			"STORAGE_PUBLIC_DRIVER": "local",
			"EVENTS_AUDIT_DRIVER":   "inproc",
		},
	})
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(root, ".env")}, map[string]string{
		"QUEUE_DRIVER":            "workerpool",
		"QUEUE_SUPPORTED_DRIVERS": "workerpool",
	}); err != nil {
		t.Fatalf("persist workerpool queue selection: %v", err)
	}
	appendFile(t, filepath.Join(root, ".env"), `
QUEUE_REPORTS_DRIVER=workerpool
CACHE_SESSIONS_DRIVER=memory
STORAGE_PUBLIC_DRIVER=local
EVENTS_AUDIT_DRIVER=inproc
`)

	if _, err := os.Stat(filepath.Join(root, "cmd", "app", "main.go")); err != nil {
		t.Fatalf("rendered app missing: %v", err)
	}

	project := Project(root)
	if project.FrontendKit != "vue" || !containsApp(project.Apps, "app") {
		t.Fatalf("project = %#v", project)
	}

	inventory := Inventory(root)
	if !containsString(inventory.Routes["app"], "group /api/v1") {
		t.Fatalf("routes = %#v", inventory.Routes)
	}
	if !containsString(inventory.Queues, "reports") ||
		!containsString(inventory.Caches, "sessions") ||
		!containsString(inventory.Disks, "public") ||
		!containsString(inventory.EventBuses, "audit") {
		t.Fatalf("inventory = %#v", inventory)
	}

	diagnostics := Diagnostics(root)
	connections, err := diagnostics.DatabaseConnections(t.Context())
	if err != nil {
		t.Fatalf("database connections: %v", err)
	}
	if len(connections) == 0 || connections[0].Driver != "sqlite" {
		t.Fatalf("connections = %#v", connections)
	}

	server := atlasmcp.New(atlasmcp.Server{
		Project:     project,
		Diagnostics: diagnostics,
		Inventory:   inventory,
		Version:     "integration",
	})
	if server == nil {
		t.Fatal("expected MCP server")
	}
}

// appendFile centralizes append file behavior so callers follow the same contract.
func appendFile(t *testing.T, path string, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer file.Close()
	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}
