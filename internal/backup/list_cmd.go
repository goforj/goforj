package backup

import (
	"context"
	"fmt"
)

// ListCmd lists completed manifest-backed backup sets.
type ListCmd struct {
	Path string `help:"Backup directory" default:".goforj/backups"`
}

// NewListCmd creates a backup list command.
func NewListCmd() *ListCmd { return &ListCmd{Path: DefaultPath()} }

// Signature defines CLI metadata for the backup list command.
func (*ListCmd) Signature() string { return `name:"backup:list" help:"List database backups"` }

// Run lists completed backup sets in newest-first order.
func (c *ListCmd) Run() error {
	if repository, err := ConfiguredBackupRepository(); err != nil {
		return err
	} else if repository != nil {
		names, err := repository.List(context.Background(), "")
		if err != nil {
			return err
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	}
	manifests, err := List(c.Path)
	if err != nil {
		return err
	}
	for _, manifest := range manifests {
		fmt.Printf("%s\t%d resources\n", manifest.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"), len(manifest.Resources))
	}
	return nil
}
