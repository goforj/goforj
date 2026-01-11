package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/logger"
)

type MakeMigrationCmd struct {
	Name string `arg:"" help:"Name of the migration (e.g. AddUsersTable)"`

	logger *logger.AppLogger
}

func NewMakeMigrationCmd(logger *logger.AppLogger) *MakeMigrationCmd {
	return &MakeMigrationCmd{
		logger: logger,
	}
}

func (c *MakeMigrationCmd) Run() error {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return fmt.Errorf("migration name cannot be empty")
	}

	// Prepare timestamped base name
	timestamp := time.Now().Format("2006_01_02_150405")
	snake := snakeCase(name)
	baseName := fmt.Sprintf("%s_%s", timestamp, snake)

	migrationsDir := "internal/migrations"
	upPath := filepath.Join(migrationsDir, baseName+".up.sql")
	downPath := filepath.Join(migrationsDir, baseName+".down.sql")

	// Ensure migrations dir exists
	if err := os.MkdirAll(migrationsDir, os.ModePerm); err != nil {
		return err
	}

	// Create empty Up SQL file
	if err := os.WriteFile(upPath, []byte("-- Up migration\n"), 0644); err != nil {
		return err
	}

	// Create empty Down SQL file
	if err := os.WriteFile(downPath, []byte("-- Down migration\n"), 0644); err != nil {
		return err
	}

	fmt.Printf("%s generated %s\n", successMark(), upPath)
	fmt.Printf("%s generated %s\n", successMark(), downPath)

	return nil
}
