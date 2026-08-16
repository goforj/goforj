package forj

import (
	"fmt"
	"strings"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/envcontract"
	"github.com/goforj/goforj/internal/envfile"
)

// EnvInitCmd creates a private local environment from the committed project contract.
type EnvInitCmd struct{}

// Signature exposes local environment initialization as a source-aware framework command.
func (*EnvInitCmd) Signature() string {
	return `name:"env:init" help:"Create .env with fresh local secrets"`
}

// Run initializes the local environment without replacing an existing developer file.
func (*EnvInitCmd) Run() error {
	created, err := envcontract.Initialize(".")
	if err != nil {
		return err
	}
	if !created {
		console.Infof(".env already exists")
		return nil
	}
	console.Successf("Created .env with fresh local secrets")
	return nil
}

// EnvSetCmd stores one local value through a non-echoed terminal prompt.
type EnvSetCmd struct {
	Key        string `arg:"" help:"Environment key to set"`
	readSecret func(string) (string, error)
}

// Signature exposes local value entry without accepting secret material in argv.
func (*EnvSetCmd) Signature() string {
	return `name:"env:set" help:"Set a private local environment value"`
}

// NewEnvSetCmd creates the command with terminal-owned hidden input.
func NewEnvSetCmd() *EnvSetCmd {
	return &EnvSetCmd{readSecret: console.AskSecret}
}

// Run prompts without echo and refreshes the safe committed contracts after publication.
func (c *EnvSetCmd) Run() error {
	key := strings.TrimSpace(c.Key)
	if !envfile.IsValidKey(key) {
		return fmt.Errorf("environment key %q must contain only letters, digits, and underscores and cannot start with a digit", key)
	}
	if c.readSecret == nil {
		c.readSecret = console.AskSecret
	}
	value, err := c.readSecret(fmt.Sprintf("Value for %s", key))
	if err != nil {
		return err
	}
	if err := envcontract.SetLocal(".", key, value); err != nil {
		return err
	}
	console.Successf("Updated %s in .env", key)
	return nil
}

// EnvCheckCmd verifies that committed example and testing contracts are synchronized.
type EnvCheckCmd struct{}

// Signature exposes a read-only environment contract check suitable for CI.
func (*EnvCheckCmd) Signature() string {
	return `name:"env:check" help:"Check committed environment contracts"`
}

// Run reports success only when synchronization would produce no committed diff.
func (*EnvCheckCmd) Run() error {
	if err := envcontract.Check("."); err != nil {
		return err
	}
	console.Successf("Environment contracts are current")
	return nil
}
