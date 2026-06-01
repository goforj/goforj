package makecmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/str"
)

// MigrationCmd generates database migration files for configured drivers.
type MigrationCmd struct {
	Name       string `arg:"" help:"Name of the migration (e.g. AddUsersTable)"`
	Connection string `help:"Database connection name" default:"default"`
}

// Signature returns CLI metadata for the make:migration generator.
func (*MigrationCmd) Signature() string {
	return `name:"make:migration" help:"Generate a new migration"`
}

// NewMigrationCmd creates the make:migration generator command.
func NewMigrationCmd() *MigrationCmd {
	return &MigrationCmd{}
}

// Run creates migration files for the resolved database drivers.
func (c *MigrationCmd) Run() error {
	name := str.Of(c.Name).TrimSpace().String()
	if name == "" {
		return fmt.Errorf("migration name cannot be empty")
	}

	// Prepare timestamped base name
	timestamp := time.Now().Format("2006_01_02_150405")
	snake := str.Of(name).Snake("_").String()
	baseName := fmt.Sprintf("%s_%s", timestamp, snake)

	connName := str.Of(c.Connection).TrimSpace().ToLower().String()
	if connName == "" {
		connName = "default"
	}

	drivers := resolveSupportedMigrationDrivers()
	if len(drivers) == 0 {
		return fmt.Errorf("no supported drivers resolved from DB_SUPPORTED_DRIVERS/DB_DRIVER")
	}

	migrationsDir := "migrations"
	if connName != "default" {
		migrationsDir = filepath.Join(migrationsDir, connName)
	}

	// Ensure migrations dir exists
	if err := os.MkdirAll(migrationsDir, os.ModePerm); err != nil {
		return err
	}

	for _, driver := range drivers {
		upPath := filepath.Join(migrationsDir, fmt.Sprintf("%s.%s.up.sql", baseName, driver))
		downPath := filepath.Join(migrationsDir, fmt.Sprintf("%s.%s.down.sql", baseName, driver))
		if len(drivers) == 1 {
			// Keep legacy naming when only one DB driver is supported.
			upPath = filepath.Join(migrationsDir, baseName+".up.sql")
			downPath = filepath.Join(migrationsDir, baseName+".down.sql")
		}

		// Create empty Up SQL file
		if err := os.WriteFile(upPath, []byte(fmt.Sprintf("-- Up migration (%s)\n", driver)), 0644); err != nil {
			return err
		}

		// Create empty Down SQL file
		if err := os.WriteFile(downPath, []byte(fmt.Sprintf("-- Down migration (%s)\n", driver)), 0644); err != nil {
			return err
		}

		console.Successf("generated %s", upPath)
		console.Successf("generated %s", downPath)
	}

	return nil
}

// resolveSupportedMigrationDrivers returns the migration drivers requested by environment.
func resolveSupportedMigrationDrivers() []string {
	var drivers []string
	supported := str.Of(os.Getenv("DB_SUPPORTED_DRIVERS")).TrimSpace().String()
	if supported != "" {
		for _, part := range strings.Split(supported, ",") {
			driver := normalizeMigrationDriver(part)
			if driver == "" || slices.Contains(drivers, driver) {
				continue
			}
			drivers = append(drivers, driver)
		}
	}
	if len(drivers) > 0 {
		return drivers
	}

	defaultDriver := normalizeMigrationDriver(os.Getenv("DB_DRIVER"))
	if defaultDriver != "" {
		return []string{defaultDriver}
	}

	return []string{"sqlite"}
}

// normalizeMigrationDriver converts database driver aliases to migration suffixes.
func normalizeMigrationDriver(driver string) string {
	switch str.Of(driver).TrimSpace().ToLower().String() {
	case "mysql", "mariadb":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return ""
	}
}
