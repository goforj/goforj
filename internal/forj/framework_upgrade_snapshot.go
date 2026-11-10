package forj

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"golang.org/x/mod/module"
	"gopkg.in/yaml.v3"
)

// frameworkRenderInventory derives artifact ownership from the same write boundaries used by ordinary rendering.
type frameworkRenderInventory struct {
	artifacts map[string]projectupgrade.Artifact
}

// record preserves template provenance while classifying initial frontend sources as app-owned seeds.
func (inventory *frameworkRenderInventory) record(destination, template string, content []byte, ownership string, mode fs.FileMode) {
	destination = filepath.ToSlash(destination)
	if strings.HasPrefix(template, "starter-kits/") || strings.Contains(destination, "/frontend/") {
		ownership = "seed"
	}
	if strings.HasPrefix(destination, "migrations/") && isMigrationSQLFile(destination) {
		ownership = "migration"
	}
	inventory.artifacts[destination] = projectupgrade.Artifact{Path: destination, Ownership: ownership, Template: template, Mode: uint32(mode.Perm())}
}

// frameworkUpgradeConfig projects supported render intent while stripping secrets, commands, local replacements, and owner extras.
func frameworkUpgradeConfig(root string) (*project.Config, error) {
	workspace, err := resolveProjectRenderWorkspace(root)
	if err != nil {
		return nil, err
	}
	if err := workspace.rejectEnvironmentSpecialFile(".goforj.yml"); err != nil {
		return nil, err
	}
	original, err := workspace.loadProjectConfig()
	if err != nil {
		return nil, err
	}
	if err := module.CheckPath(original.GoModuleName); err != nil {
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
	discovery, err := projectlayout.Discover(root)
	if err != nil {
		return nil, err
	}
	for _, app := range discovery.NamedApps() {
		if _, exists := config.Apps[app.Name]; !exists {
			config.Apps[app.Name] = project.AppConfig{Components: appRenderComponents(original, app)}
		}
	}
	return config, nil
}

// writeFrameworkProjection creates only conventional discovery markers and neutral environment layers in a disposable workspace.
func writeFrameworkProjection(root string, config *project.Config) error {
	data, err := yaml.Marshal(config)
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
	for name := range config.Apps {
		if name == project.DefaultAppName {
			continue
		}
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
	inputs, err := json.Marshal(config)
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
	resources, err := project.DefaultResourcePlan(project.ProjectComponents(config))
	if err != nil {
		return projectupgrade.Snapshot{}, err
	}
	inventory := &frameworkRenderInventory{artifacts: map[string]projectupgrade.Artifact{}}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.inspection = inventory
	if err := renderer.Render(ComponentRenderInput{root: staging, renderAll: true, initializeProject: true, resourcePlan: resources}); err != nil {
		return projectupgrade.Snapshot{}, err
	}
	snapshot := projectupgrade.Snapshot{Protocol: projectupgrade.ProtocolVersion, Scope: "framework", InputsDigest: projectupgrade.Digest(inputs),
		Engine: projectupgrade.Engine{Version: version.Version, Commit: version.Commit, Dirty: version.Dirty}, Artifacts: []projectupgrade.Artifact{}}
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
		case name == "go.mod" || name == ".goforj.yml" || name == ".gitignore" || name == ".db-relationships.yaml":
			artifact.Ownership = "semantic"
		case !known && strings.Contains(name, "/frontend/"):
			artifact.Ownership = "seed"
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
	for _, name := range frameworkDerivedPaths(config) {
		snapshot.Artifacts = append(snapshot.Artifacts, projectupgrade.Artifact{Path: name, Ownership: "app-derived"})
	}
	sort.Slice(snapshot.Artifacts, func(i, j int) bool { return snapshot.Artifacts[i].Path < snapshot.Artifacts[j].Path })
	return snapshot, projectupgrade.ValidateSnapshot(snapshot)
}

// frameworkDerivedPaths lists actual generator contracts independently of a particular component's runtime implementation.
func frameworkDerivedPaths(config *project.Config) []string {
	paths := []string{"app/wire/wire_gen.go", "internal/database/connections_gen.go", "cmd/app/frontend/dist"}
	for name := range config.Apps {
		if name != project.DefaultAppName {
			paths = append(paths, "app/"+name+"/wire/wire_gen.go")
			paths = append(paths, "cmd/"+name+"/frontend/dist")
		}
	}
	for _, resource := range []string{"caches", "queues", "storages", "events", "mail"} {
		for _, output := range []string{"manager_gen.go", "accessors_gen.go"} {
			paths = append(paths, "internal/"+resource+"/"+output)
		}
	}
	return paths
}
