package forj

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// eventSubscriberOwnerMigration describes one unambiguous legacy owner move.
type eventSubscriberOwnerMigration struct {
	source string
	target string
}

// jobsOwnerMigration describes the preserved default-App Jobs injector move from the former top-level Wire package.
type jobsOwnerMigration struct {
	source string
	target string
}

// migrateAppOwnedWireFilenames preserves user-owned injector contents while adopting clearer app/wire names.
func (p *ProjectRenderer) migrateAppOwnedWireFilenames() error {
	eventMigrations, err := p.workspace.planEventSubscriberOwnerMigrations(p.config)
	if err != nil {
		return err
	}
	jobsMigration, err := p.workspace.planJobsOwnerMigration(p.config)
	if err != nil {
		return err
	}
	if err := p.workspace.applyEventSubscriberOwnerMigrations(eventMigrations); err != nil {
		return err
	}
	if err := p.workspace.applyJobsOwnerMigration(jobsMigration); err != nil {
		return err
	}
	if err := p.workspace.migrateLegacyCacheShellCommandOwners(); err != nil {
		return err
	}
	if err := p.workspace.repairLegacyEventSubscriberOwnerSetNames(p.config); err != nil {
		return err
	}
	return p.workspace.migratePreservedFile(
		filepath.Join("app", "wire", "inject_controllers_app.go"),
		filepath.Join("app", "wire", "inject_http_controllers_app.go"),
	)
}

// migrateLegacyCacheShellCommandOwners repairs preserved commands inside one project workspace.
func (w projectRenderWorkspace) migrateLegacyCacheShellCommandOwners() error {
	for _, app := range projectlayout.ConventionalApps(w.discoveryRoot()) {
		path := filepath.Join(projectlayout.AppDir(".", app), "commands.go")
		source, err := w.readFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		updated, changed, err := removeLegacyCacheShellCommandSource(path, source)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := w.writeFileAtomically(path, updated, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// removeLegacyCacheShellCommandSource removes the former generated Cache command wiring without depending on gofmt alignment.
func removeLegacyCacheShellCommandSource(path string, source []byte) ([]byte, bool, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		return nil, false, fmt.Errorf("parse legacy Cache command owner %s: %w", path, err)
	}
	cacheShellSelectors := 0
	ast.Inspect(file, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if ok && selectorExpressionMatches(selector, "cmd", "CacheShellCmd") {
			cacheShellSelectors++
		}
		return true
	})
	if cacheShellSelectors == 0 {
		return source, false, nil
	}
	if cacheShellSelectors != 2 {
		return nil, false, fmt.Errorf("cannot migrate legacy Cache command owner %s because its CacheShellCmd wiring was customized or is incomplete; move CacheShellCmd out of the App-owned Commands type manually", path)
	}

	targets := []*regexp.Regexp{
		regexp.MustCompile(`^[ \t]*CacheShellCmd[ \t]+cmd\.CacheShellCmd[ \t]+` + "`cmd:\"\"`" + `[ \t\r]*$`),
		regexp.MustCompile(`^[ \t]*cacheShellCmd[ \t]+\*cmd\.CacheShellCmd,[ \t\r]*$`),
		regexp.MustCompile(`^[ \t]*CacheShellCmd:[ \t]*\*cacheShellCmd,[ \t\r]*$`),
	}
	found := make([]int, len(targets))
	lines := strings.SplitAfter(string(source), "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		candidate := strings.TrimSuffix(line, "\n")
		matched := false
		for index, target := range targets {
			if !target.MatchString(candidate) {
				continue
			}
			found[index]++
			matched = true
			break
		}
		if matched {
			continue
		}
		filtered = append(filtered, line)
	}
	for _, count := range found {
		if count != 1 {
			return nil, false, fmt.Errorf("cannot migrate legacy Cache command owner %s because its CacheShellCmd wiring was customized or is incomplete; move CacheShellCmd out of the App-owned Commands type manually", path)
		}
	}
	updated := []byte(strings.Join(filtered, ""))
	if _, err := parser.ParseFile(token.NewFileSet(), path, updated, 0); err != nil {
		return nil, false, fmt.Errorf("validate migrated Cache command owner %s: %w", path, err)
	}
	return updated, true, nil
}

// planJobsOwnerMigration resolves legacy owner paths inside one project workspace.
func (w projectRenderWorkspace) planJobsOwnerMigration(config *project.Config) (*jobsOwnerMigration, error) {
	if config == nil || !appRenderComponents(config, project.DefaultApp()).Jobs {
		return nil, nil
	}
	source := filepath.Join("wire", "inject_jobs_app.go")
	target := filepath.Join(projectlayout.WireDir(".", project.DefaultApp()), "inject_jobs_app.go")
	sourceExists, err := w.renderPathExists(source)
	if err != nil {
		return nil, err
	}
	if !sourceExists {
		return nil, nil
	}
	targetExists, err := w.renderPathExists(target)
	if err != nil {
		return nil, err
	}
	if targetExists {
		return nil, fmt.Errorf("cannot migrate legacy Jobs owner %s to %s because both exist; reconcile the App-owned files manually", source, target)
	}
	return &jobsOwnerMigration{source: source, target: target}, nil
}

// applyJobsOwnerMigration moves one preserved Jobs owner inside a project workspace.
func (w projectRenderWorkspace) applyJobsOwnerMigration(migration *jobsOwnerMigration) error {
	if migration == nil {
		return nil
	}
	if err := w.ensureDir(filepath.Dir(migration.target)); err != nil {
		return fmt.Errorf("prepare Jobs owner destination %s: %w", migration.target, err)
	}
	targetExists, err := w.renderPathExists(migration.target)
	if err != nil {
		return err
	}
	if targetExists {
		return fmt.Errorf("cannot migrate legacy Jobs owner %s to %s because the destination now exists; reconcile the App-owned files manually", migration.source, migration.target)
	}
	if err := w.move(migration.source, migration.target); err != nil {
		return fmt.Errorf("migrate legacy Jobs owner %s to %s: %w", migration.source, migration.target, err)
	}
	return nil
}

// eventSubscriberOwnerPath returns the current App-owned subscriber injector path.
func eventSubscriberOwnerPath(app project.App) string {
	return filepath.Join(projectlayout.WireDir(".", app), "inject_subscribers_app.go")
}

// legacyEventSubscriberOwnerPaths returns historical subscriber injector paths that belong to one App.
func legacyEventSubscriberOwnerPaths(app project.App) []string {
	app = projectlayout.NormalizeApp(app)
	paths := []string{filepath.Join(projectlayout.WireDir(".", app), "inject_event_subscribers.go")}
	if app.Name == project.DefaultAppName {
		paths = append(paths,
			filepath.Join("wire", "inject_event_subscribers.go"),
			filepath.Join("wire", "inject_subscribers_app.go"),
		)
	}
	return paths
}

// planEventSubscriberOwnerMigrations resolves preserved Events owners inside one project workspace.
func (w projectRenderWorkspace) planEventSubscriberOwnerMigrations(config *project.Config) ([]eventSubscriberOwnerMigration, error) {
	migrations := make([]eventSubscriberOwnerMigration, 0)
	for _, app := range projectlayout.RuntimeApps(w.discoveryRoot(), config) {
		if !appRenderComponents(config, app).Events {
			continue
		}
		target := eventSubscriberOwnerPath(app)
		sources := make([]string, 0)
		for _, source := range legacyEventSubscriberOwnerPaths(app) {
			exists, err := w.renderPathExists(source)
			if err != nil {
				return nil, err
			}
			if exists {
				sources = append(sources, source)
			}
		}
		if len(sources) == 0 {
			continue
		}
		targetExists, err := w.renderPathExists(target)
		if err != nil {
			return nil, err
		}
		if targetExists {
			return nil, fmt.Errorf("cannot migrate legacy Events subscriber owner %s to %s because both exist; reconcile the App-owned files manually", strings.Join(sources, ", "), target)
		}
		if len(sources) > 1 {
			return nil, fmt.Errorf("cannot migrate multiple legacy Events subscriber owners %s to %s; reconcile the App-owned files manually", strings.Join(sources, ", "), target)
		}
		if _, _, err := w.parsedRenderGoFile(sources[0]); err != nil {
			return nil, fmt.Errorf("parse legacy Events subscriber owner %s: %w", sources[0], err)
		}
		migrations = append(migrations, eventSubscriberOwnerMigration{source: sources[0], target: target})
	}
	return migrations, nil
}

// applyEventSubscriberOwnerMigrations moves preserved Events owners inside one project workspace.
func (w projectRenderWorkspace) applyEventSubscriberOwnerMigrations(migrations []eventSubscriberOwnerMigration) error {
	for _, migration := range migrations {
		if err := w.ensureDir(filepath.Dir(migration.target)); err != nil {
			return fmt.Errorf("prepare Events subscriber owner destination %s: %w", migration.target, err)
		}
		targetExists, err := w.renderPathExists(migration.target)
		if err != nil {
			return err
		}
		if targetExists {
			return fmt.Errorf("cannot migrate legacy Events subscriber owner %s to %s because the destination now exists; reconcile the App-owned files manually", migration.source, migration.target)
		}
		if err := w.move(migration.source, migration.target); err != nil {
			return fmt.Errorf("migrate legacy Events subscriber owner %s to %s: %w", migration.source, migration.target, err)
		}
	}
	return nil
}

// repairLegacyEventSubscriberOwnerSetNames updates preserved Events owners inside one project workspace.
func (w projectRenderWorkspace) repairLegacyEventSubscriberOwnerSetNames(config *project.Config) error {
	for _, app := range projectlayout.RuntimeApps(w.discoveryRoot(), config) {
		if !appRenderComponents(config, app).Events {
			continue
		}
		path := eventSubscriberOwnerPath(app)
		source, err := w.readFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read Events subscriber owner %s: %w", path, err)
		}
		updated, changed, err := renameLegacyEventSubscriberSetIdentifier(path, source)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := w.writeFileAtomically(path, updated, 0o644); err != nil {
			return fmt.Errorf("repair Events subscriber owner %s: %w", path, err)
		}
	}
	return nil
}

// renameLegacyEventSubscriberSetIdentifier rewrites bound identifiers while preserving owner formatting, comments, strings, and providers.
func renameLegacyEventSubscriberSetIdentifier(path string, source []byte) ([]byte, bool, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, source, parser.ParseComments)
	if err != nil {
		return nil, false, fmt.Errorf("parse Events subscriber owner %s: %w", path, err)
	}
	legacyObject := file.Scope.Lookup("eventSubscriberSet")
	if legacyObject == nil {
		return source, false, nil
	}
	if legacyObject.Kind != ast.Var {
		return nil, false, fmt.Errorf("cannot repair Events subscriber owner %s because eventSubscriberSet is not a package-level variable; reconcile the App-owned file manually", path)
	}
	if file.Scope.Lookup("appSubscriberSet") != nil {
		return nil, false, fmt.Errorf("cannot repair Events subscriber owner %s because both eventSubscriberSet and appSubscriberSet are declared; reconcile the App-owned file manually", path)
	}
	tokenFile := fileSet.File(file.Pos())
	if tokenFile == nil {
		return nil, false, fmt.Errorf("locate Events subscriber owner source positions for %s", path)
	}
	type sourceRange struct {
		start int
		end   int
	}
	ranges := make([]sourceRange, 0)
	ast.Inspect(file, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if !ok || identifier.Obj != legacyObject {
			return true
		}
		ranges = append(ranges, sourceRange{
			start: tokenFile.Offset(identifier.Pos()),
			end:   tokenFile.Offset(identifier.End()),
		})
		return true
	})
	if len(ranges) == 0 {
		return nil, false, fmt.Errorf("locate eventSubscriberSet declaration in Events subscriber owner %s", path)
	}
	sort.Slice(ranges, func(left int, right int) bool {
		return ranges[left].start < ranges[right].start
	})
	var updated bytes.Buffer
	cursor := 0
	for _, sourceRange := range ranges {
		if sourceRange.start < cursor || sourceRange.end > len(source) {
			return nil, false, fmt.Errorf("repair overlapping eventSubscriberSet identifiers in Events subscriber owner %s", path)
		}
		updated.Write(source[cursor:sourceRange.start])
		updated.WriteString("appSubscriberSet")
		cursor = sourceRange.end
	}
	updated.Write(source[cursor:])
	return updated.Bytes(), true, nil
}

// appOwnedWirePathsForApp lists render-once injectors that may need compatibility repairs.
func appOwnedWirePathsForApp(app project.App) []string {
	wireDir := app.WireDir
	if wireDir == "" {
		wireDir = projectlayout.WireDir(".", project.DefaultApp())
	}
	return []string{
		filepath.Join(wireDir, "inject_cmd_app.go"),
		filepath.Join(wireDir, "inject_http_controllers_app.go"),
		filepath.Join(wireDir, "inject_jobs_app.go"),
		filepath.Join(wireDir, "inject_repositories_app.go"),
		filepath.Join(wireDir, "inject_schedules_app.go"),
		filepath.Join(wireDir, "inject_services_app.go"),
		filepath.Join(wireDir, "inject_subscribers_app.go"),
	}
}

// migratePreservedFile renames one render-once owner inside a project workspace.
func (w projectRenderWorkspace) migratePreservedFile(oldPath string, newPath string) error {
	oldExists, err := w.exists(oldPath)
	if err != nil {
		return err
	}
	if !oldExists {
		return nil
	}
	if newExists, err := w.exists(newPath); err != nil {
		return err
	} else if newExists {
		return nil
	}
	return w.move(oldPath, newPath)
}
