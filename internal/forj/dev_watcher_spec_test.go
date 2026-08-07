package forj

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/devwatch"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestCompileDevWatchersStructuredDefaultsAndSPAGraph verifies the conventional app lifecycle expansion.
func TestCompileDevWatchersStructuredDefaultsAndSPAGraph(t *testing.T) {
	t.Setenv("FORJ_APP", "")
	config := &project.Config{Dev: project.DevConfig{Apps: map[string]project.DevApp{
		project.DefaultAppName: {
			Build: &project.DevAppCommand{Env: map[string]string{
				"FORJ_APP": "wrong", "FORJ_COMMAND_PREFIX": "wrong", "BUILD": "yes",
			}},
			Run: &project.DevAppCommand{Exec: "run", Shorthand: true, Env: map[string]string{
				"FORJ_APP": "wrong", "FORJ_COMMAND_PREFIX": "wrong", "RUN": "yes",
			}},
			SPAs: map[string]project.DevSPA{
				"portal": {Path: "./cmd/app/frontend"},
				"admin":  {Path: "./cmd/app/admin"},
			},
		},
	}}}

	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compileDevWatchers() error = %v", err)
	}
	if got, want := compiledDevWatcherNames(watchers), []string{
		"Build App",
		"Build app SPA admin",
		"Build app SPA portal",
		"Run App",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled watcher names = %#v, want %#v", got, want)
	}

	build := watchers[0]
	if build.Kind != devWatcherAppBuild || build.App != project.DefaultAppName {
		t.Fatalf("build identity = (%q, %q), want app build", build.Kind, build.App)
	}
	if build.Command.Shell != "forj build -o ./bin/app" {
		t.Fatalf("build command = %q", build.Command.Shell)
	}
	if !build.Postpone || !build.WatchChanges || build.Restart {
		t.Fatalf("build lifecycle flags = %#v", build)
	}
	if got, want := build.OnSuccess, []string{watchers[3].ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("build successors = %#v, want %#v", got, want)
	}
	if build.ID == build.Name || build.Watch.Name != build.ID {
		t.Fatalf("build internal identity = (%q, %q), display = %q", build.ID, build.Watch.Name, build.Name)
	}
	if build.Command.Env["FORJ_BUILD_PROGRESS"] != "1" || build.Command.Env["FORJ_APP"] != "app" ||
		build.Command.Env["FORJ_COMMAND_PREFIX"] != "forj" || build.Command.Env["BUILD"] != "yes" {
		t.Fatalf("build environment = %#v", build.Command.Env)
	}
	for _, candidate := range []string{"main.go", "internal/app/service.go", ".env", ".env.local"} {
		if !build.Watch.Matches(candidate) {
			t.Errorf("default build watcher did not match %q", candidate)
		}
	}
	for _, candidate := range []string{"forj/generated.go", "_data/cache.go", "app/wire/wire_gen.go", ".git/worktree.go", "cmd/app/frontend/node_modules/dependency.go"} {
		if build.Watch.Matches(candidate) {
			t.Errorf("default build watcher unexpectedly matched %q", candidate)
		}
	}

	spaPaths := map[string]string{
		"admin":  "./cmd/app/admin",
		"portal": "./cmd/app/frontend",
	}
	for index, spaName := range []string{"admin", "portal"} {
		spa := watchers[index+1]
		if spa.Kind != devWatcherSPABuild || spa.App != project.DefaultAppName {
			t.Fatalf("SPA identity = (%q, %q), want app SPA build", spa.Kind, spa.App)
		}
		if spa.Command.Dir != spaPaths[spaName] {
			t.Fatalf("%s SPA workdir = %q", spaName, spa.Command.Dir)
		}
		if spa.Command.Shell != "npm run build -s -- --logLevel silent" {
			t.Fatalf("%s SPA command = %q", spaName, spa.Command.Shell)
		}
		if got, want := spa.OnSuccess, []string{build.ID}; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s SPA successors = %#v, want %#v", spaName, got, want)
		}
		if !spa.Watch.Matches("src/app.ts") || !spa.Watch.Matches("src/app.tsx") ||
			!spa.Watch.Matches("src/app.jsx") || spa.Watch.Matches("node_modules/pkg/index.ts") {
			t.Fatalf("%s SPA default matcher controls are incorrect", spaName)
		}
	}

	runtime := watchers[3]
	if runtime.Kind != devWatcherAppRun || runtime.Command.Shell != "./bin/app run" || !runtime.Restart {
		t.Fatalf("runtime = %#v", runtime)
	}
	if runtime.Command.Env["FORJ_APP"] != "app" || runtime.Command.Env["FORJ_COMMAND_PREFIX"] != "forj" ||
		runtime.Command.Env["RUN"] != "yes" {
		t.Fatalf("runtime environment = %#v", runtime.Command.Env)
	}
}

// TestCompileDevWatchersAllowsDuplicateLegacyDisplayNames preserves historical dev.watches naming freedom.
func TestCompileDevWatchersAllowsDuplicateLegacyDisplayNames(t *testing.T) {
	config := &project.Config{Dev: project.DevConfig{Watches: []project.DevWatch{
		{Name: "Assets", Watch: "-file .css -postpone", Exec: "echo first"},
		{Name: "Assets", Watch: "-file .js -postpone", Exec: "echo second"},
	}}}

	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compileDevWatchers() error = %v", err)
	}
	if len(watchers) != 2 || watchers[0].Name != "Assets" || watchers[1].Name != "Assets" {
		t.Fatalf("duplicate display names were not preserved: %#v", watchers)
	}
	if watchers[0].ID == watchers[1].ID {
		t.Fatalf("duplicate legacy watchers shared internal ID %q", watchers[0].ID)
	}
	for _, watcher := range watchers {
		if watcher.Watch.Name != watcher.ID {
			t.Fatalf("physical watcher name = %q, want internal ID %q", watcher.Watch.Name, watcher.ID)
		}
	}
}

// TestCompileDevWatchersDisplayCollisionUsesIDsForGraph keeps custom labels from capturing structured edges.
func TestCompileDevWatchersDisplayCollisionUsesIDsForGraph(t *testing.T) {
	t.Setenv("FORJ_APP", "")
	config := &project.Config{Dev: project.DevConfig{
		Apps: map[string]project.DevApp{
			project.DefaultAppName: {
				Run: &project.DevAppCommand{Disabled: true},
				SPAs: map[string]project.DevSPA{
					"portal": {Path: "./cmd/app/frontend"},
				},
			},
		},
		Watches: []project.DevWatch{{
			Name: "Build App", Include: []string{".md"}, Exec: "echo custom",
		}},
	}}

	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compileDevWatchers() error = %v", err)
	}
	var build devCompiledWatcher
	var spa devCompiledWatcher
	var custom devCompiledWatcher
	for _, watcher := range watchers {
		switch watcher.Kind {
		case devWatcherAppBuild:
			build = watcher
		case devWatcherSPABuild:
			spa = watcher
		case devWatcherCustom:
			custom = watcher
		}
	}
	if build.Name != "Build App" || custom.Name != "Build App" || build.ID == custom.ID {
		t.Fatalf("display collision identities = build(%q, %q), custom(%q, %q)", build.ID, build.Name, custom.ID, custom.Name)
	}
	if got, want := spa.OnSuccess, []string{build.ID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SPA successors = %#v, want structured build ID %#v", got, want)
	}
	if len(spa.OnSuccess) == 1 && spa.OnSuccess[0] == custom.ID {
		t.Fatalf("custom display collision captured SPA graph edge %q", custom.ID)
	}
}

// TestCompileDevWatchersMultiAppSelectionAndOmittedApps covers deterministic ordering and presence-based participation.
func TestCompileDevWatchersMultiAppSelectionAndOmittedApps(t *testing.T) {
	config := &project.Config{Dev: project.DevConfig{Apps: map[string]project.DevApp{
		"billing": {Run: &project.DevAppCommand{Exec: "serve", Shorthand: true}},
		"app":     {Run: &project.DevAppCommand{Exec: "run", Shorthand: true}},
	}}}

	t.Setenv("FORJ_APP", "")
	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compileDevWatchers() error = %v", err)
	}
	if got, want := compiledDevWatcherNames(watchers), []string{
		"Build App", "Run App", "Build billing", "Run billing",
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled watcher names = %#v, want %#v", got, want)
	}
	if watchers[2].Command.Shell != "forj billing build -o ./bin/billing" {
		t.Fatalf("named build command = %q", watchers[2].Command.Shell)
	}
	if watchers[3].Command.Shell != "./bin/billing serve" {
		t.Fatalf("named runtime command = %q", watchers[3].Command.Shell)
	}
	t.Setenv("FORJ_APP", "billing")
	selected, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compileDevWatchers() selected error = %v", err)
	}
	if got, want := compiledDevWatcherNames(selected), []string{"Build billing", "Run billing"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("selected watcher names = %#v, want %#v", got, want)
	}
}

// TestCompileStructuredRuntimeStates verifies capability defaults and both override command shapes.
func TestCompileStructuredRuntimeStates(t *testing.T) {
	t.Parallel()
	runtimeComponents := project.Components{WebAPI: true}
	cliComponents := project.Components{CLI: true}
	tests := []struct {
		name       string
		components project.Components
		app        project.DevApp
		want       string
		wantFull   bool
	}{
		{name: "runtime default", components: runtimeComponents, want: "./bin/app"},
		{name: "explicit runtime default", components: runtimeComponents, app: project.DevApp{Run: &project.DevAppCommand{Exec: "./bin/app"}}, want: "./bin/app"},
		{name: "CLI default", components: cliComponents},
		{name: "disabled", components: runtimeComponents, app: project.DevApp{Run: &project.DevAppCommand{Disabled: true}}},
		{name: "scalar App command", components: cliComponents, app: project.DevApp{Run: &project.DevAppCommand{Exec: "http:serve", Shorthand: true}}, want: "./bin/app http:serve"},
		{name: "full override", components: cliComponents, app: project.DevApp{Run: &project.DevAppCommand{Exec: " env MODE=dev ./tools/server --once "}}, want: " env MODE=dev ./tools/server --once ", wantFull: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runtime, err := compileStructuredAppRuntime(project.DefaultApp(), test.components, test.app, true)
			if err != nil {
				t.Fatalf("compileStructuredAppRuntime() error = %v", err)
			}
			if test.want == "" {
				if runtime != nil {
					t.Fatalf("runtime = %#v, want no runtime node", runtime)
				}
				return
			}
			if runtime == nil || runtime.Command.Shell != test.want {
				t.Fatalf("runtime = %#v, want command %q", runtime, test.want)
			}
			if runtime.FullProcessOverride != test.wantFull {
				t.Fatalf("FullProcessOverride = %t, want %t", runtime.FullProcessOverride, test.wantFull)
			}
		})
	}
}

// TestCompileStructuredRuntimeMappingRequiresExec rejects an incomplete full-process override.
func TestCompileStructuredRuntimeMappingRequiresExec(t *testing.T) {
	t.Parallel()
	var config project.Config
	err := yaml.Unmarshal([]byte(`dev:
  apps:
    app:
      run:
        watch: [.toml]
`), &config)
	if err != nil {
		t.Fatalf("unmarshal dev config: %v", err)
	}
	_, err = compileDevWatchers(&config)
	if err == nil || !strings.Contains(err.Error(), "run.exec is required for a run mapping") {
		t.Fatalf("compileDevWatchers() error = %v, want missing mapped exec", err)
	}
}

// TestCompileStructuredRuntimeTrueAliasUsesCapabilityDefault keeps the compatibility alias from launching CLI help.
func TestCompileStructuredRuntimeTrueAliasUsesCapabilityDefault(t *testing.T) {
	t.Parallel()
	var config project.Config
	err := yaml.Unmarshal([]byte(`dev:
  apps:
    app:
      run: true
`), &config)
	if err != nil {
		t.Fatalf("unmarshal dev config: %v", err)
	}
	watchers, err := compileDevWatchers(&config)
	if err != nil {
		t.Fatalf("compileDevWatchers() error = %v", err)
	}
	if got, want := compiledDevWatcherNames(watchers), []string{"Build App"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("compiled watcher names = %#v, want %#v", got, want)
	}
}

// TestCompileStructuredRuntimeWatchControls keeps custom restart inputs while deduplicating the build-owned binary edge.
func TestCompileStructuredRuntimeWatchControls(t *testing.T) {
	app := project.DevApp{
		Run: &project.DevAppCommand{
			Exec: "serve", Shorthand: true,
			Watch: []string{"./bin/app", ".toml"}, Ignore: []string{"generated"},
			Root: ".", Debounce: "80ms", Poll: "2s", Postpone: true, PostponeSet: true,
		},
	}
	runtime, err := compileStructuredAppRuntime(project.DefaultApp(), project.Components{}, app, true)
	if err != nil {
		t.Fatalf("compileStructuredAppRuntime() error = %v", err)
	}
	if !runtime.WatchChanges || !runtime.Postpone || runtime.PollInterval != 2*time.Second {
		t.Fatalf("runtime watch controls = %#v", runtime)
	}
	if runtime.Watch.Debounce != 80*time.Millisecond {
		t.Fatalf("runtime debounce = %s", runtime.Watch.Debounce)
	}
	if !runtime.Watch.Matches("config/server.toml") || runtime.Watch.Matches("generated/server.toml") {
		t.Fatalf("runtime matchers did not preserve explicit non-binary inputs")
	}
	if runtime.Watch.Matches("bin/app") {
		t.Fatal("runtime binary must be restarted by the successful build edge, not an early filesystem event")
	}
}

// TestCompileStructuredRuntimeKeepsBinaryWatchWithoutManagedBuild supports externally rebuilt runtime artifacts.
func TestCompileStructuredRuntimeKeepsBinaryWatchWithoutManagedBuild(t *testing.T) {
	t.Parallel()
	runtime, err := compileStructuredAppRuntime(project.DefaultApp(), project.Components{}, project.DevApp{
		Build: &project.DevAppCommand{Disabled: true},
		Run: &project.DevAppCommand{
			Exec: "./external/app", Watch: []string{"./bin/app"},
		},
	}, false)
	if err != nil {
		t.Fatalf("compileStructuredAppRuntime() error = %v", err)
	}
	if runtime == nil || !runtime.WatchChanges || !runtime.Watch.Matches("bin/app") {
		t.Fatalf("external binary watcher was discarded: %#v", runtime)
	}
}

// TestCompileNativeDevWatcherControls verifies the complete structured custom watcher surface.
func TestCompileNativeDevWatcherControls(t *testing.T) {
	watcher, err := compileCustomDevWatcher(project.DevWatch{
		Name:    "Schema generation",
		Include: []string{".go"},
		Ignore:  []string{"re:_test\\.go$"},
		Roots:   []string{"."},
		WorkDir: "tools/schema",
		Files: project.DevWatchMatchers{
			Include: []string{`re:^schemas/.+\.graphql$`},
			Exclude: []string{"generated.go"},
		},
		Dirs: project.DevWatchMatchers{
			Include: []string{"schemas"},
			Exclude: []string{"generated"},
		},
		Exec:     "MODE=dev make schema",
		Env:      map[string]string{"MODE": "configured", "KEEP": "yes"},
		Debounce: "125ms",
		Poll:     "2s",
		Postpone: true,
		Restart:  true,
		Exit:     true,
		Stdin:    true,
	}, 0)
	if err != nil {
		t.Fatalf("compileCustomDevWatcher() error = %v", err)
	}
	if watcher.Legacy || watcher.Kind != devWatcherCustom || !watcher.WatchChanges {
		t.Fatalf("native watcher identity = %#v", watcher)
	}
	if got, want := watcher.Watch.Roots, []string{"."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("roots = %#v, want %#v", got, want)
	}
	if watcher.Watch.Debounce != 125*time.Millisecond || watcher.PollInterval != 2*time.Second {
		t.Fatalf("timing = (%s, %s)", watcher.Watch.Debounce, watcher.PollInterval)
	}
	if !watcher.Postpone || !watcher.Restart || !watcher.Exit || watcher.Command.Stdin != os.Stdin {
		t.Fatalf("native lifecycle flags = %#v", watcher)
	}
	if watcher.Command.Shell != "make schema" || watcher.Command.Dir != "tools/schema" {
		t.Fatalf("native command = %#v", watcher.Command)
	}
	if got, want := watcher.Command.Env, map[string]string{"MODE": "dev", "KEEP": "yes"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("native environment = %#v, want %#v", got, want)
	}
	for _, candidate := range []string{"schemas/model.go", "schemas/nested/model.graphql"} {
		if !watcher.Watch.Matches(candidate) {
			t.Errorf("native watcher did not match %q", candidate)
		}
	}
	for _, candidate := range []string{
		"cmd/app/main.go",
		"schemas/model_test.go",
		"schemas/generated.go",
		"schemas/generated/model.go",
	} {
		if watcher.Watch.Matches(candidate) {
			t.Errorf("native watcher unexpectedly matched %q", candidate)
		}
	}
}

func TestCompileLegacyDevWatcherFullFlagSurface(t *testing.T) {
	watcher, err := compileLegacyDevWatcher(project.DevWatch{
		Name: "Legacy",
		Watch: strings.Join([]string{
			`-root ./other`,
			`-root=./third`,
			`--file=.go`,
			`-file '^schema/.+\.json$'`,
			`-xfile '_test\.go$'`,
			`-dir '^cmd($|/)'`,
			`-xdir '(^|/)vendor($|/)'`,
			`-cd ./tools`,
			`-debounce 175ms`,
			`-poll=3s`,
			`--postpone`,
			`-exit`,
			`-stdin`,
			`-verbose`,
			`-exec-msg 'Building assets'`,
			`-exec-log`,
			`-log-prefix '[legacy] '`,
		}, " "),
		Exec: "APP_ENV=dev make watch",
		Env:  map[string]string{"KEEP": "yes"},
	})
	if err != nil {
		t.Fatalf("compileLegacyDevWatcher() error = %v", err)
	}
	if !watcher.Legacy || !watcher.Watch.LegacyDirectoryRegex || watcher.Kind != devWatcherCustom {
		t.Fatalf("legacy watcher identity = %#v", watcher)
	}
	if got, want := watcher.Watch.Roots, []string{".", "./other", "./third"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy roots = %#v, want %#v", got, want)
	}
	if len(watcher.Watch.Includes) != 2 || len(watcher.Watch.Excludes) != 1 ||
		len(watcher.Watch.DirectoryIncludes) != 1 || len(watcher.Watch.DirectoryExcludes) != 1 {
		t.Fatalf("legacy matcher counts = includes:%d excludes:%d dirs:%d xdirs:%d",
			len(watcher.Watch.Includes), len(watcher.Watch.Excludes),
			len(watcher.Watch.DirectoryIncludes), len(watcher.Watch.DirectoryExcludes))
	}
	if watcher.Watch.Debounce != 175*time.Millisecond || watcher.PollInterval != 3*time.Second {
		t.Fatalf("legacy timing = (%s, %s)", watcher.Watch.Debounce, watcher.PollInterval)
	}
	if !watcher.Postpone || !watcher.Restart || !watcher.Exit || !watcher.Verbose || watcher.Command.Stdin != os.Stdin {
		t.Fatalf("legacy lifecycle flags = %#v", watcher)
	}
	if !watcher.ExecLog || watcher.ExecMessage != "Building assets" ||
		!watcher.LogPrefixSet || watcher.LogPrefix != "[legacy] " {
		t.Fatalf("legacy logging controls = %#v", watcher)
	}
	if watcher.Command.Shell != "make watch" || watcher.Command.Dir != "./tools" {
		t.Fatalf("legacy command = %#v", watcher.Command)
	}
	if got, want := watcher.Command.Env, map[string]string{"APP_ENV": "dev", "KEEP": "yes"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy environment = %#v, want %#v", got, want)
	}
}

// TestCompileLegacyDevWatcherCapturesOnlyFrameworkNativeRuntimes keeps custom wgo commands on their historical wrapper.
func TestCompileLegacyDevWatcherCapturesOnlyFrameworkNativeRuntimes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		watchName  string
		exec       string
		wantKind   devWatcherKind
		wantApp    string
		wantNative string
	}{
		{
			name: "framework binary", watchName: "Run App", exec: "MODE=dev ./bin/app http:serve",
			wantKind: devWatcherAppRun, wantApp: "app", wantNative: "./bin/app http:serve",
		},
		{
			name: "framework shell command", watchName: "Run App", exec: "air -c .air.toml",
			wantKind: devWatcherAppRun, wantApp: "app",
		},
		{
			name: "custom binary command", watchName: "Run Tests", exec: "./bin/app test",
			wantKind: devWatcherCustom,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			watcher, err := compileLegacyDevWatcher(project.DevWatch{
				Name: test.watchName,
				Exec: test.exec,
				Env:  map[string]string{"FORJ_APP": "app"},
			})
			if err != nil {
				t.Fatalf("compileLegacyDevWatcher() error = %v", err)
			}
			if watcher.Kind != test.wantKind || watcher.App != test.wantApp {
				t.Fatalf("legacy watcher identity = (%q, %q), want (%q, %q)", watcher.Kind, watcher.App, test.wantKind, test.wantApp)
			}
			if watcher.NativeRuntimeCommand != test.wantNative {
				t.Fatalf("NativeRuntimeCommand = %q, want %q", watcher.NativeRuntimeCommand, test.wantNative)
			}
		})
	}
}

// TestCompileLegacyDevWatcherRequiresExactFrameworkIdentity keeps user-named watches out of app lifecycle routing.
func TestCompileLegacyDevWatcherRequiresExactFrameworkIdentity(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"Run Tests", "Build Docs"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			watcher, err := compileLegacyDevWatcher(project.DevWatch{
				Name: name, Watch: "-file .go", Exec: "go test ./...", Env: map[string]string{"FORJ_APP": "app"},
			})
			if err != nil {
				t.Fatalf("compileLegacyDevWatcher() error = %v", err)
			}
			if watcher.Kind != devWatcherCustom || watcher.App != "" {
				t.Fatalf("legacy watcher identity = (%q, %q), want custom", watcher.Kind, watcher.App)
			}
		})
	}

	watcher, err := compileLegacyDevWatcher(project.DevWatch{
		Name: "Build App", Watch: "-file .go", Exec: "forj build -o ./bin/app", Env: map[string]string{"FORJ_APP": "app"},
	})
	if err != nil {
		t.Fatalf("compileLegacyDevWatcher() framework error = %v", err)
	}
	if watcher.Kind != devWatcherAppBuild || watcher.App != "app" || watcher.Command.Env["FORJ_BUILD_PROGRESS"] != "1" {
		t.Fatalf("framework watcher identity = %#v", watcher)
	}
}

// TestLegacyWgoMatcherExactRegexSemantics covers wgo dot escaping, path trimming, and unanchored expressions.
func TestLegacyWgoMatcherExactRegexSemantics(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		pattern  string
		matches  []string
		excludes []string
	}{
		{
			name:     "extension dot becomes literal",
			pattern:  `^cmd/.+.go$`,
			matches:  []string{"cmd/main.go"},
			excludes: []string{"cmd/mainXgo", "internal/cmd/main.go"},
		},
		{
			name:     "current directory prefix is removed without adding an anchor",
			pattern:  `./cmd/app/main.go$`,
			matches:  []string{"cmd/app/main.go", "other/cmd/app/main.go"},
			excludes: []string{"cmd/app/main.go.bak"},
		},
		{
			name:     "ordinary regex remains unanchored",
			pattern:  `wire/wire_gen.go$`,
			matches:  []string{"app/wire/wire_gen.go"},
			excludes: []string{"app/wire/wire_gen.go.bak"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			matcher, err := devwatch.NewLegacyRegexpMatcher(test.pattern)
			if err != nil {
				t.Fatalf("newLegacyWgoMatcher() error = %v", err)
			}
			for _, candidate := range test.matches {
				if !matcher.Matches(candidate) {
					t.Errorf("legacy pattern %q did not match %q", test.pattern, candidate)
				}
			}
			for _, candidate := range test.excludes {
				if matcher.Matches(candidate) {
					t.Errorf("legacy pattern %q unexpectedly matched %q", test.pattern, candidate)
				}
			}
		})
	}
}

// TestParseLegacyDevWatchOptionsRejectsInvalidInput keeps compatibility failures focused.
func TestParseLegacyDevWatchOptionsRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		watch     string
		wantError string
	}{
		{name: "unknown flag", watch: "-wat .go", wantError: `unsupported wgo flag "-wat"`},
		{name: "missing value", watch: "-root", wantError: "-root requires a value"},
		{name: "invalid regex", watch: "-file '['", wantError: "error parsing regexp"},
		{name: "invalid boolean", watch: "-postpone=maybe", wantError: "invalid syntax"},
		{name: "invalid debounce", watch: "-debounce never", wantError: "-debounce"},
		{name: "negative polling", watch: "-poll -1s", wantError: "must not be negative"},
		{name: "unexpected argument", watch: "main.go", wantError: "unexpected legacy watch argument"},
		{name: "unterminated quote", watch: `-file "main.go`, wantError: "unterminated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseLegacyDevWatchOptions(test.watch)
			if err == nil || !strings.Contains(err.Error(), test.wantError) {
				t.Fatalf("parseLegacyDevWatchOptions() error = %v, want substring %q", err, test.wantError)
			}
		})
	}
}

// TestCompileDevWatchTimingDefaultsAndValidation verifies native polling and debounce normalization.
func TestCompileDevWatchTimingDefaultsAndValidation(t *testing.T) {
	t.Parallel()
	watchConfig := project.DevWatch{Include: []string{".md"}}
	compiled, err := compileStructuredWatchSpec("docs", watchConfig)
	if err != nil {
		t.Fatalf("compileStructuredWatchSpec() defaults error = %v", err)
	}
	if compiled.spec.Debounce != devwatch.DefaultDebounce || compiled.pollInterval != 0 {
		t.Fatalf("default timing = (%s, %s)", compiled.spec.Debounce, compiled.pollInterval)
	}
	if got, want := compiled.spec.Roots, []string{"."}; !reflect.DeepEqual(got, want) {
		t.Fatalf("default roots = %#v, want %#v", got, want)
	}

	watchConfig.Debounce = "40ms"
	watchConfig.Poll = "750ms"
	compiled, err = compileStructuredWatchSpec("docs", watchConfig)
	if err != nil {
		t.Fatalf("compileStructuredWatchSpec() explicit error = %v", err)
	}
	if compiled.spec.Debounce != 40*time.Millisecond || compiled.pollInterval != 750*time.Millisecond {
		t.Fatalf("explicit timing = (%s, %s)", compiled.spec.Debounce, compiled.pollInterval)
	}

	watchConfig.Debounce = "0s"
	watchConfig.Poll = ""
	compiled, err = compileStructuredWatchSpec("docs", watchConfig)
	if err != nil {
		t.Fatalf("compileStructuredWatchSpec() zero debounce error = %v", err)
	}
	if compiled.spec.Debounce != 0 || !compiled.spec.DebounceSet {
		t.Fatalf("explicit zero debounce = (%s, set %t)", compiled.spec.Debounce, compiled.spec.DebounceSet)
	}

	for _, test := range []struct {
		name     string
		debounce string
		poll     string
	}{
		{name: "bad debounce", debounce: "soon"},
		{name: "negative debounce", debounce: "-1ms"},
		{name: "bad poll", poll: "sometimes"},
		{name: "negative poll", poll: "-1ms"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileStructuredWatchSpec("docs", project.DevWatch{
				Include:  []string{".md"},
				Debounce: test.debounce,
				Poll:     test.poll,
			})
			if err == nil {
				t.Fatal("compileStructuredWatchSpec() unexpectedly accepted invalid timing")
			}
		})
	}
}

// TestCompileStructuredAppCommandsRequireExplicitExecInNestedWorkdir prevents project-relative defaults from silently pointing elsewhere.
func TestCompileStructuredAppCommandsRequireExplicitExecInNestedWorkdir(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		configure func(*project.DevApp)
		want      string
	}{
		{
			name: "build",
			configure: func(app *project.DevApp) {
				app.Build = &project.DevAppCommand{WorkDir: "tools/build"}
			},
			want: "build.workdir requires an explicit build.exec",
		},
		{
			name: "run",
			configure: func(app *project.DevApp) {
				app.Run = &project.DevAppCommand{WorkDir: "tools/runtime"}
			},
			want: "run.workdir requires an explicit run.exec",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			app := project.DevApp{}
			test.configure(&app)
			_, err := compileDevWatchers(&project.Config{Dev: project.DevConfig{
				Apps: map[string]project.DevApp{project.DefaultAppName: app},
			}})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compileDevWatchers() error = %v, want %q", err, test.want)
			}
		})
	}
}

// compiledDevWatcherNames returns graph node names in compiler order.
func compiledDevWatcherNames(watchers []devCompiledWatcher) []string {
	names := make([]string, len(watchers))
	for index, watcher := range watchers {
		names[index] = watcher.Name
	}
	return names
}
