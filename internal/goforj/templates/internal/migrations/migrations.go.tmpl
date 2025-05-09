package migrations

import (
	"embed"
	"fmt"
	"gorm.io/gorm"
	"strings"
)

// registry holds all registered migrations
var registry []Migration

// Migration interface defines the methods that a migration must implement
type Migration interface {
	Name() string
	Up(dbConn *gorm.DB) error
	Down(dbConn *gorm.DB) error
}

// RegisterMigration registers a new migration
func RegisterMigration(m Migration) {
	registry = append(registry, m)
}

// GetMigrations returns all registered migrations
func GetMigrations() []Migration {
	return registry
}

//go:embed *
var files embed.FS

// init function is called when the package is imported
func init() {
	// Register all SQL migrations found in the embedded file system
	if err := AutoRegisterMigrations(); err != nil {
		panic(err)
	}
}

// AutoRegisterMigrations automatically registers all SQL migrations
func AutoRegisterMigrations() error {
	entries, err := files.ReadDir(".")
	if err != nil {
		return err
	}

	migrationNames := make(map[string]struct{})

	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".go") {
			continue // Skip Go files
		}

		name := entry.Name()
		if strings.HasSuffix(name, ".up.sql") {
			base := strings.TrimSuffix(name, ".up.sql")
			migrationNames[base] = struct{}{}
		}
	}

	for base := range migrationNames {
		// Check for missing Down migration
		downFilename := base + ".down.sql"
		if _, err := files.Open(downFilename); err != nil {
			// Soft warning — don't crash
			fmt.Printf("⚠️  Warning: migration %s is missing Down file (%s)\n", base, downFilename)
		}

		registerSQLMigration(base)
	}

	return nil
}

// GetMigrationSQL reads the SQL file from the embedded file system
func GetMigrationSQL(filename string) (string, error) {
	data, err := files.ReadFile(filename)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// registerSQLMigration registers a SQL migration with the given base name
func registerSQLMigration(base string) {
	RegisterMigration(&migration{name: base})
}

// migration represents a SQL migration
type migration struct {
	name string
}

// Name returns the name of the migration
func (m *migration) Name() string {
	return m.name
}

// Up executes the Up migration SQL
func (m *migration) Up(dbConn *gorm.DB) error {
	sql, err := GetMigrationSQL(m.name + ".up.sql")
	if err != nil {
		return err
	}
	return dbConn.Exec(sql).Error
}

// Down executes the Down migration SQL
func (m *migration) Down(dbConn *gorm.DB) error {
	sql, err := GetMigrationSQL(m.name + ".down.sql")
	if err != nil {
		return err
	}
	return dbConn.Exec(sql).Error
}
