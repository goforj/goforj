package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// MakeAppCmd creates an additional conventional app target in an existing project.
type MakeAppCmd struct {
	logger   *logger.AppLogger
	renderer *ProjectRenderer

	Name     string `arg:"" help:"App target name, such as billing or customer-portal"`
	SkipWire bool   `name:"skip-wire" help:"Render files without running Wire generation"`
}

// Signature exposes make:app as a framework-owned target creation command.
func (*MakeAppCmd) Signature() string {
	return `name:"make:app" help:"Create a new app target"`
}

// NewMakeAppCmd creates a new MakeAppCmd.
func NewMakeAppCmd(logger *logger.AppLogger, renderer *ProjectRenderer) *MakeAppCmd {
	return &MakeAppCmd{logger: logger, renderer: renderer}
}

// Help shows the intentionally small target creation flow.
func (*MakeAppCmd) Help() string {
	return "Examples:\n  forj make:app billing\n  forj make:app customer-portal --skip-wire\n"
}

// Run validates the target name and renders only the files required for the new target.
func (c *MakeAppCmd) Run() error {
	target, err := c.target()
	if err != nil {
		return err
	}
	if err := c.ensureTargetDoesNotExist(target); err != nil {
		return err
	}

	if c.logger != nil {
		c.logger.Info().Str("target", target.Name).Msg("Creating app target")
	}
	if err := c.renderer.RenderAppTargetOnly(target, AppTargetRenderOptions{SkipWire: c.SkipWire}); err != nil {
		return err
	}

	console.Successf("Created app target: %s", target.Name)
	console.Infof("Entrypoint: %s", target.Entrypoint)
	console.Infof("Composition: %s", target.AppDir)
	if c.SkipWire {
		console.Infof("Run forj render or wire after reviewing the generated target.")
	}
	return nil
}

// target resolves the command input into the conventional target paths used by render and dev.
func (c *MakeAppCmd) target() (project.AppTarget, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return project.AppTarget{}, fmt.Errorf("app target name is required")
	}
	if name == project.DefaultAppTargetName {
		return project.AppTarget{}, fmt.Errorf("app target %q already exists as the default target", name)
	}
	if !project.IsSafeAppTargetName(name) {
		return project.AppTarget{}, fmt.Errorf("invalid app target name %q; use a path-safe slug such as billing or customer-portal", name)
	}
	if project.IsReservedAppTargetName(name) {
		return project.AppTarget{}, fmt.Errorf("app target name %q is reserved by the app layout", name)
	}
	if project.IsNativeFrameworkCommandName(name) {
		return project.AppTarget{}, fmt.Errorf("app target name %q conflicts with a native forj command", name)
	}
	return project.DefaultNamedAppTarget(name), nil
}

// ensureTargetDoesNotExist prevents make:app from overwriting an app owner-created target.
func (c *MakeAppCmd) ensureTargetDoesNotExist(target project.AppTarget) error {
	for _, path := range []string{
		target.Entrypoint,
		target.AppDir,
		target.WireDir,
		filepath.Dir(target.Entrypoint),
	} {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("app target %q already has files at %s; run forj render to refresh an existing target", target.Name, path)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
