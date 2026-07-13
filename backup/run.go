// Package backup exposes the framework backup command runner to generated Apps.
package backup

import (
	"fmt"

	"github.com/alecthomas/kong"
	internalbackup "github.com/goforj/goforj/internal/backup"
)

// Commands contains the framework backup command surface for generated Apps.
type Commands struct {
	PlanCmd    internalbackup.PlanCmd    `cmd:""`
	ListCmd    internalbackup.ListCmd    `cmd:""`
	CreateCmd  internalbackup.CreateCmd  `cmd:""`
	VerifyCmd  internalbackup.VerifyCmd  `cmd:""`
	RestoreCmd internalbackup.RestoreCmd `cmd:""`
	PruneCmd   internalbackup.PruneCmd   `cmd:""`
	StatusCmd  internalbackup.StatusCmd  `cmd:""`
}

// Run executes one framework backup command using the generated App's environment.
func Run(args ...string) error {
	parser, err := kong.New(&Commands{
		PlanCmd:    *internalbackup.NewPlanCmd(),
		ListCmd:    *internalbackup.NewListCmd(),
		CreateCmd:  *internalbackup.NewCreateCmd(),
		VerifyCmd:  *internalbackup.NewVerifyCmd(),
		RestoreCmd: *internalbackup.NewRestoreCmd(),
		PruneCmd:   *internalbackup.NewPruneCmd(),
		StatusCmd:  *internalbackup.NewStatusCmd(),
	})
	if err != nil {
		return fmt.Errorf("setup backup command parser: %w", err)
	}
	context, err := parser.Parse(args)
	if err != nil {
		return err
	}
	return context.Run()
}
