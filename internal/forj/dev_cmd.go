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
	goruntime "runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	envx "github.com/goforj/env/v2"
	"github.com/goforj/execx"
	"github.com/goforj/goforj/internal/console"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str"
)

var errDevInterrupted = errors.New("dev interrupted")

type DevCmd struct {
	logger *logger.AppLogger
}

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
	config        *project.Config
	baseWatches   []project.DevWatch
	streamer      *devwatchStreamer
	restartCh     chan struct{}
	buildCh       chan struct{}
	renderCh      chan struct{}
	commandCh     chan devShellCommandRequest
	stopCh        <-chan struct{}
	outWriter     io.Writer
	errWriter     io.Writer
	reloadRuntime func() (*devwatchStreamer, error)
}

func (*DevCmd) Signature() string {
	return `name:"dev" help:"Run development watchers"`
}

func NewDevCmd(logger *logger.AppLogger) *DevCmd {
	return &DevCmd{logger: logger}
}

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
	var err error
	if r.firstLoad {
		err = envx.Load()
		r.firstLoad = false
	} else {
		err = envx.Reload()
	}
	if err != nil {
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

// Run executes the dev workflow (pre tasks, watchers, and shutdown handling).
func (c *DevCmd) Run() error {
	// Prevent concurrent dev sessions from clobbering each other.
	unlock, err := c.acquireLock()
	if err != nil {
		return err
	}
	defer unlock()

	shutdownWriters := func() {}
	var cleanupOnce sync.Once
	cleanupDevTerminal := func() {
		cleanupOnce.Do(func() {
			shutdownWriters()
			shutdownWriters = func() {}
			restoreDevTerminalState(nil, nil)
		})
	}
	defer cleanupDevTerminal()

	config, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}
	baseWatches := copyDevWatches(config.Dev.Watches)
	config.Dev.Watches = devWatchesForApps(config, baseWatches)

	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	if len(config.Dev.Watches) == 0 {
		console.Warnf("No dev watches defined in .goforj.yml")
		return nil
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
	var outWriter io.Writer
	var errWriter io.Writer
	runtimeState := newDevRuntimeState(restartCh, buildCh, renderCh)
	defer runtimeState.Close()

	currentStreamer, err := runtimeState.Sync()
	if err != nil {
		return err
	}
	writeDevAppBuildLine(os.Stdout, activeDevApps())
	if err := runDevInitialBuild(config, os.Stdout, os.Stderr); err != nil {
		return err
	}
	if err := runPreDevSetup(config); err != nil {
		return err
	}
	if outWriter == nil || errWriter == nil {
		outWriter, errWriter, shutdownWriters, runtimeState.refreshWriters = buildDevOutputWriters(config, requestRestart, requestRender, requestCommand)
		runtimeState.refreshWriters()
	}

	session := &devWatchSession{
		config:        config,
		baseWatches:   baseWatches,
		streamer:      currentStreamer,
		restartCh:     restartCh,
		buildCh:       buildCh,
		renderCh:      renderCh,
		commandCh:     commandCh,
		stopCh:        runCtx.Done(),
		outWriter:     outWriter,
		errWriter:     errWriter,
		reloadRuntime: runtimeState.Sync,
	}

	if err := c.runWatchersLoop(session); err != nil {
		if errors.Is(err, errDevInterrupted) {
			cleanupDevTerminal()
			outWriter = nil
			errWriter = nil
			if config != nil && config.Dev.DownOnExit {
				console.Actionf("forj down > auto (set dev.down_on_exit: false to disable)")
				if err := runDevDownTasks(config.Dev.Down); err != nil {
					console.Errorf("forj down failed: %v", err)
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

func ensureDevDatabaseExists(config *project.Config) error {
	return ensureDevDatabaseExistsWithWriters(config, os.Stdout, os.Stderr)
}

func ensureDevDatabaseExistsWithWriters(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	if config == nil {
		return nil
	}
	databases, err := devDatabasesForApps(config, activeDevApps())
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
	if config == nil {
		return nil, nil
	}
	seen := map[string]bool{}
	databases := make([]devDatabase, 0, len(apps))
	for _, app := range apps {
		app = normalizeRenderApp(app)
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
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "mysql", "mariadb":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return strings.ToLower(strings.TrimSpace(driver))
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
	prefix := strEnvPrefix(app.Name)
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
		res, err := execx.Command("bash", "-c", task.Cmd).
			EnvInherit().
			StdinReader(devNull).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
		if err != nil {
			return fmt.Errorf("pre-dev task '%s' failed: %v", task.Name, err)
		}
		if !res.OK() {
			return fmt.Errorf("pre-dev task '%s' failed with exit code %d", task.Name, res.ExitCode)
		}
	}
	return nil
}

func runPreDevSetup(config *project.Config) error {
	if config == nil {
		return nil
	}
	preTasks := config.Dev.Pre
	postMigrateTasks := make([]project.DevTask, 0, len(config.Dev.Pre))
	if shouldRunDevAutoMigrate(config) {
		preTasks = make([]project.DevTask, 0, len(config.Dev.Pre))
		for _, task := range config.Dev.Pre {
			if shouldRunAfterMigrate(task) {
				postMigrateTasks = append(postMigrateTasks, task)
				continue
			}
			preTasks = append(preTasks, task)
		}
	}
	if err := runDevTasks("Running pre-dev setup", preTasks); err != nil {
		return err
	}
	if err := runDevAppSetup(config, os.Stdout, os.Stderr); err != nil {
		return err
	}
	if err := runDevTasks("Running post-migrate setup", postMigrateTasks); err != nil {
		return err
	}
	return nil
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
	res, err := execx.Command("bash", "-c", devAutoMigrateShellCommand()).
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
	if config == nil || !config.Dev.AutoMigrate {
		return false
	}
	for _, target := range activeDevApps() {
		if appRenderComponents(config, target).HasDatabase() {
			return true
		}
	}
	return false
}

// devAutoMigrateShellCommand runs migrations through the active binary so dev stays aligned with generated app commands.
func devAutoMigrateShellCommand() string {
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
	return "./bin/" + activeDevApp().Name
}

// devWatchesForApps expands the single-app default watchers across every discovered app.
func devWatchesForApps(config *project.Config, watches []project.DevWatch) []project.DevWatch {
	apps := activeDevApps()
	if len(apps) == 1 && apps[0].Name == project.DefaultAppName {
		return watches
	}
	rewritten := make([]project.DevWatch, 0, len(watches)*len(apps))
	for _, watch := range watches {
		switch watch.Name {
		case "Build App", "Run App":
			for _, app := range apps {
				rewritten = append(rewritten, devWatchForApp(watch, app))
			}
		default:
			rewritten = append(rewritten, watch)
		}
	}
	return rewritten
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
	apps := configuredDevApps()
	if len(apps) == 0 {
		return []project.App{project.DefaultApp()}
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

// configuredDevApps discovers all-app dev apps from conventional project layout only.
func configuredDevApps() []project.App {
	seen := map[string]project.App{}
	add := func(app project.App) {
		app = normalizeRenderApp(app)
		if app.Name == "" || !project.IsSafeAppName(app.Name) || project.IsReservedAppName(app.Name) {
			return
		}
		seen[app.Name] = app
	}
	add(project.DefaultApp())
	for _, app := range discoverConventionalApps() {
		add(app)
	}

	apps := make([]project.App, 0, len(seen))
	if app, ok := seen[project.DefaultAppName]; ok {
		apps = append(apps, app)
		delete(seen, project.DefaultAppName)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		apps = append(apps, seen[name])
	}
	return apps
}

// devWatchForApp rewrites one default watcher for a specific app without editing config.
func devWatchForApp(watch project.DevWatch, app project.App) project.DevWatch {
	app = normalizeRenderApp(app)
	baseName := watch.Name
	if app.Name != project.DefaultAppName {
		watch.Name = strings.ReplaceAll(watch.Name, "App", app.Name)
	}
	appBinary := "./bin/" + app.Name
	appWireGen := filepath.ToSlash(filepath.Join(app.WireDir, "wire_gen\\.go$"))
	if isDevBuildWatcher(baseName) {
		watch.Exec = devBuildCommandForApp(watch.Exec, app)
	} else {
		watch.Exec = strings.ReplaceAll(watch.Exec, "./bin/app", appBinary)
	}
	watch.Watch = strings.ReplaceAll(watch.Watch, "./bin/app", appBinary)
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
}

// devBuildCommands returns the build commands needed for the active dev app set.
func devBuildCommands(config *project.Config) []string {
	jobs := devBuildJobs(config, false)
	commands := make([]string, 0, len(jobs))
	for _, job := range jobs {
		commands = append(commands, job.command)
	}
	return commands
}

// devInitialBuildCommands returns bootstrap builds for every active app so dev never starts from a stale binary.
func devInitialBuildCommands(config *project.Config) []string {
	jobs := devBuildJobs(config, false)
	commands := make([]string, 0, len(jobs))
	for _, job := range jobs {
		commands = append(commands, job.command)
	}
	return commands
}

// devBuildJobs resolves app build commands while preserving the app label for compact output.
func devBuildJobs(config *project.Config, missingOnly bool) []devBuildJob {
	apps := activeDevApps()
	jobs := make([]devBuildJob, 0, len(apps))
	baseCommand := devBaseBuildCommand(config)
	for _, app := range apps {
		app = normalizeRenderApp(app)
		if missingOnly {
			if _, err := os.Stat(filepath.Join("bin", app.Name)); err == nil {
				continue
			}
		}
		command := devBuildCommandForApp(baseCommand, app)
		if strings.TrimSpace(command) == "" {
			continue
		}
		jobs = append(jobs, devBuildJob{app: app, command: command})
	}
	return jobs
}

// devBaseBuildCommand uses the configured default build watcher as the template for app builds.
func devBaseBuildCommand(config *project.Config) string {
	if config != nil {
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
	}
	return "forj build -o ./bin/app"
}

// devBuildCommandForApp derives app builds from the configured default build command.
func devBuildCommandForApp(baseCommand string, app project.App) string {
	app = normalizeRenderApp(app)
	command := strings.TrimSpace(baseCommand)
	if command == "" {
		command = "forj build -o ./bin/app"
	}

	defaultApp := project.DefaultApp()
	defaultBinary := "./bin/" + defaultApp.Name
	appBinary := "./bin/" + app.Name
	defaultPackage := "./" + filepath.ToSlash(filepath.Dir(defaultApp.Entrypoint))
	appPackage := "./" + filepath.ToSlash(filepath.Dir(app.Entrypoint))

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
	command = strings.ReplaceAll(command, " "+targetPackage, "")
	command = strings.ReplaceAll(command, " "+strings.TrimPrefix(targetPackage, "./"), "")
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

// runningWatcher tracks a watcher process and its configured name.
type runningWatcher struct {
	name string
	proc *execx.Process
}

// watcherExit reports the result of a watcher process after it finishes.
type watcherExit struct {
	name   string
	result execx.Result
	err    error
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
		if err := reloadDevWatchSessionConfig(session); err != nil {
			return err
		}
		session.config.Dev.Watches = devWatchesForApps(session.config, session.baseWatches)
		watchers, exitCh := startWatchers(
			session.config.ProjectName,
			session.config.Dev.Watches,
			session.streamer,
			session.outWriter,
			session.errWriter,
			session.config.Dev.SoundOnWatchError,
		)
		printDevReadySummary(session.outWriter, session.config, snapshotProcessEnv())
		for {
			select {
			case <-session.stopCh:
				disableDevFooter(session.outWriter)
				disableDevFooter(session.errWriter)
				fmt.Println(buildDevFooterSeparatorLine())
				stopWatchers(watchers, 5*time.Second, session.outWriter, session.streamer, true)
				drainWatcherExits(exitCh, len(watchers), session.outWriter, session.streamer, true)
				return errDevInterrupted
			case <-session.restartCh:
				writeDevActionLine(session.outWriter, "Restarting dev watchers")
				stopWatchers(watchers, 5*time.Second, session.outWriter, session.streamer, true)
				drainWatcherExits(exitCh, len(watchers), session.outWriter, session.streamer, true)
				drainRestartSignals(session.restartCh)
				refreshedStreamer, err := session.reloadRuntime()
				if err != nil {
					return err
				}
				session.streamer = refreshedStreamer
				continue watcherLoop
			case <-session.buildCh:
				writeDevActionLine(session.outWriter, "Rebuilding app and restarting watchers")
				stopWatchers(watchers, 5*time.Second, session.outWriter, session.streamer, true)
				drainWatcherExits(exitCh, len(watchers), session.outWriter, session.streamer, true)
				if err := reloadDevWatchSessionConfig(session); err != nil {
					return err
				}
				refreshedStreamer, err := session.reloadRuntime()
				if err != nil {
					return err
				}
				session.streamer = refreshedStreamer
				if err := runDevBuild(session.config, session.outWriter, session.errWriter); err != nil {
					disableDevFooter(session.outWriter)
					disableDevFooter(session.errWriter)
					fmt.Println(buildDevFooterSeparatorLine())
					console.Errorf("forj build failed: %v", err)
					return fmt.Errorf("forj build failed: %w", err)
				}
				if err := runDevAppSetup(session.config, session.outWriter, session.errWriter); err != nil {
					disableDevFooter(session.outWriter)
					disableDevFooter(session.errWriter)
					fmt.Println(buildDevFooterSeparatorLine())
					console.Errorf("dev app setup failed: %v", err)
					return fmt.Errorf("dev app setup failed: %w", err)
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
				waitForStop := beginStopWatchers(watchers, 5*time.Second, session.outWriter, session.streamer, true)
				if err := reloadDevWatchSessionConfig(session); err != nil {
					waitForStop()
					drainWatcherExits(exitCh, len(watchers), session.outWriter, session.streamer, true)
					return err
				}
				refreshedStreamer, err := session.reloadRuntime()
				if err != nil {
					waitForStop()
					drainWatcherExits(exitCh, len(watchers), session.outWriter, session.streamer, true)
					return err
				}
				session.streamer = refreshedStreamer
				renderErr := runDevRender(session.config, session.outWriter, session.errWriter)
				waitForStop()
				drainWatcherExits(exitCh, len(watchers), session.outWriter, session.streamer, true)
				if renderErr != nil {
					disableDevFooter(session.outWriter)
					disableDevFooter(session.errWriter)
					fmt.Println(buildDevFooterSeparatorLine())
					console.Errorf("forj render failed: %v", renderErr)
					return fmt.Errorf("forj render failed: %w", renderErr)
				}
				if err := reloadDevWatchSessionConfig(session); err != nil {
					return err
				}
				if err := runDevAppSetup(session.config, session.outWriter, session.errWriter); err != nil {
					disableDevFooter(session.outWriter)
					disableDevFooter(session.errWriter)
					fmt.Println(buildDevFooterSeparatorLine())
					console.Errorf("dev app setup failed: %v", err)
					return fmt.Errorf("dev app setup failed: %w", err)
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
			case exit := <-exitCh:
				emitWatcherLifecycleLine(session.outWriter, session.streamer, exit.name, watcherStateStopped)
				stopWatchers(removeWatcherByName(watchers, exit.name), 5*time.Second, session.outWriter, session.streamer, false)
				drainWatcherExits(exitCh, len(watchers)-1, session.outWriter, session.streamer, false)
				if exit.err != nil {
					return exit.err
				}
				if !exit.result.OK() {
					return fmt.Errorf("dev watchers exited with code %d", exit.result.ExitCode)
				}
				return nil
			}
		}
	}
}

// reloadDevWatchSessionConfig keeps long-running dev sessions aligned with make:app and render metadata changes.
func reloadDevWatchSessionConfig(session *devWatchSession) error {
	if session == nil {
		return nil
	}
	config, err := project.LoadProjectConfig()
	if err != nil {
		return err
	}
	session.config = config
	session.baseWatches = copyDevWatches(config.Dev.Watches)
	return nil
}

func snapshotProcessEnv() map[string]string {
	envMap := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		envMap[key] = value
	}
	return envMap
}

func runDevRender(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	if err := runDevTerminalCommand(outWriter, errWriter, "Running forj render", "forj render --timings"); err != nil {
		return fmt.Errorf("forj render failed: %w", err)
	}
	if err := runDevBuild(config, outWriter, errWriter); err != nil {
		return err
	}
	return nil
}

func runDevBuild(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	return runDevBuildJobs(config, outWriter, errWriter, false, "forj build failed")
}

// runDevInitialBuild builds every active app before pre-dev tasks can call generated app commands.
func runDevInitialBuild(config *project.Config, outWriter io.Writer, errWriter io.Writer) error {
	return runDevBuildJobs(config, outWriter, errWriter, false, "initial forj build failed")
}

type devBuildResult struct {
	job     devBuildJob
	stdout  string
	stderr  string
	elapsed time.Duration
	err     error
}

// runDevBuildJobs runs app builds together so multi-app dev startup scales with the slowest build.
func runDevBuildJobs(config *project.Config, outWriter io.Writer, errWriter io.Writer, missingOnly bool, failurePrefix string) error {
	jobs := devBuildJobs(config, missingOnly)
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
	setDevStatusLine(outWriter, heading)
	defer clearDevStatusLine(outWriter)

	results := make([]devBuildResult, len(jobs))
	var wg sync.WaitGroup
	for index, job := range jobs {
		writeDevActionLine(outWriter, "Building "+job.app.Name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[index] = runDevBuildJobBuffered(job)
		}()
	}
	wg.Wait()

	failures := make([]string, 0)
	for _, result := range results {
		if result.err == nil {
			writeDevTimingLine(outWriter, "Built "+result.job.app.Name+" in "+formatDevElapsed(result.elapsed))
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
	err := runDevSubprocessCommand(&stdout, &stderr, job.command, true)
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
	writeDevActionLine(outWriter, heading)
	setDevStatusLine(outWriter, heading)
	defer clearDevStatusLine(outWriter)

	if _, ok := outWriter.(*devBubbleWriter); ok {
		return runDevSubprocessCommand(outWriter, errWriter, job.command, true)
	}
	if _, ok := errWriter.(*devBubbleWriter); ok {
		return runDevSubprocessCommand(outWriter, errWriter, job.command, true)
	}

	disableDevFooter(outWriter)
	disableDevFooter(errWriter)
	defer enableDevFooter(outWriter)
	defer enableDevFooter(errWriter)
	return runDevSubprocessCommand(os.Stdout, os.Stderr, job.command, false)
}

// runDevSubprocessCommand centralizes dev subprocess environment and exit handling.
func runDevSubprocessCommand(outWriter io.Writer, errWriter io.Writer, command string, transcript bool) error {
	stdin := io.Reader(os.Stdin)
	env := map[string]string{"CLICOLOR_FORCE": "1"}
	if transcript {
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
	cmd := execx.Command("bash", "-c", command).
		EnvInherit().
		EnvAppend(env).
		StdinReader(stdin).
		StdoutWriter(outWriter).
		StderrWriter(errWriter)
	res, err := cmd.Run()
	if err != nil {
		return err
	}
	if !res.OK() {
		return fmt.Errorf("%s exited with code %d", command, res.ExitCode)
	}
	return nil
}

func runDevTranscriptCommand(outWriter io.Writer, errWriter io.Writer, heading string, command string) error {
	writeDevCommandLine(outWriter, heading)
	setDevStatusLine(outWriter, heading)
	defer clearDevStatusLine(outWriter)
	if err := runDevSubprocessCommand(outWriter, errWriter, command, true); err != nil {
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

	return runDevSubprocessCommand(os.Stdout, os.Stderr, command, false)
}

func writeDevActionLine(out io.Writer, message string) {
	if out == nil {
		console.Actionf("%s", message)
		return
	}
	_, _ = io.WriteString(out, fmt.Sprintf("%s %s\n", console.ActionMark(), message))
}

func writeDevTimingLine(out io.Writer, message string) {
	line := console.Colorize(console.ColorGray, "  "+strings.TrimSpace(message))
	if out == nil {
		_, _ = io.WriteString(os.Stdout, line+"\n")
		return
	}
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
		app = normalizeRenderApp(app)
		if app.Name == "" {
			continue
		}
		names = append(names, app.Name)
	}
	return names
}

func writeDevCommandLine(out io.Writer, message string) {
	if out == nil {
		console.Actionf("%s", message)
		return
	}
	label := console.Colorize(console.ColorBoldWhite, strings.TrimSpace(message))
	_, _ = io.WriteString(out, buildDevSectionSeparatorLine(label)+"\n")
}

func writeDevCommandBoundary(out io.Writer) {
	if out == nil {
		_, _ = io.WriteString(os.Stdout, buildDevFooterSeparatorLine()+"\n")
		return
	}
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

	if config == nil {
		return tools
	}

	components := config.Render.Components
	if components.Mail && components.Docker {
		tools = append(tools, devToolLink{
			Label:  "Mailpit",
			Detail: "(inbox)",
			URL:    resolveURLWithPort(env, "http", "localhost", "MAILPIT_HTTP_PORT", "8025"),
		})
	}
	if components.Observability {
		tools = append(tools, devToolLink{
			Label: "VictoriaMetrics",
			URL:   resolveURLWithPort(env, "http", "localhost", "OBSERVABILITY_VM_PORT", "8428"),
		})
	}
	if components.Grafana {
		adminUser := strings.TrimSpace(envValue(env, "GRAFANA_ADMIN_USER"))
		if adminUser == "" {
			adminUser = "admin"
		}
		tools = append(tools, devToolLink{
			Label:  "Grafana",
			Detail: fmt.Sprintf("(%s / admin)", adminUser),
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
	enabled := strings.ToLower(strings.TrimSpace(envValue(env, "API_SWAGGER_ENABLED")))
	if enabled == "" {
		enabled = strings.ToLower(strings.TrimSpace(envValue(env, "SWAGGER_ENABLED")))
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
	enabled := strings.ToLower(strings.TrimSpace(envValue(env, "LIGHTHOUSE_ENABLED")))
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

// startWatchers launches each watcher command with its own process and returns a channel for exits.
func startWatchers(
	projectName string,
	watches []project.DevWatch,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	soundOnError bool,
) ([]runningWatcher, <-chan watcherExit) {
	return startWatchersWithStarter(projectName, watches, streamer, outWriter, errWriter, soundOnError, func(cmd *execx.Cmd) *execx.Process {
		return cmd.Start()
	})
}

func startWatchersWithStarter(
	projectName string,
	watches []project.DevWatch,
	streamer *devwatchStreamer,
	outWriter io.Writer,
	errWriter io.Writer,
	soundOnError bool,
	start func(*execx.Cmd) *execx.Process,
) ([]runningWatcher, <-chan watcherExit) {
	exitCh := make(chan watcherExit, len(watches))
	watchers := make([]runningWatcher, len(watches))
	// Only non-postponed watchers emit an initial trigger during boot, so the
	// startup block should close after those watchers have reported "starting".
	// The single runtime supervisor watcher restarts after a fresh binary lands,
	// so we bracket that restart with explicit shutdown/start section separators.
	lifecycleState := newDevwatchLifecycleState(countImmediateStartupWatchers(watches), devRuntimeWatcherNames(watches))
	runtimeTargets := devAppNames(activeDevApps())
	showAppColumn := len(runtimeTargets) > 1
	appNameWidth := devAppColumnWidth(runtimeTargets)
	if len(watches) > 0 {
		_, _ = io.WriteString(outWriter, buildDevFooterSeparatorLine()+"\n")
	}
	var wg sync.WaitGroup
	for i, watch := range watches {
		wg.Add(1)
		go func(i int, watch project.DevWatch) {
			defer wg.Done()
			watchEnv, watchExecCmd := splitWatcherEnvAssignments(watch.Exec)
			watchExec := buildWatcherExec(watchExecCmd)
			triggerCmd := strings.Join(strings.Fields(watch.Exec), " ")
			wgoArgs := buildWatcherCommandArgs(watch.Watch, watchExec)
			cmdEnv := snapshotProcessEnv()
			for key, value := range watch.Env {
				cmdEnv[key] = value
			}
			for key, value := range watchEnv {
				cmdEnv[key] = value
			}
			if isDevBuildWatcher(watch.Name) {
				cmdEnv["FORJ_BUILD_PROGRESS"] = "1"
			}
			appName := devRuntimeWatcherApp(watch.Name)
			cmd := execx.Command("wgo").
				Arg(wgoArgs).
				EnvOnly(cmdEnv).
				StdoutWriter(newDevwatchWriterForApp(outWriter, streamer, "stdout", watch.Name, triggerCmd, appName, appNameWidth, showAppColumn, lifecycleState)).
				StderrWriter(newDevwatchWriterForApp(errWriter, streamer, "stderr", watch.Name, triggerCmd, appName, appNameWidth, showAppColumn, lifecycleState))
			cmd = configureWatcherPTY(cmd, soundOnError)
			proc := start(cmd)
			watchers[i] = runningWatcher{name: watch.Name, proc: proc}
			go func(name string, proc *execx.Process) {
				res, err := proc.Wait()
				exitCh <- watcherExit{name: name, result: res, err: err}
			}(watch.Name, proc)
		}(i, watch)
	}
	wg.Wait()
	startedNames := make([]string, 0, len(watches))
	for _, watch := range watches {
		startedNames = append(startedNames, watch.Name)
	}
	emitWatcherLifecycleSummary(outWriter, streamer, startedNames, watcherStateStarted)
	return watchers, exitCh
}

// devAppNames returns app names in the same order used by active dev.
func devAppNames(apps []project.App) []string {
	names := make([]string, 0, len(apps))
	seen := map[string]bool{}
	for _, app := range apps {
		app = normalizeRenderApp(app)
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
	watcher = str.Of(watcher).TrimSpace().String()
	switch {
	case watcher == "Run App":
		return project.DefaultAppName
	case strings.HasPrefix(watcher, "Run "):
		return strings.TrimSpace(strings.TrimPrefix(watcher, "Run "))
	default:
		return ""
	}
}

// devAppColumnWidth keeps app log columns aligned without letting long slugs dominate output.
func devAppColumnWidth(apps []string) int {
	const maxWidth = 18
	width := len(project.DefaultAppName)
	for _, app := range apps {
		app = str.Of(app).TrimSpace().String()
		if len(app) > width {
			width = len(app)
		}
	}
	if width > maxWidth {
		return maxWidth
	}
	return width
}

// devRuntimeWatcherNames identifies app runtime watchers that should bracket restart output.
func devRuntimeWatcherNames(watches []project.DevWatch) []string {
	names := make([]string, 0)
	for _, watch := range watches {
		if watch.Name == "Run App" || strings.HasPrefix(watch.Name, "Run ") {
			names = append(names, watch.Name)
		}
	}
	return names
}

// isDevBuildWatcher reports whether a watcher executes app build work.
func isDevBuildWatcher(name string) bool {
	return name == "Build App" || strings.HasPrefix(name, "Build ")
}

func buildWatcherExec(execCmd string) string {
	return fmt.Sprintf("echo __FORJ_WATCHER_TRIGGER__; exec %s", execCmd)
}

func buildWatcherCommandArgs(watchExpr string, execCmd string) []string {
	args, err := shellSplitArgs(watchExpr)
	if err != nil {
		args = strings.Fields(watchExpr)
	}
	args = append(args, "sh", "-c", execCmd)
	return args
}

// stopWatchers gracefully terminates every running watcher process.
// Coordinated shutdowns (restart/render/Ctrl+C) collapse the in-progress signal
// into one summary line; single watcher failures still emit per-watcher stops.
func stopWatchers(watchers []runningWatcher, timeout time.Duration, out io.Writer, streamer *devwatchStreamer, collapse bool) {
	wait := beginStopWatchers(watchers, timeout, out, streamer, collapse)
	wait()
}

func beginStopWatchers(watchers []runningWatcher, timeout time.Duration, out io.Writer, streamer *devwatchStreamer, collapse bool) func() {
	if collapse {
		names := make([]string, 0, len(watchers))
		for _, watcher := range watchers {
			if watcher.proc == nil {
				continue
			}
			names = append(names, watcher.name)
		}
		emitWatcherLifecycleSummary(out, streamer, names, watcherStateStopping)
	}
	for _, watcher := range watchers {
		if watcher.proc == nil {
			continue
		}
		if !collapse {
			emitWatcherLifecycleLine(out, streamer, watcher.name, watcherStateStopping)
		}
	}
	var wg sync.WaitGroup
	for _, watcher := range watchers {
		if watcher.proc == nil {
			continue
		}
		wg.Add(1)
		go func(proc *execx.Process) {
			defer wg.Done()
			_ = proc.GracefulShutdown(os.Interrupt, timeout)
		}(watcher.proc)
	}
	return wg.Wait
}

// drainWatcherExits drains a fixed number of exit events to ensure goroutines finish cleanly.
func drainWatcherExits(exitCh <-chan watcherExit, count int, out io.Writer, streamer *devwatchStreamer, collapse bool) {
	names := make([]string, 0, count)
	for i := 0; i < count; i++ {
		exit := <-exitCh
		if collapse {
			names = append(names, exit.name)
			continue
		}
		emitWatcherLifecycleLine(out, streamer, exit.name, watcherStateStopped)
	}
	if collapse {
		emitWatcherLifecycleSummary(out, streamer, names, watcherStateStopped)
	}
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

func removeWatcherByName(watchers []runningWatcher, name string) []runningWatcher {
	if len(watchers) == 0 {
		return nil
	}
	filtered := make([]runningWatcher, 0, len(watchers))
	for _, watcher := range watchers {
		if watcher.name == name {
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

func countImmediateStartupWatchers(watches []project.DevWatch) int {
	count := 0
	for _, watch := range watches {
		// wgo -postpone waits for the first file change, so those watchers do not
		// participate in the initial startup burst.
		if strings.Contains(watch.Watch, "-postpone") {
			continue
		}
		count++
	}
	return count
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

func copyEnvMap(vars map[string]string) map[string]string {
	if len(vars) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(vars))
	for key, value := range vars {
		out[key] = value
	}
	return out
}

func splitWatcherEnvAssignments(execCmd string) (map[string]string, string) {
	fields := strings.Fields(execCmd)
	if len(fields) == 0 {
		return nil, execCmd
	}
	env := map[string]string{}
	consumed := 0
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || key == "" || !isShellEnvName(key) {
			break
		}
		env[key] = value
		consumed++
	}
	if consumed == 0 {
		return nil, execCmd
	}
	rest := strings.Join(fields[consumed:], " ")
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
		res, err := execx.Command("bash", "-c", task.Cmd).
			EnvInherit().
			StdinReader(os.Stdin).
			StdoutWriter(os.Stdout).
			StderrWriter(os.Stderr).
			Run()
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

// configureWatcherPTY wires PTY and output hooks based on platform constraints.
// PTY preserves native TTY behavior (colors, cursor control) but merges stdout/stderr.
// On PTY platforms, we avoid attaching stderr writers to prevent duplicate output.
func configureWatcherPTY(cmd *execx.Cmd, soundEnabled bool) *execx.Cmd {
	switch goruntime.GOOS {
	case "linux", "darwin":
		// PTY merges stdout/stderr into a single stream.
		cmd = cmd.WithPTY()
		cmd = cmd.StderrWriter(nil)
		if soundEnabled {
			cmd = cmd.OnStdout(errorSoundHook(true))
		}
	default:
		if soundEnabled {
			cmd = cmd.OnStdout(errorSoundHook(true)).OnStderr(errorSoundHook(true))
		}
	}
	return cmd
}

// errorSoundHook emits a sound when matching error output appears.
func errorSoundHook(enabled bool) func(string) {
	if !enabled {
		return nil
	}
	errorLimiter := newSoundLimiter(2 * time.Second)
	hadError := false
	var recoveryTimer *time.Timer
	return func(line string) {
		if isWatcherTriggerLine(line) {
			if hadError {
				if recoveryTimer != nil {
					recoveryTimer.Stop()
				}
				recoveryTimer = time.AfterFunc(750*time.Millisecond, func() {
					hadError = false
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
	if goruntime.GOOS != "darwin" {
		return
	}
	_ = execx.Command("afplay", "/System/Library/Sounds/Submarine.aiff").Start()
}

// playRecoverySound plays a macOS recovery sound when available.
func playRecoverySound() {
	if goruntime.GOOS != "darwin" {
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
	watcher = str.Of(watcher).TrimSpace().String()
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
		watcher = str.Of(watcher).TrimSpace().String()
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
	if err := ensureTool("wgo", "github.com/bokwoon95/wgo@v0.6.3"); err != nil {
		return err
	}
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

// isProcessRunning checks whether a PID exists on this host.
func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	return err == syscall.EPERM
}
