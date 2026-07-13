package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CreateCmd creates a native database backup set.
type CreateCmd struct {
	Path     string `name:"to" help:"Backup directory" default:".goforj/backups"`
	Resource string `help:"Optional database connection name"`
	Portable bool   `help:"Create a database-neutral portable archive"`
}

// NewCreateCmd creates a native backup creation command.
func NewCreateCmd() *CreateCmd { return &CreateCmd{Path: DefaultPath()} }

// Signature defines CLI metadata for the backup creation command.
func (*CreateCmd) Signature() string {
	return `name:"backup:create" help:"Create a native database backup"`
}

// Run creates and reports a native backup set.
func (c *CreateCmd) Run() error {
	if c.Portable {
		connection, err := ConnectionForResource(context.Background(), normalizeResourceName(c.Resource))
		if err != nil {
			return err
		}
		db, dialect, err := OpenSQLConnection(context.Background(), connection)
		if err != nil {
			return err
		}
		defer db.Close()
		tables, err := ListTables(context.Background(), db, dialect)
		if err != nil {
			return err
		}
		migrationFingerprint, err := ProjectMigrationFingerprint(".")
		if err != nil {
			return err
		}
		dir := filepath.Join(c.Path, "portable-"+time.Now().UTC().Format("20060102T150405Z"))
		archive, err := NewPortableService().Create(context.Background(), dir, db, dialect, tables, migrationFingerprint)
		if err != nil {
			return err
		}
		if err := uploadCompletedBackup(context.Background(), dir); err != nil {
			return err
		}
		fmt.Printf("portable backup complete %s (%d tables)\n", dir, len(archive.Tables))
		return nil
	}
	dir, manifest, err := NewService().Create(context.Background(), c.Path, c.Resource)
	if err != nil {
		return err
	}
	if err := uploadCompletedBackup(context.Background(), dir); err != nil {
		return err
	}
	fmt.Printf("backup complete %s (%d resources)\n", dir, len(manifest.Resources))
	return nil
}

// uploadCompletedBackup publishes a verified backup when a remote repository is configured.
func uploadCompletedBackup(ctx context.Context, dir string) error {
	repository, err := ConfiguredBackupRepository()
	if err != nil {
		return err
	}
	if repository == nil {
		return nil
	}
	if _, err := NewService().Verify(dir); err != nil {
		return fmt.Errorf("verify backup before repository upload: %w", err)
	}
	if err := repository.Upload(ctx, filepath.Base(dir), dir); err != nil {
		return fmt.Errorf("upload backup repository: %w", err)
	}
	return nil
}

// DefaultPath returns the configured backup path or the framework default.
func DefaultPath() string {
	if path := os.Getenv("BACKUP_PATH"); path != "" {
		return path
	}
	return ".goforj/backups"
}
