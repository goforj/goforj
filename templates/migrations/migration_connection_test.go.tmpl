//go:build !integration

package migrations

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestAutoRegisterMigrationsByConnection(t *testing.T) {
	dir := t.TempDir()
	writeSQL(t, dir, "2026_01_01_init", "CREATE TABLE init_test (id INTEGER);", "DROP TABLE init_test;")
	writeSQL(t, filepath.Join(dir, "analytics"), "2026_01_02_metrics", "CREATE TABLE metrics (id INTEGER);", "DROP TABLE metrics;")
	writeSQL(t, filepath.Join(dir, "billing", "default"), "2026_01_03_billing", "CREATE TABLE billing_test (id INTEGER);", "DROP TABLE billing_test;")
	writeSQL(t, filepath.Join(dir, "billing", "ledger"), "2026_01_04_ledger", "CREATE TABLE ledger_test (id INTEGER);", "DROP TABLE ledger_test;")

	originalFS := migrationFS
	t.Cleanup(func() {
		migrationFS = originalFS
		resetRegistry()
	})
	migrationFS = os.DirFS(dir)
	resetRegistry()

	if err := AutoRegisterMigrations(); err != nil {
		t.Fatalf("AutoRegisterMigrations failed: %v", err)
	}

	migrations := GetMigrations()
	if len(migrations) != 4 {
		t.Fatalf("expected 4 migrations, got %d", len(migrations))
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Name() < migrations[j].Name()
	})

	if migrations[0].Name() != "2026_01_01_init" || migrations[0].App() != "app" || migrations[0].Connection() != "default" || migrations[0].DatabaseConnection() != "default" {
		t.Fatalf("expected app/default migration, got %s (%s/%s -> %s)", migrations[0].Name(), migrations[0].App(), migrations[0].Connection(), migrations[0].DatabaseConnection())
	}
	if migrations[1].Name() != "2026_01_02_metrics" || migrations[1].App() != "app" || migrations[1].Connection() != "analytics" || migrations[1].DatabaseConnection() != "analytics" {
		t.Fatalf("expected app/analytics migration, got %s (%s/%s -> %s)", migrations[1].Name(), migrations[1].App(), migrations[1].Connection(), migrations[1].DatabaseConnection())
	}
	if migrations[2].Name() != "2026_01_03_billing" || migrations[2].App() != "billing" || migrations[2].Connection() != "default" || migrations[2].DatabaseConnection() != "billing" {
		t.Fatalf("expected billing/default migration, got %s (%s/%s -> %s)", migrations[2].Name(), migrations[2].App(), migrations[2].Connection(), migrations[2].DatabaseConnection())
	}
	if migrations[3].Name() != "2026_01_04_ledger" || migrations[3].App() != "billing" || migrations[3].Connection() != "ledger" || migrations[3].DatabaseConnection() != "billing_ledger" {
		t.Fatalf("expected billing/ledger migration, got %s (%s/%s -> %s)", migrations[3].Name(), migrations[3].App(), migrations[3].Connection(), migrations[3].DatabaseConnection())
	}
}

func writeSQL(t *testing.T, dir, name, up, down string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	upPath := filepath.Join(dir, name+".up.sql")
	downPath := filepath.Join(dir, name+".down.sql")
	if err := os.WriteFile(upPath, []byte(up), 0644); err != nil {
		t.Fatalf("write up failed: %v", err)
	}
	if err := os.WriteFile(downPath, []byte(down), 0644); err != nil {
		t.Fatalf("write down failed: %v", err)
	}
}

func resetRegistry() {
	registry = nil
}
