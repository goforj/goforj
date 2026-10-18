package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RestoreCmd restores a native backup set into its matching database drivers.
type RestoreCmd struct {
	PrepareTargets bool   `name:"prepare-targets" help:"Create missing selected S3 target buckets before a confirmed restore"`
	From           string `name:"from" required:"" help:"Backup set directory"`
	Resource       string `help:"Optional database connection name"`
	Portable       bool   `help:"Restore a database-neutral portable archive"`
	TargetDriver   string `help:"Target SQL driver for portable restore"`
	DryRun         bool   `help:"Print the restore plan without changing data"`
	Confirm        string `help:"Type restore-production to confirm destructive restore"`
}

// NewRestoreCmd creates a native backup restore command.
func NewRestoreCmd() *RestoreCmd { return &RestoreCmd{} }

// Signature defines CLI metadata for the backup restore command.
func (*RestoreCmd) Signature() string {
	return `name:"backup:restore" help:"Restore a native database backup"`
}

// Run restores the selected backup set after explicit confirmation.
func (c *RestoreCmd) Run() error {
	source, err := resolveBackupSource(context.Background(), c.From)
	if err != nil {
		return err
	}
	defer source.cleanup()
	c.From = source.path
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
		connection, err := ConnectionForResource(context.Background(), normalizeResourceName(c.Resource))
		if err != nil {
			return err
		}
		if c.TargetDriver != "" {
			connection.Driver = c.TargetDriver
		}
		sqlConnection, err := OpenSQLConnection(context.Background(), connection)
		if err != nil {
			return err
		}
		defer sqlConnection.DB.Close()
		migrationFingerprint, err := ProjectMigrationFingerprint(".")
		if err != nil {
			return err
		}
		if err := NewPortableService().Restore(context.Background(), c.From, sqlConnection.DB, sqlConnection.Dialect, migrationFingerprint); err != nil {
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
	if c.PrepareTargets {
		if c.Confirm != "restore-production" {
			return fmt.Errorf("target preparation requires --confirm restore-production")
		}
		if err := prepareObjectRestoreTargets(context.Background(), manifest, c.Resource); err != nil {
			return err
		}
	}
	if err := NewService().Restore(context.Background(), c.From, c.Resource, c.Confirm); err != nil {
		return err
	}
	fmt.Println("backup restored")
	return nil
}

// backupSource keeps a resolved path and its matching lifecycle cleanup together.
type backupSource struct {
	path    string
	cleanup func()
}

// resolveBackupSource downloads a named remote backup into a temporary directory when configured.
func resolveBackupSource(ctx context.Context, from string) (backupSource, error) {
	if _, err := os.Stat(from); err == nil {
		return backupSource{path: from, cleanup: func() {}}, nil
	} else if !os.IsNotExist(err) {
		return backupSource{}, fmt.Errorf("inspect backup source %q: %w", from, err)
	}
	repository, err := ConfiguredBackupRepository()
	if err != nil {
		return backupSource{}, err
	}
	if repository == nil || from != filepath.Base(from) {
		return backupSource{path: from, cleanup: func() {}}, nil
	}
	destination, err := os.MkdirTemp("", "goforj-restore-")
	if err != nil {
		return backupSource{}, err
	}
	if err := repository.Download(ctx, from, destination); err != nil {
		_ = os.RemoveAll(destination)
		return backupSource{}, err
	}
	return backupSource{path: destination, cleanup: func() { _ = os.RemoveAll(destination) }}, nil
}

// prepareObjectRestoreTargets validates every selected bucket against the current App before provisioning any.
func prepareObjectRestoreTargets(ctx context.Context, manifest Manifest, resourceName string) error {
	plan, err := BuildPlan()
	if err != nil {
		return err
	}
	resourceName = normalizeResourceName(resourceName)
	var targets []string
	for _, resource := range manifest.Resources {
		if resource.Strategy != "object-archive" || (resourceName != "" && resourceName != resource.ID && resourceName != resource.Name) {
			continue
		}
		target := findStoragePlan(plan.Storage, resource.Name)
		if target == nil {
			return fmt.Errorf("restore %s: storage configuration not found", resource.ID)
		}
		if resource.Kind != "storage" || resource.Driver != "s3" || target.Driver != "s3" {
			return fmt.Errorf("restore %s requires a matching S3 storage target", resource.ID)
		}
		targets = append(targets, resource.Name)
	}
	for _, name := range targets {
		if err := PrepareObjectRestore(ctx, name); err != nil {
			return err
		}
	}
	return nil
}
