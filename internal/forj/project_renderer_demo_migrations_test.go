package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

func TestDemoMigrationsRenderToTopLevelMigrations(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = &project.Config{
		ProjectName:  "DemoApp",
		GoModuleName: "example.com/demoapp",
		Render: project.RenderConfig{
			Components: project.Components{
				DemoApp:        true,
				Jobs:           true,
				DatabaseSQLite: true,
			},
		},
	}

	includeDemoInternal := func(tmpl string) bool {
		// Mirror project renderer behavior when jobs are enabled.
		return !strings.HasPrefix(filepath.ToSlash(tmpl), "demo/internal/migrations/")
	}
	if err := renderer.writeTemplatesUnder("demo/internal", "internal", includeDemoInternal); err != nil {
		t.Fatalf("write demo/internal: %v", err)
	}
	if err := renderer.writeTemplatesUnder("demo/internal/migrations", "migrations", nil); err != nil {
		t.Fatalf("write demo/internal/migrations: %v", err)
	}

	if _, err := os.Stat(filepath.Join("migrations", "2026_02_11_000012_monitor_alert_policy_columns.sqlite.up.sql")); err != nil {
		t.Fatalf("expected top-level migration: %v", err)
	}
	if _, err := os.Stat(filepath.Join("internal", "migrations", "2026_02_11_000012_monitor_alert_policy_columns.sqlite.up.sql")); !os.IsNotExist(err) {
		t.Fatalf("expected no internal demo migration file, got err=%v", err)
	}
}
