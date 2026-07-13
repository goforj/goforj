package backup

import (
	"context"
	"fmt"
)

// VerifyCmd verifies a backup set and all referenced artifact checksums.
type VerifyCmd struct {
	From string `name:"from" required:"" help:"Backup set directory"`
}

// NewVerifyCmd creates a backup verification command.
func NewVerifyCmd() *VerifyCmd { return &VerifyCmd{} }

// Signature defines CLI metadata for the backup verification command.
func (*VerifyCmd) Signature() string { return `name:"backup:verify" help:"Verify a database backup"` }

// Run verifies the selected backup set.
func (c *VerifyCmd) Run() error {
	from, cleanup, err := resolveBackupSource(context.Background(), c.From)
	if err != nil {
		return err
	}
	defer cleanup()
	if _, err := NewService().Verify(from); err != nil {
		return err
	}
	fmt.Println("backup verified")
	return nil
}
