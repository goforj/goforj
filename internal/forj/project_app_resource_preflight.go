package forj

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/joho/godotenv"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// appResourceFamily ties inventory keys to established framework-owned manager packages.
type appResourceFamily struct {
	key  project.ResourceKey
	path string
	flag string
}

// appResourceFamilies lists the generated manager boundaries whose registrations App ownership can change.
func appResourceFamilies() []appResourceFamily {
	return []appResourceFamily{
		{project.ResourceFeature, "internal/features", "feature"},
		{project.ResourceSearch, "internal/searches", "search"},
		{project.ResourceNotification, "internal/notifications", "notification"},
		{project.ResourceCache, "internal/caches", "cache"},
		{project.ResourceDatabase, "internal/database", "db"},
		{project.ResourceQueue, "internal/queues", "queue"},
		{project.ResourceEvents, "internal/events", "events"},
		{project.ResourceStorage, "internal/storages", "storage"},
		{project.ResourceMail, "internal/mail", "mail"},
	}
}

// validateAppResourceOwnership compares candidate scopes without changing configuration or publishing environment migrations.
func (p *ProjectRenderer) validateAppResourceOwnership(app project.App, opts makeapp.RenderOptions) error {
	candidate := *p.config
	candidate.Apps = maps.Clone(p.config.Apps)
	prospective := ProjectRenderer{config: &candidate}
	if _, err := prospective.setAppConfig(app.Name, opts.Components, opts.StarterKit, opts.StarterKitOptions, opts.HelpFormat); err != nil {
		return err
	}
	values, err := p.appResourceEnvironmentValues()
	if err != nil {
		return err
	}
	return p.validateAppResourceInventory(app, &candidate, values, values, false)
}

// validateAppRemovalResourceOwnership rejects removal before cleanup can strand surviving resource consumers.
func (p *ProjectRenderer) validateAppRemovalResourceOwnership(app project.App, candidate *project.Config, preserve func(string) bool) error {
	beforeValues := map[string]string{}
	afterValues := map[string]string{}
	for _, name := range []string{".env.example", ".env"} {
		source, err := p.workspace.readFile(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect App removal resource environment %s: %w", name, err)
		}
		beforeLayer, err := godotenv.Unmarshal(string(source))
		if err != nil {
			return fmt.Errorf("parse App removal resource environment %s: %w", name, err)
		}
		afterLayer, err := godotenv.Unmarshal(appEnvironmentWithoutDefaults(string(source), app.Name, preserve))
		if err != nil {
			return fmt.Errorf("parse projected App removal resource environment %s: %w", name, err)
		}
		maps.Copy(beforeValues, beforeLayer)
		maps.Copy(afterValues, afterLayer)
	}
	return p.validateAppResourceInventory(app, candidate, beforeValues, afterValues, true)
}

// appResourceEnvironmentValues merges both owned dotenv layers without consulting ambient process values.
func (p *ProjectRenderer) appResourceEnvironmentValues() (map[string]string, error) {
	values := map[string]string{}
	for _, name := range []string{".env.example", ".env"} {
		source, err := p.workspace.readFile(name)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect App resource environment %s: %w", name, err)
		}
		layer, err := godotenv.Unmarshal(string(source))
		if err != nil {
			return nil, fmt.Errorf("parse App resource environment %s: %w", name, err)
		}
		maps.Copy(values, layer)
	}
	return values, nil
}

// validateAppResourceInventory protects surviving owner source from removed registrations and global accessors.
func (p *ProjectRenderer) validateAppResourceInventory(app project.App, candidate *project.Config, beforeValues, afterValues map[string]string, removing bool) error {
	before := generate.ResourceNamesByApp(p.workspace.root, beforeValues, p.config)
	var omittedApps []string
	if removing {
		omittedApps = []string{app.Name}
	}
	after := generate.ResourceNamesByApp(p.workspace.root, afterValues, candidate, omittedApps...)
	removedPrefix := project.AppEnvironmentPrefix(app.Name)
	if removing {
		for _, scopes := range after {
			delete(scopes, removedPrefix)
		}
	}
	for _, family := range appResourceFamilies() {
		var survivingUnion []string
		if removing {
			for _, registered := range after[family.key] {
				survivingUnion = append(survivingUnion, registered...)
			}
		}
		lost := ""
		for _, scope := range slices.Sorted(maps.Keys(before[family.key])) {
			names := before[family.key][scope]
			for _, name := range names {
				surviving := after[family.key][scope]
				if removing && scope == removedPrefix {
					surviving = survivingUnion
				}
				if !slices.Contains(surviving, name) {
					lost = name
					break
				}
			}
			if lost != "" {
				break
			}
		}
		if lost == "" {
			continue
		}
		established, err := p.establishedAppResourceFamily(family)
		if err != nil {
			return err
		}
		if !established {
			continue
		}
		owner, err := p.appResourceOwnerDependency(family, app, removing)
		if err != nil {
			return err
		}
		if owner != "" {
			operation := "make app"
			remedy := "reconcile resource names and owner source first, or choose another App name"
			if removing {
				operation = "remove app"
				remedy = "reconcile resource names and surviving owner source before removing the App"
			}
			return fmt.Errorf("%s %q would remove the established %s resource %q registration while owner source %s depends on %s; %s (the dependency check conservatively includes Default-only consumers)", operation, app.Name, family.key, lost, owner, family.path, remedy)
		}
	}
	return nil
}

// establishedAppResourceFamily avoids reserving package names that have never contained framework resource output.
func (p *ProjectRenderer) establishedAppResourceFamily(family appResourceFamily) (bool, error) {
	marker := "// Code generated by forj generate --" + family.flag + ". DO NOT EDIT."
	for _, name := range []string{"manager_gen.go", "accessors_gen.go", "connections_gen.go"} {
		source, err := p.workspace.readFile(filepath.Join(family.path, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		if appResourceSourceMarker(source, marker) {
			return true, nil
		}
	}
	return false, nil
}

// appResourceSourceMarker accepts exact ownership lines in the leading comment header, including build-constrained source.
func appResourceSourceMarker(source []byte, marker string) bool {
	for _, line := range strings.Split(string(source), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == marker {
			return true
		}
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "//") {
			return false
		}
	}
	return false
}

// appResourceOwnerDependency follows package imports so split files, wrappers, interfaces, and dynamic manager calls share one conservative boundary.
func (p *ProjectRenderer) appResourceOwnerDependency(family appResourceFamily, app project.App, removing bool) (string, error) {
	manager := filepath.FromSlash(family.path)
	imports := map[string][]string{}
	owners := map[string][]string{}
	directOwners := map[string]bool{}
	framework := p.cacheReconciledGoPaths(project.ProjectComponents(p.config))
	wireOutputs := map[string]bool{}
	for _, knownApp := range projectlayout.RuntimeApps(p.workspace.discoveryRoot(), p.config) {
		wireOutputs[filepath.Clean(filepath.Join(projectlayout.WireDir(".", knownApp), "wire_gen.go"))] = true
	}
	for _, path := range generate.OutputPaths(generate.GenerationSelectionFromComponents(project.ProjectComponents(p.config))) {
		framework[filepath.Clean(path)] = true
	}
	search := p.cacheOwnerDependencySearch(nil, nil)
	err := filepath.WalkDir(p.workspace.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		logical := filepath.Clean(p.workspace.logicalLabel(path))
		if removing {
			if cacheTransitionPathContains(projectlayout.AppDir(".", app), logical) || cacheTransitionPathContains(projectlayout.FrontendDir(".", app), logical) {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if logical == filepath.Clean(projectlayout.Entrypoint(".", app)) {
				return nil
			}
		}
		if entry.IsDir() {
			if logical != "." && cacheTransitionSkippedDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), logical, source, 0)
		if err != nil {
			return fmt.Errorf("parse App resource owner source %s: %w", logical, err)
		}
		directory := filepath.Dir(logical)
		// Known marked framework assembly is a dependency bridge, while markerless or unknown files remain owner origins.
		frameworkOwned := false
		if framework[logical] {
			frameworkOwned = appResourceSourceMarker(source, "// Code generated by GoForj CLI. DO NOT EDIT.") || appResourceSourceMarker(source, "// Code generated by Wire. DO NOT EDIT.")
			for _, family := range appResourceFamilies() {
				frameworkOwned = frameworkOwned || appResourceSourceMarker(source, "// Code generated by forj generate --"+family.flag+". DO NOT EDIT.")
			}
		}
		harness := filepath.Base(logical) == "wire.go" && framework[logical] && appResourceSourceMarker(source, "// GoForj Wire harness. Edit this file when customizing root assembly.")
		frameworkOwned = frameworkOwned || wireOutputs[logical] && appResourceSourceMarker(source, "// Code generated by Wire. DO NOT EDIT.")
		if !frameworkOwned {
			owners[logical] = nil
			if directory == manager {
				directOwners[logical] = true
			}
			if strings.HasPrefix(filepath.ToSlash(directory), "app/") && filepath.Base(directory) == "wire" {
				ast.Inspect(file, func(node ast.Node) bool {
					if identifier, ok := node.(*ast.Ident); ok && identifier.Name == "App" && !harness {
						directOwners[logical] = true
					}
					if selector, ok := node.(*ast.SelectorExpr); ok && (appResourceGetterMatches(family.key, selector.Sel.Name) || appResourcePrivateFieldMatches(family.key, selector.Sel.Name)) {
						directOwners[logical] = true
					}
					return true
				})
			}
		}
		for _, imported := range file.Imports {
			value, ok := cacheImportPath(imported.Path.Value)
			if !ok {
				continue
			}
			if local, ok := search.localImportDirectory(value); ok {
				if appResourceWireDirectory(local) && appResourceWireLaunchOnly(file, imported) {
					continue
				}
				imports[directory] = append(imports[directory], local)
				if !frameworkOwned {
					owners[logical] = append(owners[logical], local)
				}
			}
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("inspect App resource owner dependencies: %w", err)
	}
	dependent := map[string]bool{filepath.FromSlash(manager): true}
	for changed := true; changed; {
		changed = false
		for directory, dependencies := range imports {
			if dependent[directory] {
				continue
			}
			for _, dependency := range dependencies {
				if dependent[dependency] {
					dependent[directory] = true
					changed = true
					break
				}
			}
		}
	}
	for _, source := range slices.Sorted(maps.Keys(owners)) {
		if directOwners[source] {
			return source, nil
		}
		for _, dependency := range owners[source] {
			if dependent[dependency] {
				return source, nil
			}
		}
	}
	return "", nil
}

// appResourceGetterMatches identifies the public App boundaries for same-package owner extensions.
func appResourceGetterMatches(key project.ResourceKey, name string) bool {
	switch key {
	case project.ResourceSearch:
		return name == "Searches"
	case project.ResourceNotification:
		return name == "Notifications"
	case project.ResourceFeature:
		return name == "Features"
	case project.ResourceCache:
		return name == "Cache" || name == "Caches"
	case project.ResourceDatabase:
		return name == "DB"
	case project.ResourceQueue:
		return name == "Queue" || name == "Queues"
	case project.ResourceEvents:
		return name == "Events" || name == "Bus"
	case project.ResourceStorage:
		return name == "Storage"
	case project.ResourceMail:
		return name == "Mail"
	default:
		return false
	}
}

// appResourcePrivateFieldMatches protects owner extensions that access App managers within the Wire package.
func appResourcePrivateFieldMatches(key project.ResourceKey, name string) bool {
	switch key {
	case project.ResourceSearch:
		return name == "search"
	case project.ResourceNotification:
		return name == "notifications"
	case project.ResourceFeature:
		return name == "features"
	case project.ResourceCache:
		return name == "cache" || name == "caches"
	case project.ResourceDatabase:
		return name == "db"
	case project.ResourceQueue:
		return name == "queues"
	case project.ResourceEvents:
		return name == "events"
	case project.ResourceStorage:
		return name == "storage"
	case project.ResourceMail:
		return name == "mail"
	default:
		return false
	}
}

// appResourceWireDirectory recognizes the generated App boundary without treating launch-only imports as resource consumption.
func appResourceWireDirectory(directory string) bool {
	return strings.HasPrefix(filepath.ToSlash(directory), "app/") && filepath.Base(directory) == "wire"
}

// appResourceWireLaunchOnly exempts the generated launcher, while factories and opaque App uses remain conservative dependencies.
func appResourceWireLaunchOnly(file *ast.File, imported *ast.ImportSpec) bool {
	name := "wire"
	if imported.Name != nil {
		name = imported.Name.Name
	}
	if name == "." || name == "_" {
		return false
	}
	references := 0
	launches := 0
	ast.Inspect(file, func(node ast.Node) bool {
		if _, ok := node.(*ast.ImportSpec); ok {
			return false
		}
		if selector, ok := node.(*ast.SelectorExpr); ok {
			if receiver, ok := selector.X.(*ast.Ident); ok && receiver.Name == name && selector.Sel.Name == "LaunchApplication" {
				launches++
			}
		}
		if identifier, ok := node.(*ast.Ident); ok && identifier.Name == name {
			references++
		}
		return true
	})
	return launches > 0 && references == launches
}
