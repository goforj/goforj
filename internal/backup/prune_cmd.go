package backup

import (
	"context"
	"fmt"
	"os"
	"time"
)

// PruneCmd removes older completed backup sets.
type PruneCmd struct {
	Path   string `help:"Backup directory" default:".goforj/backups"`
	Keep   int    `help:"Number of completed backups to retain" default:"14"`
	DryRun bool   `help:"Print removals without deleting"`
}

// NewPruneCmd creates a backup prune command.
func NewPruneCmd() *PruneCmd { return &PruneCmd{Path: DefaultPath()} }

// Signature defines CLI metadata for the backup prune command.
func (*PruneCmd) Signature() string { return `name:"backup:prune" help:"Prune old database backups"` }

// Run prunes old backup sets according to the retention policy.
func (c *PruneCmd) Run() error {
	if repository, err := ConfiguredBackupRepository(); err != nil {
		return err
	} else if repository != nil {
		names, err := repository.List(context.Background(), "")
		if err != nil {
			return err
		}
		if c.Keep < 0 {
			return fmt.Errorf("backup retention must not be negative")
		}
		cut := len(names) - c.Keep
		if cut < 0 {
			cut = 0
		}
		for _, name := range names[:cut] {
			if !c.DryRun {
				if err := repository.Delete(context.Background(), name); err != nil {
					return err
				}
			}
			fmt.Println(name)
		}
		return nil
	}
	if os.Getenv("APP_BACKUP_KEEP_DAILY") != "" || os.Getenv("APP_BACKUP_KEEP_WEEKLY") != "" || os.Getenv("APP_BACKUP_KEEP_MONTHLY") != "" {
		removed, err := PrunePolicy(c.Path, DefaultRetentionPolicy(), time.Now().UTC(), c.DryRun)
		if err != nil {
			return err
		}
		for _, path := range removed {
			fmt.Println(path)
		}
		return nil
	}
	removed, err := Prune(c.Path, c.Keep, c.DryRun)
	if err != nil {
		return err
	}
	for _, path := range removed {
		fmt.Println(path)
	}
	return nil
}
