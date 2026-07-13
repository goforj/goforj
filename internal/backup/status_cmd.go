package backup

import (
	"fmt"
	"time"
)

// StatusCmd reports the freshness of the newest local backup.
type StatusCmd struct {
	Path string `help:"Backup directory" default:".goforj/backups"`
}

// NewStatusCmd creates a backup status command.
func NewStatusCmd() *StatusCmd { return &StatusCmd{Path: DefaultPath()} }

// Signature defines CLI metadata for the backup status command.
func (*StatusCmd) Signature() string { return `name:"backup:status" help:"Show backup freshness"` }

// Run prints the newest local backup status.
func (c *StatusCmd) Run() error {
	status, err := ReadStatus(c.Path, time.Now().UTC())
	if err != nil {
		return err
	}
	fmt.Println(FormatStatus(status))
	return nil
}
