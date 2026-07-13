package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RestoreCmd restores a native backup set into its matching database drivers.
type RestoreCmd struct {
	From         string `name:"from" required:"" help:"Backup set directory"`
	Resource     string `help:"Optional database connection name"`
	Portable     bool   `help:"Restore a database-neutral portable archive"`
	TargetDriver string `help:"Target SQL driver for portable restore"`
	DryRun       bool   `help:"Print the restore plan without changing data"`
	Confirm      string `help:"Type restore-production to confirm destructive restore"`
}

// NewRestoreCmd creates a native backup restore command.
func NewRestoreCmd() *RestoreCmd { return &RestoreCmd{} }

// Signature defines CLI metadata for the backup restore command.
func (*RestoreCmd) Signature() string {
	return `name:"backup:restore" help:"Restore a native database backup"`
}

// Run restores the selected backup set after explicit confirmation.
func (c *RestoreCmd) Run() error {
	from, cleanup, err := resolveBackupSource(context.Background(), c.From)
	if err != nil {
		return err
	}
	defer cleanup()
	c.From = from
	if c.Portable {
		if c.DryRun {
			if _, err := ReadPortableArchive(c.From); err != nil {
				return err
			}
			fmt.Printf("portable restore from %s target-driver=%s\n", c.From, c.TargetDriver)
			return nil
		}
		if c.Confirm != "restore-production" {
			return fmt.Errorf("restore requires --confirm restore-production")
		}
		connection := ConnectionFromEnv(normalizeResourceName(c.Resource))
		if c.TargetDriver != "" {
			connection.Driver = c.TargetDriver
		}
		db, dialect, err := OpenSQLConnection(context.Background(), connection)
		if err != nil {
			return err
		}
		defer db.Close()
		migrationFingerprint, err := ProjectMigrationFingerprint(".")
		if err != nil {
			return err
		}
		if err := NewPortableService().Restore(context.Background(), c.From, db, dialect, migrationFingerprint); err != nil {
			return err
		}
		fmt.Println("portable backup restored")
		return nil
	}
	manifest, err := NewService().Verify(c.From)
	if err != nil {
		return err
	}
	for _, resource := range manifest.Resources {
		if c.Resource == "" || c.Resource == resource.Name {
			fmt.Printf("restore %s driver=%s artifact=%s\n", resource.ID, resource.Driver, resource.Artifact)
		}
	}
	if c.DryRun {
		return nil
	}
	if err := NewService().Restore(context.Background(), c.From, c.Resource, c.Confirm); err != nil {
		return err
	}
	fmt.Println("backup restored")
	return nil
}

// resolveBackupSource downloads a named remote backup into a temporary directory when configured.
func resolveBackupSource(ctx context.Context, from string) (string, func(), error) {
	if _, err := os.Stat(from); err == nil {
		return from, func() {}, nil
	}
	repository, err := ConfiguredBackupRepository()
	if err != nil {
		return "", nil, err
	}
	if repository == nil || from != filepath.Base(from) {
		return from, func() {}, nil
	}
	destination, err := os.MkdirTemp("", "goforj-restore-")
	if err != nil {
		return "", nil, err
	}
	if err := repository.Download(ctx, from, destination); err != nil {
		_ = os.RemoveAll(destination)
		return "", nil, err
	}
	return destination, func() { _ = os.RemoveAll(destination) }, nil
}
