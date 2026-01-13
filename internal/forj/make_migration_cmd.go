package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/str"
)

type MakeMigrationCmd struct {
	Name       string `arg:"" help:"Name of the migration (e.g. AddUsersTable)"`
	Connection string `help:"Database connection name" default:"default"`

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

	connName := str.Of(c.Connection).TrimSpace().ToLower().String()
	if connName == "" {
		connName = "default"
	}

	migrationsDir := "internal/migrations"
	if connName != "default" {
		migrationsDir = filepath.Join(migrationsDir, connName)
	}
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

	console.Successf("generated %s", upPath)
	console.Successf("generated %s", downPath)

	return nil
}
