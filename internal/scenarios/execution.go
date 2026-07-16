package scenarios

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// scenarioWorkspace owns the isolated directory lifecycle for one executable scenario.
type scenarioWorkspace struct {
	root        string
	removeAfter bool
}

// scenarioExecution keeps the invariant collaborators for every dependency, step, and verification command in one run.
type scenarioExecution struct {
	logger              *logger.AppLogger
	workspace           scenarioWorkspace
	forjExec            string
	catalog             scenarioCatalog
	appliedDependencies map[string]bool
}

const scenarioCommandTimeout = 3 * time.Minute

// runScenario executes one selected scenario against the same validated catalog used to select it.
func runScenario(options ValidateOptions, catalog scenarioCatalog, spec ScenarioSpec, forjExec string) error {
	workspace, err := createScenarioWorkspace(options, spec)
	if err != nil {
		return err
	}

	execution := scenarioExecution{
		logger:              options.Logger,
		workspace:           workspace,
		forjExec:            forjExec,
		catalog:             catalog,
		appliedDependencies: map[string]bool{},
	}
	console.Actionf("scenario %s", spec.ID)
	runErr := workspace.cleanupAfter(execution.run(spec))
	if options.Keep {
		console.Infof("scenario workdir: %s", workspace.root)
	}
	if runErr != nil {
		return runErr
	}
	console.Successf("scenario passed: %s", spec.ID)
	return nil
}

// createScenarioWorkspace creates an isolated directory only after the scenario ID is known to be path-safe.
func createScenarioWorkspace(options ValidateOptions, spec ScenarioSpec) (scenarioWorkspace, error) {
	if err := validateScenarioID(spec.ID); err != nil {
		return scenarioWorkspace{}, err
	}
	if strings.TrimSpace(options.WorkDir) != "" {
		if err := os.MkdirAll(options.WorkDir, 0o755); err != nil {
			return scenarioWorkspace{}, fmt.Errorf("create scenario work root: %w", err)
		}
		root, err := os.MkdirTemp(options.WorkDir, spec.ID+"-")
		if err != nil {
			return scenarioWorkspace{}, fmt.Errorf("create scenario workspace: %w", err)
		}
		return scenarioWorkspace{root: root, removeAfter: !options.Keep}, nil
	}
	root, err := os.MkdirTemp("", "forj-scenario-"+spec.ID+"-")
	if err != nil {
		return scenarioWorkspace{}, err
	}
	return scenarioWorkspace{root: root, removeAfter: !options.Keep}, nil
}

// cleanupAfter removes an owned temporary workspace while retaining both scenario and cleanup failures for diagnosis.
func (workspace scenarioWorkspace) cleanupAfter(runErr error) error {
	if !workspace.removeAfter {
		return runErr
	}
	if err := os.RemoveAll(workspace.root); err != nil {
		cleanupErr := fmt.Errorf("remove temporary scenario workspace %q: %w", workspace.root, err)
		return errors.Join(runErr, cleanupErr)
	}
	return runErr
}

// run applies the rendered project, inherited steps, selected steps, and verification commands in their contract order.
func (execution scenarioExecution) run(spec ScenarioSpec) error {
	if err := writeScenarioProjectConfig(execution.workspace.root, spec); err != nil {
		return err
	}
	if err := execution.runCommand(ScenarioCommand{Run: []string{"forj", "render"}}, "render app"); err != nil {
		return err
	}
	if err := execution.applyDependencies(spec); err != nil {
		return err
	}
	for _, step := range spec.Steps {
		if err := execution.applyStep(spec, step); err != nil {
			return fmt.Errorf("%s step %q: %w", spec.ID, step.Title, err)
		}
	}
	for _, command := range spec.Verify.Commands {
		if err := execution.runCommand(command, "verify"); err != nil {
			return err
		}
	}
	return nil
}

// applyDependencies applies ancestors before dependents so cumulative scenarios reproduce the documented golden path exactly once.
func (execution scenarioExecution) applyDependencies(spec ScenarioSpec) error {
	for _, dependencyID := range spec.DependsOn {
		dependency, ok := execution.catalog.byID[dependencyID]
		if !ok {
			return fmt.Errorf("%s depends on unknown scenario %q", spec.ID, dependencyID)
		}
		if err := execution.applyDependencies(dependency); err != nil {
			return err
		}
		if execution.appliedDependencies[dependency.ID] {
			continue
		}
		for _, step := range dependency.Steps {
			if err := execution.applyStep(dependency, step); err != nil {
				return fmt.Errorf("%s dependency %s step %q: %w", spec.ID, dependency.ID, step.Title, err)
			}
		}
		execution.appliedDependencies[dependency.ID] = true
	}
	return nil
}

// writeScenarioProjectConfig relies on the canonical component sequence so intentionally absent primitives stay disabled.
func writeScenarioProjectConfig(root string, spec ScenarioSpec) error {
	config := project.Config{
		ProjectName:  spec.Title,
		GoModuleName: spec.App.ModuleName,
		UpdatedAt:    "2026-01-01 00:00:00 UTC",
		Render: project.RenderConfig{
			Components: spec.App.Components,
		},
		Dev: project.DevConfig{
			Pre:               []project.DevTask{},
			Down:              []project.DevTask{},
			DownOnExit:        false,
			SoundOnWatchError: false,
			Watches:           []project.DevWatch{},
			WirePaths:         []string{project.DefaultApp().WireDir},
		},
	}
	body, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".goforj.yml"), body, 0o644)
}

// applyStep preserves the declared action order because later scenario edits can depend on earlier generated content.
func (execution scenarioExecution) applyStep(spec ScenarioSpec, step ScenarioStep) error {
	if step.Write != nil {
		if err := execution.writeFile(spec, *step.Write); err != nil {
			return err
		}
	}
	if step.Append != nil {
		if err := execution.appendFile(spec, *step.Append); err != nil {
			return err
		}
	}
	if step.Replace != nil {
		if err := execution.replaceText(spec, *step.Replace); err != nil {
			return err
		}
	}
	if step.Run != nil {
		if err := execution.runCommand(*step.Run, step.Title); err != nil {
			return err
		}
	}
	return nil
}

// writeFile formats valid Go sources so executable examples remain stable across generated documentation and tests.
func (execution scenarioExecution) writeFile(spec ScenarioSpec, change ScenarioFileChange) error {
	path, err := execution.workspace.path(change.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := []byte(expandScenarioText(spec, change.Content))
	if strings.HasSuffix(path, ".go") {
		formatted, err := format.Source(content)
		if err != nil {
			return fmt.Errorf("format %s: %w", change.Path, err)
		}
		content = formatted
	}
	return os.WriteFile(path, content, 0o644)
}

// appendFile preserves existing generated content for scenario steps that intentionally extend a file.
func (execution scenarioExecution) appendFile(spec ScenarioSpec, change ScenarioFileChange) error {
	path, err := execution.workspace.path(change.Path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	content := []byte(expandScenarioText(spec, change.Content))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return appendAndCloseScenarioFile(file, content)
}

// appendAndCloseScenarioFile reports delayed filesystem failures without discarding an earlier write failure.
func appendAndCloseScenarioFile(file io.WriteCloser, content []byte) error {
	_, writeErr := file.Write(content)
	return errors.Join(writeErr, file.Close())
}

// replaceText requires an exact old value so template drift cannot silently produce an incomplete scenario.
func (execution scenarioExecution) replaceText(spec ScenarioSpec, change ScenarioReplace) error {
	path, err := execution.workspace.path(change.Path)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	oldText := expandScenarioText(spec, change.Old)
	newText := expandScenarioText(spec, change.New)
	if oldText == "" {
		return fmt.Errorf("replace target is required for %s", change.Path)
	}
	if oldText == newText {
		return fmt.Errorf("replacement must differ from target in %s", change.Path)
	}
	matches := strings.Count(string(body), oldText)
	if matches == 0 {
		return fmt.Errorf("replace target not found in %s", change.Path)
	}
	if matches > 1 {
		return fmt.Errorf("replace target occurs %d times in %s", matches, change.Path)
	}
	updated := strings.Replace(string(body), oldText, newText, 1)
	return os.WriteFile(path, []byte(updated), 0o644)
}

// path confines every declared file change to the isolated scenario root.
func (workspace scenarioWorkspace) path(relative string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(relative))
	if clean == "." || clean == "" {
		return "", fmt.Errorf("path is required")
	}
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("invalid scenario path %q", relative)
	}
	return filepath.Join(workspace.root, clean), nil
}

// runCommand substitutes the current Forj executable so scenarios validate the build under test rather than an installed release.
func (execution scenarioExecution) runCommand(command ScenarioCommand, label string) error {
	if len(command.Run) == 0 {
		return fmt.Errorf("command is required")
	}
	args := append([]string{}, command.Run...)
	if args[0] == "forj" {
		args[0] = execution.forjExec
	}
	ctx, cancel := context.WithTimeout(context.Background(), scenarioCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = execution.workspace.root
	cmd.Env = scenarioProcessEnv()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		text := strings.TrimSpace(output.String())
		if text == "" {
			text = err.Error()
		}
		execution.logger.Error().Str("step", label).Str("command", strings.Join(command.Run, " ")).Str("output", text).Msg("scenario command failed")
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("%s: timed out after %s: %w\n%s", strings.Join(command.Run, " "), scenarioCommandTimeout, context.DeadlineExceeded, text)
		}
		return fmt.Errorf("%s: %w\n%s", strings.Join(command.Run, " "), err, text)
	}
	for _, expected := range command.Contains {
		if !strings.Contains(output.String(), expected) {
			return fmt.Errorf("%s: output missing %q\n%s", strings.Join(command.Run, " "), expected, output.String())
		}
	}
	return nil
}

// scenarioProcessEnv isolates Go caches so scenario subprocesses do not depend on a developer's global cache state.
func scenarioProcessEnv() []string {
	modCache, buildCache := testkit.GoCachePaths()
	return testkit.ProcessGoEnv("", map[string]string{
		"GOMODCACHE": modCache,
		"GOCACHE":    buildCache,
	})
}

// expandScenarioText resolves the executable module independently from the documentation-safe module name.
func expandScenarioText(spec ScenarioSpec, value string) string {
	return strings.ReplaceAll(value, "{{module}}", spec.App.ModuleName)
}
