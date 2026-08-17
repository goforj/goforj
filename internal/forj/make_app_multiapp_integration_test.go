//go:build integration && multiapp

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

// TestMakeAppMultiAppRuntimeSmoke proves make:app-created apps can run together without port conflicts.
func TestMakeAppMultiAppRuntimeSmoke(t *testing.T) {
	requirePortsAvailable(t, []string{
		"127.0.0.1:3000",
		"127.0.0.1:3001",
		"127.0.0.1:3002",
		"127.0.0.1:10000",
		"127.0.0.1:10001",
		"127.0.0.1:10002",
		"127.0.0.1:10010",
		"127.0.0.1:10011",
		"127.0.0.1:10012",
		"127.0.0.1:10020",
		"127.0.0.1:10021",
		"127.0.0.1:10022",
	})

	projectDir := t.TempDir()
	renderMakeAppSmokeProject(t, projectDir)
	runMakeAppSmokeForj(t, projectDir, "make:app", "billing", "--components", "web-api,scheduler,jobs")
	runMakeAppSmokeForj(t, projectDir, "make:app", "reporting", "--components", "web-api,scheduler,jobs")
	buildMultiAppRuntimeBinaries(t, projectDir, []multiAppRuntimeSpec{
		{name: "app", packagePath: "./cmd/app"},
		{name: "billing", packagePath: "./cmd/billing"},
		{name: "reporting", packagePath: "./cmd/reporting"},
	})

	apps := []multiAppRuntimeSpec{
		{name: "app", httpPort: "3000", metricsPort: "10000", schedulerMetricsPort: "10001", workerMetricsPort: "10002"},
		{name: "billing", httpPort: "3001", metricsPort: "10010", schedulerMetricsPort: "10011", workerMetricsPort: "10012"},
		{name: "reporting", httpPort: "3002", metricsPort: "10020", schedulerMetricsPort: "10021", workerMetricsPort: "10022"},
	}
	procs := make([]*procHandle, 0, len(apps))
	for _, app := range apps {
		procs = append(procs, startAppHTTPRuntime(t, projectDir, app))
		procs = append(procs, startAppSchedulerRuntime(t, projectDir, app))
		procs = append(procs, startAppWorkerRuntime(t, projectDir, app))
	}
	defer stopMakeAppSmokeRuntimes(t, procs)

	checks := make([]runtimeReadinessCheck, 0, len(apps)*3)
	for _, app := range apps {
		app := app
		httpProc := findRuntimeProc(t, procs, app.name+" http")
		schedulerProc := findRuntimeProc(t, procs, app.name+" scheduler")
		workerProc := findRuntimeProc(t, procs, app.name+" worker")
		checks = append(checks,
			runtimeReadinessCheck{
				name: app.name + " http",
				run: func() error {
					return assertAppHTTPRuntimeReady(app, httpProc)
				},
			},
			runtimeReadinessCheck{
				name: app.name + " scheduler metrics",
				run: func() error {
					return assertAppMetricsRuntimeReady(app.name+" scheduler", app.schedulerMetricsPort, schedulerProc)
				},
			},
			runtimeReadinessCheck{
				name: app.name + " worker metrics",
				run: func() error {
					return assertAppMetricsRuntimeReady(app.name+" worker", app.workerMetricsPort, workerProc)
				},
			},
		)
	}
	assertRuntimeReadiness(t, checks)
}

// TestMakeAppMultiAppSQLiteMigrationsSmoke proves make:app-created apps own independent SQLite migration streams.
func TestMakeAppMultiAppSQLiteMigrationsSmoke(t *testing.T) {
	projectDir := t.TempDir()
	renderMakeAppMigrationSmokeProject(t, projectDir)
	runMakeAppSmokeForj(t, projectDir, "make:app", "billing", "--components", "web-api,database-sqlite")
	runMakeAppSmokeForj(t, projectDir, "make:app", "reporting", "--components", "web-api,database-sqlite")
	writeMakeAppSmokeMigration(t, projectDir, "app", "app_widgets")
	writeMakeAppSmokeMigration(t, projectDir, "billing", "billing_widgets")
	writeMakeAppSmokeMigration(t, projectDir, "reporting", "reporting_widgets")
	buildMultiAppRuntimeBinaries(t, projectDir, []multiAppRuntimeSpec{
		{name: "app", packagePath: "./cmd/app"},
		{name: "billing", packagePath: "./cmd/billing"},
		{name: "reporting", packagePath: "./cmd/reporting"},
	})

	runMakeAppSmokeBinary(t, projectDir, "app", "migrate")
	runMakeAppSmokeBinary(t, projectDir, "billing", "migrate")
	runMakeAppSmokeBinary(t, projectDir, "reporting", "migrate")

	assertMakeAppSmokeSQLiteTables(t, filepath.Join(projectDir, "_data", "sqlite", "app.db"), []string{"app_widgets"}, []string{"billing_widgets", "reporting_widgets"})
	assertMakeAppSmokeSQLiteTables(t, filepath.Join(projectDir, "_data", "sqlite", "billing.db"), []string{"billing_widgets"}, []string{"app_widgets", "reporting_widgets"})
	assertMakeAppSmokeSQLiteTables(t, filepath.Join(projectDir, "_data", "sqlite", "reporting.db"), []string{"reporting_widgets"}, []string{"app_widgets", "billing_widgets"})
}

// renderMakeAppSmokeProject keeps the fixture dependency-light so CI can prove multi-app behavior without Docker.
func renderMakeAppSmokeProject(t *testing.T, dir string) {
	t.Helper()
	testkit.RenderProjectWithForj(t, dir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "MakeAppSmoke",
			GoModuleName: "example.com/makeappsmoke",
			UpdatedAt:    "2026-06-10 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:       true,
					WebAPI:    true,
					Metrics:   true,
					Scheduler: true,
					Jobs:      true,
				},
			},
		},
	})
	if err := testkit.ReplaceOrAppendEnvValues([]string{filepath.Join(dir, ".env")}, map[string]string{
		"QUEUE_DRIVER":            "workerpool",
		"QUEUE_SUPPORTED_DRIVERS": "workerpool",
	}); err != nil {
		t.Fatalf("persist workerpool queue selection: %v", err)
	}
	runMakeAppSmokeForj(t, dir, "generate", "--queue")
}

// renderMakeAppMigrationSmokeProject enables SQLite so app migrations can run without external services.
func renderMakeAppMigrationSmokeProject(t *testing.T, dir string) {
	t.Helper()
	testkit.RenderProjectWithForj(t, dir, testkit.RenderProjectRequest{
		Config: project.Config{
			ProjectName:  "MakeAppMigrationSmoke",
			GoModuleName: "example.com/makeappmigrationsmoke",
			UpdatedAt:    "2026-06-10 00:00:00 UTC",
			Render: project.RenderConfig{
				Components: project.Components{
					CLI:            true,
					WebAPI:         true,
					DatabaseSQLite: true,
				},
			},
		},
	})
}

// runMakeAppSmokeForj uses the real CLI binary so the smoke test covers the public make:app path.
func runMakeAppSmokeForj(t *testing.T, projectDir string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, testkit.EnsureIntegrationForjBinary(t), args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationGoProcessEnv(t, nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("forj %s failed: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

// runMakeAppSmokeBinary exercises generated app binaries instead of renderer internals.
func runMakeAppSmokeBinary(t *testing.T, projectDir string, appName string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(projectDir, "bin", appName), args...)
	cmd.Dir = projectDir
	cmd.Env = testkit.IntegrationProcessEnv(t, nil)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", appName, strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

// writeMakeAppSmokeMigration writes real SQL into the app migration layout that generated apps embed.
func writeMakeAppSmokeMigration(t *testing.T, projectDir string, appName string, table string) {
	t.Helper()
	dir := filepath.Join(projectDir, "migrations", appName, "default")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create migration dir %s: %v", dir, err)
	}
	base := filepath.Join(dir, "2026_06_10_000001_create_"+table)
	up := "CREATE TABLE " + table + " (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL);\n"
	down := "DROP TABLE " + table + ";\n"
	if err := os.WriteFile(base+".up.sql", []byte(up), 0o644); err != nil {
		t.Fatalf("write up migration: %v", err)
	}
	if err := os.WriteFile(base+".down.sql", []byte(down), 0o644); err != nil {
		t.Fatalf("write down migration: %v", err)
	}
}

// assertMakeAppSmokeSQLiteTables verifies each app migrated only its own SQLite database.
func assertMakeAppSmokeSQLiteTables(t *testing.T, dbPath string, present []string, absent []string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db %s: %v", dbPath, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("sqlite sql db %s: %v", dbPath, err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	for _, table := range present {
		if !db.Migrator().HasTable(table) {
			t.Fatalf("expected %s to contain table %s", dbPath, table)
		}
	}
	for _, table := range absent {
		if db.Migrator().HasTable(table) {
			t.Fatalf("expected %s not to contain table %s", dbPath, table)
		}
	}
}

// stopMakeAppSmokeRuntimes stops all app processes concurrently to keep failed CI runs short.
func stopMakeAppSmokeRuntimes(t *testing.T, procs []*procHandle) {
	t.Helper()
	done := make(chan struct{}, len(procs))
	for _, proc := range procs {
		proc := proc
		go func() {
			stopProcAsync(t, proc.name, proc, time.Second)
			done <- struct{}{}
		}()
	}
	for range procs {
		<-done
	}
}
