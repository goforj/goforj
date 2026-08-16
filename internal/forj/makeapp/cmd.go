package makeapp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"golang.org/x/term"
)

var errAppCreationCancelled = errors.New("app creation cancelled")
var errAppNameRequired = errors.New("app name is required")

// appExistsError carries the colliding path so Run can print useful no-op guidance.
type appExistsError struct {
	app  project.App
	path string
}

// Error returns the command failure in a user-facing form.
func (e appExistsError) Error() string {
	return fmt.Sprintf("app %q already has files at %s", e.app.Name, e.path)
}

// isInteractiveTerminal is replaceable in tests because wizard behavior depends on TTY state.
var isInteractiveTerminal = func() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// appWizardRunner is replaceable in tests so cancellation and selection paths do not need a real TUI.
var appWizardRunner = runAppWizard

// RenderOptions controls the narrow render used by make:app.
type RenderOptions struct {
	Components        project.Components
	StarterKit        project.StarterKit
	StarterKitOptions *project.StarterKitOptions
	HelpFormat        project.HelpFormat
	DevRunCommand     string
	SkipWire          bool
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
	// RenderAppOnly defines the render app only behavior required from implementations.
	RenderAppOnly(project.App, RenderOptions) error
	// RemoveApp defines the remove app behavior required from implementations.
	RemoveApp(project.App) (RemoveResult, error)
}

// Cmd creates an additional conventional app in an existing project.
type Cmd struct {
	logger   *logger.AppLogger
	renderer Renderer

	Name             string `arg:"" optional:"" help:"App name, such as admin or statuspage"`
	Components       string `name:"components" help:"Comma-separated app components, such as web-api,jobs"`
	Without          string `name:"without" help:"Comma-separated app components to remove from the default app selection"`
	StarterKit       string `name:"starter-kit" help:"Frontend starter kit for apps with Web UI"`
	ComponentLibrary string `name:"component-library" help:"Include the selected starter kit's component showcase: on or off"`
	HelpFormat       string `name:"help-format" help:"Help output format for app CLI commands: framework, external_cli, or guided"`
	DevRun           string `name:"dev-run" help:"Command for forj dev to run through this app binary, such as run or queue:work"`
	SkipWire         bool   `name:"skip-wire" help:"Render files without running Wire generation"`
	Remove           bool   `name:"remove" help:"Remove the conventional files and metadata for an app"`
}

// Signature exposes make:app as a framework-owned app creation command.
func (*Cmd) Signature() string {
	return `name:"make:app" help:"Create a new app"`
}

// NewCmd creates a new Cmd.
func NewCmd(logger *logger.AppLogger, renderer Renderer) *Cmd {
	return &Cmd{logger: logger, renderer: renderer}
}

// Help shows the app creation flow.
func (*Cmd) Help() string {
	return strings.Join([]string{
		"Examples:",
		"  forj make:app admin",
		"  forj make:app admin --components web-api,jobs",
		"  forj make:app statuspage --components web-api,web-ui --starter-kit vue",
		"  forj make:app statuspage --without jobs --skip-wire",
		"  forj make:app admin --remove",
		"",
	}, "\n")
}

// Run validates the app name and renders only the files required for the new app.
func (c *Cmd) Run() error {
	app, err := c.app()
	if err != nil {
		if errors.Is(err, errAppNameRequired) {
			c.printMissingNameHelp()
			return nil
		}
		return err
	}
	if c.Remove {
		return c.removeApp(app)
	}
	if err := c.ensureAppDoesNotExist(app); err != nil {
		var exists appExistsError
		if errors.As(err, &exists) {
			c.printExistingAppHelp(exists)
			return nil
		}
		return err
	}

	config, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}
	renderOptions, err := c.appSelection(config)
	if err != nil {
		if errors.Is(err, errAppCreationCancelled) {
			return nil
		}
		return err
	}

	c.logger.Info().Str("app", app.Name).Msg("Creating app")
	renderOptions.SkipWire = c.SkipWire
	if err := c.renderer.RenderAppOnly(app, renderOptions); err != nil {
		return err
	}

	console.Successf("Created app: %s", app.Name)
	console.Infof("Entrypoint: %s", app.Entrypoint)
	console.Infof("Composition: %s", app.AppDir)
	if c.SkipWire {
		console.Infof("Review the new App, then run forj render or wire.")
	}
	return nil
}

// removeApp removes the app through the renderer so config and runtime metadata stay in sync.
func (c *Cmd) removeApp(app project.App) error {
	result, err := c.renderer.RemoveApp(app)
	if err != nil {
		return err
	}
	if !result.Changed() {
		console.Infof("App not found: %s", app.Name)
		console.Infof("Nothing removed.")
		return nil
	}

	console.Successf("Removed app: %s", app.Name)
	for _, path := range result.Removed {
		console.Infof("Removed: %s", path)
	}
	for _, path := range result.Updated {
		console.Infof("Updated: %s", path)
	}
	return nil
}

// appSelection resolves one complete set of per-App render and development choices.
func (c *Cmd) appSelection(config *project.Config) (RenderOptions, error) {
	available := config.Render.Components
	if c.shouldRunWizard() {
		return appWizardRunner(c.Name, config)
	}
	components := project.AppDefaultComponents(available)
	if strings.TrimSpace(c.Components) != "" {
		keys, err := project.ParseComponentKeys(c.Components)
		if err != nil {
			return RenderOptions{}, err
		}
		components, err = project.AppComponentsFromKeys(available, keys)
		if err != nil {
			return RenderOptions{}, err
		}
	}
	if strings.TrimSpace(c.Without) != "" {
		keys, err := project.ParseComponentKeys(c.Without)
		if err != nil {
			return RenderOptions{}, err
		}
		for _, key := range keys {
			if key == project.ComponentCLI {
				return RenderOptions{}, fmt.Errorf("CLI is always enabled for apps and cannot be removed per app")
			}
			if !project.IsAppComponentKey(key) {
				definition, _ := project.ComponentDefinitionByKey(key)
				return RenderOptions{}, fmt.Errorf("%s is project-level only and cannot be removed per app", definition.Label)
			}
			project.DeselectAppComponent(&components, key)
		}
		components = project.NormalizeAppComponents(available, components)
	}

	starterKit := project.NormalizeStarterKit(project.StarterKit(c.StarterKit))
	if strings.TrimSpace(c.StarterKit) == "" {
		starterKit = config.Render.StarterKit
	}
	if !components.WebUI {
		starterKit = project.StarterKitNone
	}
	if err := project.ValidateStarterKitContract(starterKit, components); err != nil {
		return RenderOptions{}, err
	}
	componentLibrary := config.Render.StarterKitOptions.ComponentLibraryEnabled()
	if strings.TrimSpace(c.ComponentLibrary) != "" {
		switch strings.ToLower(strings.TrimSpace(c.ComponentLibrary)) {
		case "on":
			componentLibrary = true
		case "off":
			componentLibrary = false
		default:
			return RenderOptions{}, fmt.Errorf("invalid component library %q; use on or off", c.ComponentLibrary)
		}
	}
	var starterKitOptions *project.StarterKitOptions
	if starterKit != project.StarterKitNone && !componentLibrary {
		starterKitOptions = project.NewStarterKitOptions(false)
	}
	if err := components.ValidateRenderContract(); err != nil {
		return RenderOptions{}, err
	}
	helpFormat := project.NormalizeHelpFormat(project.HelpFormat(c.HelpFormat))
	if strings.TrimSpace(c.HelpFormat) == "" {
		helpFormat = project.NormalizeHelpFormat(config.Render.HelpFormat)
	}
	return RenderOptions{
		Components:        components,
		StarterKit:        starterKit,
		StarterKitOptions: starterKitOptions,
		HelpFormat:        helpFormat,
		DevRunCommand:     strings.TrimSpace(c.DevRun),
	}, nil
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
		strings.TrimSpace(c.StarterKit) != "" ||
		strings.TrimSpace(c.ComponentLibrary) != "" ||
		strings.TrimSpace(c.HelpFormat) != "" ||
		strings.TrimSpace(c.DevRun) != ""
}

// printMissingNameHelp keeps the no-arg command path instructional instead of error-styled.
func (c *Cmd) printMissingNameHelp() {
	console.Infof("App name is required")
	console.Infof("Usage: forj make:app <name>")
	console.Infof("Example: forj make:app billing")
}

// printExistingAppHelp explains the no-op when the conventional app files already exist.
func (c *Cmd) printExistingAppHelp(err appExistsError) {
	console.Infof("App already exists: %s", err.app.Name)
	console.Infof("Existing path: %s", err.path)
	console.Infof("Run forj render to refresh an existing app.")
}

// app resolves the command input into the conventional app paths used by render and dev.
func (c *Cmd) app() (project.App, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return project.App{}, errAppNameRequired
	}
	if name == project.DefaultAppName {
		return project.App{}, fmt.Errorf("app %q already exists as the default app", name)
	}
	if !project.IsSafeAppName(name) {
		return project.App{}, fmt.Errorf("invalid app name %q; use a lowercase kebab-case slug such as admin or statuspage", name)
	}
	if project.IsReservedAppName(name) {
		return project.App{}, fmt.Errorf("app name %q is reserved by the app layout", name)
	}
	if project.IsNativeFrameworkCommandName(name) {
		return project.App{}, fmt.Errorf("app name %q conflicts with a native forj command", name)
	}
	return project.AppForName(name), nil
}

// ensureAppDoesNotExist prevents make:app from overwriting an app owner-created app.
func (c *Cmd) ensureAppDoesNotExist(app project.App) error {
	for _, path := range []string{
		app.Entrypoint,
		app.AppDir,
		app.WireDir,
		filepath.Dir(app.Entrypoint),
	} {
		exists, err := appPathHasUserContent(path)
		if err != nil {
			return err
		}
		if exists {
			return appExistsError{app: app, path: path}
		}
	}
	return nil
}

// appPathHasUserContent treats empty leftover directories as reusable because Git deletion commonly leaves them behind.
func appPathHasUserContent(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return true, nil
	}
	hasContent := false
	err = filepath.WalkDir(path, func(candidate string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if candidate == path {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		hasContent = true
		return filepath.SkipAll
	})
	if err != nil {
		return false, err
	}
	return hasContent, nil
}
