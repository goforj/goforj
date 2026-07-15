package forj

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

type devWatcherKind string

const (
	devWatcherAppBuild devWatcherKind = "app_build"
	devWatcherAppRun   devWatcherKind = "app_run"
	devWatcherSPABuild devWatcherKind = "spa_build"
	devWatcherCustom   devWatcherKind = "custom"
)

type devCompiledWatcher struct {
	Watch                devwatch.Spec
	ID                   string
	Name                 string
	Kind                 devWatcherKind
	App                  string
	Command              devwatch.Command
	Postpone             bool
	Restart              bool
	Exit                 bool
	WatchChanges         bool
	Legacy               bool
	PollInterval         time.Duration
	OnSuccess            []string
	Verbose              bool
	ExecLog              bool
	ExecMessage          string
	LogPrefix            string
	LogPrefixSet         bool
	DisplayCommand       string
	NativeRuntimeCommand string
	FullProcessOverride  bool
}

// compileDevWatchers turns app lifecycle intent and custom watches into one native execution graph.
func compileDevWatchers(config *project.Config) ([]devCompiledWatcher, error) {
	if config == nil {
		return nil, fmt.Errorf("compile dev watchers: project config is required")
	}
	if err := validateStructuredDevAppNames(config); err != nil {
		return nil, err
	}

	compiled, err := compileStructuredDevApps(config)
	if err != nil {
		return nil, err
	}
	for index, watch := range config.Dev.Watches {
		watcher, err := compileCustomDevWatcher(watch, index)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, watcher)
	}
	compiled, err = normalizeCompiledDevWatchers(compiled)
	if err != nil {
		return nil, err
	}
	if err := validateCompiledDevWatchers(compiled); err != nil {
		return nil, err
	}
	return compiled, nil
}

// validateStructuredDevAppNames prevents lifecycle configuration from escaping conventional app-owned paths.
func validateStructuredDevAppNames(config *project.Config) error {
	if config == nil {
		return nil
	}
	names := make([]string, 0, len(config.Dev.Apps))
	for name := range config.Dev.Apps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			return fmt.Errorf("compile dev app %q: app name must be a safe non-reserved slug", name)
		}
	}
	return nil
}

// compileStructuredDevApps expands listed apps and their SPAs using framework conventions.
func compileStructuredDevApps(config *project.Config) ([]devCompiledWatcher, error) {
	apps := selectedStructuredDevApps(config)
	compiled := make([]devCompiledWatcher, 0, len(apps)*2)
	for _, selected := range apps {
		app := project.DefaultNamedApp(selected.name)
		build, err := compileStructuredAppBuild(config, app, selected.config)
		if err != nil {
			return nil, err
		}
		spas := make([]devCompiledWatcher, 0, len(selected.config.SPAs))
		spaNames := make([]string, 0, len(selected.config.SPAs))
		for name := range selected.config.SPAs {
			spaNames = append(spaNames, name)
		}
		sort.Strings(spaNames)
		for _, name := range spaNames {
			spa, err := compileStructuredSPA(app, name, selected.config.SPAs[name], build)
			if err != nil {
				return nil, err
			}
			spas = append(spas, spa)
		}

		runtime, err := compileStructuredAppRuntime(app, appRenderComponents(config, app), selected.config, build != nil)
		if err != nil {
			return nil, err
		}
		if runtime != nil {
			if build != nil {
				build.OnSuccess = append(build.OnSuccess, runtime.ID)
			}
		}
		if build != nil {
			compiled = append(compiled, *build)
		}
		compiled = append(compiled, spas...)
		if runtime != nil {
			compiled = append(compiled, *runtime)
		}
	}
	return compiled, nil
}

type selectedStructuredDevApp struct {
	name   string
	config project.DevApp
}

// selectedStructuredDevApps respects explicit app selection while keeping generated app ordering stable.
func selectedStructuredDevApps(config *project.Config) []selectedStructuredDevApp {
	if config == nil || len(config.Dev.Apps) == 0 {
		return nil
	}
	requested := requestedDevAppName()
	names := make([]string, 0, len(config.Dev.Apps))
	for name := range config.Dev.Apps {
		if requested != "" && name != requested {
			continue
		}
		names = append(names, name)
	}
	sort.Slice(names, func(left, right int) bool {
		if names[left] == project.DefaultAppName {
			return true
		}
		if names[right] == project.DefaultAppName {
			return false
		}
		return names[left] < names[right]
	})
	apps := make([]selectedStructuredDevApp, 0, len(names))
	for _, name := range names {
		apps = append(apps, selectedStructuredDevApp{name: name, config: config.Dev.Apps[name]})
	}
	return apps
}

// compileStructuredAppBuild creates the build node that owns successful runtime publication.
func compileStructuredAppBuild(config *project.Config, app project.App, appConfig project.DevApp) (*devCompiledWatcher, error) {
	commandConfig := appConfig.Build
	if commandConfig != nil && commandConfig.Disabled {
		return nil, nil
	}
	name := devAppWatcherName("Build", app.Name)
	id := devStructuredAppWatcherID(devWatcherAppBuild, app.Name)
	conventional := conventionalDevAppBuildCommand(config, app)
	execCommand := conventional.Exec
	includes := conventional.Watch
	ignores := conventional.Ignore
	root := conventional.Root
	postpone := conventional.Postpone
	var workDir string
	var env map[string]string
	var debounce string
	var poll string
	if commandConfig != nil {
		if strings.TrimSpace(commandConfig.Exec) == "" && isNestedDevWorkDir(commandConfig.WorkDir) {
			return nil, fmt.Errorf("compile %s: build.workdir requires an explicit build.exec", name)
		}
		if strings.TrimSpace(commandConfig.Exec) != "" {
			execCommand = commandConfig.Exec
		}
		if len(commandConfig.Watch) > 0 {
			includes = commandConfig.Watch
		}
		if len(commandConfig.Ignore) > 0 {
			ignores = appendUniqueDevMatchers(ignores, commandConfig.Ignore...)
		}
		if strings.TrimSpace(commandConfig.Root) != "" {
			root = commandConfig.Root
		}
		workDir = commandConfig.WorkDir
		env = commandConfig.Env
		debounce = commandConfig.Debounce
		poll = commandConfig.Poll
		if commandConfig.PostponeSet {
			postpone = commandConfig.Postpone
		}
	}
	watch, pollInterval, err := compileStructuredWatchSpec(id, []string{root}, includes, ignores, nil, nil, debounce, poll)
	if err != nil {
		return nil, fmt.Errorf("compile %s: %w", name, err)
	}
	commandEnv := frameworkDevAppEnv(app, env)
	commandEnv["FORJ_BUILD_PROGRESS"] = "1"
	return &devCompiledWatcher{
		Watch: watch, ID: id, Name: name, Kind: devWatcherAppBuild, App: app.Name,
		Command:  devwatch.Command{Shell: execCommand, Dir: workDir, Env: commandEnv},
		Postpone: postpone, WatchChanges: true, PollInterval: pollInterval,
	}, nil
}

// compileStructuredAppRuntime creates a runtime node that is restarted only by a successful app build.
func compileStructuredAppRuntime(app project.App, components project.Components, appConfig project.DevApp, managedBuild bool) (*devCompiledWatcher, error) {
	commandConfig := appConfig.Run
	if commandConfig != nil && commandConfig.Disabled {
		return nil, nil
	}
	conventional := commandConfig == nil || commandConfig.IsConventional()
	if conventional && !components.HasRuntime() {
		return nil, nil
	}
	name := devAppWatcherName("Run", app.Name)
	id := devStructuredAppWatcherID(devWatcherAppRun, app.Name)
	if commandConfig != nil && commandConfig.Shorthand && strings.TrimSpace(commandConfig.Exec) == "" {
		return nil, fmt.Errorf("compile %s: scalar run command cannot be empty", name)
	}
	if commandConfig != nil && strings.TrimSpace(commandConfig.Exec) == "" && isNestedDevWorkDir(commandConfig.WorkDir) {
		return nil, fmt.Errorf("compile %s: run.workdir requires an explicit run.exec", name)
	}
	if commandConfig != nil && commandConfig.IsMapping() && strings.TrimSpace(commandConfig.Exec) == "" {
		return nil, fmt.Errorf("compile %s: run.exec is required for a run mapping", name)
	}
	binary := conventionalDevAppRuntimeCommand(app).Exec
	execCommand := binary
	if commandConfig != nil && strings.TrimSpace(commandConfig.Exec) != "" {
		execCommand = commandConfig.Exec
		if commandConfig.Shorthand {
			execCommand = binary + " " + strings.TrimSpace(commandConfig.Exec)
		}
	}
	var configuredWatch []string
	if commandConfig != nil {
		configuredWatch = commandConfig.Watch
	}
	runtimeMatchers := make([]string, 0, len(configuredWatch))
	for _, matcher := range configuredWatch {
		if managedBuild && isStructuredRuntimeBuildOutput(matcher, binary) {
			continue
		}
		runtimeMatchers = append(runtimeMatchers, matcher)
	}
	var watch devwatch.Spec
	var pollInterval time.Duration
	watchChanges := len(runtimeMatchers) > 0
	if watchChanges {
		root := commandConfig.Root
		if strings.TrimSpace(root) == "" {
			root = "."
		}
		var err error
		watch, pollInterval, err = compileStructuredWatchSpec(
			id, []string{root}, runtimeMatchers, commandConfig.Ignore, nil, nil,
			commandConfig.Debounce, commandConfig.Poll,
		)
		if err != nil {
			return nil, fmt.Errorf("compile %s: %w", name, err)
		}
	}
	postpone := false
	if commandConfig != nil && commandConfig.PostponeSet {
		postpone = commandConfig.Postpone
	}
	var workDir string
	var env map[string]string
	if commandConfig != nil {
		workDir = commandConfig.WorkDir
		env = commandConfig.Env
	}
	return &devCompiledWatcher{
		Watch: watch, ID: id, Name: name, Kind: devWatcherAppRun, App: app.Name,
		Command: devwatch.Command{
			Shell: execCommand, Dir: workDir, Env: frameworkDevAppEnv(app, env),
			Stdin: devWatcherStdin(false),
		},
		Postpone: postpone, Restart: true, WatchChanges: watchChanges, PollInterval: pollInterval,
		FullProcessOverride: commandConfig != nil && commandConfig.IsMapping() && !isExplicitConventionalDevRuntime(commandConfig, binary),
	}, nil
}

// isExplicitConventionalDevRuntime recognizes a rendered bare-binary snapshot without weakening custom process overrides.
func isExplicitConventionalDevRuntime(command *project.DevAppCommand, binary string) bool {
	if command == nil || command.Exec != binary {
		return false
	}
	return len(command.Watch) == 0 && len(command.Ignore) == 0 && command.Root == "" &&
		command.WorkDir == "" && len(command.Env) == 0 && command.Debounce == "" &&
		command.Poll == "" && !command.PostponeSet
}

// appendUniqueDevMatchers extends invariant matcher lists without duplicating rendered defaults.
func appendUniqueDevMatchers(values []string, additions ...string) []string {
	merged := append([]string(nil), values...)
	seen := make(map[string]struct{}, len(values)+len(additions))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, ok := seen[value]; ok {
			continue
		}
		merged = append(merged, value)
		seen[value] = struct{}{}
	}
	return merged
}

// isNestedDevWorkDir identifies command roots where project-relative lifecycle defaults would point at the wrong files.
func isNestedDevWorkDir(workDir string) bool {
	workDir = strings.TrimSpace(workDir)
	return workDir != "" && filepath.Clean(workDir) != "."
}

// frameworkDevAppEnv keeps structured builds and snapshot runtimes aligned with app-aware framework commands.
func frameworkDevAppEnv(app project.App, configured map[string]string) map[string]string {
	app = projectlayout.NormalizeApp(app)
	env := copyDevWatchEnv(configured)
	env["FORJ_APP"] = app.Name
	if app.Name == project.DefaultAppName {
		env["FORJ_COMMAND_PREFIX"] = "forj"
	} else {
		env["FORJ_COMMAND_PREFIX"] = "forj " + app.Name
	}
	return env
}

// isStructuredRuntimeBuildOutput removes the conventional binary edge because app-build success owns it directly.
func isStructuredRuntimeBuildOutput(matcher string, binary string) bool {
	matcher = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(matcher)), "./")
	binary = strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(binary)), "./")
	return matcher == binary
}

// compileStructuredSPA creates a frontend build node that feeds its owning app build.
func compileStructuredSPA(app project.App, name string, spa project.DevSPA, build *devCompiledWatcher) (devCompiledWatcher, error) {
	name = strings.TrimSpace(name)
	if !project.IsSafeAppName(name) {
		return devCompiledWatcher{}, fmt.Errorf("compile SPA for app %q: SPA name %q must be a safe slug", app.Name, name)
	}
	root := strings.TrimSpace(spa.Path)
	if root == "" {
		return devCompiledWatcher{}, fmt.Errorf("compile SPA %q for app %q: path is required", name, app.Name)
	}
	watchName := "Build " + app.Name + " SPA " + name
	id := devStructuredSPAWatcherID(app.Name, name)
	conventional := conventionalDevSPAConfig(root)
	command := strings.TrimSpace(spa.Build)
	if command == "" {
		command = conventional.Build
	}
	includes := spa.Watch
	if len(includes) == 0 {
		includes = conventional.Watch
	}
	ignores := spa.Ignore
	if len(ignores) == 0 {
		ignores = conventional.Ignore
	}
	watch, _, err := compileStructuredWatchSpec(id, []string{root}, includes, ignores, nil, nil, "", "")
	if err != nil {
		return devCompiledWatcher{}, fmt.Errorf("compile %s: %w", watchName, err)
	}
	onSuccess := make([]string, 0, 1)
	if build != nil {
		onSuccess = append(onSuccess, build.ID)
	}
	return devCompiledWatcher{
		Watch: watch, ID: id, Name: watchName, Kind: devWatcherSPABuild, App: app.Name,
		Command: devwatch.Command{Shell: command, Dir: root}, Postpone: true,
		WatchChanges: true, OnSuccess: onSuccess,
	}, nil
}

// compileCustomDevWatcher preserves scalar wgo grammar and opts list-shaped watches into native matchers.
func compileCustomDevWatcher(watch project.DevWatch, index int) (devCompiledWatcher, error) {
	watch.Name = strings.TrimSpace(watch.Name)
	if watch.Name == "" {
		watch.Name = fmt.Sprintf("Watch %d", index+1)
	}
	id := devCustomWatcherID(index)
	if watch.IsLegacy() {
		compiled, err := compileLegacyDevWatcher(watch)
		if err != nil {
			return devCompiledWatcher{}, err
		}
		compiled.ID = id
		compiled.Watch.Name = id
		return compiled, nil
	}
	roots := watch.Roots
	if len(roots) == 0 {
		roots = []string{"."}
	}
	includes := append([]string(nil), watch.Include...)
	includes = append(includes, watch.Files.Include...)
	ignores := append([]string(nil), watch.Ignore...)
	ignores = append(ignores, watch.Files.Exclude...)
	compiledWatch, pollInterval, err := compileStructuredWatchSpec(
		id, roots, includes, ignores, watch.Dirs.Include, watch.Dirs.Exclude,
		watch.Debounce, watch.Poll,
	)
	if err != nil {
		return devCompiledWatcher{}, fmt.Errorf("compile dev watcher %q: %w", watch.Name, err)
	}
	watchEnv, execCommand := splitWatcherEnvAssignments(watch.Exec)
	env := copyDevWatchEnv(watch.Env)
	for key, value := range watchEnv {
		env[key] = value
	}
	return devCompiledWatcher{
		Watch: compiledWatch, ID: id, Name: watch.Name, Kind: devWatcherCustom,
		Command: devwatch.Command{
			Shell: execCommand, Dir: watch.WorkDir, Env: env, Stdin: devWatcherStdin(watch.Stdin),
		},
		Postpone: watch.Postpone, Restart: watch.Restart, Exit: watch.Exit,
		WatchChanges: true, PollInterval: pollInterval,
	}, nil
}

// compileStructuredWatchSpec compiles readable path rules and timing into an engine specification.
func compileStructuredWatchSpec(
	name string,
	roots []string,
	includes []string,
	ignores []string,
	directoryIncludes []string,
	directoryExcludes []string,
	debounceValue string,
	pollValue string,
) (devwatch.Spec, time.Duration, error) {
	includeMatchers, err := compileNativeDevWatchMatchers(includes)
	if err != nil {
		return devwatch.Spec{}, 0, err
	}
	excludeMatchers, err := compileNativeDevWatchMatchers(ignores)
	if err != nil {
		return devwatch.Spec{}, 0, err
	}
	directoryIncludeMatchers, err := compileNativeDevWatchMatchers(directoryIncludes)
	if err != nil {
		return devwatch.Spec{}, 0, err
	}
	directoryIgnoreValues := append(append([]string(nil), ignores...), directoryExcludes...)
	directoryExcludeMatchers, err := compileNativeDevWatchMatchers(directoryIgnoreValues)
	if err != nil {
		return devwatch.Spec{}, 0, err
	}
	debounce, err := parseDevWatchDuration("debounce", debounceValue, devwatch.DefaultDebounce)
	if err != nil {
		return devwatch.Spec{}, 0, err
	}
	poll, err := parseDevWatchDuration("poll", pollValue, 0)
	if err != nil {
		return devwatch.Spec{}, 0, err
	}
	return devwatch.Spec{
		Name: name, Roots: roots, Includes: includeMatchers, Excludes: excludeMatchers,
		DirectoryIncludes: directoryIncludeMatchers, DirectoryExcludes: directoryExcludeMatchers,
		Debounce: debounce, DebounceSet: true,
	}, poll, nil
}

// compileNativeDevWatchMatchers preserves the simple matcher contract while surfacing invalid regexes early.
func compileNativeDevWatchMatchers(values []string) ([]devwatch.Matcher, error) {
	matchers := make([]devwatch.Matcher, 0, len(values))
	for _, value := range values {
		matcher, err := devwatch.NewMatcher(value)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, matcher)
	}
	return matchers, nil
}

type devLegacyWatchOptions struct {
	roots               []string
	files               []devwatch.Matcher
	excludedFiles       []devwatch.Matcher
	directories         []devwatch.Matcher
	excludedDirectories []devwatch.Matcher
	workDir             string
	debounce            time.Duration
	poll                time.Duration
	postpone            bool
	exit                bool
	stdin               bool
	verbose             bool
	execLog             bool
	execMessage         string
	logPrefix           string
	logPrefixSet        bool
}

// compileLegacyDevWatcher translates the wgo flag grammar without requiring the external executable.
func compileLegacyDevWatcher(watch project.DevWatch) (devCompiledWatcher, error) {
	options, err := parseLegacyDevWatchOptions(watch.Watch)
	if err != nil {
		return devCompiledWatcher{}, fmt.Errorf("compile legacy dev watcher %q: %w", watch.Name, err)
	}
	watchEnv, execCommand := splitWatcherEnvAssignments(watch.Exec)
	env := copyDevWatchEnv(watch.Env)
	for key, value := range watchEnv {
		env[key] = value
	}
	kind := devWatcherCustom
	appName := ""
	nativeRuntimeCommand := ""
	configuredApp := strings.TrimSpace(watch.Env["FORJ_APP"])
	frameworkApp := project.IsSafeAppName(configuredApp) && !project.IsReservedAppName(configuredApp)
	if frameworkApp && watch.Name == devAppWatcherName("Build", configuredApp) {
		kind = devWatcherAppBuild
		appName = configuredApp
		env["FORJ_BUILD_PROGRESS"] = "1"
	} else if frameworkApp && watch.Name == devAppWatcherName("Run", configuredApp) {
		kind = devWatcherAppRun
		appName = configuredApp
		if _, ok := devExecutableTarget(execCommand); ok {
			nativeRuntimeCommand = execCommand
		}
	}
	return devCompiledWatcher{
		Watch: devwatch.Spec{
			Name: watch.Name, Roots: options.roots, Includes: options.files,
			Excludes: options.excludedFiles, DirectoryIncludes: options.directories,
			DirectoryExcludes: options.excludedDirectories,
			Debounce:          options.debounce, DebounceSet: true, LegacyDirectoryRegex: true,
		},
		Name: watch.Name, Kind: kind, App: appName,
		Command: devwatch.Command{
			Shell: execCommand, Dir: options.workDir, Env: env, Stdin: devWatcherStdin(options.stdin),
		},
		Postpone: options.postpone, Restart: true, Exit: options.exit,
		WatchChanges: true, Legacy: true, PollInterval: options.poll, Verbose: options.verbose,
		ExecLog: options.execLog, ExecMessage: options.execMessage,
		LogPrefix: options.logPrefix, LogPrefixSet: options.logPrefixSet,
		NativeRuntimeCommand: nativeRuntimeCommand,
	}, nil
}

// parseLegacyDevWatchOptions implements wgo v0.6.3's watcher flags and common logging-fork additions.
func parseLegacyDevWatchOptions(value string) (devLegacyWatchOptions, error) {
	args, err := shellSplitArgs(value)
	if err != nil {
		return devLegacyWatchOptions{}, err
	}
	options := devLegacyWatchOptions{roots: []string{"."}, debounce: devwatch.DefaultDebounce}
	for index := 0; index < len(args); index++ {
		key, inlineValue, hasInlineValue := strings.Cut(args[index], "=")
		if strings.HasPrefix(key, "--") && len(key) > 2 {
			key = "-" + strings.TrimPrefix(key, "--")
		}
		if !strings.HasPrefix(key, "-") {
			return devLegacyWatchOptions{}, fmt.Errorf("unexpected legacy watch argument %q", args[index])
		}
		readValue := func() (string, error) {
			if hasInlineValue {
				return inlineValue, nil
			}
			if index+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", key)
			}
			index++
			return args[index], nil
		}
		readBool := func() (bool, error) {
			if !hasInlineValue {
				return true, nil
			}
			parsed, err := strconv.ParseBool(inlineValue)
			if err != nil {
				return false, fmt.Errorf("%s: %w", key, err)
			}
			return parsed, nil
		}
		switch key {
		case "-root":
			root, err := readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
			options.roots = append(options.roots, root)
		case "-file", "-xfile", "-dir", "-xdir":
			pattern, err := readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
			matcher, err := devwatch.NewLegacyRegexpMatcher(pattern)
			if err != nil {
				return devLegacyWatchOptions{}, fmt.Errorf("%s %q: %w", key, pattern, err)
			}
			switch key {
			case "-file":
				options.files = append(options.files, matcher)
			case "-xfile":
				options.excludedFiles = append(options.excludedFiles, matcher)
			case "-dir":
				options.directories = append(options.directories, matcher)
			case "-xdir":
				options.excludedDirectories = append(options.excludedDirectories, matcher)
			}
		case "-cd":
			options.workDir, err = readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
		case "-debounce":
			duration, err := readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
			options.debounce, err = parseDevWatchDuration("-debounce", duration, devwatch.DefaultDebounce)
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
		case "-poll":
			duration, err := readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
			options.poll, err = parseDevWatchDuration("-poll", duration, 0)
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
		case "-postpone", "-exit", "-stdin", "-verbose":
			enabled, err := readBool()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
			switch key {
			case "-postpone":
				options.postpone = enabled
			case "-exit":
				options.exit = enabled
			case "-stdin":
				options.stdin = enabled
			case "-verbose":
				options.verbose = enabled
			}
		case "-exec-log":
			options.execLog, err = readBool()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
		case "-exec-msg":
			options.execMessage, err = readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
		case "-log-prefix":
			options.logPrefix, err = readValue()
			if err != nil {
				return devLegacyWatchOptions{}, err
			}
			options.logPrefixSet = true
		default:
			return devLegacyWatchOptions{}, fmt.Errorf("unsupported wgo flag %q; use structured watch fields or a supported legacy flag", key)
		}
	}
	return options, nil
}

// parseDevWatchDuration treats an omitted value as a semantic default and rejects negative intervals.
func parseDevWatchDuration(name string, value string, fallback time.Duration) (time.Duration, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", name, value, err)
	}
	if duration < 0 {
		return 0, fmt.Errorf("%s must not be negative", name)
	}
	return duration, nil
}

// devWatcherStdin keeps watcher commands non-interactive unless configuration opts in.
func devWatcherStdin(enabled bool) *os.File {
	if enabled {
		return os.Stdin
	}
	return nil
}

// devAppWatcherName preserves the existing default-app labels used by the transcript UI.
func devAppWatcherName(action string, appName string) string {
	if appName == project.DefaultAppName {
		return action + " App"
	}
	return action + " " + appName
}

// devStructuredAppWatcherID returns a stable graph identity independent of the lifecycle display label.
func devStructuredAppWatcherID(kind devWatcherKind, appName string) string {
	return "structured:app:" + appName + ":" + string(kind)
}

// devStructuredSPAWatcherID returns a stable graph identity for one App-owned frontend.
func devStructuredSPAWatcherID(appName string, spaName string) string {
	return "structured:app:" + appName + ":spa:" + spaName
}

// devCustomWatcherID preserves configuration order as the identity for user-defined watchers.
func devCustomWatcherID(index int) string {
	return "custom:" + strconv.Itoa(index+1)
}

// devAdHocWatcherID gives manually assembled runner specifications a deterministic fallback identity.
func devAdHocWatcherID(index int) string {
	return "compiled:" + strconv.Itoa(index+1)
}

// normalizeCompiledDevWatchers upgrades display-name graph fixtures and routes physical watches through internal IDs.
func normalizeCompiledDevWatchers(watchers []devCompiledWatcher) ([]devCompiledWatcher, error) {
	normalized := append([]devCompiledWatcher(nil), watchers...)
	for index := range normalized {
		normalized[index].OnSuccess = append([]string(nil), normalized[index].OnSuccess...)
	}
	displayCounts := make(map[string]int, len(normalized))
	for _, watcher := range normalized {
		displayCounts[watcher.Name]++
	}
	ids := make(map[string]struct{}, len(normalized))
	for index := range normalized {
		id := strings.TrimSpace(normalized[index].ID)
		if id == "" && displayCounts[normalized[index].Name] == 1 {
			id = normalized[index].Name
		}
		if id == "" {
			id = devAdHocWatcherID(index)
		}
		if _, exists := ids[id]; exists {
			return nil, fmt.Errorf("duplicate compiled dev watcher ID %q", id)
		}
		normalized[index].ID = id
		ids[id] = struct{}{}
	}
	displayIDs := make(map[string][]string, len(normalized))
	for index := range normalized {
		watcher := &normalized[index]
		displayIDs[watcher.Name] = append(displayIDs[watcher.Name], watcher.ID)
		if watcher.WatchChanges {
			watcher.Watch.Name = watcher.ID
		}
	}
	for index := range normalized {
		for successorIndex, successor := range normalized[index].OnSuccess {
			if _, exists := ids[successor]; exists {
				continue
			}
			matches := displayIDs[successor]
			if len(matches) != 1 {
				return nil, fmt.Errorf(
					"dev watcher %q has ambiguous display-name successor %q; graph edges must use watcher IDs",
					normalized[index].Name,
					successor,
				)
			}
			normalized[index].OnSuccess[successorIndex] = matches[0]
		}
	}
	return normalized, nil
}

// validateCompiledDevWatchers prevents duplicate internal identities and broken graph edges.
func validateCompiledDevWatchers(watchers []devCompiledWatcher) error {
	ids := make(map[string]struct{}, len(watchers))
	for _, watcher := range watchers {
		if strings.TrimSpace(watcher.Name) == "" {
			return fmt.Errorf("compiled dev watcher name is required")
		}
		if strings.TrimSpace(watcher.ID) == "" {
			return fmt.Errorf("compiled dev watcher %q ID is required", watcher.Name)
		}
		if _, exists := ids[watcher.ID]; exists {
			return fmt.Errorf("duplicate compiled dev watcher ID %q", watcher.ID)
		}
		ids[watcher.ID] = struct{}{}
		if strings.TrimSpace(watcher.Command.Shell) == "" && len(watcher.Command.Args) == 0 {
			return fmt.Errorf("dev watcher %q has no command", watcher.Name)
		}
	}
	for _, watcher := range watchers {
		for _, successor := range watcher.OnSuccess {
			if _, exists := ids[successor]; !exists {
				return fmt.Errorf("dev watcher %q triggers missing watcher %q", watcher.Name, successor)
			}
		}
	}
	return nil
}
