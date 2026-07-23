package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

func TestDevArtifactRootDefaultsToProjectBin(t *testing.T) {
	t.Setenv(devArtifactRootEnv, "")
	if got := devRuntimeExecutable(project.DefaultApp()); got != "./bin/app" {
		t.Fatalf("runtime executable = %q, want ./bin/app", got)
	}
	if got := devRuntimeReadyStamp(project.DefaultApp()); got != "./bin/.app.ready" {
		t.Fatalf("ready stamp = %q, want ./bin/.app.ready", got)
	}
	if got := devArtifactBuildEnvironment(nil); got != nil {
		t.Fatalf("default build environment = %#v, want nil", got)
	}
	if got := devBuildCommandForApp("forj build -o ./bin/app", project.DefaultApp()); got != "forj build -o ./bin/app" {
		t.Fatalf("default build command = %q, want unchanged command", got)
	}
}

func TestDevArtifactRootRedirectsCompiledRuntimeAndBuild(t *testing.T) {
	artifacts := t.TempDir()
	t.Setenv(devArtifactRootEnv, artifacts)
	config := &project.Config{Dev: project.DevConfig{Apps: map[string]project.DevApp{
		"app": {Run: &project.DevAppCommand{Exec: "run", Shorthand: true}},
	}}}
	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compile dev watchers: %v", err)
	}
	binary := filepath.Join(artifacts, "bin", "app")
	cache := filepath.Join(artifacts, ".gocache")
	var build, runtime *devCompiledWatcher
	for index := range watchers {
		watcher := &watchers[index]
		switch watcher.Kind {
		case devWatcherAppBuild:
			build = watcher
		case devWatcherAppRun:
			runtime = watcher
		}
	}
	if build == nil || !strings.Contains(build.Command.Shell, "-o "+shellSingleQuote(filepath.ToSlash(binary))) {
		t.Fatalf("build watcher = %#v, want output %q", build, binary)
	}
	if build.Command.Env["GOCACHE"] != cache {
		t.Fatalf("build GOCACHE = %q, want %q", build.Command.Env["GOCACHE"], cache)
	}
	if runtime == nil || runtime.Command.Shell != shellSingleQuote(filepath.ToSlash(binary))+" run" {
		t.Fatalf("runtime watcher = %#v, want executable %q", runtime, binary)
	}
	if got := devRuntimeReadyStamp(project.DefaultApp()); got != filepath.Join(artifacts, "bin", ".app.ready") {
		t.Fatalf("ready stamp = %q", got)
	}
}

func TestDevArtifactRootRejectsRelativeAndProjectPaths(t *testing.T) {
	t.Setenv(devArtifactRootEnv, "artifacts")
	if _, err := devArtifactRoot(); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative root error = %v, want absolute-path rejection", err)
	}
	checkout, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for _, root := range []string{checkout, filepath.Join(checkout, "artifacts")} {
		t.Setenv(devArtifactRootEnv, root)
		if _, err := compileDevWatchers(&project.Config{}); err == nil || !strings.Contains(err.Error(), "outside") {
			t.Fatalf("root %q error = %v, want checkout rejection", root, err)
		}
	}
	link := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(checkout, link); err != nil {
		t.Fatalf("symlink checkout: %v", err)
	}
	t.Setenv(devArtifactRootEnv, filepath.Join(link, "artifacts"))
	if _, err := compileDevWatchers(&project.Config{}); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("symlinked checkout root error = %v, want checkout rejection", err)
	}
}

func TestDevArtifactRootCleanupUsesExternalReadyStamp(t *testing.T) {
	artifacts := t.TempDir()
	t.Setenv(devArtifactRootEnv, artifacts)
	app := project.DefaultApp()
	ready := devRuntimeReadyStamp(app)
	if err := os.MkdirAll(filepath.Dir(ready), 0o755); err != nil {
		t.Fatalf("mkdir artifact bin: %v", err)
	}
	if err := os.WriteFile(ready, []byte("ready\n"), 0o644); err != nil {
		t.Fatalf("write ready stamp: %v", err)
	}
	clearDevBuildReadyStamps([]devBuildJob{{app: app}})
	if _, err := os.Stat(ready); !os.IsNotExist(err) {
		t.Fatalf("external ready stamp cleanup: %v", err)
	}
}

func TestDevArtifactRootRedirectsExplicitStructuredAndInitialBuildCommands(t *testing.T) {
	artifacts := filepath.Join(t.TempDir(), "artifacts with spaces;$(unsafe)'quote")
	t.Setenv(devArtifactRootEnv, artifacts)
	app := project.DefaultApp()
	rawBuild := conventionalDevAppBuildExec(app)
	rawRun := projectlayout.RuntimeExecutable(".", app)
	config := &project.Config{Dev: project.DevConfig{Apps: map[string]project.DevApp{
		"app": {
			Build: &project.DevAppCommand{Exec: rawBuild},
			Run:   &project.DevAppCommand{Exec: rawRun},
		},
	}}}
	watchers, err := compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compile explicit structured watchers: %v", err)
	}
	wantBinary := filepath.Join(artifacts, "bin", "app")
	wantShellBinary := shellSingleQuote(filepath.ToSlash(wantBinary))
	foundBuild := false
	foundRuntime := false
	for _, watcher := range watchers {
		switch watcher.Kind {
		case devWatcherAppBuild:
			foundBuild = true
			if watcher.Command.Shell != "forj build -o "+wantShellBinary {
				t.Fatalf("explicit build watcher = %q", watcher.Command.Shell)
			}
		case devWatcherAppRun:
			foundRuntime = true
			if watcher.Command.Shell != wantShellBinary || watcher.FullProcessOverride {
				t.Fatalf("explicit run watcher = %#v", watcher)
			}
		}
	}
	if !foundBuild || !foundRuntime {
		t.Fatalf("explicit structured watchers did not include build and runtime: %#v", watchers)
	}
	jobs := devBuildJobs(config)
	if len(jobs) != 1 || jobs[0].command != "forj build -o "+wantShellBinary {
		t.Fatalf("initial build jobs = %#v", jobs)
	}

	config.Dev.Apps["app"] = project.DevApp{Build: &project.DevAppCommand{Exec: "make custom"}, Run: &project.DevAppCommand{Exec: "./scripts/custom-run"}}
	watchers, err = compileDevWatchers(config)
	if err != nil {
		t.Fatalf("compile custom structured watchers: %v", err)
	}
	for _, watcher := range watchers {
		if watcher.Kind == devWatcherAppBuild && watcher.Command.Shell != "make custom" {
			t.Fatalf("custom build watcher = %q", watcher.Command.Shell)
		}
		if watcher.Kind == devWatcherAppRun && watcher.Command.Shell != "./scripts/custom-run" {
			t.Fatalf("custom run watcher = %q", watcher.Command.Shell)
		}
	}
}

func TestDevArtifactRootRedirectsLegacyCommandsAndQuotesShellPaths(t *testing.T) {
	artifacts := filepath.Join(t.TempDir(), "artifacts with spaces;$(unsafe)'quote")
	t.Setenv(devArtifactRootEnv, artifacts)
	wantBinary := filepath.Join(artifacts, "bin", "app")
	wantShellBinary := shellSingleQuote(filepath.ToSlash(wantBinary))
	build, err := compileLegacyDevWatcher(project.DevWatch{Name: "Build App", Exec: "forj build -o ./bin/app", Env: map[string]string{"FORJ_APP": "app"}})
	if err != nil {
		t.Fatalf("compile legacy build: %v", err)
	}
	if build.Command.Shell != "forj build -o "+wantShellBinary || build.Command.Env["GOCACHE"] != filepath.Join(artifacts, ".gocache") {
		t.Fatalf("legacy build = %#v", build)
	}
	runtime, err := compileLegacyDevWatcher(project.DevWatch{Name: "Run App", Exec: "./bin/app http:serve", Env: map[string]string{"FORJ_APP": "app"}})
	if err != nil {
		t.Fatalf("compile legacy runtime: %v", err)
	}
	if runtime.Command.Shell != wantShellBinary+" http:serve" || runtime.NativeRuntimeCommand != wantShellBinary+" http:serve" {
		t.Fatalf("legacy runtime = %#v", runtime)
	}
	if target, ok := devExecutableTarget(runtime.Command.Shell); !ok || target != wantBinary {
		t.Fatalf("quoted legacy target = (%q, %t), want (%q, true)", target, ok, wantBinary)
	}
	if script := buildWatcherExec(runtime.Command.Shell); !strings.Contains(script, shellSingleQuote(filepath.ToSlash(wantBinary))) {
		t.Fatalf("runtime wrapper did not preserve a safely quoted path: %q", script)
	}
	if got := devAutoMigrateShellCommand(&project.Config{}); got != wantShellBinary+" migrate" {
		t.Fatalf("auto-migrate command = %q", got)
	}
}
