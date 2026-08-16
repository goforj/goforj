package scenarios

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/goforj/console"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// scenarioWorkspace owns the isolated directory lifecycle for one executable scenario.
type scenarioWorkspace struct {
	root        string
	toolRoot    string
	removeAfter bool
}

// scenarioExecution keeps the invariant collaborators for every dependency, step, and verification command in one run.
type scenarioExecution struct {
	context     context.Context
	logger      *logger.AppLogger
	workspace   scenarioWorkspace
	forjExec    string
	tools       map[string]string
	toolDigest  string
	environment []string
}

const scenarioCommandTimeout = 3 * time.Minute

const scenarioCommandOutputLimit = 1 << 20

// runScenario executes one selected scenario against the same validated catalog used to select it.
func runScenario(options ValidateOptions, catalog scenarioCatalog, spec ScenarioSpec, forjExec string) error {
	plan, ok := catalog.plans[spec.ID]
	if !ok {
		return fmt.Errorf("scenario plan %q is unavailable", spec.ID)
	}
	environment := append([]string(nil), options.Environment...)
	if environment == nil {
		environment = scenarioProcessEnv()
	}
	tools, toolDigest, err := resolveScenarioPlanTools(forjExec, environment, plan, true)
	if err != nil {
		return err
	}
	workspace, err := createScenarioWorkspace(options, spec)
	if err != nil {
		return err
	}
	execution := scenarioExecution{
		context:     context.Background(),
		logger:      options.Logger,
		workspace:   workspace,
		forjExec:    forjExec,
		tools:       tools,
		toolDigest:  toolDigest,
		environment: environment,
	}
	execution, err = execution.snapshotTools()
	if err != nil {
		return workspace.cleanupAfter(err)
	}
	console.Actionf("scenario %s", spec.ID)
	runErr := workspace.cleanupAfter(execution.run(plan))
	if options.Keep {
		console.Infof("scenario workdir: %s", workspace.root)
	}
	if runErr != nil {
		return runErr
	}
	console.Successf("scenario passed: %s", spec.ID)
	return nil
}

// prepare applies the immutable live prefix and never observes target steps or final checks.
func (execution scenarioExecution) prepare(plan scenarioPlan) error {
	return execution.execute(plan, false)
}

// run applies the complete golden path through the same prefix used by live preparation.
func (execution scenarioExecution) run(plan scenarioPlan) error {
	return execution.execute(plan, true)
}

// execute stops at the one explicit preparation boundary instead of maintaining separate stage interpreters.
func (execution scenarioExecution) execute(plan scenarioPlan, includeTarget bool) error {
	if err := execution.contextErr(); err != nil {
		return err
	}
	if err := writeScenarioProjectConfig(execution.workspace.root, plan.spec); err != nil {
		return err
	}
	if err := execution.runCommand(ScenarioCommand{Run: []string{"forj", "render"}}, "render app"); err != nil {
		return err
	}
	if err := execution.applyPlannedSteps(plan.spec.ID, "dependency", plan.dependencySteps); err != nil {
		return err
	}
	if err := execution.applyPlannedSteps(plan.spec.ID, "preparation", plan.preparationSteps); err != nil {
		return err
	}
	if err := execution.runChecks(plan.startingChecks, "verify starting state"); err != nil {
		return err
	}
	if !includeTarget {
		return nil
	}
	if err := execution.applyPlannedSteps(plan.spec.ID, "target", plan.targetSteps); err != nil {
		return err
	}
	return execution.runChecks(plan.finalChecks, "verify")
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
	var cleanupErr error
	if workspace.removeAfter {
		cleanupErr = removeScenarioTree(workspace.root)
	}
	if workspace.toolRoot != "" {
		cleanupErr = errors.Join(cleanupErr, removeScenarioTree(workspace.toolRoot))
	}
	if cleanupErr != nil {
		return errors.Join(runErr, fmt.Errorf("remove temporary scenario workspace %q: %w", workspace.root, cleanupErr))
	}
	return runErr
}

// applyPlannedSteps preserves owner metadata while executing one immutable plan stage.
func (execution scenarioExecution) applyPlannedSteps(scenarioID, stage string, steps []plannedScenarioStep) error {
	for _, planned := range steps {
		if err := execution.contextErr(); err != nil {
			return err
		}
		if err := execution.applyStep(planned.spec, planned.step); err != nil {
			return fmt.Errorf("%s %s %s step %q: %w", scenarioID, stage, planned.spec.ID, planned.step.Title, err)
		}
	}
	return nil
}

// runChecks applies one check stage without exposing whether execution is full or prefix-only to the command runner.
func (execution scenarioExecution) runChecks(commands []ScenarioCommand, label string) error {
	for _, command := range commands {
		if err := execution.contextErr(); err != nil {
			return err
		}
		if err := execution.runCommand(command, label); err != nil {
			return err
		}
	}
	return nil
}

// writeScenarioProjectConfig relies on the canonical component sequence so intentionally absent primitives stay disabled.
func writeScenarioProjectConfig(root string, spec ScenarioSpec) error {
	config := scenarioProjectConfig(spec)
	body, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".goforj.yml"), body, 0o644)
}

// scenarioProjectConfig projects scenario semantics without coupling preparation metadata to YAML serialization.
func scenarioProjectConfig(spec ScenarioSpec) project.Config {
	return project.Config{
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
}

// applyStep preserves the declared action order because later scenario edits can depend on earlier generated content.
func (execution scenarioExecution) applyStep(spec ScenarioSpec, step ScenarioStep) error {
	if err := execution.contextErr(); err != nil {
		return err
	}
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
	if err := execution.contextErr(); err != nil {
		return err
	}
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
	if err := execution.contextErr(); err != nil {
		return err
	}
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
	if err := execution.contextErr(); err != nil {
		return err
	}
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
	if err := execution.contextErr(); err != nil {
		return err
	}
	if len(command.Run) == 0 {
		return fmt.Errorf("command is required")
	}
	args := append([]string{}, command.Run...)
	tool := execution.tools[args[0]]
	if tool == "" && args[0] == "forj" {
		tool = execution.forjExec
	}
	if tool == "" {
		return fmt.Errorf("command executable %q is not bound to a resolved tool", args[0])
	}
	args[0] = tool
	parent := execution.context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, scenarioCommandTimeout)
	defer cancel()
	environment := execution.environment
	if len(environment) == 0 {
		environment = scenarioProcessEnv()
	}
	output := newScenarioOutputBuffer(command.Contains)
	supervisor := devwatch.NewSupervisor(devwatch.SupervisorOptions{})
	defer supervisor.Close()
	_, err := supervisor.Run(ctx, "scenario command", devwatch.Command{
		Args:       args,
		Dir:        execution.workspace.root,
		Env:        scenarioEnvironmentMap(environment),
		ReplaceEnv: true,
		Stdout:     &output,
		Stderr:     &output,
	})
	if err != nil {
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
	for _, expected := range output.missingContains() {
		if !strings.Contains(output.String(), expected) {
			return fmt.Errorf("%s: output missing %q\n%s", strings.Join(command.Run, " "), expected, output.String())
		}
	}
	return nil
}

// scenarioOutputBuffer bounds diagnostic retention while allowing commands to stream indefinitely.
type scenarioOutputBuffer struct {
	buffer    bytes.Buffer
	truncated bool
	wanted    map[string]bool
	found     map[string]bool
	tail      []byte
	tailLimit int
}

// newScenarioOutputBuffer retains bounded diagnostics while observing required output across the complete stream.
func newScenarioOutputBuffer(contains []string) scenarioOutputBuffer {
	output := scenarioOutputBuffer{wanted: make(map[string]bool, len(contains)), found: make(map[string]bool, len(contains))}
	for _, expected := range contains {
		if expected == "" {
			continue
		}
		output.wanted[expected] = true
		if len(expected) > output.tailLimit {
			output.tailLimit = len(expected)
		}
	}
	if output.tailLimit > 0 {
		output.tailLimit--
	}
	return output
}

// Write retains at most the scenario diagnostic limit to prevent a failed command exhausting runner memory.
func (buffer *scenarioOutputBuffer) Write(content []byte) (int, error) {
	buffer.observe(content)
	remaining := scenarioCommandOutputLimit - buffer.buffer.Len()
	if remaining <= 0 {
		buffer.truncated = true
		return len(content), nil
	}
	if len(content) > remaining {
		_, _ = buffer.buffer.Write(content[:remaining])
		buffer.truncated = true
		return len(content), nil
	}
	_, _ = buffer.buffer.Write(content)
	return len(content), nil
}

// observe tracks expected fragments independently from the bounded diagnostic retention buffer.
func (buffer *scenarioOutputBuffer) observe(content []byte) {
	if len(buffer.wanted) == 0 {
		return
	}
	joined := make([]byte, len(buffer.tail)+len(content))
	copy(joined, buffer.tail)
	copy(joined[len(buffer.tail):], content)
	for expected := range buffer.wanted {
		if !buffer.found[expected] && bytes.Contains(joined, []byte(expected)) {
			buffer.found[expected] = true
		}
	}
	if buffer.tailLimit == 0 {
		buffer.tail = nil
		return
	}
	if len(joined) > buffer.tailLimit {
		joined = joined[len(joined)-buffer.tailLimit:]
	}
	buffer.tail = append(buffer.tail[:0], joined...)
}

// missingContains returns expected stream fragments that were never observed.
func (buffer *scenarioOutputBuffer) missingContains() []string {
	missing := make([]string, 0, len(buffer.wanted))
	for expected := range buffer.wanted {
		if !buffer.found[expected] {
			missing = append(missing, expected)
		}
	}
	return missing
}

// String makes truncation visible in diagnostics without changing command success semantics.
func (buffer *scenarioOutputBuffer) String() string {
	if !buffer.truncated {
		return buffer.buffer.String()
	}
	return buffer.buffer.String() + "\n[scenario command output truncated]"
}

// scenarioEnvironmentMap translates an explicit environment slice to the supervisor's replacement environment.
func scenarioEnvironmentMap(environment []string) map[string]string {
	values := make(map[string]string, len(environment))
	for _, entry := range environment {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			values[scenarioEnvironmentKey(key)] = value
		}
	}
	return values
}

// scenarioEnvironmentKey mirrors Windows process lookup, where environment variable names are case-insensitive.
func scenarioEnvironmentKey(key string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(key)
	}
	return key
}

// contextErr keeps file actions from continuing after their caller has cancelled the evaluation.
func (execution scenarioExecution) contextErr() error {
	if execution.context == nil {
		return nil
	}
	return execution.context.Err()
}

// snapshotTools copies already-resolved executables into the owned workspace so PATH changes or in-place replacements cannot alter execution.
func (execution scenarioExecution) snapshotTools() (scenarioExecution, error) {
	if err := execution.contextErr(); err != nil {
		return scenarioExecution{}, err
	}
	root, err := os.MkdirTemp("", "forj-scenario-tools-")
	if err != nil {
		return scenarioExecution{}, fmt.Errorf("create scenario tool root: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = removeScenarioTree(root)
		}
	}()
	execution.workspace.toolRoot = root
	tools := make(map[string]string, len(execution.tools))
	digests := make(map[string]string, len(execution.tools))
	for name, source := range execution.tools {
		if err := execution.contextErr(); err != nil {
			return scenarioExecution{}, err
		}
		if err := validateScenarioToolName(name); err != nil {
			return scenarioExecution{}, err
		}
		destination := filepath.Join(root, name+filepath.Ext(source))
		if err := copyScenarioExecutable(source, destination); err != nil {
			return scenarioExecution{}, fmt.Errorf("snapshot tool %q: %w", name, err)
		}
		tools[name] = destination
		_, digest, err := resolveScenarioExecutable(destination)
		if err != nil {
			return scenarioExecution{}, fmt.Errorf("verify tool %q snapshot: %w", name, err)
		}
		digests[name] = digest
	}
	if execution.toolDigest != "" && execution.toolDigest != digestScenarioTools(digests) {
		return scenarioExecution{}, fmt.Errorf("scenario tool bytes changed before execution")
	}
	execution.tools = tools
	if forj := tools["forj"]; forj != "" {
		execution.forjExec = forj
	}
	success = true
	return execution, nil
}

// digestScenarioTools uses the same stable name-and-byte identity as preparation resolution.
func digestScenarioTools(tools map[string]string) string {
	identities := make([]string, 0, len(tools))
	for name, digest := range tools {
		identities = append(identities, name+"\x00"+digest)
	}
	sort.Strings(identities)
	hash := sha256.New()
	for _, identity := range identities {
		fmt.Fprintf(hash, "%s\x00", identity)
	}
	return fmt.Sprintf("sha256:%x", hash.Sum(nil))
}

// validateScenarioToolName keeps tool snapshots beneath their owned directory on every supported host.
func validateScenarioToolName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\\`) || filepath.Base(name) != name {
		return fmt.Errorf("invalid scenario tool name %q", name)
	}
	return nil
}

// copyScenarioExecutable verifies the selected regular file as it is copied and makes the private snapshot executable.
func copyScenarioExecutable(source, destination string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a regular file", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return errors.Join(err, input.Close())
	}
	_, copyErr := io.Copy(output, input)
	return errors.Join(copyErr, input.Close(), output.Close())
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
