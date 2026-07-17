package makecmd

import (
	"path/filepath"
	"slices"
	"testing"
)

// TestResolveSupportedMigrationDriversUsesTypedEnvironmentSlice verifies trimming, alias normalization, and de-duplication remain stable.
func TestResolveSupportedMigrationDriversUsesTypedEnvironmentSlice(t *testing.T) {
	t.Setenv("DB_SUPPORTED_DRIVERS", " mysql, postgres , mariadb, sqlite ")
	if got, want := resolveSupportedMigrationDrivers(), []string{"mysql", "postgres", "sqlite"}; !slices.Equal(got, want) {
		t.Fatalf("resolveSupportedMigrationDrivers() = %#v, want %#v", got, want)
	}

	t.Setenv("DB_SUPPORTED_DRIVERS", "")
	t.Setenv("DB_DRIVER", "mariadb")
	if got, want := resolveSupportedMigrationDrivers(), []string{"mysql"}; !slices.Equal(got, want) {
		t.Fatalf("resolveSupportedMigrationDrivers() fallback = %#v, want %#v", got, want)
	}
}

func TestMigrationCmdRemoveDeletesMatchingMigrationFiles(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	writeMakeCmdTestFile(t, filepath.Join("migrations", "2026_01_01_000001_create_widgets.up.sql"), "-- up\n")
	writeMakeCmdTestFile(t, filepath.Join("migrations", "2026_01_01_000001_create_widgets.down.sql"), "-- down\n")
	writeMakeCmdTestFile(t, filepath.Join("migrations", "2026_01_01_000002_create_widgets.mysql.up.sql"), "-- up\n")
	writeMakeCmdTestFile(t, filepath.Join("migrations", "2026_01_01_000002_create_widgets.mysql.down.sql"), "-- down\n")
	writeMakeCmdTestFile(t, filepath.Join("migrations", "2026_01_01_000003_create_accounts.up.sql"), "-- up\n")

	cmd := &MigrationCmd{Name: "create_widgets", Remove: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("remove migration: %v", err)
	}

	assertMakeCmdTestFileMissing(t, filepath.Join("migrations", "2026_01_01_000001_create_widgets.up.sql"))
	assertMakeCmdTestFileMissing(t, filepath.Join("migrations", "2026_01_01_000001_create_widgets.down.sql"))
	assertMakeCmdTestFileMissing(t, filepath.Join("migrations", "2026_01_01_000002_create_widgets.mysql.up.sql"))
	assertMakeCmdTestFileMissing(t, filepath.Join("migrations", "2026_01_01_000002_create_widgets.mysql.down.sql"))
	assertMakeCmdTestContains(t, filepath.Join("migrations", "2026_01_01_000003_create_accounts.up.sql"), []string{"-- up"})
}

func TestMigrationCmdUsesActiveApp(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("FORJ_APP", "billing")
	t.Setenv("DB_DRIVER", "sqlite")

	cmd := &MigrationCmd{Name: "create_invoices", Connection: "ledger", NoOpen: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("create app migration: %v", err)
	}

	assertMakeCmdTestGlob(t, filepath.Join("migrations", "billing", "ledger", "*create_invoices.up.sql"))
	assertMakeCmdTestGlob(t, filepath.Join("migrations", "billing", "ledger", "*create_invoices.down.sql"))
}

func TestMigrationCmdExpandedLayoutUsesAppDefault(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	t.Setenv("DB_DRIVER", "sqlite")
	writeMakeCmdTestFile(t, filepath.Join("migrations", "app", "default", ".keep"), "")

	cmd := &MigrationCmd{Name: "create_widgets", NoOpen: true}
	if err := cmd.Run(); err != nil {
		t.Fatalf("create app migration: %v", err)
	}

	assertMakeCmdTestGlob(t, filepath.Join("migrations", "app", "default", "*create_widgets.up.sql"))
	assertMakeCmdTestGlob(t, filepath.Join("migrations", "app", "default", "*create_widgets.down.sql"))
}
