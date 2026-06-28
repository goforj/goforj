package generate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateDBFilesUsesDatabasePackageAndSelectedDrivers(t *testing.T) {
	t.Setenv("DB_DRIVER", "postgres")
	t.Setenv("DB_ANALYTICS_DRIVER", "sqlite")

	repoRoot := repoRoot(t)
	root, err := os.MkdirTemp(repoRoot, ".tmp-db-generate-*")
	if err != nil {
		t.Fatalf("mkdir temp module root: %v", err)
	}
	defer os.RemoveAll(root)
	if err := os.MkdirAll(filepath.Join(root, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database package: %v", err)
	}

	written, err := GenerateDBFiles(root)
	if err != nil {
		t.Fatalf("GenerateDBFiles returned error: %v", err)
	}
	if written == 0 {
		t.Fatal("expected generated db files to be written")
	}

	genPath := filepath.Join(root, "internal", "database", "connections_gen.go")
	src, err := os.ReadFile(genPath)
	if err != nil {
		t.Fatalf("read connections_gen.go: %v", err)
	}
	content := string(src)
	for _, expected := range []string{
		"package database",
		`"gorm.io/driver/postgres"`,
		`"github.com/glebarez/sqlite"`,
		`return postgres.Open(dsn), nil`,
		`return sqlite.Open(dsn), nil`,
		`func (c *Connections) GetAnalytics()`,
		`func (c *Connections) ReadinessChecks() []ReadinessCheck`,
		`Name: "db_analytics"`,
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected generated db source to contain %q", expected)
		}
	}
	if strings.Contains(content, `"gorm.io/driver/mysql"`) {
		t.Fatal("did not expect generated db source to import mysql driver")
	}
}

func TestGenerateDBFilesUsesSupportedDrivers(t *testing.T) {
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_SUPPORTED_DRIVERS", "mysql")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database package: %v", err)
	}

	if _, err := GenerateDBFiles(root); err != nil {
		t.Fatalf("GenerateDBFiles returned error: %v", err)
	}

	src, err := os.ReadFile(filepath.Join(root, "internal", "database", "connections_gen.go"))
	if err != nil {
		t.Fatalf("read connections_gen.go: %v", err)
	}
	content := string(src)
	if !strings.Contains(content, `"github.com/glebarez/sqlite"`) {
		t.Fatal("expected generated db source to keep sqlite as the no-env fallback")
	}
	if !strings.Contains(content, `"gorm.io/driver/mysql"`) {
		t.Fatal("expected generated db source to include mysql from DB_SUPPORTED_DRIVERS")
	}
}

func TestGenerateDBFilesIgnoresDriverHelperDatabaseKeys(t *testing.T) {
	t.Setenv("DB_DRIVER", "mysql")
	t.Setenv("DB_DATABASE", "db")
	t.Setenv("DB_SQLITE_DATABASE", "./_data/sqlite/app.db")

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "database"), 0o755); err != nil {
		t.Fatalf("mkdir database package: %v", err)
	}

	if _, err := GenerateDBFiles(root); err != nil {
		t.Fatalf("GenerateDBFiles returned error: %v", err)
	}

	src, err := os.ReadFile(filepath.Join(root, "internal", "database", "connections_gen.go"))
	if err != nil {
		t.Fatalf("read connections_gen.go: %v", err)
	}
	content := string(src)
	for _, unwanted := range []string{
		`func (c *Connections) GetSqlite()`,
		`Name: "db_sqlite"`,
		`readinessCheck(ctx, "sqlite")`,
	} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("generated db source should not contain helper connection %q:\n%s", unwanted, content)
		}
	}
}
