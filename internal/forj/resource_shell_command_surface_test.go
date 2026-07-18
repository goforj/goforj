package forj

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestResourceShellCommandWiringFollowsResourceCapabilities prevents infrastructure providers from owning public commands.
func TestResourceShellCommandWiringFollowsResourceCapabilities(t *testing.T) {
	tests := []struct {
		name       string
		components project.Components
		wantDB     bool
		wantCache  bool
	}{
		{name: "CLI only", components: project.Components{CLI: true}},
		{name: "database", components: project.Components{CLI: true, DatabaseSQLite: true}, wantDB: true},
		{name: "cache", components: project.Components{CLI: true, Cache: true}, wantCache: true},
		{name: "database and cache", components: project.Components{CLI: true, Cache: true, DatabaseSQLite: true}, wantDB: true, wantCache: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := test.components.WithResolvedDependencies()
			config := &project.Config{
				GoModuleName: "example.com/resource-shells",
				Render:       project.RenderConfig{Components: components},
			}
			data := currentProjectRenderWorkspace(t).templateDataForApp(config, project.DefaultApp())
			data.Components = components
			data.ProjectComponents = components
			data.HelpFormatterFunc = "FrameworkFormatter"

			root := renderSharedTemplate(t, "app/root_cmd.go.tmpl", data)
			wire := renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data)
			assertFormattedGoTemplate(t, "app/root_cmd.go.tmpl", root)
			assertFormattedGoTemplate(t, "wire/inject_cmd.go.tmpl", wire)
			assertTemplateMarker(t, "app/root_cmd.go.tmpl", root, "DBShellCmd", test.wantDB)
			assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", wire, "cmd.NewDBShellCmd", test.wantDB)
			assertTemplateMarker(t, "app/root_cmd.go.tmpl", root, "CacheShellCmd", test.wantCache)
			assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", wire, "cmd.NewCacheShellCmd", test.wantCache)
			for _, source := range []string{root, wire} {
				if strings.Contains(source, "RedisShellCmd") || strings.Contains(source, "redis:shell") {
					t.Fatalf("generated command surface exposed provider-specific Redis shell wiring:\n%s", source)
				}
			}
		})
	}
}

// TestResourceShellTemplateInventoryExcludesProviderCommands keeps driver names behind resource-oriented shells.
func TestResourceShellTemplateInventoryExcludesProviderCommands(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to resolve current file path")
	}
	templateRoot := filepath.Join(filepath.Dir(currentFile), "..", "..", "templates")
	err := filepath.WalkDir(templateRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, forbidden := range []string{`name:"redis:shell"`, "NewRedisShellCmd"} {
			if strings.Contains(string(contents), forbidden) {
				t.Errorf("provider-specific command marker %q found in %s", forbidden, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect generated template command surface: %v", err)
	}
}
