package makeapp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"golang.org/x/term"
)

var errAppTargetCreationCancelled = errors.New("app target creation cancelled")
var errAppTargetNameRequired = errors.New("app target name is required")

// appTargetExistsError carries the colliding path so Run can print useful no-op guidance.
type appTargetExistsError struct {
	target project.AppTarget
	path   string
}

func (e appTargetExistsError) Error() string {
	return fmt.Sprintf("app target %q already has files at %s", e.target.Name, e.path)
}

// isInteractiveTerminal is replaceable in tests because wizard behavior depends on TTY state.
var isInteractiveTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// appTargetWizardRunner is replaceable in tests so cancellation and selection paths do not need a real TUI.
var appTargetWizardRunner = runAppTargetWizard

// RenderOptions controls the narrow render used by make:app.
type RenderOptions struct {
	Components project.Components
	StarterKit project.StarterKit
	SkipWire   bool
}

// RemoveResult describes what changed during a make:app removal.
type RemoveResult struct {
	Removed []string
	Updated []string
}

// Changed reports whether the removal touched files or project metadata.
func (r RemoveResult) Changed() bool {
	return len(r.Removed) > 0 || len(r.Updated) > 0
}

// Renderer is the small project renderer surface needed by make:app.
type Renderer interface {
	RenderAppTargetOnly(project.AppTarget, RenderOptions) error
	RemoveAppTarget(project.AppTarget) (RemoveResult, error)
}

// Cmd creates an additional conventional app target in an existing project.
type Cmd struct {
	logger   *logger.AppLogger
	renderer Renderer

	Name       string `arg:"" optional:"" help:"App target name, such as billing or customer-portal"`
	Components string `name:"components" help:"Comma-separated target components, such as web-api,jobs"`
	Without    string `name:"without" help:"Comma-separated target components to remove from the default app-slice selection"`
	StarterKit string `name:"starter-kit" help:"Frontend starter kit for targets with Web UI"`
	SkipWire   bool   `name:"skip-wire" help:"Render files without running Wire generation"`
	Remove     bool   `name:"remove" help:"Remove the conventional files and metadata for an app target"`
}

// Signature exposes make:app as a framework-owned target creation command.
func (*Cmd) Signature() string {
	return `name:"make:app" help:"Create a new app target"`
}

// NewCmd creates a new Cmd.
func NewCmd(logger *logger.AppLogger, renderer Renderer) *Cmd {
	return &Cmd{logger: logger, renderer: renderer}
}

// Help shows the target creation flow.
func (*Cmd) Help() string {
	return strings.Join([]string{
		"Examples:",
		"  forj make:app billing",
		"  forj make:app billing --components web-api,jobs",
		"  forj make:app portal --components web-api,web-ui --starter-kit vue",
		"  forj make:app customer-portal --without web-ui --skip-wire",
		"  forj make:app billing --remove",
		"",
	}, "\n")
}

// Run validates the target name and renders only the files required for the new target.
func (c *Cmd) Run() error {
	target, err := c.target()
	if err != nil {
		if errors.Is(err, errAppTargetNameRequired) {
			c.printMissingNameHelp()
			return nil
		}
		return err
	}
	if c.Remove {
		return c.removeTarget(target)
	}
	if err := c.ensureTargetDoesNotExist(target); err != nil {
		var exists appTargetExistsError
		if errors.As(err, &exists) {
			c.printExistingTargetHelp(exists)
			return nil
		}
		return err
	}

	config, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}
	components, starterKit, err := c.targetSelection(config)
	if err != nil {
		if errors.Is(err, errAppTargetCreationCancelled) {
			return nil
		}
		return err
	}

	if c.logger != nil {
		c.logger.Info().Str("target", target.Name).Msg("Creating app target")
	}
	if err := c.renderer.RenderAppTargetOnly(target, RenderOptions{
		Components: components,
		StarterKit: starterKit,
		SkipWire:   c.SkipWire,
	}); err != nil {
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

// removeTarget removes the target through the renderer so config and runtime metadata stay in sync.
func (c *Cmd) removeTarget(target project.AppTarget) error {
	result, err := c.renderer.RemoveAppTarget(target)
	if err != nil {
		return err
	}
	if !result.Changed() {
		console.Infof("App target not found: %s", target.Name)
		console.Infof("Nothing removed.")
		return nil
	}

	console.Successf("Removed app target: %s", target.Name)
	for _, path := range result.Removed {
		console.Infof("Removed: %s", path)
	}
	for _, path := range result.Updated {
		console.Infof("Updated: %s", path)
	}
	return nil
}

// targetSelection resolves the per-target component and starter-kit choices.
func (c *Cmd) targetSelection(config *project.Config) (project.Components, project.StarterKit, error) {
	available := config.Render.Components
	if c.shouldRunWizard() {
		if !isInteractiveTerminal() {
			return project.Components{}, project.StarterKitNone, fmt.Errorf("app target wizard requires an interactive terminal")
		}
		components, starterKit, cancelled, err := appTargetWizardRunner(c.Name, config)
		if err != nil {
			return project.Components{}, project.StarterKitNone, err
		}
		if cancelled {
			return project.Components{}, project.StarterKitNone, errAppTargetCreationCancelled
		}
		return components, starterKit, nil
	}
	components := project.TargetDefaultComponents(available)
	if strings.TrimSpace(c.Components) != "" {
		keys, err := project.ParseComponentKeys(c.Components)
		if err != nil {
			return project.Components{}, project.StarterKitNone, err
		}
		components, err = project.TargetComponentsFromKeys(available, keys)
		if err != nil {
			return project.Components{}, project.StarterKitNone, err
		}
	}
	if strings.TrimSpace(c.Without) != "" {
		keys, err := project.ParseComponentKeys(c.Without)
		if err != nil {
			return project.Components{}, project.StarterKitNone, err
		}
		for _, key := range keys {
			if !project.IsTargetComponentKey(key) {
				definition, _ := project.ComponentDefinitionByKey(key)
				return project.Components{}, project.StarterKitNone, fmt.Errorf("%s is project-level only and cannot be removed per app target", definition.Label)
			}
			components.SetEnabled(key, false)
		}
		components = project.NormalizeTargetComponents(available, components)
	}

	starterKit := project.NormalizeStarterKit(project.StarterKit(c.StarterKit))
	if strings.TrimSpace(c.StarterKit) == "" {
		starterKit = config.Render.StarterKit
	}
	if !components.WebUI {
		starterKit = project.StarterKitNone
	}
	if err := project.ValidateStarterKitContract(starterKit, components); err != nil {
		return project.Components{}, project.StarterKitNone, err
	}
	if err := components.ValidateRenderContract(); err != nil {
		return project.Components{}, project.StarterKitNone, err
	}
	return components, starterKit, nil
}

// shouldRunWizard keeps the default flow interactive while allowing explicit flags to stay scriptable.
func (c *Cmd) shouldRunWizard() bool {
	if c.hasExplicitSelection() {
		return false
	}
	return isInteractiveTerminal()
}

// hasExplicitSelection reports whether the caller has already provided enough input to skip the TUI.
func (c *Cmd) hasExplicitSelection() bool {
	return strings.TrimSpace(c.Components) != "" ||
		strings.TrimSpace(c.Without) != "" ||
		strings.TrimSpace(c.StarterKit) != ""
}

// printMissingNameHelp keeps the no-arg command path instructional instead of error-styled.
func (c *Cmd) printMissingNameHelp() {
	console.Infof("App target name is required")
	console.Infof("Usage: forj make:app <name>")
	console.Infof("Example: forj make:app billing")
}

// printExistingTargetHelp explains the no-op when the conventional target files already exist.
func (c *Cmd) printExistingTargetHelp(err appTargetExistsError) {
	console.Infof("App target already exists: %s", err.target.Name)
	console.Infof("Existing path: %s", err.path)
	console.Infof("Run forj render to refresh an existing target.")
}

// target resolves the command input into the conventional target paths used by render and dev.
func (c *Cmd) target() (project.AppTarget, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return project.AppTarget{}, errAppTargetNameRequired
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
func (c *Cmd) ensureTargetDoesNotExist(target project.AppTarget) error {
	for _, path := range []string{
		target.Entrypoint,
		target.AppDir,
		target.WireDir,
		filepath.Dir(target.Entrypoint),
	} {
		if _, err := os.Stat(path); err == nil {
			return appTargetExistsError{target: target, path: path}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
