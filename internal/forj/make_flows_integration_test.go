//go:build integration

package forj

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"gorm.io/gorm"
)

func TestMakeFlowsIntegration(t *testing.T) {
	projectDir := t.TempDir()
	renderAppAtDir(t, projectDir)
	binPath := testkit.EnsureIntegrationForjBinary(t)
	_ = testkit.EnsureIntegrationToolsDir(t)

	runForj := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	buildApp := func(tb testing.TB) {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, "build", "-o", "./bin/app")
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj build failed: %v\n%s", err, out.String())
		}
		if _, err := os.Stat(filepath.Join(projectDir, "bin", "app")); err != nil {
			tb.Fatalf("expected built app binary: %v", err)
		}
	}

	runApp := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, filepath.Join(projectDir, "bin", "app"), args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("app %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	runForj(t, "make:controller", "Audit")
	runForj(t, "make:controller", "Audit")
	assertFileContains(t, filepath.Join(projectDir, "internal", "audit", "controller.go"), []string{
		"package audit",
		`web.NewRoute(http.MethodGet, "/audit", c.Get)`,
	})
	assertFileContains(t, filepath.Join(projectDir, "wire", "inject_http_controllers.go"), []string{
		`"example.com/testapp/internal/audit"`,
		"audit.NewController",
	})
	assertFileContains(t, filepath.Join(projectDir, "internal", "router", "routes_registry.go"), []string{
		`"example.com/testapp/internal/audit"`,
		"auditController *audit.Controller",
		"publicRoutes = append(publicRoutes, auditController.Routes()...)",
	})

	runForj(t, "make:controller", "Billing:Reports")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "reports", "controller.go"), []string{
		"package reports",
		`web.NewRoute(http.MethodGet, "/billing/reports", c.Get)`,
	})
	assertFileContains(t, filepath.Join(projectDir, "wire", "inject_http_controllers.go"), []string{
		`billingReports "example.com/testapp/internal/billing/reports"`,
		"billingReports.NewController",
	})
	assertFileContains(t, filepath.Join(projectDir, "internal", "router", "routes_registry.go"), []string{
		`billingReports "example.com/testapp/internal/billing/reports"`,
		"billingReportsController *billingReports.Controller",
		"publicRoutes = append(publicRoutes, billingReportsController.Routes()...)",
	})

	runForj(t, "run", "make:event", "UserRegistered")
	assertFileContains(t, filepath.Join(projectDir, "internal", "events", "user_registered_event.go"), []string{
		"package events",
		"const UserRegisteredEventTopic",
		"type UserRegisteredEvent struct",
	})
	runForj(t, "run", "make:event", "Billing:InvoicePaid")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "invoice_paid_event.go"), []string{
		"package billing",
		"const InvoicePaidEventTopic",
		"type InvoicePaidEvent struct",
	})

	runForj(t, "run", "make:job", "SyncReports")
	assertFileContains(t, filepath.Join(projectDir, "internal", "jobs", "sync_reports_job.go"), []string{
		"package jobs",
		"const SyncReportsJobTypeName",
		"type SyncReportsJob struct",
	})
	assertFileContains(t, filepath.Join(projectDir, "wire", "inject_jobs_app.go"), []string{
		"jobs.NewSyncReportsJob",
	})
	runForj(t, "run", "make:job", "Billing:SyncReports")
	assertFileContains(t, filepath.Join(projectDir, "internal", "billing", "sync_reports_job.go"), []string{
		"package billing",
		"const SyncReportsJobTypeName",
		"type SyncReportsJob struct",
	})
	assertFileContains(t, filepath.Join(projectDir, "wire", "inject_jobs_app.go"), []string{
		`"example.com/testapp/internal/billing"`,
		"billing.NewSyncReportsJob",
	})

	runForj(t, "make:migration", "create_widgets")
	assertGlob(t, filepath.Join(projectDir, "migrations", "*create_widgets.up.sql"))
	assertGlob(t, filepath.Join(projectDir, "migrations", "*create_widgets.down.sql"))

	buildApp(t)
	routes := runApp(t, "route:list")
	if !strings.Contains(routes, "/audit") {
		t.Fatalf("expected route:list to include generated audit route, got:\n%s", routes)
	}
	if !strings.Contains(routes, "/billing/reports") {
		t.Fatalf("expected route:list to include generated billing reports route, got:\n%s", routes)
	}
}

func TestMakeModelFlowIntegration(t *testing.T) {
	projectDir := t.TempDir()
	testkit.RenderProjectWithForj(t, projectDir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "TestApp",
			GoModuleName: "example.com/testapp",
			UpdatedAt:    "2026-01-01 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					WebAPI:         true,
					DatabaseSQLite: true,
				},
			},
		},
	})

	dbPath := filepath.Join(projectDir, "_data", "sqlite", "app.db")
	seedSQLiteMakeModelTable(t, dbPath)

	binPath := testkit.EnsureIntegrationForjBinary(t)
	runForj := func(tb testing.TB, args ...string) string {
		tb.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binPath, args...)
		cmd.Dir = projectDir
		cmd.Env = testkit.IntegrationProcessEnv(t, nil)
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Run(); err != nil {
			tb.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}

	runForj(t, "run", "make:model", "widgets")
	runForj(t, "run", "make:model", "widgets")

	assertFileContains(t, filepath.Join(projectDir, "internal", "models", "widget.go"), []string{
		"type Widget struct",
		`gorm:"column:name`,
		`func (*Widget) TableName() string`,
		"type WidgetRepo struct",
		"func NewWidgetRepo(",
	})
	assertFileContains(t, filepath.Join(projectDir, "wire", "inject_repositories.go"), []string{
		"models.NewWidgetRepo",
	})

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "build", "-o", "./bin/app")
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("forj build failed after make:model: %v\n%s", err, out.String())
	}
}

func seedSQLiteMakeModelTable(t *testing.T, dbPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatalf("prepare sqlite dir: %v", err)
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	if err := db.Exec(`CREATE TABLE widgets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		enabled BOOLEAN NOT NULL DEFAULT 1,
		created_at DATETIME NULL
	)`).Error; err != nil {
		t.Fatalf("create widgets table: %v", err)
	}
}

func assertGlob(t *testing.T, pattern string) {
	t.Helper()
	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob %s: %v", pattern, err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one match for %s", pattern)
	}
}
