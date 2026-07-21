package forj

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"

	"github.com/goforj/console"
	envx "github.com/goforj/env/v2"
	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
	"golang.org/x/term"
)

var errDevInterrupted = errors.New("dev interrupted")

// DevCmd runs the project development lifecycle from durable project configuration.
type DevCmd struct{}

type devRuntimeState struct {
	restartCh      chan struct{}
	buildCh        chan struct{}
	renderCh       chan struct{}
	refreshWriters func()
	streamer       *devwatchStreamer
	firstLoad      bool
}

type devShellCommandRequest struct {
	DisplayName  string
	ShellCommand string
}

type devWatchSession struct {
	config                *project.Config
	baseWatches           []project.DevWatch
	streamer              *devwatchStreamer
	restartCh             chan struct{}
	buildCh               chan struct{}
	renderCh              chan struct{}
	commandCh             chan devShellCommandRequest
	stopCh                <-chan struct{}
	outWriter             io.Writer
	errWriter             io.Writer
	reloadRuntime         func() (*devwatchStreamer, error)
	reconcile             bool
	reconcileFrontendDeps bool
	managedContext        context.Context
	managedBarrier        func(context.Context) error
	managedHeartbeat      func()
}

// Signature declares the development command exposed by the root CLI.
func (*DevCmd) Signature() string {
	return `name:"dev" help:"Run development watchers"`
}

// newDevRuntimeState keeps dotenv reloads and output streaming on one boundary.
func newDevRuntimeState(restartCh chan struct{}, buildCh chan struct{}, renderCh chan struct{}) *devRuntimeState {
	return &devRuntimeState{
		restartCh:      restartCh,
		buildCh:        buildCh,
		renderCh:       renderCh,
		refreshWriters: func() {},
		firstLoad:      true,
	}
}

func (r *devRuntimeState) Close() {
	if r.streamer != nil {
		r.streamer.Close()
	}
}

func (r *devRuntimeState) Sync() (*devwatchStreamer, error) {
	firstLoad := r.firstLoad
	r.firstLoad = false
	if err := loadDevEnvironment(!firstLoad); err != nil {
		return nil, err
	}
	if r.streamer != nil {
		r.streamer.Close()
	}
	r.streamer = newDevwatchStreamerFromEnv()
	if r.streamer != nil {
		r.streamer.SetRestartChannel(r.restartCh)
		r.streamer.SetRenderChannel(r.renderCh)
	}
	r.refreshWriters()
	return r.streamer, nil
}

// loadDevEnvironment applies the env package's canonical file precedence for startup or reload.
func loadDevEnvironment(reload bool) error {
	if reload {
		return envx.Reload()
	}
	return envx.Load()
}

// Run executes the dev workflow (pre tasks, watchers, and shutdown handling).
func (c *DevCmd) Run() error {
	inheritedContext, err := managedsession.CaptureInheritedLaunchContext()
	if err != nil {
		return fmt.Errorf("capture managed launch context: %w", err)
	}
	config, err := loadDevProjectConfig(console.Default())
	if err != nil {
		return err
	}
	if config == nil {
		return nil
	}
	if inheritedContext != nil {
		if err := validateManagedDevLifecycle(config); err != nil {
			return err
		}
	}

	// Prevent concurrent dev sessions from clobbering each other.
	unlock, err := c.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()
	var managedConnection *reconnectingManagedSession
	var managedRegistration managedsession.RegisterResponse
	var managedIdentity string
	var managedBarrier func(context.Context) error
	if inheritedContext != nil {
		managedClient, registration, openErr := managedsession.OpenLaunchSession(context.Background(), *inheritedContext)
		managedRegistration = registration
		err = openErr
		if err != nil {
			return err
		}
		managedConnection, err = newReconnectingManagedSession(
			managedClient,
			managedRegistration,
			*inheritedContext,
			func(ctx context.Context, launch managedsession.LaunchContext) (managedSessionClient, managedsession.RegisterResponse, error) {
				client, response, err := managedsession.OpenLaunchSession(ctx, launch)
				return client, response, err
			},
		)
		if err != nil {
			_ = managedClient.Close()
			return err
		}
		defer managedConnection.Close()
		managedIdentity = managedProjectIdentity(config, inheritedContext.ProjectRoot)
		managedBarrier = func(ctx context.Context) error {
			return waitForManagedComposeBarrier(ctx, managedConnection, managedRegistration, managedIdentity)
		}
	}

	outputSession := devOutputSession{}
	var cleanupOnce sync.Once
	cleanupDevTerminal := func() {
		cleanupOnce.Do(func() {
			finishDevOutputSession(outputSession, func() {
				restoreDevTerminalState(nil, nil)
			})
		})
	}
	defer cleanupDevTerminal()

	baseWatches := normalizeDevWatchesForRuntime(config, copyDevWatches(config.Dev.Watches))
	config.Dev.Watches = devWatchesForApps(config, baseWatches)

	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if len(config.Dev.Watches) == 0 && len(config.Dev.Apps) == 0 {
		console.Warnf("No dev watches defined in .goforj.yml")
		return nil
	}
	if _, err := compileDevWatchers(config); err != nil {
		return err
	}

	if err := ensureDevTools(); err != nil {
		return err
	}
	if err := ensureBinDir(); err != nil {
		return err
	}

	restartCh := make(chan struct{}, 1)
	buildCh := make(chan struct{}, 1)
	renderCh := make(chan struct{}, 1)
	commandCh := make(chan devShellCommandRequest, 1)
	requestRestart := func() {
		select {
		case restartCh <- struct{}{}:
		default:
		}
	}
	requestRender := func() {
		select {
		case renderCh <- struct{}{}:
		default:
		}
	}
	requestCommand := func(req devShellCommandRequest) {
		select {
		case commandCh <- req:
		default:
		}
	}
	requestBuild := func() {
		select {
		case buildCh <- struct{}{}:
		default:
		}
	}
	stopEnvWatch := startDevEnvFileWatcher(runCtx, requestBuild, 250*time.Millisecond)
	defer stopEnvWatch()
	stopTargetWatch := startDevAppWatcher(runCtx, requestBuild, 500*time.Millisecond)
	defer stopTargetWatch()
	runtimeState := newDevRuntimeState(restartCh, buildCh, renderCh)
	defer runtimeState.Close()

	currentStreamer, err := runtimeState.Sync()
	if err != nil {
		return err
	}
	if managedBarrier != nil {
		if err := runManagedDevInitialLifecycle(config, os.Stdout, os.Stderr, runCtx, managedBarrier); err != nil {
			return err
		}
	} else if err := runDevInitialLifecycle(config, os.Stdout, os.Stderr); err != nil {
		return err
	}
	outputSession = buildDevOutputSession(config, requestRestart, requestRender, requestCommand)
	outWriter := outputSession.stdout
	errWriter := outputSession.stderr
	runtimeState.refreshWriters = outputSession.refresh
	runtimeState.refreshWriters()

	session := &devWatchSession{
		config:         config,
		baseWatches:    baseWatches,
		streamer:       currentStreamer,
		restartCh:      restartCh,
		buildCh:        buildCh,
		renderCh:       renderCh,
		commandCh:      commandCh,
		stopCh:         runCtx.Done(),
		outWriter:      outWriter,
		errWriter:      errWriter,
		reloadRuntime:  runtimeState.Sync,
		managedContext: runCtx,
	}
	if managedConnection != nil {
		session.managedHeartbeat = func() {
			go runManagedSessionHeartbeat(runCtx, managedConnection, managedRegistration, managedIdentity, errWriter)
		}
	}

	if err := c.runWatchersLoop(session); err != nil {
		if errors.Is(err, errDevInterrupted) {
			cleanupDevTerminal()
			if config.Dev.DownOnExit {
				console.Actionf("forj down > auto (set dev.down_on_exit: false to disable)")
				var downErr error
				if managedConnection != nil {
					downErr = runManagedDevDownTasks(config)
				} else {
					downErr = runDevDownTasks(effectiveDevDownTasks(config))
				}
				if downErr != nil {
					console.Errorf("forj down failed: %v", downErr)
				} else {
					console.Successf("forj down complete")
				}
			}
			return nil
		}
		cleanupDevTerminal()
		return err
	}

	return fmt.Errorf("dev watchers exited unexpectedly")
}

// loadDevProjectConfig turns an absent project into guidance while preserving real configuration failures.
func loadDevProjectConfig(commandConsole *console.Console) (*project.Config, error) {
	config, err := project.LoadProjectConfig()
	if err == nil {
		return config, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		writeDevProjectNotFound(commandConsole)
		return nil, nil
	}
	return nil, fmt.Errorf("load project configuration: %w", err)
}

// writeDevProjectNotFound explains the project-root requirement without presenting normal navigation as a failure.
func writeDevProjectNotFound(commandConsole *console.Console) {
	commandConsole.Info(commandConsole.Style("No GoForj project found", console.StyleBold))
	commandConsole.Printf(
		"  Run %s from a project root containing %s.\n",
		commandConsole.Style("forj dev", console.ColorCyan),
		commandConsole.Style(".goforj.yml", console.ColorCyan),
	)
	commandConsole.Printf("  Starting fresh? Run %s.\n", commandConsole.Style("forj new", console.ColorCyan))
}

// devInitialTaskPlan separates framework setup from work that must retain the historical binary-ready contract.
type devInitialTaskPlan struct {
	preBuild    []project.DevTask
	postMigrate []project.DevTask
	fastPath    bool
}

// runDevInitialLifecycle builds structured projects once when every configured setup task has a known safe phase.
func runDevInitialLifecycle(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	plan := planDevInitialTasks(config)
	if plan.fastPath {
		if err := runDevTasks("Running pre-dev setup", plan.preBuild); err != nil {
			return err
		}
		if _, err := runDevInitialSPABuilds(config, outWriter, errWriter); err != nil {
			return err
		}
		writeDevAppBuildLine(outWriter, activeDevAppsForConfig(config))
		if err := runDevInitialBuild(config, outWriter, errWriter); err != nil {
			return err
		}
		if err := runDevAppSetup(config, outWriter, errWriter); err != nil {
			return err
		}
		if err := runDevTasks("Running post-migrate setup", plan.postMigrate); err != nil {
			return err
		}
		if len(plan.postMigrate) > 0 {
			if err := runDevBuild(config, outWriter, errWriter); err != nil {
				return fmt.Errorf("post-setup forj build failed: %w", err)
			}
		}
		return nil
	}

	writeDevAppBuildLine(outWriter, activeDevAppsForConfig(config))
	if err := runDevInitialBuild(config, outWriter, errWriter); err != nil {
		return err
	}
	rebuildAfterSetup, err := runPreDevSetup(config)
	if err != nil {
		return err
	}
	spaBuilt, err := runDevInitialSPABuilds(config, outWriter, errWriter)
	if err != nil {
		return err
	}
	if spaBuilt || rebuildAfterSetup {
		if err := runDevBuild(config, outWriter, errWriter); err != nil {
			return fmt.Errorf("post-setup forj build failed: %w", err)
		}
	}
	return nil
}

// planDevInitialTasks preserves arbitrary pre-task ordering unless every task matches a framework-owned bootstrap convention.
func planDevInitialTasks(config *project.Config) devInitialTaskPlan {
	if config == nil {
		return devInitialTaskPlan{}
	}
	tasks := effectiveDevPreTasks(config)
	plan := devInitialTaskPlan{
		preBuild:    make([]project.DevTask, 0, len(tasks)),
		postMigrate: make([]project.DevTask, 0, len(tasks)),
		fastPath:    config.Dev.UsesStructuredApps(),
	}
	autoMigrate := shouldRunDevAutoMigrate(config)
	for _, task := range tasks {
		if autoMigrate && shouldRunAfterMigrate(task) {
			plan.postMigrate = append(plan.postMigrate, task)
			continue
		}
		if isConventionalDevBootstrapTask(task) {
			plan.preBuild = append(plan.preBuild, task)
			continue
		}
		plan.fastPath = false
	}
	return plan
}

// isConventionalDevBootstrapTask recognizes only framework task identities whose commands cannot require an App binary.
func isConventionalDevBootstrapTask(task project.DevTask) bool {
	name := strings.TrimSpace(task.Name)
	switch {
	case name == "Run Docker Compose":
		return isConventionalDevComposeUpCommand(task.Cmd)
	case name == "Waiting for Database to be ready":
		command := normalizeDevComposeExecutable(task.Cmd)
		return command == generatedMySQLDevWaitCommand || command == generatedPostgresDevWaitCommand
	case name == "Seed Grafana Dashboards":
		command := normalizeDevComposeExecutable(task.Cmd)
		return command == "docker-compose up -d --force-recreate grafana-seed" ||
			command == "docker-compose run --rm --no-deps grafana-seed"
	case isFrontendDependencyDevTaskName(name):
		return isConventionalDevFrontendInstallCommand(task)
	default:
		return false
	}
}

// isConventionalDevComposeUpCommand matches the normalized Compose startup whose behavior is independent of an existing App binary.
func isConventionalDevComposeUpCommand(command string) bool {
	command = normalizeDevComposeExecutable(command)
	return command == "docker-compose up -d"
}

// normalizeDevComposeExecutable treats the current plugin spelling like the historical generated binary.
func normalizeDevComposeExecutable(command string) string {
	command = strings.TrimSpace(command)
	if strings.HasPrefix(command, "docker compose ") {
		return "docker-compose " + strings.TrimPrefix(command, "docker compose ")
	}
	return command
}

// isFrontendDependencyDevTaskName matches the stable default and named-App task identities emitted by GoForj.
func isFrontendDependencyDevTaskName(name string) bool {
	return name == "Install Frontend Dependencies" ||
		strings.HasPrefix(name, "Install ") && strings.HasSuffix(name, " Frontend Dependencies")
}

// isConventionalDevFrontendInstallCommand accepts generated npm setup plus the quiet flags used by upgraded projects.
func isConventionalDevFrontendInstallCommand(task project.DevTask) bool {
	app, ok := devFrontendInstallTaskApp(task.Name)
	if !ok {
		return false
	}
	base := devFrontendInstallTask(app, "npm install").Cmd
	command := strings.TrimSpace(task.Cmd)
	if command == base {
		return true
	}
	if !strings.HasPrefix(command, base+" ") {
		return false
	}
	for _, field := range strings.Fields(strings.TrimPrefix(command, base+" ")) {
		switch field {
		case "--no-audit", "--no-fund", "--no-progress", "--loglevel=error", ">/dev/null":
		default:
			return false
		}
	}
	return true
}

// devFrontendInstallTaskApp resolves the App encoded in GoForj's stable frontend task identity.
func devFrontendInstallTaskApp(name string) (project.App, bool) {
	name = strings.TrimSpace(name)
	if name == "Install Frontend Dependencies" {
		return project.DefaultApp(), true
	}
	if !strings.HasPrefix(name, "Install ") || !strings.HasSuffix(name, " Frontend Dependencies") {
		return project.App{}, false
	}
	appName := strings.TrimSuffix(strings.TrimPrefix(name, "Install "), " Frontend Dependencies")
	if !project.IsSafeAppName(appName) || project.IsReservedAppName(appName) {
		return project.App{}, false
	}
	return project.DefaultNamedApp(appName), true
}

// ensureDevDatabaseExistsWithWriters provisions only service-backed databases because SQLite needs no external preparation.
func ensureDevDatabaseExistsWithWriters(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	databases, err := devDatabasesForApps(config, activeDevAppsForConfig(config))
	if err != nil {
		return err
	}
	databasesByDriver := map[string][]string{}
	for _, database := range databases {
		databasesByDriver[database.Driver] = append(databasesByDriver[database.Driver], database.Name)
	}
	if names := databasesByDriver["mysql"]; len(names) > 0 {
		res, err := execx.Command("docker-compose", "exec", "-T", "mysql", "sh", "-c", mysqlCreateDatabasesScript(names)).
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(outWriter).
			StderrWriter(errWriter).
			Run()
		if err != nil {
			return fmt.Errorf("ensure mysql database failed: %v", err)
		}
		if !res.OK() {
			return fmt.Errorf("ensure mysql database failed with exit code %d", res.ExitCode)
		}
	}
	if names := databasesByDriver["postgres"]; len(names) > 0 {
		res, err := execx.Command("docker-compose", "exec", "-T", "postgres", "sh", "-c", postgresCreateDatabasesScript(names)).
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(outWriter).
			StderrWriter(errWriter).
			Run()
		if err != nil {
			return fmt.Errorf("ensure postgres database failed: %v", err)
		}
		if !res.OK() {
			return fmt.Errorf("ensure postgres database failed with exit code %d", res.ExitCode)
		}
	}
	return nil
}

type devDatabase struct {
	App    string
	Driver string
	Name   string
}

// devDatabasesForApps discovers every server database the current dev session must create.
func devDatabasesForApps(config *project.Config, apps []project.App) ([]devDatabase, error) {
	seen := map[string]bool{}
	databases := make([]devDatabase, 0, len(apps))
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		components := appRenderComponents(config, app)
		if !components.HasDatabase() {
			continue
		}
		driver := normalizeDevDatabaseDriver(appScopedEnvValue(app, "DB_DRIVER"))
		if driver == "" {
			driver = normalizeDevDatabaseDriver(components.DatabaseDriver())
		}
		if driver == "" || driver == "sqlite" {
			continue
		}
		name := appScopedEnvValue(app, "DB_DATABASE")
		if name == "" {
			return nil, fmt.Errorf("missing %s for %s database app %s", appScopedEnvKey(app, "DB_DATABASE"), driver, app.Name)
		}
		if err := validateDevDatabaseName(name); err != nil {
			return nil, fmt.Errorf("%s for app %s: %w", appScopedEnvKey(app, "DB_DATABASE"), app.Name, err)
		}
		key := driver + "\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		databases = append(databases, devDatabase{App: app.Name, Driver: driver, Name: name})
	}
	sort.Slice(databases, func(left, right int) bool {
		if databases[left].Driver != databases[right].Driver {
			return databases[left].Driver < databases[right].Driver
		}
		return databases[left].Name < databases[right].Name
	})
	return databases, nil
}

// normalizeDevDatabaseDriver maps common aliases to the compose service driver names.
func normalizeDevDatabaseDriver(driver string) string {
	switch str.Of(driver).Trim().ToLower().String() {
	case "mysql", "mariadb":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return str.Of(driver).Trim().ToLower().String()
	}
}

// appScopedEnvValue reads app-specific env with the same fallback shape used by app runtime.
func appScopedEnvValue(app project.App, key string) string {
	if app.Name != "" && app.Name != project.DefaultAppName {
		if value := strings.TrimSpace(os.Getenv(appScopedEnvKey(app, key))); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}

// appScopedEnvKey returns the app-prefixed env key that overrides a base env value.
func appScopedEnvKey(app project.App, key string) string {
	if app.Name == "" || app.Name == project.DefaultAppName {
		return key
	}
	prefix := project.AppEnvironmentPrefix(app.Name)
	if prefix == "" {
		return key
	}
	return prefix + "_" + key
}

// validateDevDatabaseName keeps local database creation scripts simple and injection-safe.
func validateDevDatabaseName(name string) error {
	if name == "" {
		return fmt.Errorf("database name is empty")
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' {
			continue
		}
		return fmt.Errorf("database name %q must contain only letters, numbers, and underscores", name)
	}
	return nil
}

// mysqlCreateDatabasesScript creates every needed MySQL database after the service is ready.
func mysqlCreateDatabasesScript(names []string) string {
	return `while ! mysqladmin ping -h "mysql" --silent; do sleep .5; done; for db in ` + strings.Join(names, " ") + `; do mysql -h "mysql" -uroot -p"$MARIADB_ROOT_PASSWORD" -e "CREATE DATABASE IF NOT EXISTS \` + "`" + `$db\` + "`" + `; GRANT ALL PRIVILEGES ON \` + "`" + `$db\` + "`" + `.* TO '$MARIADB_USER'@'%';"; done; mysql -h "mysql" -uroot -p"$MARIADB_ROOT_PASSWORD" -e "FLUSH PRIVILEGES;"`
}

// postgresCreateDatabasesScript creates every needed Postgres database after the service is ready.
func postgresCreateDatabasesScript(names []string) string {
	return `until pg_isready -h "postgres" -p 5432; do sleep .5; done; for db in ` + strings.Join(names, " ") + `; do psql -U "$POSTGRES_USER" -h "postgres" -d postgres -v ON_ERROR_STOP=1 -tc "SELECT 1 FROM pg_database WHERE datname = '$db'" | grep -q 1 || psql -U "$POSTGRES_USER" -h "postgres" -d postgres -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$db\";"; done`
}

func runDevTasks(heading string, tasks []project.DevTask) error {
	if len(tasks) == 0 {
		return nil
	}
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()
	console.Actionf("%s", heading)
	for _, task := range tasks {
		fmt.Printf(" %s %s\n", console.ActionMark(), task.Name)
		outputTail := newDevTaskOutputTail(40)
		cmd := newDevTaskCommand(task, devNull, os.Stdout, os.Stderr, outputTail)
		res, err := cmd.Run()
		if err != nil {
			clearPreDevTaskProgressLine()
			return devTaskFailureError(task.Name, fmt.Sprintf("failed: %v", err), outputTail.String())
		}
		if !res.OK() {
			clearPreDevTaskProgressLine()
			return devTaskFailureError(task.Name, fmt.Sprintf("failed with exit code %d", res.ExitCode), outputTail.String())
		}
	}
	return nil
}

// newDevTaskCommand buffers routine output only for framework-shaped dependency installs so successful setup stays compact without hiding failure details.
func newDevTaskCommand(task project.DevTask, stdin io.Reader, stdout io.Writer, stderr io.Writer, outputTail *devTaskOutputTail) *execx.Cmd {
	cmd := execx.Command("bash", "-c", task.Cmd).
		EnvInherit().
		StdinReader(stdin)
	if isConventionalDevFrontendInstallCommand(task) {
		return cmd.
			StdoutWriter(outputTail).
			StderrWriter(io.MultiWriter(stderr, outputTail))
	}
	cmd = cmd.
		StdoutWriter(io.MultiWriter(stdout, outputTail)).
		StderrWriter(io.MultiWriter(stderr, outputTail))
	return configureDevTaskTTYWithWriter(cmd, stdout, outputTail)
}

// clearPreDevTaskProgressLine keeps BuildKit-style progress output from visually merging with GoForj's final error line.
func clearPreDevTaskProgressLine() {
	if term.IsTerminal(int(os.Stderr.Fd())) {
		fmt.Fprint(os.Stderr, "\r\x1b[2K")
	}
}

// devTaskFailureError includes the task output tail because pre-dev failures often happen before the TUI can preserve details.
func devTaskFailureError(name string, reason string, output string) error {
	output = strings.TrimSpace(output)
	if output == "" {
		return fmt.Errorf("pre-dev task '%s' %s", name, reason)
	}
	return fmt.Errorf("pre-dev task '%s' %s\n\nLast task output:\n%s", name, reason, output)
}

type devTaskOutputTail struct {
	mu      sync.Mutex
	max     int
	lines   []string
	partial string
}

// newDevTaskOutputTail records the final lines of a streamed pre-dev command without hiding live output.
func newDevTaskOutputTail(max int) *devTaskOutputTail {
	return &devTaskOutputTail{max: max}
}

// Write records complete output lines and treats carriage returns as progress-line boundaries.
func (t *devTaskOutputTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	input := t.partial + string(p)
	input = strings.ReplaceAll(input, "\r", "\n")
	parts := strings.Split(input, "\n")
	t.partial = parts[len(parts)-1]
	for _, line := range parts[:len(parts)-1] {
		t.appendLine(line)
	}
	return len(p), nil
}

// String returns a normalized output tail suitable for the final error message.
func (t *devTaskOutputTail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()

	lines := append([]string(nil), t.lines...)
	if strings.TrimSpace(t.partial) != "" {
		lines = append(lines, t.normalizeLine(t.partial))
	}
	return strings.Join(lines, "\n")
}

// appendLine keeps only non-empty, normalized lines so progress noise does not bury the useful error.
func (t *devTaskOutputTail) appendLine(line string) {
	line = t.normalizeLine(line)
	if line == "" {
		return
	}
	t.lines = append(t.lines, line)
	if t.max > 0 && len(t.lines) > t.max {
		t.lines = t.lines[len(t.lines)-t.max:]
	}
}

// normalizeLine strips terminal control noise before the line is repeated outside the TUI.
func (t *devTaskOutputTail) normalizeLine(line string) string {
	line = stripANSIForSearch(line)
	return strings.TrimSpace(line)
}

// runPreDevSetup orders configured tasks around migration and reports when generated source requires another build.
func runPreDevSetup(config *project.Config) (bool, error) {
	preTasks := effectiveDevPreTasks(config)
	postMigrateTasks := make([]project.DevTask, 0, len(config.Dev.Pre))
	rebuildAfterSetup := false
	for _, task := range preTasks {
		if shouldRunAfterMigrate(task) {
			rebuildAfterSetup = true
		}
	}
	if shouldRunDevAutoMigrate(config) {
		allTasks := preTasks
		preTasks = make([]project.DevTask, 0, len(allTasks))
		for _, task := range allTasks {
			if shouldRunAfterMigrate(task) {
				postMigrateTasks = append(postMigrateTasks, task)
				continue
			}
			preTasks = append(preTasks, task)
		}
	}
	if err := runDevTasks("Running pre-dev setup", preTasks); err != nil {
		return false, err
	}
	if err := runDevAppSetup(config, os.Stdout, os.Stderr); err != nil {
		return false, err
	}
	if err := runDevTasks("Running post-migrate setup", postMigrateTasks); err != nil {
		return false, err
	}
	return rebuildAfterSetup, nil
}

// runDevAppSetup catches databases and migrations up before dev starts or restarts app processes.
func runDevAppSetup(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	if !shouldRunDevAutoMigrate(config) {
		return nil
	}
	if config.Render.Components.Docker {
		start := time.Now()
		writeDevActionLine(outWriter, "Ensuring dev databases")
		if err := ensureDevDatabaseExistsWithWriters(config, outWriter, errWriter); err != nil {
			return err
		}
		writeDevTimingLine(outWriter, "Dev databases ready in "+formatDevElapsed(time.Since(start)))
	}
	start := time.Now()
	writeDevActionLine(outWriter, "Running auto-migrate")
	res, err := execx.Command("bash", "-c", devAutoMigrateShellCommand(config)).
		EnvInherit().
		Env(devAutoMigrateEnv()).
		StdinReader(os.Stdin).
		StdoutWriter(outWriter).
		StderrWriter(errWriter).
		Run()
	if err != nil {
		return fmt.Errorf("auto-migrate failed: %v", err)
	}
	if !res.OK() {
		return fmt.Errorf("auto-migrate failed with exit code %d", res.ExitCode)
	}
	writeDevTimingLine(outWriter, "Auto-migrate finished in "+formatDevElapsed(time.Since(start)))
	return nil
}

// shouldRunDevAutoMigrate checks app-local components, not only the root app selection.
func shouldRunDevAutoMigrate(config *project.Config) bool {
	if !config.Dev.AutoMigrate {
		return false
	}
	for _, target := range activeDevAppsForConfig(config) {
		if appRenderComponents(config, target).HasDatabase() {
			return true
		}
	}
	return false
}

// devAutoMigrateShellCommand runs migrations through a participating database App whose binary the native graph built.
func devAutoMigrateShellCommand(config *project.Config) string {
	for _, app := range activeDevAppsForConfig(config) {
		if appRenderComponents(config, app).HasDatabase() {
			return projectlayout.RuntimeExecutable(".", app) + " migrate"
		}
	}
	return activeDevAppBinaryPath() + " migrate"
}

// devAutoMigrateEnv marks auto-migrate as an unqualified framework command so the generated migration planner can fan out across Apps.
func devAutoMigrateEnv() map[string]string {
	return map[string]string{
		"FORJ_COMMAND_PREFIX": "forj",
	}
}

// activeDevApp returns the app selected for this dev session.
func activeDevApp() project.App {
	appName := requestedDevAppName()
	if appName == "" {
		appName = project.DefaultAppName
	}
	return project.DefaultNamedApp(appName)
}

// activeDevAppBinaryPath points dev helpers at the active app binary.
func activeDevAppBinaryPath() string {
	return projectlayout.RuntimeExecutable(".", activeDevApp())
}

// devWatchesForApps expands the single-app default watchers across every discovered app.
func devWatchesForApps(config *project.Config, watches []project.DevWatch) []project.DevWatch {
	apps := activeDevAppsForConfig(config)
	rewritten := make([]project.DevWatch, 0, len(watches)*len(apps))
	for _, watch := range watches {
		if !watch.IsLegacy() {
			rewritten = append(rewritten, watch)
			continue
		}
		switch watch.Name {
		case "Build App", "Run App":
			for _, app := range apps {
				appWatch, ok := devWatchForAppWithConfig(config, watch, app)
				if ok {
					rewritten = append(rewritten, appWatch)
				}
			}
		default:
			rewritten = append(rewritten, watch)
		}
	}
	return rewritten
}

// devWatchForAppWithConfig preserves pre-allowlist watcher commands while applying an explicit legacy dev.run allowlist when present.
func devWatchForAppWithConfig(config *project.Config, watch project.DevWatch, app project.App) (project.DevWatch, bool) {
	if watch.Name != "Run App" {
		return devWatchForApp(watch, app), true
	}
	if config == nil || config.Dev.Run == nil {
		return devWatchForApp(watch, app), true
	}

	app = projectlayout.NormalizeApp(app)
	runCommand, ok := devRunCommandForApp(config, app)
	if !ok {
		return project.DevWatch{}, false
	}

	appWatch := devWatchForApp(watch, app)
	binary := projectlayout.RuntimeExecutable(".", app)
	appWatch.Exec = strings.TrimSpace(binary + " " + runCommand)
	return appWatch, true
}

// devRunCommandForApp resolves an App command from the explicit legacy dev.run allowlist.
func devRunCommandForApp(config *project.Config, app project.App) (string, bool) {
	if config == nil || config.Dev.Run == nil {
		return "", false
	}
	app = projectlayout.NormalizeApp(app)
	command, ok := config.Dev.Run[app.Name]
	if !ok {
		return "", false
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return "", false
	}
	return command, true
}

func normalizeDevWatchesForRuntime(config *project.Config, watches []project.DevWatch) []project.DevWatch {
	usesTempl := configUsesTemplHTMX(config)
	normalized := copyDevWatches(watches)
	for i := range normalized {
		normalized[i].Watch = normalizeDevWatchWireGenExclusion(normalized[i].Watch)
		if isDevBuildWatcher(normalized[i].Name) || isDevRunWatcher(normalized[i].Name) {
			normalized[i].Watch = normalizeDevWatchEnvFileTriggers(normalized[i].Watch)
		}
		if normalized[i].Name == "NPM" && strings.TrimSpace(normalized[i].Exec) == "npm run dev" {
			normalized[i].Watch = normalizeFrontendNPMWatchExclusions(normalized[i].Watch)
		}
		if usesTempl && normalized[i].Name == "Build App" {
			normalized[i].Watch = normalizeTemplBuildWatchExclusions(normalized[i].Watch)
		}
	}
	return normalized
}

// normalizeDevWatchEnvFileTriggers lets the coordinated env watcher own env rebuilds.
func normalizeDevWatchEnvFileTriggers(watch string) string {
	return removeWatchArgs(watch, map[string]map[string]bool{
		"-file": {
			".env":   true,
			".env.*": true,
		},
	})
}

// configUsesTemplHTMX identifies generated templ output so build watchers can avoid feedback loops.
func configUsesTemplHTMX(config *project.Config) bool {
	if project.NormalizeStarterKit(config.Render.StarterKit) == project.StarterKitTemplHTMX {
		return true
	}
	for _, app := range config.Apps {
		if project.NormalizeStarterKit(app.StarterKit) == project.StarterKitTemplHTMX {
			return true
		}
	}
	return false
}

// copyDevWatches preserves the configured watcher template before dev expands app watchers.
func copyDevWatches(watches []project.DevWatch) []project.DevWatch {
	copied := make([]project.DevWatch, len(watches))
	for index, watch := range watches {
		watch.Env = copyDevWatchEnv(watch.Env)
		copied[index] = watch
	}
	return copied
}

// activeDevApps returns one explicit app or every conventional app for all-app dev.
func activeDevApps() []project.App {
	if appName := requestedDevAppName(); appName != "" {
		return []project.App{project.DefaultNamedApp(appName)}
	}
	apps := projectlayout.ConventionalApps(".")
	if len(apps) == 0 {
		return []project.App{project.DefaultApp()}
	}
	return apps
}

// activeDevAppsForConfig uses structured dev.apps as the authority and falls back to legacy discovery.
func activeDevAppsForConfig(config *project.Config) []project.App {
	if !config.Dev.UsesStructuredApps() {
		return activeDevApps()
	}
	selected := selectedStructuredDevApps(config)
	apps := make([]project.App, 0, len(selected))
	for _, app := range selected {
		apps = append(apps, project.DefaultNamedApp(app.name))
	}
	return apps
}

// requestedDevAppName reports whether the CLI selected a single app for this dev session.
func requestedDevAppName() string {
	appName := strings.TrimSpace(os.Getenv("FORJ_APP"))
	if appName == "" || !project.IsSafeAppName(appName) || project.IsReservedAppName(appName) {
		return ""
	}
	return appName
}

// devWatchForApp rewrites one default watcher for a specific app without editing config.
func devWatchForApp(watch project.DevWatch, app project.App) project.DevWatch {
	app = projectlayout.NormalizeApp(app)
	baseName := watch.Name
	if app.Name != project.DefaultAppName {
		watch.Name = strings.ReplaceAll(watch.Name, "App", app.Name)
	}
	appBinary := projectlayout.RuntimeExecutable(".", app)
	appReady := projectlayout.RuntimeReadyStamp(".", app)
	appWireGen := filepath.ToSlash(filepath.Join(projectlayout.WireDir(".", app), "wire_gen")) + `\.go$`
	if isDevBuildWatcher(baseName) {
		watch.Exec = devBuildCommandForApp(watch.Exec, app)
	} else {
		watch.Exec = strings.ReplaceAll(watch.Exec, "./bin/app", appBinary)
	}
	watch.Watch = str.Of(watch.Watch).
		ReplaceAll("./bin/app", appBinary).
		ReplaceAll("bin/app", strings.TrimPrefix(appBinary, "./")).
		String()
	if isDevRunWatcher(baseName) {
		watch.Watch = str.Of(watch.Watch).
			ReplaceAll(appBinary, appReady).
			ReplaceAll(strings.TrimPrefix(appBinary, "./"), strings.TrimPrefix(appReady, "./")).
			String()
	}
	watch.Watch = strings.ReplaceAll(watch.Watch, "app/wire/wire_gen\\.go$", appWireGen)
	watch.Env = copyDevWatchEnv(watch.Env)
	watch.Env["FORJ_APP"] = app.Name
	if app.Name == project.DefaultAppName {
		watch.Env["FORJ_COMMAND_PREFIX"] = "forj"
	} else {
		watch.Env["FORJ_COMMAND_PREFIX"] = "forj " + app.Name
	}
	return watch
}

// isDevRunWatcher reports whether a watcher supervises a built app process.
func isDevRunWatcher(name string) bool {
	return name == "Run App" || strings.HasPrefix(name, "Run ")
}

// copyDevWatchEnv prevents synthesized app watchers from sharing mutable env maps.
func copyDevWatchEnv(env map[string]string) map[string]string {
	cloned := make(map[string]string, len(env)+3)
	for key, value := range env {
		cloned[key] = value
	}
	return cloned
}

type devBuildJob struct {
	app     project.App
	command string
	dir     string
	env     map[string]string
}

// devBuildJobs resolves app build commands while preserving the app label for compact output.
func devBuildJobs(config *project.Config) []devBuildJob {
	apps := activeDevAppsForConfig(config)
	jobs := make([]devBuildJob, 0, len(apps))
	baseCommand := devBaseBuildCommand(config)
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		command := devBuildCommandForConfigApp(config, baseCommand, app)
		if strings.TrimSpace(command) == "" {
			continue
		}
		dir, env := devBuildExecutionForConfigApp(config, app)
		jobs = append(jobs, devBuildJob{
			app: app, command: command, dir: dir,
			env: env,
		})
	}
	return jobs
}

// devBuildExecutionForConfigApp carries structured workdir and environment overrides into every build path.
func devBuildExecutionForConfigApp(config *project.Config, app project.App) (string, map[string]string) {
	devApp, ok := config.Dev.Apps[app.Name]
	if !ok {
		return "", nil
	}
	var workDir string
	var configuredEnv map[string]string
	if devApp.Build != nil {
		workDir = devApp.Build.WorkDir
		configuredEnv = devApp.Build.Env
	}
	env := frameworkDevAppEnv(app, configuredEnv)
	return workDir, env
}

// devBuildCommandForConfigApp applies a structured per-app override before legacy template rewriting.
func devBuildCommandForConfigApp(config *project.Config, baseCommand string, app project.App) string {
	if devApp, ok := config.Dev.Apps[app.Name]; ok && devApp.Build != nil {
		if devApp.Build.Disabled {
			return ""
		}
		if command := strings.TrimSpace(devApp.Build.Exec); command != "" {
			return command
		}
	}
	return devBuildCommandForApp(baseCommand, app)
}

// devBaseBuildCommand uses the configured default build watcher as the template for app builds.
func devBaseBuildCommand(config *project.Config) string {
	if len(config.Dev.Apps) > 0 {
		return "forj build -o ./bin/app"
	}
	for _, watch := range config.Dev.Watches {
		if watch.Name == "Build App" && strings.TrimSpace(watch.Exec) != "" {
			return watch.Exec
		}
	}
	for _, watch := range config.Dev.Watches {
		if isDevBuildWatcher(watch.Name) && strings.TrimSpace(watch.Exec) != "" {
			return watch.Exec
		}
	}
	return "forj build -o ./bin/app"
}

// devBuildCommandForApp derives app builds from the configured default build command.
func devBuildCommandForApp(baseCommand string, app project.App) string {
	app = projectlayout.NormalizeApp(app)
	command := strings.TrimSpace(baseCommand)
	if command == "" {
		command = "forj build -o ./bin/app"
	}

	defaultApp := project.DefaultApp()
	defaultBinary := projectlayout.RuntimeExecutable(".", defaultApp)
	appBinary := projectlayout.RuntimeExecutable(".", app)
	defaultPackage := "./" + filepath.ToSlash(projectlayout.CommandDir(".", defaultApp))
	appPackage := "./" + filepath.ToSlash(projectlayout.CommandDir(".", app))

	command = replaceBuildToken(command, defaultBinary, appBinary)
	command = replaceBuildToken(command, strings.TrimPrefix(defaultBinary, "./"), strings.TrimPrefix(appBinary, "./"))
	command = replaceBuildToken(command, defaultPackage, appPackage)
	command = replaceBuildToken(command, strings.TrimPrefix(defaultPackage, "./"), strings.TrimPrefix(appPackage, "./"))
	command = removeBuildPackageToken(command, appPackage)
	if app.Name != project.DefaultAppName {
		command = prefixForjBuildCommandForApp(command, app.Name)
	}
	return strings.TrimSpace(command)
}

// replaceBuildToken preserves the configured command shape while rewriting conventional app paths.
func replaceBuildToken(command string, from string, to string) string {
	return strings.ReplaceAll(command, from, to)
}

// removeBuildPackageToken lets the app-aware build command infer its own entrypoint package.
func removeBuildPackageToken(command string, targetPackage string) string {
	targetPackage = strings.TrimSpace(targetPackage)
	if targetPackage == "" {
		return strings.TrimSpace(command)
	}
	command = str.Of(command).
		ReplaceAll(" "+targetPackage, "").
		ReplaceAll(" "+strings.TrimPrefix(targetPackage, "./"), "").
		String()
	return strings.TrimSpace(command)
}

// prefixForjBuildCommandForApp uses the same app command shape users run by hand.
func prefixForjBuildCommandForApp(command string, appName string) string {
	appName = strings.TrimSpace(appName)
	if appName == "" || appName == project.DefaultAppName {
		return command
	}
	return strings.Replace(command, "forj build", "forj "+appName+" build", 1)
}

func shouldRunAfterMigrate(task project.DevTask) bool {
	name := str.Of(task.Name)
	cmd := str.Of(task.Cmd)
	if name.ContainsFold("generate db accessors") {
		return true
	}
	return cmd.ContainsFold("generate")
}

// runningWatcher retains logical identity so one shared controller generation can report and drain every configured watcher.
type runningWatcher struct {
	id   string
	name string
}

// watcherExit carries one logical watcher's terminal event from the shared controller to the outer loop.
type watcherExit struct {
	id      string
	name    string
	process *devwatch.Exit
	err     error
	output  string
}

type watcherLifecycleState string

const (
	watcherStateStarted  watcherLifecycleState = "started"
	watcherStateStopping watcherLifecycleState = "stopping"
	watcherStateStopped  watcherLifecycleState = "stopped"
)

// runWatchersLoop starts all configured watchers, handles restart requests, and surfaces exit errors.
func (c *DevCmd) runWatchersLoop(session *devWatchSession) error {
watcherLoop:
	for {
		if err := session.reloadProjectConfig(); err != nil {
			return err
		}
		session.config.Dev.Watches = devWatchesForApps(session.config, session.baseWatches)
		runtime, err := startDevWatcherRuntime(session)
		if err != nil {
			return err
		}
		if session.managedHeartbeat != nil {
			heartbeat := session.managedHeartbeat
			session.managedHeartbeat = nil
			heartbeat()
		}
		if session.managedBarrier != nil {
			barrier := session.managedBarrier
			session.managedBarrier = nil
			go func() {
				if err := barrier(session.managedContext); err != nil && !errors.Is(err, context.Canceled) {
					_, _ = fmt.Fprintf(session.errWriter, "forj dev: managed Compose barrier unavailable: %v\n", err)
				}
			}()
		}
		if session.reconcile {
			reconcileErr := runDevWatcherReconciliation(
				session.config,
				session.outWriter,
				session.errWriter,
				session.reconcileFrontendDeps,
			)
			session.reconcile = false
			session.reconcileFrontendDeps = false
			if reconcileErr != nil {
				runtime.stopAndDrain(true)
				return fmt.Errorf("dev watcher reconciliation failed: %w", reconcileErr)
			}
			runtime.controller.finishReconciliation(true)
		}
		printDevReadySummary(session.outWriter, session.config, snapshotProcessEnv())
		for {
			select {
			case <-session.stopCh:
				disableDevFooter(session.outWriter)
				disableDevFooter(session.errWriter)
				fmt.Println(buildDevWatcherStopSeparatorLine())
				runtime.stopAndDrain(true)
				return errDevInterrupted
			case <-session.restartCh:
				writeDevActionLine(session.outWriter, "Restarting dev watchers")
				runtime.stopAndDrain(true)
				drainRestartSignals(session.restartCh)
				refreshedStreamer, err := session.reloadRuntime()
				if err != nil {
					return err
				}
				session.streamer = refreshedStreamer
				continue watcherLoop
			case <-session.buildCh:
				writeDevActionLine(session.outWriter, "Rebuilding app and restarting watchers")
				spaRootsBeforeBuild := structuredDevSPARoots(session.config)
				if err := session.reloadProjectConfig(); err != nil {
					runtime.stopAndDrain(true)
					return err
				}
				if hasNewStructuredDevSPARoot(spaRootsBeforeBuild, session.config) {
					session.reconcileFrontendDeps = true
				}
				quiesceContext, cancelQuiesce := devWatchStopContext(session.stopCh)
				err := runtime.controller.quiesceBuilds(quiesceContext)
				cancelQuiesce()
				if err != nil {
					runtime.stopAndDrain(true)
					if errors.Is(err, context.Canceled) {
						return errDevInterrupted
					}
					return fmt.Errorf("quiesce dev builds: %w", err)
				}
				if err := loadDevEnvironment(true); err != nil {
					runtime.stopAndDrain(true)
					return fmt.Errorf("reload dev environment: %w", err)
				}
				if err := runDevBuild(session.config, session.outWriter, session.errWriter); err != nil {
					runtime.controller.resumeBuilds()
					console.Errorf("forj build failed: %v", err)
					resetDevFooterLine(session.outWriter)
					resetDevFooterLine(session.errWriter)
					clearDevStatusLine(session.outWriter)
					clearDevStatusLine(session.errWriter)
					drainBuildSignals(session.buildCh)
					continue
				}
				session.reconcile = shouldReconcileStructuredDevApps(session.config)
				runtime.stopAndDrain(true)
				refreshedStreamer, err := session.reloadRuntime()
				if err != nil {
					return err
				}
				session.streamer = refreshedStreamer
				if !session.reconcile {
					if err := runDevAppSetup(session.config, session.outWriter, session.errWriter); err != nil {
						disableDevFooter(session.outWriter)
						disableDevFooter(session.errWriter)
						fmt.Println(buildDevFooterSeparatorLine())
						console.Errorf("dev app setup failed: %v", err)
						return fmt.Errorf("dev app setup failed: %w", err)
					}
				}
				refreshedStreamer, err = session.reloadRuntime()
				if err != nil {
					return err
				}
				session.streamer = refreshedStreamer
				resetDevFooterLine(session.outWriter)
				resetDevFooterLine(session.errWriter)
				clearDevStatusLine(session.outWriter)
				clearDevStatusLine(session.errWriter)
				drainBuildSignals(session.buildCh)
				continue watcherLoop
			case <-session.renderCh:
				writeDevActionLine(session.outWriter, "Rendering app and restarting watchers")
				spaRootsBeforeRender := structuredDevSPARoots(session.config)
				waitForStop := runtime.beginStop(5*time.Second, true)
				if err := session.reloadProjectConfig(); err != nil {
					waitForStop()
					runtime.drainAllExits(true)
					return err
				}
				refreshedStreamer, err := session.reloadRuntime()
				if err != nil {
					waitForStop()
					runtime.drainAllExits(true)
					return err
				}
				session.streamer = refreshedStreamer
				renderErr := runDevRenderCommand(session.outWriter, session.errWriter)
				waitForStop()
				runtime.drainAllExits(true)
				if renderErr != nil {
					disableDevFooter(session.outWriter)
					disableDevFooter(session.errWriter)
					fmt.Println(buildDevFooterSeparatorLine())
					console.Errorf("forj render failed: %v", renderErr)
					return fmt.Errorf("forj render failed: %w", renderErr)
				}
				if err := session.reloadProjectConfig(); err != nil {
					return err
				}
				session.reconcile = shouldReconcileStructuredDevApps(session.config)
				if hasNewStructuredDevSPARoot(spaRootsBeforeRender, session.config) {
					session.reconcileFrontendDeps = true
				}
				if !session.reconcile {
					if err := runDevBuild(session.config, session.outWriter, session.errWriter); err != nil {
						return err
					}
				}
				if !session.reconcile {
					if err := runDevAppSetup(session.config, session.outWriter, session.errWriter); err != nil {
						disableDevFooter(session.outWriter)
						disableDevFooter(session.errWriter)
						fmt.Println(buildDevFooterSeparatorLine())
						console.Errorf("dev app setup failed: %v", err)
						return fmt.Errorf("dev app setup failed: %w", err)
					}
				}
				refreshedStreamer, err = session.reloadRuntime()
				if err != nil {
					return err
				}
				session.streamer = refreshedStreamer
				resetDevFooterLine(session.outWriter)
				resetDevFooterLine(session.errWriter)
				clearDevStatusLine(session.outWriter)
				clearDevStatusLine(session.errWriter)
				drainRenderSignals(session.renderCh)
				continue watcherLoop
			case req := <-session.commandCh:
				if strings.TrimSpace(req.ShellCommand) == "" {
					continue
				}
				if err := runDevTranscriptCommand(session.outWriter, session.errWriter, "Running "+req.DisplayName, req.ShellCommand); err != nil {
					_, _ = fmt.Fprintf(session.outWriter, "%s %v\n", console.ErrorMark(), err)
				}
			case exit := <-runtime.exitCh:
				runtime.stopAfterExit(exit, 5*time.Second)
				if err := unexpectedWatcherExitError(exit); err != nil {
					return err
				}
				return nil
			}
		}
	}
}

// devWatchStopContext makes a session stop interrupt build quiescing without leaking a waiter goroutine.
func devWatchStopContext(stop <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

// reloadProjectConfig keeps a long-running dev session aligned with make:app and render metadata changes.
func (session *devWatchSession) reloadProjectConfig() error {
	config, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}
	session.config = config
	session.baseWatches = normalizeDevWatchesForRuntime(config, copyDevWatches(config.Dev.Watches))
	return nil
}

// snapshotProcessEnv splits only well-formed process entries while preserving values that contain equals signs.
func snapshotProcessEnv() map[string]string {
	return processEnvironmentMap(os.Environ())
}

// processEnvironmentMap converts process entries without truncating values or admitting empty platform pseudo-keys.
func processEnvironmentMap(entries []string) map[string]string {
	envMap := map[string]string{}
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			continue
		}
		envMap[key] = value
	}
	return envMap
}

// runDevRenderCommand renders generated sources before the reloaded config chooses the build graph.
func runDevRenderCommand(outWriter io.Writer, errWriter io.Writer) error {
	if err := runDevTerminalCommand(outWriter, errWriter, "Running forj render", "forj render --timings"); err != nil {
		return fmt.Errorf("forj render failed: %w", err)
	}
	return nil
}

// runDevRender keeps the legacy render-and-build entrypoint available to callers
// that do not need the native watcher's finer reconciliation barrier.
func runDevRender(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	if err := runDevRenderCommand(outWriter, errWriter); err != nil {
		return err
	}
	if err := runDevBuild(config, outWriter, errWriter); err != nil {
		return err
	}
	return nil
}

// runDevWatcherReconciliation rebuilds frontend assets as a barrier before publishing app runtimes.
func runDevWatcherReconciliation(config *project.Config, outWriter io.Writer, errWriter io.Writer, installFrontendDeps bool) error {
	if installFrontendDeps {
		if err := runDevFrontendDependencySetup(config); err != nil {
			return fmt.Errorf("install frontend dependencies: %w", err)
		}
	}
	if _, err := runDevInitialSPABuilds(config, outWriter, errWriter); err != nil {
		return err
	}
	if err := runDevBuild(config, outWriter, errWriter); err != nil {
		return err
	}
	if err := runDevAppSetup(config, outWriter, errWriter); err != nil {
		return fmt.Errorf("dev app setup failed: %w", err)
	}
	return nil
}

// shouldReconcileStructuredDevApps limits the gated handoff to listed native lifecycle graphs.
func shouldReconcileStructuredDevApps(config *project.Config) bool {
	return len(selectedStructuredDevApps(config)) > 0
}

// runDevFrontendDependencySetup runs only generated frontend installation tasks after render introduces a new SPA.
func runDevFrontendDependencySetup(config *project.Config) error {
	tasks := make([]project.DevTask, 0)
	for _, selected := range selectedStructuredDevApps(config) {
		if len(selected.config.SPAs) == 0 {
			continue
		}
		want := generatedDevFrontendInstallTask(project.DefaultNamedApp(selected.name))
		for _, task := range config.Dev.Pre {
			if task == want {
				tasks = append(tasks, task)
				break
			}
		}
	}
	return runDevTasks("Installing frontend dependencies", tasks)
}

// structuredDevSPARoots snapshots configured frontend roots so render can identify newly generated workspaces.
func structuredDevSPARoots(config *project.Config) map[string]bool {
	roots := map[string]bool{}
	for _, app := range config.Dev.Apps {
		for _, spa := range app.SPAs {
			root := filepath.Clean(strings.TrimSpace(spa.Path))
			if root != "." && root != "" {
				roots[root] = true
			}
		}
	}
	return roots
}

// hasNewStructuredDevSPARoot reports whether render introduced a frontend that may still need dependencies.
func hasNewStructuredDevSPARoot(before map[string]bool, config *project.Config) bool {
	for root := range structuredDevSPARoots(config) {
		if !before[root] {
			return true
		}
	}
	return false
}

// runDevBuild rebuilds every active app so a previous binary never masks current source changes.
func runDevBuild(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	return runDevBuildJobs(config, outWriter, errWriter, "forj build failed")
}

// runDevInitialBuild builds every active app before pre-dev tasks can call generated app commands.
func runDevInitialBuild(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	return runDevBuildJobs(config, outWriter, errWriter, "initial forj build failed")
}

// runDevInitialSPABuilds publishes frontend assets after dependency setup and before the runtime-owning rebuild.
func runDevInitialSPABuilds(config *project.Config, outWriter io.Writer, errWriter io.Writer) (bool, error) {
	watchers, err := compileStructuredDevApps(config)
	if err != nil {
		return false, err
	}
	built := false
	for _, watcher := range watchers {
		if watcher.Kind != devWatcherSPABuild {
			continue
		}
		writeDevActionLine(outWriter, "Building "+watcher.App+" frontend")
		if err := runDevSubprocess(devSubprocessRun{
			command:    watcher.Command.Shell,
			dir:        watcher.Command.Dir,
			env:        watcher.Command.Env,
			stdout:     outWriter,
			stderr:     errWriter,
			transcript: true,
		}); err != nil {
			return false, fmt.Errorf("initial SPA build %q failed: %w", watcher.Name, err)
		}
		built = true
	}
	return built, nil
}

type devBuildResult struct {
	job     devBuildJob
	stdout  string
	stderr  string
	elapsed time.Duration
	err     error
}

// runDevBuildJobs runs app builds together so multi-app dev startup scales with the slowest build.
func runDevBuildJobs(config *project.Config, outWriter io.Writer, errWriter io.Writer, failurePrefix string) error {
	jobs := devBuildJobs(config)
	clearDevBuildReadyStamps(jobs)
	switch len(jobs) {
	case 0:
		return nil
	case 1:
		start := time.Now()
		if err := runDevBuildCommand(outWriter, errWriter, jobs[0]); err != nil {
			return fmt.Errorf("%s: %w", failurePrefix, err)
		}
		writeDevTimingLine(outWriter, "Built "+jobs[0].app.Name+" in "+formatDevElapsed(time.Since(start)))
		return nil
	}

	heading := "Building apps"
	start := time.Now()

	results := make([]devBuildResult, len(jobs))
	runJobs := func() {
		var wg sync.WaitGroup
		for index, job := range jobs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				results[index] = runDevBuildJobBuffered(job)
			}()
		}
		wg.Wait()
	}
	if usesDevBootstrapConsole(outWriter, errWriter) {
		if err := runWithLoader(heading, func() error {
			runJobs()
			return nil
		}); err != nil {
			return err
		}
	} else {
		setDevStatusLine(outWriter, heading)
		defer clearDevStatusLine(outWriter)
		runJobs()
	}

	failures := make([]string, 0)
	for _, result := range results {
		if result.err == nil {
			continue
		}
		writeDevBuildFailureOutput(outWriter, errWriter, result)
		failures = append(failures, fmt.Sprintf("%s: %v", result.job.app.Name, result.err))
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s: %s", failurePrefix, strings.Join(failures, "; "))
	}
	writeDevTimingLine(outWriter, "Built apps in "+formatDevElapsed(time.Since(start)))
	return nil
}

// runDevBuildJobBuffered keeps concurrent build output grouped by app instead of interleaving lines.
func runDevBuildJobBuffered(job devBuildJob) devBuildResult {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	start := time.Now()
	err := runDevSubprocess(devSubprocessRun{
		command:    job.command,
		dir:        job.dir,
		env:        job.env,
		stdout:     &stdout,
		stderr:     &stderr,
		transcript: true,
	})
	if err == nil {
		err = publishDevBuildReadyStamp(job.app)
	}
	return devBuildResult{
		job:     job,
		stdout:  stripBuildProgressMarkerLines(stdout.String()),
		stderr:  stripBuildProgressMarkerLines(stderr.String()),
		elapsed: time.Since(start),
		err:     err,
	}
}

// stripBuildProgressMarkerLines keeps the subprocess progress protocol out of replayed build logs.
func stripBuildProgressMarkerLines(output string) string {
	if !strings.Contains(output, buildProgressMarker) {
		return output
	}
	lines := strings.SplitAfter(output, "\n")
	var filtered strings.Builder
	for _, line := range lines {
		normalized := strings.ReplaceAll(line, "\r", "")
		normalized = ansiCSI.ReplaceAllString(normalized, "")
		if strings.HasPrefix(strings.TrimSpace(normalized), buildProgressMarker) {
			continue
		}
		filtered.WriteString(line)
	}
	return filtered.String()
}

// writeDevBuildFailureOutput prints a failed app's buffered transcript only when it is useful.
func writeDevBuildFailureOutput(outWriter io.Writer, errWriter io.Writer, result devBuildResult) {
	writeDevActionLine(outWriter, "Build failed for "+result.job.app.Name)
	writeDevBuildBufferedOutput(outWriter, errWriter, result)
}

// writeDevBuildBufferedOutput preserves owner-authored build messages after transient progress has released the terminal.
func writeDevBuildBufferedOutput(outWriter io.Writer, errWriter io.Writer, result devBuildResult) {
	if strings.TrimSpace(result.stdout) != "" {
		_, _ = io.WriteString(outWriter, result.stdout)
		if !strings.HasSuffix(result.stdout, "\n") {
			_, _ = io.WriteString(outWriter, "\n")
		}
	}
	if strings.TrimSpace(result.stderr) != "" {
		_, _ = io.WriteString(errWriter, result.stderr)
		if !strings.HasSuffix(result.stderr, "\n") {
			_, _ = io.WriteString(errWriter, "\n")
		}
	}
}

// runDevBuildCommand keeps app builds compact in the dev transcript.
func runDevBuildCommand(outWriter io.Writer, errWriter io.Writer, job devBuildJob) error {
	heading := "Building " + job.app.Name
	if usesDevBootstrapConsole(outWriter, errWriter) {
		return runDevBuildJobWithLoader(outWriter, errWriter, heading, job)
	}
	writeDevActionLine(outWriter, heading)
	setDevStatusLine(outWriter, heading)
	defer clearDevStatusLine(outWriter)

	_, stdoutIsBubble := outWriter.(*devBubbleWriter)
	_, stderrIsBubble := errWriter.(*devBubbleWriter)
	if stdoutIsBubble || stderrIsBubble {
		if err := runDevSubprocess(devSubprocessRun{
			command:    job.command,
			dir:        job.dir,
			env:        job.env,
			stdout:     outWriter,
			stderr:     errWriter,
			transcript: true,
		}); err != nil {
			return err
		}
		return publishDevBuildReadyStamp(job.app)
	}

	disableDevFooter(outWriter)
	disableDevFooter(errWriter)
	defer enableDevFooter(outWriter)
	defer enableDevFooter(errWriter)
	if err := runDevSubprocess(devSubprocessRun{
		command: job.command,
		dir:     job.dir,
		env:     job.env,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	}); err != nil {
		return err
	}
	return publishDevBuildReadyStamp(job.app)
}

// runDevBuildJobWithLoader keeps the bootstrap terminal active while deferring child output until the transient loader releases the terminal.
func runDevBuildJobWithLoader(outWriter io.Writer, errWriter io.Writer, heading string, job devBuildJob) error {
	var result devBuildResult
	if err := runWithLoader(heading, func() error {
		result = runDevBuildJobBuffered(job)
		return nil
	}); err != nil {
		return err
	}
	if result.err != nil {
		writeDevBuildFailureOutput(outWriter, errWriter, result)
		return result.err
	}
	writeDevBuildBufferedOutput(outWriter, errWriter, result)
	return nil
}

// usesDevBootstrapConsole selects the shared console loader only before Bubble Tea owns the terminal streams.
func usesDevBootstrapConsole(outWriter io.Writer, errWriter io.Writer) bool {
	stdout, stdoutOK := outWriter.(*os.File)
	stderr, stderrOK := errWriter.(*os.File)
	return stdoutOK && stderrOK && stdout == os.Stdout && stderr == os.Stderr
}

// publishDevBuildReadyStamp makes custom successful build commands obey the same runtime publication gate as forj build.
func publishDevBuildReadyStamp(app project.App) error {
	app = projectlayout.NormalizeApp(app)
	binary := projectlayout.RuntimeBinary(".", app)
	if _, err := os.Stat(binary); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("inspect built app binary: %w", err)
	}
	ready := projectlayout.RuntimeReadyStamp(".", app)
	if err := os.MkdirAll(filepath.Dir(ready), 0o755); err != nil {
		return fmt.Errorf("create build ready directory: %w", err)
	}
	temporaryFile, err := os.CreateTemp(filepath.Dir(ready), "."+filepath.Base(ready)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create build ready stamp: %w", err)
	}
	temporary := temporaryFile.Name()
	if _, err := temporaryFile.WriteString(time.Now().UTC().Format(time.RFC3339Nano) + "\n"); err != nil {
		_ = temporaryFile.Close()
		_ = os.Remove(temporary)
		return fmt.Errorf("write build ready stamp: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("close build ready stamp: %w", err)
	}
	if err := os.Rename(temporary, ready); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("publish build ready stamp: %w", err)
	}
	return nil
}

// devSubprocessRun keeps command execution context together so callers cannot confuse positional strings, maps, and mode flags.
type devSubprocessRun struct {
	command    string
	dir        string
	env        map[string]string
	stdout     io.Writer
	stderr     io.Writer
	transcript bool
}

// runDevSubprocess keeps watcher-only progress records out of commands whose output is meant for people.
func runDevSubprocess(run devSubprocessRun) error {
	stdin := io.Reader(os.Stdin)
	env := map[string]string{"CLICOLOR_FORCE": "1"}
	for key, value := range run.env {
		env[key] = value
	}
	// Direct dev commands have no protocol consumer, so watcher-only progress must never reach their writers.
	env["FORJ_BUILD_PROGRESS"] = "0"
	if run.transcript {
		devNull, err := os.Open(os.DevNull)
		if err != nil {
			return fmt.Errorf("open %s: %w", os.DevNull, err)
		}
		defer devNull.Close()
		stdin = devNull
		env["FORJ_SUBPROCESS"] = "1"
		env["FORJ_COMMAND_ORIGIN"] = "dev_command"
		env["TERM"] = "dumb"
	}
	cmd := execx.Command("bash", "-c", run.command).
		EnvInherit().
		EnvAppend(env).
		Dir(run.dir).
		StdinReader(stdin).
		StdoutWriter(run.stdout).
		StderrWriter(run.stderr)
	res, err := cmd.Run()
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("%s exited with code %d", run.command, res.ExitCode)
	}
	return nil
}

func runDevTranscriptCommand(outWriter io.Writer, errWriter io.Writer, heading string, command string) error {
	writeDevCommandLine(outWriter, heading)
	setDevStatusLine(outWriter, heading)
	defer clearDevStatusLine(outWriter)
	if err := runDevSubprocess(devSubprocessRun{
		command:    command,
		stdout:     outWriter,
		stderr:     errWriter,
		transcript: true,
	}); err != nil {
		return err
	}
	writeDevCommandBoundary(outWriter)
	return nil
}

func runDevTerminalCommand(outWriter io.Writer, errWriter io.Writer, heading string, command string) error {
	if _, ok := outWriter.(*devBubbleWriter); ok {
		return runDevTranscriptCommand(outWriter, errWriter, heading, command)
	}
	if _, ok := errWriter.(*devBubbleWriter); ok {
		return runDevTranscriptCommand(outWriter, errWriter, heading, command)
	}

	writeDevActionLine(outWriter, heading)
	// Render output should go straight to the terminal so the renderer keeps
	// its native colors/box drawing and the sticky footer does not get replayed
	// into the transcript while ad hoc commands are running.
	disableDevFooter(outWriter)
	disableDevFooter(errWriter)
	defer enableDevFooter(outWriter)
	defer enableDevFooter(errWriter)

	return runDevSubprocess(devSubprocessRun{
		command: command,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
	})
}

// writeDevActionLine uses the session writer so plain terminals and the TUI preserve one action format.
func writeDevActionLine(out io.Writer, message string) {
	_, _ = io.WriteString(out, fmt.Sprintf("%s %s\n", console.ActionMark(), message))
}

// writeDevTimingLine keeps secondary duration details visually quieter than the action they describe.
func writeDevTimingLine(out io.Writer, message string) {
	line := console.Colorize(console.ColorGray, "  "+strings.TrimSpace(message))
	_, _ = io.WriteString(out, line+"\n")
}

func formatDevElapsed(elapsed time.Duration) string {
	if elapsed < time.Second {
		return elapsed.Round(time.Millisecond).String()
	}
	return elapsed.Round(100 * time.Millisecond).String()
}

// writeDevAppBuildLine makes convention-expanded apps visible without adding a large startup block.
func writeDevAppBuildLine(out io.Writer, apps []project.App) {
	names := devAppBuildNames(apps)
	if len(names) == 0 || len(names) == 1 && names[0] == project.DefaultAppName {
		return
	}
	label := "Building app"
	if len(names) > 1 {
		label = "Building apps"
	}
	writeDevActionLine(out, label+": "+strings.Join(names, ", "))
}

// devAppBuildNames formats app names in runtime order for user-facing dev output.
func devAppBuildNames(apps []project.App) []string {
	names := make([]string, 0, len(apps))
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		if app.Name == "" {
			continue
		}
		names = append(names, app.Name)
	}
	return names
}

// writeDevCommandLine frames ad hoc commands so their output remains distinct in the shared transcript.
func writeDevCommandLine(out io.Writer, message string) {
	label := console.Colorize(console.ColorBoldWhite, strings.TrimSpace(message))
	_, _ = io.WriteString(out, buildDevSectionSeparatorLine(label)+"\n")
}

// writeDevCommandBoundary restores a clear visual edge before watcher output resumes.
func writeDevCommandBoundary(out io.Writer) {
	_, _ = io.WriteString(out, buildDevFooterSeparatorLine()+"\n")
}

func printDevReadySummary(out io.Writer, config *project.Config, env map[string]string) {
	for _, line := range buildDevReadySummaryLines(config, env) {
		_, _ = io.WriteString(out, line+"\n")
	}
}

type devToolLink struct {
	Label  string
	URL    string
	Detail string
}

func buildDevReadySummaryLines(config *project.Config, env map[string]string) []string {
	tools := collectDevToolLinks(config, env)
	if len(tools) == 0 {
		return nil
	}

	lines := []string{
		fmt.Sprintf("%s %s", console.SuccessMark(), console.Colorize(console.ColorBoldWhite, "Dev ready")),
	}
	for _, tool := range tools {
		line := fmt.Sprintf("  %s %s", console.Colorize(console.ColorBoldGreen, "→"), console.Colorize(console.ColorBoldWhite, tool.Label))
		if tool.Detail != "" {
			line += " " + console.Colorize(console.ColorGray, tool.Detail)
		}
		line += ": " + console.Colorize(console.ColorBoldWhite, tool.URL)
		lines = append(lines, line)
	}
	return lines
}

// collectDevToolLinks advertises only resources enabled by the current project configuration.
func collectDevToolLinks(config *project.Config, env map[string]string) []devToolLink {
	tools := []devToolLink{}

	if apiURL := resolveAPIURL(env); apiURL != "" {
		tools = append(tools, devToolLink{Label: "App", URL: apiURL})
	}
	if lighthouseURL := resolveLighthouseUIURL(env); lighthouseURL != "" {
		tools = append(tools, devToolLink{Label: "Lighthouse", URL: lighthouseURL})
	}
	if swaggerURL := resolveSwaggerUIURL(env); swaggerURL != "" {
		tools = append(tools, devToolLink{Label: "Swagger", URL: swaggerURL})
	}

	components := config.Render.Components
	profiles := envValue(env, "COMPOSE_PROFILES")
	if components.Docker && exactCSVToken(profiles, "mailpit") {
		tools = append(tools, devToolLink{
			Label:  "Mailpit",
			Detail: "(inbox)",
			URL:    resolveURLWithPort(env, "http", "localhost", "MAILPIT_HTTP_PORT", "8025"),
		})
	}
	if components.Docker && (exactCSVToken(profiles, "victoriametrics") || exactCSVToken(profiles, "grafana")) {
		tools = append(tools, devToolLink{
			Label: "VictoriaMetrics",
			URL:   resolveURLWithPort(env, "http", "localhost", "OBSERVABILITY_VM_PORT", "8428"),
		})
	}
	if components.Docker && exactCSVToken(profiles, "grafana") {
		adminUser := strings.TrimSpace(envValue(env, "GRAFANA_ADMIN_USER"))
		if adminUser == "" {
			adminUser = "admin"
		}
		tools = append(tools, devToolLink{
			Label:  "Grafana",
			Detail: fmt.Sprintf("(%s)", adminUser),
			URL:    resolveURLWithPort(env, "http", "localhost", "GRAFANA_PORT", "13001"),
		})
	}

	return tools
}

func resolveAPIURL(env map[string]string) string {
	if raw := strings.TrimSpace(envValue(env, "APP_URL")); raw != "" {
		return raw
	}
	return "http://localhost:3000"
}

func resolveSwaggerUIURL(env map[string]string) string {
	enabled := str.Of(envValue(env, "API_SWAGGER_ENABLED")).Trim().ToLower().String()
	if enabled == "" {
		enabled = str.Of(envValue(env, "SWAGGER_ENABLED")).Trim().ToLower().String()
	}
	if enabled == "false" || enabled == "0" || enabled == "off" || enabled == "no" {
		return ""
	}

	apiURL := strings.TrimSpace(resolveAPIURL(env))
	if apiURL == "" {
		return ""
	}
	return strings.TrimRight(apiURL, "/") + "/swagger"
}

func resolveLighthouseUIURL(env map[string]string) string {
	enabled := str.Of(envValue(env, "LIGHTHOUSE_ENABLED")).Trim().ToLower().String()
	if enabled == "false" || enabled == "0" || enabled == "off" || enabled == "no" {
		return ""
	}

	raw := strings.TrimSpace(envValue(env, "LIGHTHOUSE_URL"))
	if raw == "" {
		return "http://localhost:3000/lighthouse"
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "http://localhost:3000/lighthouse"
	}
	switch strings.ToLower(u.Scheme) {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "":
		u.Scheme = "http"
	}
	u.Path = "/lighthouse"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func resolveURLWithPort(env map[string]string, scheme, host, portKey, fallbackPort string) string {
	port := strings.TrimSpace(envValue(env, portKey))
	if port == "" {
		port = fallbackPort
	}
	return fmt.Sprintf("%s://%s:%s", scheme, host, port)
}

func envValue(env map[string]string, key string) string {
	if env != nil {
		return env[key]
	}
	return os.Getenv(key)
}

// devAppNames returns app names in the same order used by active dev.
func devAppNames(apps []project.App) []string {
	names := make([]string, 0, len(apps))
	seen := map[string]bool{}
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		if app.Name == "" || seen[app.Name] {
			continue
		}
		seen[app.Name] = true
		names = append(names, app.Name)
	}
	return names
}

// devRuntimeWatcherApps returns runtime app names in watcher order.
func devRuntimeWatcherApps(watches []project.DevWatch) []string {
	apps := make([]string, 0)
	seen := map[string]bool{}
	for _, watch := range watches {
		app := devRuntimeWatcherApp(watch.Name)
		if app == "" || seen[app] {
			continue
		}
		seen[app] = true
		apps = append(apps, app)
	}
	return apps
}

// devRuntimeWatcherApp derives the app from generated runtime watcher names.
func devRuntimeWatcherApp(watcher string) string {
	watcher = str.Of(watcher).Trim().String()
	switch {
	case watcher == "Run App":
		return project.DefaultAppName
	case strings.HasPrefix(watcher, "Run "):
		return str.Of(watcher).TrimPrefix("Run ").Trim().String()
	default:
		return ""
	}
}

// clearDevBuildReadyStamps prevents runtime watchers from treating a previous build as current.
func clearDevBuildReadyStamps(jobs []devBuildJob) {
	for _, job := range jobs {
		app := projectlayout.NormalizeApp(job.app)
		_ = os.Remove(projectlayout.RuntimeReadyStamp(".", app))
	}
}

// devAppColumnWidth keeps app log columns aligned without letting long slugs dominate output.
func devAppColumnWidth(apps []string) int {
	const maxWidth = 18
	width := len(project.DefaultAppName)
	for _, app := range apps {
		app = str.Of(app).Trim().String()
		if len(app) > width {
			width = len(app)
		}
	}
	if width > maxWidth {
		return maxWidth
	}
	return width
}

// isDevBuildWatcher reports whether a watcher executes app build work.
func isDevBuildWatcher(name string) bool {
	return name == "Build App" || strings.HasPrefix(name, "Build ")
}

// buildWatcherExec wraps watcher commands with restart markers and binary readiness checks.
func buildWatcherExec(execCmd string) string {
	target, ok := devExecutableTarget(execCmd)
	if !ok {
		return fmt.Sprintf("echo __FORJ_WATCHER_TRIGGER__; exec %s", execCmd)
	}
	return fmt.Sprintf("echo __FORJ_WATCHER_TRIGGER__;%s exec \"$forj_dev_exec_target\"%s", devExecutableReadinessScript(target), devExecutableArgSuffix(execCmd, target))
}

// buildNativeRuntimeExec fails closed until the controller replaces a binary target with a prepared artifact.
func buildNativeRuntimeExec(execCmd string) string {
	_, ok := devExecutableTarget(execCmd)
	if !ok {
		return fmt.Sprintf("echo __FORJ_WATCHER_TRIGGER__; exec %s", execCmd)
	}
	return "echo __FORJ_WATCHER_TRIGGER__;" +
		" echo 'forj dev: refusing to start an unprepared native executable' >&2;" +
		" exit 1"
}

// buildPreparedNativeRuntimeExec launches the immutable artifact validated before runtime replacement began.
func buildPreparedNativeRuntimeExec(execCmd string, target string, preparedPath string) string {
	return "echo __FORJ_WATCHER_TRIGGER__; exec " + shellSingleQuote(preparedPath) + devExecutableArgSuffix(execCmd, target)
}

// buildFullProcessRuntimeExec preserves an advanced runtime mapping as shell input without binary rewriting.
func buildFullProcessRuntimeExec(execCmd string) string {
	return "echo __FORJ_WATCHER_TRIGGER__;" + execCmd
}

// devExecutableReadinessScript gives file events a short settle window before a replaced binary is executed.
func devExecutableReadinessScript(target string) string {
	quotedTarget := shellSingleQuote(target)
	ready := devExecutableReadyStampTarget(target)
	maxIterations := "100"
	readySetup := ""
	readyCheck := ""
	if ready != "" {
		maxIterations = "2400"
		readySetup = " forj_dev_ready=" + shellSingleQuote(ready) + ";"
		readyCheck = " [ -f \"$forj_dev_ready\" ] &&"
	}
	return " forj_dev_target=" + quotedTarget + ";" + readySetup + " forj_dev_last_size=; forj_dev_stable=0; forj_dev_ready_ok=0; forj_dev_i=0;" +
		devBinaryMagicFunctionScript() +
		" while [ \"$forj_dev_i\" -lt " + maxIterations + " ]; do" +
		" if" + readyCheck + " [ -s \"$forj_dev_target\" ] && [ -x \"$forj_dev_target\" ] && forj_dev_binary_magic_ok \"$forj_dev_target\"; then" +
		" forj_dev_size=$(wc -c < \"$forj_dev_target\" 2>/dev/null || echo 0);" +
		" if [ \"$forj_dev_size\" = \"$forj_dev_last_size\" ]; then forj_dev_stable=$((forj_dev_stable + 1)); else forj_dev_stable=0; forj_dev_last_size=$forj_dev_size; fi;" +
		" if [ \"$forj_dev_stable\" -ge 4 ]; then forj_dev_ready_ok=1; break; fi;" +
		" fi;" +
		" forj_dev_i=$((forj_dev_i + 1)); sleep 0.05;" +
		" done;" +
		" if [ \"$forj_dev_ready_ok\" != 1 ]; then echo \"forj dev: $forj_dev_target is not ready; waiting for a successful build\" >&2; exit 0; fi;" +
		" forj_dev_snapshot=$(mktemp \"$forj_dev_target.run.XXXXXX\" 2>/dev/null || mktemp \"/tmp/forj-dev-run.XXXXXX\") || exit 0;" +
		" if ! cp \"$forj_dev_target\" \"$forj_dev_snapshot\" || ! chmod 700 \"$forj_dev_snapshot\" || ! forj_dev_binary_magic_ok \"$forj_dev_snapshot\"; then rm -f \"$forj_dev_snapshot\"; echo \"forj dev: $forj_dev_target snapshot is not executable yet\" >&2; exit 0; fi;" +
		" forj_dev_exec_target=$forj_dev_snapshot; (sleep 300; rm -f \"$forj_dev_snapshot\") >/dev/null 2>&1 &"
}

// devBinaryMagicFunctionScript validates the executable format before the shell opens it.
func devBinaryMagicFunctionScript() string {
	return " forj_dev_binary_magic_ok() {" +
		" forj_dev_magic=$(dd if=\"$1\" bs=4 count=1 2>/dev/null | od -An -tx1 | tr -d ' \n');" +
		" case \"$(uname -s 2>/dev/null)\" in" +
		" Darwin*) case \"$forj_dev_magic\" in cffaedfe|feedfacf|cefaedfe|cafebabe|bebafeca) return 0;; *) return 1;; esac;;" +
		" Linux*) case \"$forj_dev_magic\" in 7f454c46) return 0;; *) return 1;; esac;;" +
		" *) return 0;;" +
		" esac;" +
		" };"
}

// devExecutableArgSuffix returns the original command arguments after the executable path.
func devExecutableArgSuffix(execCmd string, target string) string {
	suffix := str.Of(execCmd).Trim().TrimPrefix(target).String()
	if suffix == "" {
		return ""
	}
	if !strings.HasPrefix(suffix, " ") && !strings.HasPrefix(suffix, "\t") {
		return " " + strings.TrimSpace(suffix)
	}
	return suffix
}

// devExecutableTarget extracts the binary path from watcher commands that execute built app binaries.
func devExecutableTarget(execCmd string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(execCmd))
	if len(fields) == 0 {
		return "", false
	}
	target := fields[0]
	normalized := filepath.ToSlash(target)
	if strings.HasPrefix(normalized, "./bin/") || strings.HasPrefix(normalized, "bin/") || strings.Contains(normalized, "/bin/") {
		return target, true
	}
	return "", false
}

// devExecutableReadyStampTarget returns the build stamp that publishes a watched binary.
func devExecutableReadyStampTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	normalized := filepath.ToSlash(target)
	if !(strings.HasPrefix(normalized, "./bin/") || strings.HasPrefix(normalized, "bin/") || strings.Contains(normalized, "/bin/")) {
		return ""
	}
	dir := filepath.ToSlash(filepath.Dir(target))
	if strings.HasPrefix(normalized, "./") && !strings.HasPrefix(dir, ".") && !filepath.IsAbs(dir) {
		dir = "./" + dir
	}
	return strings.TrimRight(dir, "/") + "/." + filepath.Base(target) + ".ready"
}

// shellSingleQuote returns a POSIX single-quoted shell literal.
func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

// stop waits for the shared controller generation while leaving logical exit draining to its caller.
func (runtime *devWatcherRuntime) stop(timeout time.Duration, collapse bool) {
	wait := runtime.beginStop(timeout, collapse)
	wait()
}

// stopAndDrain completes controller cleanup before the outer loop replaces this runtime generation.
func (runtime *devWatcherRuntime) stopAndDrain(collapse bool) {
	runtime.stop(5*time.Second, collapse)
	runtime.drainAllExits(collapse)
}

// beginStop lets render preparation overlap graceful process termination without losing the final wait boundary.
func (runtime *devWatcherRuntime) beginStop(timeout time.Duration, collapse bool) func() {
	if collapse {
		names := make([]string, 0, len(runtime.watchers))
		for _, watcher := range runtime.watchers {
			names = append(names, watcher.name)
		}
		emitWatcherLifecycleSummary(runtime.session.outWriter, runtime.session.streamer, names, watcherStateStopping)
	} else {
		for _, watcher := range runtime.watchers {
			emitWatcherLifecycleLine(runtime.session.outWriter, runtime.session.streamer, watcher.name, watcherStateStopping)
		}
	}
	stopped := make(chan struct{})
	go func() {
		runtime.controller.stop(timeout)
		close(stopped)
	}()
	return func() {
		<-stopped
	}
}

// drainExits consumes a fixed logical generation so its terminal reporting cannot leak into replacement watchers.
func (runtime *devWatcherRuntime) drainExits(count int, collapse bool) {
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		exit := <-runtime.exitCh
		if collapse {
			names = append(names, exit.name)
			continue
		}
		emitWatcherLifecycleLine(runtime.session.outWriter, runtime.session.streamer, exit.name, watcherStateStopped)
	}
	if collapse {
		emitWatcherLifecycleSummary(runtime.session.outWriter, runtime.session.streamer, names, watcherStateStopped)
	}
}

// drainAllExits uses the owned handle count so callers cannot split exit accounting from its watcher generation.
func (runtime *devWatcherRuntime) drainAllExits(collapse bool) {
	runtime.drainExits(len(runtime.watchers), collapse)
}

// stopAfterExit prevents an unexpected single exit from leaving sibling processes or the shared controller running.
func (runtime *devWatcherRuntime) stopAfterExit(exit watcherExit, timeout time.Duration) {
	emitWatcherLifecycleLine(runtime.session.outWriter, runtime.session.streamer, exit.name, watcherStateStopped)
	watcherCount := len(runtime.watchers)
	runtime.watchers = removeWatcherByID(runtime.watchers, exit.id)
	runtime.stop(timeout, false)
	runtime.drainExits(watcherCount-1, false)
}

// drainRestartSignals empties pending restart notifications.
func drainRestartSignals(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func drainRenderSignals(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func drainBuildSignals(ch chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// removeWatcherByID removes one logical handle without conflating duplicate display names.
func removeWatcherByID(watchers []runningWatcher, id string) []runningWatcher {
	if len(watchers) == 0 {
		return nil
	}
	filtered := make([]runningWatcher, 0, len(watchers))
	for _, watcher := range watchers {
		if watcher.id == id {
			continue
		}
		filtered = append(filtered, watcher)
	}
	return filtered
}

// formatWatcherNameList renders a compact watcher summary.
func formatWatcherNameList(watches []project.DevWatch) string {
	var b strings.Builder
	for i, watch := range watches {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strings.TrimSpace(watch.Name))
	}
	return b.String()
}

// mapToEnv converts a map into KEY=VALUE environment entries.
func mapToEnv(vars map[string]string) []string {
	if len(vars) == 0 {
		return nil
	}
	out := make([]string, 0, len(vars))
	for key, value := range vars {
		out = append(out, key+"="+value)
	}
	return out
}

// splitWatcherEnvAssignments lifts only literal leading assignments so shell
// quoting and expansion remain untouched in the command that Bash receives.
func splitWatcherEnvAssignments(execCmd string) (map[string]string, string) {
	if strings.TrimSpace(execCmd) == "" {
		return nil, execCmd
	}
	env := map[string]string{}
	consumed := 0
	for consumed < len(execCmd) {
		remaining := execCmd[consumed:]
		trimmed := strings.TrimLeftFunc(remaining, unicode.IsSpace)
		consumed += len(remaining) - len(trimmed)
		if trimmed == "" {
			break
		}
		wordLength := strings.IndexFunc(trimmed, unicode.IsSpace)
		if wordLength < 0 {
			wordLength = len(trimmed)
		}
		word := trimmed[:wordLength]
		// Shell-expanded assignments stay in the original command so quoting,
		// substitution, and escaping retain their exact Bash semantics.
		if strings.ContainsAny(word, "'\"\\$`;&|<>(){}!~") {
			break
		}
		key, value, ok := strings.Cut(word, "=")
		if !ok || key == "" || !isShellEnvName(key) {
			break
		}
		env[key] = value
		consumed += wordLength
	}
	if len(env) == 0 {
		return nil, execCmd
	}
	rest := strings.TrimLeftFunc(execCmd[consumed:], unicode.IsSpace)
	if strings.TrimSpace(rest) == "" {
		return nil, execCmd
	}
	return env, rest
}

func shellSplitArgs(value string) ([]string, error) {
	var (
		args        []string
		current     strings.Builder
		inSingle    bool
		inDouble    bool
		escaped     bool
		sawFragment bool
	)
	flush := func() {
		if !sawFragment {
			return
		}
		args = append(args, current.String())
		current.Reset()
		sawFragment = false
	}
	for _, r := range value {
		switch {
		case escaped:
			current.WriteRune(r)
			sawFragment = true
			escaped = false
		case r == '\\' && !inSingle && !inDouble:
			escaped = true
		case r == '\\' && inDouble:
			current.WriteRune(r)
			sawFragment = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
			sawFragment = true
		case r == '"' && !inSingle:
			inDouble = !inDouble
			sawFragment = true
		case (r == ' ' || r == '\t' || r == '\n') && !inSingle && !inDouble:
			flush()
		default:
			current.WriteRune(r)
			sawFragment = true
		}
	}
	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated shell argument")
	}
	flush()
	return args, nil
}

func isShellEnvName(name string) bool {
	for i, r := range name {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// runDevDownTasks executes the dev down commands sequentially.
func runDevDownTasks(tasks []project.DevTask) error {
	if len(tasks) == 0 {
		return nil
	}
	console.Infof("Bringing down resources")
	for _, task := range tasks {
		cmd := execx.Command("bash", "-c", task.Cmd).
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr)
		res, err := configureDevTaskTTY(cmd, nil).Run()
		if err != nil {
			return fmt.Errorf("dev_down task '%s' failed: %v", task.Name, err)
		}
		if !res.OK() {
			return fmt.Errorf("dev_down task '%s' failed with exit code %d", task.Name, res.ExitCode)
		}
		console.Successf("%s", task.Name)
	}
	return nil
}

// configureDevTaskTTY preserves native command output for interactive helpers like Docker Compose.
func configureDevTaskTTY(cmd *execx.Cmd, outputTail io.Writer) *execx.Cmd {
	return configureDevTaskTTYWithWriter(cmd, os.Stdout, outputTail)
}

// configureDevTaskTTYWithWriter preserves native command output while allowing focused tests and callers to select its destination.
func configureDevTaskTTYWithWriter(cmd *execx.Cmd, stdout io.Writer, outputTail io.Writer) *execx.Cmd {
	switch runtime.GOOS {
	case "linux", "darwin":
		cmd = cmd.WithPTY()
		if outputTail != nil {
			cmd = cmd.StdoutWriter(io.MultiWriter(stdout, outputTail))
		} else {
			cmd = cmd.StdoutWriter(stdout)
		}
		return cmd.StderrWriter(nil)
	default:
		return cmd
	}
}

// errorSoundHook emits a sound when matching error output appears.
func errorSoundHook(enabled bool) func(string) {
	if !enabled {
		return nil
	}
	errorLimiter := newSoundLimiter(2 * time.Second)
	var hookMu sync.Mutex
	hadError := false
	var recoveryTimer *time.Timer
	var recoveryGeneration uint64
	return func(line string) {
		hookMu.Lock()
		defer hookMu.Unlock()
		if isWatcherTriggerLine(line) {
			if hadError {
				if recoveryTimer != nil {
					recoveryTimer.Stop()
				}
				recoveryGeneration++
				generation := recoveryGeneration
				recoveryTimer = time.AfterFunc(750*time.Millisecond, func() {
					hookMu.Lock()
					if generation != recoveryGeneration || !hadError {
						hookMu.Unlock()
						return
					}
					hadError = false
					recoveryTimer = nil
					hookMu.Unlock()
					playRecoverySound()
				})
			}
			return
		}
		if !containsErrorWord(line) {
			return
		}
		if recoveryTimer != nil {
			recoveryTimer.Stop()
			recoveryTimer = nil
		}
		recoveryGeneration++
		hadError = true
		if errorLimiter.Allow() {
			playErrorSound()
		}
	}
}

// containsErrorWord reports whether a line looks like an error signal.
func containsErrorWord(line string) bool {
	matches := []string{
		"syntax error:",
		"undefined:",
		"cannot use",
		"invalid operation:",
		"assignment mismatch:",
		"too many arguments in call",
		"not in selector",
		"redeclared",
		"does not implement",
		"cannot find package",
		"no required module provides",
		"go: error",
	}
	m := str.Of(line)
	for _, match := range matches {
		if m.ContainsFold(match) {
			return true
		}
	}
	if m.ContainsFold("build app") && m.ContainsFold("error executing command") {
		return true
	}
	// Wire noise guard: only beep on actual wire failures, not successful "wire:" logs.
	if m.ContainsFold("wire:") &&
		(m.ContainsFold("error") ||
			m.ContainsFold("failed") ||
			m.ContainsFold("generate failed") ||
			(m.ContainsFold("inject") && m.ContainsFold("failed"))) {
		return true
	}
	return false
}

// playErrorSound plays a macOS alert sound when available.
func playErrorSound() {
	if runtime.GOOS != "darwin" {
		return
	}
	_ = execx.Command("afplay", "/System/Library/Sounds/Submarine.aiff").Start()
}

// playRecoverySound plays a macOS recovery sound when available.
func playRecoverySound() {
	if runtime.GOOS != "darwin" {
		return
	}
	_ = execx.Command("afplay", "/System/Library/Sounds/Glass.aiff").Start()
}

func emitWatcherLifecycleLine(out io.Writer, streamer *devwatchStreamer, watcher string, state watcherLifecycleState) {
	line := formatWatcherLifecycleLine(watcher, state)
	if line == "" {
		return
	}
	timestamp := time.Now()
	if streamer != nil {
		streamer.Send(devwatchLine{
			Line:      line,
			Stream:    "stdout",
			Timestamp: timestamp,
			ID:        timestamp.UnixMilli(),
			Watcher:   watcher,
		})
	}
	if out == nil {
		return
	}
	devwatchOutputMu.Lock()
	defer devwatchOutputMu.Unlock()
	_, _ = io.WriteString(out, line)
	_, _ = io.WriteString(out, "\n")
}

func emitWatcherLifecycleSummary(out io.Writer, streamer *devwatchStreamer, watchers []string, state watcherLifecycleState) {
	line := formatWatcherLifecycleSummary(watchers, state)
	if line == "" {
		return
	}
	timestamp := time.Now()
	if streamer != nil {
		streamer.Send(devwatchLine{
			Line:      line,
			Stream:    "stdout",
			Timestamp: timestamp,
			ID:        timestamp.UnixMilli(),
		})
	}
	if out == nil {
		return
	}
	devwatchOutputMu.Lock()
	defer devwatchOutputMu.Unlock()
	_, _ = io.WriteString(out, line)
	_, _ = io.WriteString(out, "\n")
}

func formatWatcherLifecycleLine(watcher string, state watcherLifecycleState) string {
	watcher = str.Of(watcher).Trim().String()
	if watcher == "" {
		return ""
	}

	mark := console.InfoMark()
	stateLabel := console.Colorize(console.ColorGray, string(state))
	switch state {
	case watcherStateStarted:
		mark = console.SuccessMark()
		stateLabel = console.Colorize(console.ColorGreen, string(state))
	case watcherStateStopping:
		mark = console.InfoMark()
		stateLabel = console.Colorize(console.ColorGray, string(state))
	case watcherStateStopped:
		mark = console.SuccessMark()
		stateLabel = console.Colorize(console.ColorGreen, string(state))
	}

	return fmt.Sprintf(
		"%s %s %s",
		mark,
		console.Colorize(console.ColorBoldWhite, watcher),
		stateLabel,
	)
}

func formatWatcherLifecycleSummary(watchers []string, state watcherLifecycleState) string {
	names := make([]string, 0, len(watchers))
	for _, watcher := range watchers {
		watcher = str.Of(watcher).Trim().String()
		if watcher == "" {
			continue
		}
		names = append(names, watcher)
	}
	if len(names) == 0 {
		return ""
	}

	mark := console.InfoMark()
	stateLabel := console.Colorize(console.ColorGray, string(state))
	switch state {
	case watcherStateStarted:
		mark = console.SuccessMark()
		stateLabel = console.Colorize(console.ColorGreen, string(state))
	case watcherStateStopped:
		mark = console.SuccessMark()
		stateLabel = console.Colorize(console.ColorGreen, string(state))
	}

	return fmt.Sprintf(
		"%s %s %s - %s",
		mark,
		console.Colorize(console.ColorBoldWhite, "Watchers"),
		stateLabel,
		console.Colorize(console.ColorGray, strings.Join(names, ", ")),
	)
}

type soundLimiter struct {
	cooldown time.Duration
	last     time.Time
}

// newSoundLimiter throttles repeated sound triggers.
func newSoundLimiter(cooldown time.Duration) *soundLimiter {
	return &soundLimiter{cooldown: cooldown}
}

// Allow reports whether enough time has elapsed since the last trigger.
func (l *soundLimiter) Allow() bool {
	now := time.Now()
	if now.Sub(l.last) < l.cooldown {
		return false
	}
	l.last = now
	return true
}

// ensureDevTools installs required dev binaries if they're missing.
func ensureDevTools() error {
	if err := ensureTool("wire", wireInstallTarget); err != nil {
		return err
	}
	return nil
}

// ensureBinDir creates ./bin for local builds if it's missing.
func ensureBinDir() error {
	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("ensure bin directory: %w", err)
	}
	return nil
}

// ensureTool installs a CLI if it is missing from PATH.
func ensureTool(name, module string) error {
	if _, err := exec.LookPath(name); err == nil {
		return nil
	}

	res, err := execx.Command("go", "install", module).
		EnvInherit().
		StdinReader(os.Stdin).
		StdoutWriter(os.Stdout).
		StderrWriter(os.Stderr).
		Run()
	if err != nil {
		return fmt.Errorf("install %s: %w", name, err)
	}
	if !res.OK() {
		return fmt.Errorf("install %s failed with exit code %d", name, res.ExitCode)
	}
	return nil
}

// acquireLock prevents concurrent forj dev sessions from running in the same project.
func (c *DevCmd) acquireLock() (func(), error) {
	lockPath := ".forj-dev.lock"
	pid := os.Getpid()
	lockFile, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err == nil {
		_, _ = lockFile.WriteString(strconv.Itoa(pid))
		_ = lockFile.Close()
		return func() { _ = os.Remove(lockPath) }, nil
	}

	// If the lock already exists, verify whether the PID is still alive.
	data, err := os.ReadFile(lockPath)
	if err == nil {
		if existing, parseErr := strconv.Atoi(strings.TrimSpace(string(data))); parseErr == nil {
			if isProcessRunning(existing) {
				return nil, fmt.Errorf("forj dev already running (pid %d). remove %s if you are sure it's stale", existing, lockPath)
			}
		}
	}

	// Stale lock file: remove it and retry.
	_ = os.Remove(lockPath)
	lockFile, err = os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to create dev lock: %w", err)
	}
	_, _ = lockFile.WriteString(strconv.Itoa(pid))
	_ = lockFile.Close()
	return func() { _ = os.Remove(lockPath) }, nil
}
