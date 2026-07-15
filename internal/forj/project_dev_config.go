package forj

import (
	"path/filepath"
	"strings"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

const (
	generatedFrontendSPAName           = "frontend"
	generatedFrontendNPMInstallCommand = "npm install --no-audit --no-fund --loglevel=error"
	legacyFrontendNPMInstallCommand    = "npm install"
)

// generatedDevFrontendInstallTask keeps dependency setup aligned with each app-owned SPA root.
func generatedDevFrontendInstallTask(app project.App) project.DevTask {
	return devFrontendInstallTask(app, generatedFrontendNPMInstallCommand)
}

// legacyGeneratedDevFrontendInstallTask identifies only the install task emitted before quiet npm setup became the default.
func legacyGeneratedDevFrontendInstallTask(app project.App) project.DevTask {
	return devFrontendInstallTask(app, legacyFrontendNPMInstallCommand)
}

// devFrontendInstallTask builds the app-scoped task identity shared by generation and conservative migration.
func devFrontendInstallTask(app project.App, installCommand string) project.DevTask {
	app = projectlayout.NormalizeApp(app)
	name := "Install Frontend Dependencies"
	if app.Name != project.DefaultAppName {
		name = "Install " + app.Name + " Frontend Dependencies"
	}
	return project.DevTask{
		Name: name,
		Cmd:  "cd " + filepath.ToSlash(projectlayout.FrontendDir(".", app)) + " && " + installCommand,
	}
}

// removeGeneratedDevFrontendInstallTask removes only the exact framework task owned by a deleted app.
func removeGeneratedDevFrontendInstallTask(tasks []project.DevTask, app project.App) ([]project.DevTask, bool) {
	want := generatedDevFrontendInstallTask(app)
	legacy := legacyGeneratedDevFrontendInstallTask(app)
	kept := make([]project.DevTask, 0, len(tasks))
	removed := false
	for _, task := range tasks {
		if task == want || task == legacy {
			removed = true
			continue
		}
		kept = append(kept, task)
	}
	return kept, removed
}

// migrateGeneratedDevFrontendInstallTask upgrades only the exact legacy task while collapsing generated duplicates.
func migrateGeneratedDevFrontendInstallTask(tasks []project.DevTask, app project.App) ([]project.DevTask, bool) {
	want := generatedDevFrontendInstallTask(app)
	legacy := legacyGeneratedDevFrontendInstallTask(app)
	hasReplacement := false
	for _, task := range tasks {
		if task == want || task != legacy && strings.TrimSpace(task.Name) == want.Name {
			hasReplacement = true
			break
		}
	}

	migrated := make([]project.DevTask, 0, len(tasks))
	changed := false
	for _, task := range tasks {
		if task != legacy {
			migrated = append(migrated, task)
			continue
		}
		changed = true
		if !hasReplacement {
			migrated = append(migrated, want)
			hasReplacement = true
		}
	}
	if !changed {
		return tasks, false
	}
	return migrated, true
}

// hasDevFrontendInstallTask reports whether an App's generated task identity is already present, including an owner-customized command.
func hasDevFrontendInstallTask(tasks []project.DevTask, app project.App) bool {
	want := generatedDevFrontendInstallTask(app)
	for _, task := range tasks {
		if strings.TrimSpace(task.Name) == want.Name {
			return true
		}
	}
	return false
}

// migrateGeneratedDevFrontendInstallTasks upgrades framework tasks for the default App and every configured named App.
func migrateGeneratedDevFrontendInstallTasks(config *project.Config) bool {
	if config == nil {
		return false
	}
	apps := []project.App{project.DefaultApp()}
	seen := map[string]bool{project.DefaultAppName: true}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if seen[name] || !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			return
		}
		seen[name] = true
		apps = append(apps, project.DefaultNamedApp(name))
	}
	for name := range config.Apps {
		add(name)
	}
	for name := range config.Dev.Apps {
		add(name)
	}

	changed := false
	for _, app := range apps {
		tasks, migrated := migrateGeneratedDevFrontendInstallTask(config.Dev.Pre, app)
		if !migrated {
			continue
		}
		config.Dev.Pre = tasks
		changed = true
	}
	return changed
}

// generatedDevAppConfig snapshots framework-owned lifecycle behavior into readable project configuration.
func generatedDevAppConfig(config *project.Config, app project.App, runCommand string) project.DevApp {
	app = projectlayout.NormalizeApp(app)
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
		generatedFrontendSPAName: conventionalDevSPAConfig(projectRelativeDevPath(projectlayout.FrontendDir(".", app))),
	}
	return configured
}

// conventionalDevAppBuildCommand returns the editable build snapshot generated for a managed app.
func conventionalDevAppBuildCommand(config *project.Config, app project.App) project.DevAppCommand {
	app = projectlayout.NormalizeApp(app)
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
	app = projectlayout.NormalizeApp(app)
	return project.DevAppCommand{Exec: projectlayout.RuntimeExecutable(".", app)}
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
	config.Dev.Apps = make(map[string]project.DevApp)
	for _, app := range projectlayout.ConventionalApps(".") {
		command, run := legacyDevRunCommandForMigration(runCommands, app.Name)
		configured := generatedDevAppConfig(config, app, command)
		if !run {
			// Legacy Build App discovery was independent of dev.run, so an omitted
			// runtime must remain an explicit build-only participant after migration.
			configured.Run = &project.DevAppCommand{Disabled: true}
		}
		config.Dev.Apps[app.Name] = configured
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

// legacyDevRunCommandForMigration preserves the absent pre-allowlist model while treating a present map as the exact runtime allowlist.
func legacyDevRunCommandForMigration(runCommands map[string]string, appName string) (string, bool) {
	if runCommands == nil {
		return "run", true
	}
	command, ok := runCommands[appName]
	if !ok {
		return "", false
	}
	return strings.TrimSpace(command), true
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
		len(watch.Extra) == 0 &&
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
