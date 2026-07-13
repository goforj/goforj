package forj

import (
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/project"
)

const generatedFrontendSPAName = "frontend"

// generatedDevFrontendInstallTask keeps dependency setup aligned with each app-owned SPA root.
func generatedDevFrontendInstallTask(app project.App) project.DevTask {
	app = normalizeRenderApp(app)
	name := "Install Frontend Dependencies"
	if app.Name != project.DefaultAppName {
		name = "Install " + app.Name + " Frontend Dependencies"
	}
	return project.DevTask{
		Name: name,
		Cmd:  "cd " + filepath.ToSlash(appFrontendDir(app)) + " && npm install",
	}
}

// removeGeneratedDevFrontendInstallTask removes only the exact framework task owned by a deleted app.
func removeGeneratedDevFrontendInstallTask(tasks []project.DevTask, app project.App) ([]project.DevTask, bool) {
	want := generatedDevFrontendInstallTask(app)
	kept := make([]project.DevTask, 0, len(tasks))
	removed := false
	for _, task := range tasks {
		if task == want {
			removed = true
			continue
		}
		kept = append(kept, task)
	}
	return kept, removed
}

// generatedDevAppConfig snapshots framework-owned lifecycle behavior into readable project configuration.
func generatedDevAppConfig(config *project.Config, app project.App, runCommand string) project.DevApp {
	app = normalizeRenderApp(app)
	runCommand = strings.TrimSpace(runCommand)
	build := conventionalDevAppBuildCommand(config, app)
	configured := project.DevApp{Build: &build}
	components := appRenderComponents(config, app)
	switch {
	case runCommand != "" && runCommand != "run":
		configured.Run = &project.DevAppCommand{Exec: runCommand, Shorthand: true}
	case components.HasRuntime():
		run := conventionalDevAppRuntimeCommand(app)
		configured.Run = &run
	case runCommand == "run":
		configured.Run = &project.DevAppCommand{Exec: runCommand, Shorthand: true}
	}
	if config == nil || !components.WebUI || !project.StarterKitUsesNPM(appRenderStarterKit(config, app)) {
		return configured
	}
	configured.SPAs = map[string]project.DevSPA{
		generatedFrontendSPAName: conventionalDevSPAConfig(projectRelativeDevPath(appFrontendDir(app))),
	}
	return configured
}

// conventionalDevAppBuildCommand returns the editable build snapshot generated for a managed app.
func conventionalDevAppBuildCommand(config *project.Config, app project.App) project.DevAppCommand {
	app = normalizeRenderApp(app)
	command := project.DevAppCommand{
		Exec:        devBuildCommandForApp("forj build -o ./bin/app", app),
		Watch:       []string{".go", ".env", ".env.*"},
		Ignore:      []string{"forj", "_data", "wire_gen.go", ".git", ".hg", ".svn", ".idea", ".vscode", ".settings", "node_modules"},
		Root:        ".",
		Postpone:    true,
		PostponeSet: true,
	}
	if appRenderStarterKit(config, app) == project.StarterKitTemplHTMX {
		command.Watch = append(command.Watch, ".templ")
		command.Ignore = append(command.Ignore, `re:.*_templ\.go$`)
	}
	return command
}

// conventionalDevAppRuntimeCommand returns the bare-binary runtime snapshot generated for a managed app.
func conventionalDevAppRuntimeCommand(app project.App) project.DevAppCommand {
	app = normalizeRenderApp(app)
	return project.DevAppCommand{Exec: "./bin/" + app.Name}
}

// conventionalDevSPAConfig returns the editable frontend build snapshot generated for an app-owned SPA.
func conventionalDevSPAConfig(path string) project.DevSPA {
	return project.DevSPA{
		Path:   path,
		Build:  "npm run build -s -- --logLevel silent",
		Watch:  []string{".ts", ".tsx", ".js", ".jsx", ".vue", ".css", ".html", "package.json", "package-lock.json"},
		Ignore: []string{"_data", "node_modules", "dist"},
	}
}

// projectRelativeDevPath keeps generated lifecycle paths recognizable as
// project-relative values rather than commands relative to the current shell.
func projectRelativeDevPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || strings.HasPrefix(value, "./") {
		return value
	}
	return "./" + value
}

// migrateGeneratedDevWatchers converts only historical watcher shapes emitted
// by GoForj, leaving hand-authored and modified watcher entries untouched.
func migrateGeneratedDevWatchers(config *project.Config) bool {
	if config == nil {
		return false
	}
	if config.Dev.UsesStructuredApps() {
		return false
	}

	buildIndexes := generatedWatcherIndexes(config.Dev.Watches, isGeneratedLegacyBuildWatcher)
	runIndexes := generatedWatcherIndexes(config.Dev.Watches, isGeneratedLegacyRunWatcher)
	if len(buildIndexes) != 1 || len(runIndexes) != 1 {
		return false
	}
	if !legacyDevRunMapCanMigrate(config.Dev.Run) {
		return false
	}

	runCommands := config.Dev.Run
	if runCommands == nil {
		runCommands = map[string]string{project.DefaultAppName: "run"}
	}
	if config.Dev.Apps == nil {
		config.Dev.Apps = map[string]project.DevApp{}
	}
	for name, command := range runCommands {
		name = strings.TrimSpace(name)
		if name == "" || !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			continue
		}
		if _, exists := config.Dev.Apps[name]; exists {
			continue
		}
		config.Dev.Apps[name] = generatedDevAppConfig(config, project.DefaultNamedApp(name), command)
	}
	remove := map[int]bool{
		buildIndexes[0]: true,
		runIndexes[0]:   true,
	}
	defaultDevApp := config.Dev.Apps[project.DefaultAppName]
	if _, ownsFrontend := defaultDevApp.SPAs[generatedFrontendSPAName]; ownsFrontend {
		npmIndexes := generatedWatcherIndexes(config.Dev.Watches, isGeneratedLegacyNPMWatcher)
		if len(npmIndexes) == 1 {
			remove[npmIndexes[0]] = true
		}
	}
	config.Dev.Watches = devWatchesWithoutIndexes(config.Dev.Watches, remove)
	config.Dev.Run = nil
	return true
}

// legacyDevRunMapCanMigrate avoids discarding malformed allowlist entries that could carry project-specific meaning.
func legacyDevRunMapCanMigrate(runCommands map[string]string) bool {
	for name, command := range runCommands {
		name = strings.TrimSpace(name)
		if name == "" || !project.IsSafeAppName(name) || project.IsReservedAppName(name) || strings.TrimSpace(command) == "" {
			return false
		}
	}
	return true
}

// generatedWatcherIndexes finds exact framework-owned watcher candidates while
// requiring uniqueness at the migration call site.
func generatedWatcherIndexes(watches []project.DevWatch, matches func(project.DevWatch) bool) []int {
	indexes := make([]int, 0, 1)
	for index, watch := range watches {
		if matches(watch) {
			indexes = append(indexes, index)
		}
	}
	return indexes
}

// devWatchesWithoutIndexes removes migrated framework entries without changing
// the order of custom watchers around them.
func devWatchesWithoutIndexes(watches []project.DevWatch, remove map[int]bool) []project.DevWatch {
	kept := make([]project.DevWatch, 0, len(watches)-len(remove))
	for index, watch := range watches {
		if remove[index] {
			continue
		}
		kept = append(kept, watch)
	}
	return kept
}

// isGeneratedLegacyBuildWatcher recognizes unmodified build watcher forms that
// shipped in generated GoForj project files.
func isGeneratedLegacyBuildWatcher(watch project.DevWatch) bool {
	if watch.Name != "Build App" || strings.TrimSpace(watch.Exec) != "forj build -o ./bin/app" || !legacyWatcherHasNoOverrides(watch) {
		return false
	}
	_, known := map[string]bool{
		"-file .go -file .env -file .env.* -xdir forj -xdir _data -postpone":                                                                     true,
		"-file .go -file .env -file .env.* -xdir forj -xdir _data -xfile wire/wire_gen\\.go$ -postpone":                                          true,
		"-file .go -file .env -file .env.* -xdir forj -xdir _data -xfile app/wire/wire_gen\\.go$ -postpone":                                      true,
		"-file .go -file .templ -file .env -file .env.* -xdir forj -xdir _data -xfile app/wire/wire_gen\\.go$ -xfile '.*_templ\\.go$' -postpone": true,
	}[normalizedLegacyWatch(watch.Watch)]
	return known
}

// isGeneratedLegacyRunWatcher recognizes the app runtime watcher emitted before
// native app lifecycle configuration was available.
func isGeneratedLegacyRunWatcher(watch project.DevWatch) bool {
	return watch.Name == "Run App" &&
		strings.TrimSpace(watch.Exec) == "./bin/app run" &&
		normalizedLegacyWatch(watch.Watch) == "-file ./bin/app -file .env -file .env.*" &&
		legacyWatcherHasNoOverrides(watch)
}

// isGeneratedLegacyNPMWatcher recognizes only historical frontend watchers
// whose source path and command were chosen by the generator.
func isGeneratedLegacyNPMWatcher(watch project.DevWatch) bool {
	if watch.Name != "NPM" || strings.TrimSpace(watch.Exec) != "npm run dev" || !legacyWatcherHasNoOverrides(watch) {
		return false
	}
	_, known := map[string]bool{
		"-cd ./frontend -xdir _data -xdir .":                                       true,
		"-cd ./cmd/app/frontend -xdir _data -xdir .":                               true,
		"-cd ./cmd/app/frontend -xdir _data -xdir node_modules -xdir dist":         true,
		"-cd ./cmd/app/frontend -xdir _data -xdir . -xdir node_modules -xdir dist": true,
	}[normalizedLegacyWatch(watch.Watch)]
	return known
}

// legacyWatcherHasNoOverrides prevents migration from discarding controls that
// a user added to an otherwise familiar framework watcher.
func legacyWatcherHasNoOverrides(watch project.DevWatch) bool {
	return watch.Watch != "" &&
		len(watch.Include) == 0 &&
		len(watch.Ignore) == 0 &&
		len(watch.Roots) == 0 &&
		watch.WorkDir == "" &&
		watch.Files.Empty() &&
		watch.Dirs.Empty() &&
		len(watch.Env) == 0 &&
		watch.Debounce == "" &&
		watch.Poll == "" &&
		!watch.Postpone &&
		!watch.Restart &&
		!watch.Exit &&
		!watch.Stdin
}

// normalizedLegacyWatch removes layout-only whitespace differences before
// comparing a raw watcher with the generator's historical output.
func normalizedLegacyWatch(watch string) string {
	return strings.Join(strings.Fields(watch), " ")
}
