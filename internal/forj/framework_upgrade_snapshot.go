package forj

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
)

// frameworkRenderInventory derives artifact ownership from the same write boundaries used by ordinary rendering.
type frameworkRenderInventory struct {
	artifacts map[string]projectupgrade.Artifact
}

// record preserves template provenance while classifying initial frontend sources as app-owned seeds.
func (inventory *frameworkRenderInventory) record(destination, template string, ownership string, mode fs.FileMode) {
	destination = filepath.ToSlash(destination)
	if strings.HasPrefix(template, "starter-kits/") || strings.HasPrefix(template, "demo/frontend/") || strings.HasPrefix(template, "gopher_eats/") {
		ownership = "seed"
	}
	if strings.HasPrefix(destination, "migrations/") && isMigrationSQLFile(destination) {
		ownership = "migration"
	}
	inventory.artifacts[destination] = projectupgrade.Artifact{Path: destination, Ownership: ownership, Template: template, Mode: uint32(mode.Perm())}
}

// frameworkRenderIntent keeps filesystem participation separate from explicitly configured App selections.
type frameworkRenderIntent struct {
	*project.Config
	DiscoveredApps []string
}

// namedApps includes generator participants without converting inherited Apps into explicit selections.
func (intent *frameworkRenderIntent) namedApps() []string {
	pending := make([]project.App, 0, len(intent.DiscoveredApps))
	for _, name := range intent.DiscoveredApps {
		pending = append(pending, project.AppForName(name))
	}
	apps := (projectlayout.Discovery{}).RuntimeApps(intent.Config, pending...)
	names := make([]string, 0, len(apps)-1)
	for _, app := range apps {
		if app.Name != project.DefaultAppName {
			names = append(names, app.Name)
		}
	}
	return names
}

// frameworkUpgradeConfig projects supported render intent while stripping secrets, commands, local replacements, and owner extras.
func frameworkUpgradeConfig(root string) (*frameworkRenderIntent, error) {
	source, err := projectupgrade.ReadInput(root, ".goforj.yml")
	if err != nil {
		return nil, err
	}
	original := &project.Config{}
	if err := yaml.Unmarshal(source, original); err != nil {
		return nil, fmt.Errorf("decode .goforj.yml: %w", err)
	}
	if original.GoModuleName != strings.TrimSpace(original.GoModuleName) {
		return nil, fmt.Errorf("module_name must not contain surrounding whitespace")
	}
	if err := validateNewProjectModulePath(original.GoModuleName); err != nil {
		return nil, fmt.Errorf("framework inspection requires a valid module_name: %w", err)
	}
	config := &project.Config{ProjectName: original.ProjectName, GoModuleName: original.GoModuleName,
		Render: project.RenderConfig{Components: original.Render.Components, StarterKit: original.Render.StarterKit,
			StarterKitOptions: original.Render.StarterKitOptions, HelpFormat: original.Render.HelpFormat, AgentGuidance: project.AgentGuidanceNone},
		Apps: map[string]project.AppConfig{},
	}
	for name, app := range original.Apps {
		if !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
			return nil, fmt.Errorf("unsupported App name %q", name)
		}
		config.Apps[name] = project.AppConfig{Components: app.Components, StarterKit: app.StarterKit, StarterKitOptions: app.StarterKitOptions, HelpFormat: app.HelpFormat}
	}
	filesystem, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer filesystem.Close()
	discovery, err := projectlayout.DiscoverFS(projectupgrade.InspectionFS(filesystem))
	if err != nil {
		return nil, err
	}
	intent := &frameworkRenderIntent{Config: config, DiscoveredApps: []string{}}
	for _, app := range discovery.NamedApps() {
		intent.DiscoveredApps = append(intent.DiscoveredApps, app.Name)
	}
	return intent, nil
}

// writeFrameworkProjection creates only conventional discovery markers and neutral environment layers in a disposable workspace.
func writeFrameworkProjection(root string, config *frameworkRenderIntent) error {
	data, err := yaml.Marshal(config.Config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".goforj.yml"), data, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+config.GoModuleName+"\n"), 0o600); err != nil {
		return err
	}
	for _, name := range []string{".env", ".env.host"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("APP_ENV=local\n"), 0o600); err != nil {
			return err
		}
	}
	for _, name := range config.DiscoveredApps {
		app := project.AppForName(name)
		for _, marker := range []string{app.Entrypoint, filepath.Join(app.AppDir, "root_cmd.go")} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(root, marker)), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(root, marker), []byte("package main\n"), 0o600); err != nil {
				return err
			}
		}
	}
	return nil
}

// frameworkUpgradeSnapshot executes the authoritative scaffold phase in isolation and stops before dependency, generator, and Wire tools.
func frameworkUpgradeSnapshot(root string) (projectupgrade.Snapshot, error) {
	config, err := frameworkUpgradeConfig(root)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	return frameworkSnapshotForConfig(config)
}

// frameworkSnapshotForConfig freezes one projected input recipe for all participating renderers.
func frameworkSnapshotForConfig(config *frameworkRenderIntent) (projectupgrade.Snapshot, error) {
	inputsDigest, err := frameworkInputsDigest(config)
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	staging, err := os.MkdirTemp("", "goforj-framework-snapshot-")
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	defer os.RemoveAll(staging)
	if err := writeFrameworkProjection(staging, config); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	// Keep the declared recipe intact for peer renderers and input verification.
	effective := *config.Config
	normalizeRenderComponents(&effective.Render)
	resources, err := project.DefaultResourcePlan(project.ProjectComponents(&effective))
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	inventory := &frameworkRenderInventory{artifacts: map[string]projectupgrade.Artifact{}}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.inspection = inventory
	if err := renderer.Render(ComponentRenderInput{root: staging, renderAll: true, initializeProject: true, resourcePlan: resources}); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	snapshot := projectupgrade.Snapshot{Protocol: projectupgrade.ProtocolVersion, Scope: "framework", InputsDigest: inputsDigest,
		Engine: projectupgrade.Engine{Version: version.Version, Commit: version.Commit, Dirty: version.Dirty}, Artifacts: []projectupgrade.Artifact{}}
	derivedPaths := frameworkDerivedPaths(&frameworkRenderIntent{Config: &effective, DiscoveredApps: config.DiscoveredApps})
	derived := map[string]bool{}
	for _, name := range derivedPaths {
		derived[name] = true
	}
	err = filepath.WalkDir(staging, func(physical string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(staging, physical)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		if derived[name] {
			return nil
		}
		if strings.Contains(name, "/frontend/dist/") {
			return nil
		}
		artifact, known := inventory.artifacts[name]
		if !known && strings.HasPrefix(name, "migrations/") {
			artifact, known = inventory.artifacts["migrations/"+filepath.Base(name)]
		}
		artifact.Path = name
		switch {
		case strings.HasPrefix(filepath.Base(name), ".env"):
			artifact.Ownership = "private"
		case name == "go.mod" || name == "go.sum" || name == ".goforj.yml" || name == ".gitignore" || name == ".db-relationships.yaml":
			artifact.Ownership = "semantic"
		case !known:
			artifact.Ownership = "generated-context"
		}
		if artifact.Ownership == "framework-template" || artifact.Ownership == "framework-asset" || artifact.Ownership == "migration" {
			content, err := os.ReadFile(physical)
			if err != nil {
				return err
			}
			artifact.Digest = projectupgrade.Digest(content)
			if artifact.Ownership == "framework-template" || artifact.Ownership == "migration" {
				artifact.Source = string(content)
			}
		}
		snapshot.Artifacts = append(snapshot.Artifacts, artifact)
		return nil
	})
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	snapshot.Artifacts = append(snapshot.Artifacts, projectupgrade.Artifact{Path: "go.sum", Ownership: "semantic"})
	for _, name := range derivedPaths {
		ownership := "app-derived"
		if name == generate.ObservabilityTargetsPath {
			ownership = "conditional-derived"
		}
		snapshot.Artifacts = append(snapshot.Artifacts, projectupgrade.Artifact{Path: name, Ownership: ownership})
	}
	for _, artifact := range append([]projectupgrade.Artifact(nil), snapshot.Artifacts...) {
		if strings.HasPrefix(artifact.Template, "starter-kits/templ-htmx/") && strings.HasSuffix(artifact.Path, ".templ") {
			snapshot.Artifacts = append(snapshot.Artifacts, projectupgrade.Artifact{Path: strings.TrimSuffix(artifact.Path, ".templ") + "_templ.go", Ownership: "app-derived"})
		}
	}
	sort.Slice(snapshot.Artifacts, func(i, j int) bool { return snapshot.Artifacts[i].Path < snapshot.Artifacts[j].Path })
	return snapshot, projectupgrade.ValidateSnapshot(snapshot)
}

// frameworkDerivedPaths lists actual generator contracts independently of a particular component's runtime implementation.
func frameworkDerivedPaths(config *frameworkRenderIntent) []string {
	components := project.ProjectComponents(config.Config)
	paths := generate.OutputPaths(generate.GenerationSelection{Cache: components.Cache, Queue: components.Jobs, Storage: components.Storage,
		Events: components.Events, Mail: components.Mail, Database: components.HasDatabase(), Observability: components.Observability})
	paths = append(paths, "app/wire/wire_gen.go")
	if appRenderComponents(config.Config, project.DefaultApp()).WebAPI || appRenderComponents(config.Config, project.DefaultApp()).WebUI {
		paths = append(paths, apiindex.OutputPaths(project.DefaultApp())...)
	}
	if appRenderComponents(config.Config, project.DefaultApp()).WebUI {
		paths = append(paths, "cmd/app/frontend/dist")
	}
	for _, name := range config.namedApps() {
		if name != project.DefaultAppName {
			paths = append(paths, "app/"+name+"/wire/wire_gen.go")
			app := project.AppForName(name)
			if appRenderComponents(config.Config, app).WebAPI || appRenderComponents(config.Config, app).WebUI {
				paths = append(paths, apiindex.OutputPaths(app)...)
			}
			if appRenderComponents(config.Config, project.AppForName(name)).WebUI {
				paths = append(paths, "cmd/"+name+"/frontend/dist")
			}
		}
	}
	return paths
}
