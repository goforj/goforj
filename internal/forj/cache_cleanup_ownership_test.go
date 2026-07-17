package forj

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/project"
)

// TestValidateCacheRenderTransitionRejectsUnmarkedCleanupArtifact verifies owner edits cannot be mistaken for disposable Cache output.
func TestValidateCacheRenderTransitionRejectsUnmarkedCleanupArtifact(t *testing.T) {
	t.Chdir(t.TempDir())
	path := filepath.Join("internal", "cmd", "cache_shell_cmd.go")
	writeCacheCleanupFixture(t, path, "package cmd\n\nfunc customCacheCommand() {}\n")

	err := currentProjectRenderWorkspace(t).validateCacheRenderTransition(project.Components{})
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("validate Cache transition error = %v, want ownership error for %s", err, path)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read preserved Cache artifact: %v", readErr)
	}
	if !strings.Contains(string(contents), "customCacheCommand") {
		t.Fatalf("Cache validation changed owner artifact %s", path)
	}
}

// TestCleanupDisabledCacheGeneratedFilesRequiresAndRemovesMarkers verifies cleanup is independently safe and removes only marked output.
func TestCleanupDisabledCacheGeneratedFilesRequiresAndRemovesMarkers(t *testing.T) {
	t.Chdir(t.TempDir())
	artifacts := cacheGeneratedCleanupArtifacts()
	for _, artifact := range artifacts {
		writeCacheCleanupFixture(t, artifact.path, artifact.marker+"\n")
	}

	if err := currentProjectRenderWorkspace(t).cleanupDisabledCacheGeneratedFiles(); err != nil {
		t.Fatalf("clean marked Cache artifacts: %v", err)
	}
	for _, artifact := range artifacts {
		if _, err := os.Stat(artifact.path); !os.IsNotExist(err) {
			t.Fatalf("generated Cache artifact %s still exists: %v", artifact.path, err)
		}
	}
}

// TestCacheOwnerEnvironmentPathsDiscoversOnlyRootFiles verifies custom overlays cannot widen preflight into unrelated filesystem surfaces.
func TestCacheOwnerEnvironmentPathsDiscoversOnlyRootFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, path := range []string{".env", ".env.production", ".env.qa", ".env.qa.local", "..env.example-1234", ".envrc"} {
		writeCacheCleanupFixture(t, path, "CACHE_REPORTS_DRIVER=redis\n")
	}
	for _, path := range []string{
		filepath.Join(".env.directory", "owner"),
		filepath.Join("forj_render_1234", ".env.qa"),
		filepath.Join("nested", ".env.qa"),
		filepath.Join("vendor", "dependency", ".env.qa"),
	} {
		writeCacheCleanupFixture(t, path, "CACHE_NESTED_DRIVER=redis\n")
	}

	paths, err := currentProjectRenderWorkspace(t).cacheOwnerEnvironmentPaths()
	if err != nil {
		t.Fatalf("discover Cache owner environment files: %v", err)
	}
	want := []string{".env", ".env.production", ".env.qa", ".env.qa.local"}
	if !slices.Equal(paths, want) {
		t.Fatalf("Cache owner environment paths = %v, want %v", paths, want)
	}
}

// TestCacheIgnoredAppEnvironmentAssignmentRequiresAResourceBoundary verifies colliding App prefixes cannot hide shared Cache assignments.
func TestCacheIgnoredAppEnvironmentAssignmentRequiresAResourceBoundary(t *testing.T) {
	ignoredApps := map[string]bool{"cache": true, "cache-reader": true}
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "shared root", key: "CACHE_DRIVER"},
		{name: "shared named resource matching App prefix", key: "CACHE_READER_DRIVER"},
		{name: "shared named resource", key: "CACHE_SESSIONS_DRIVER"},
		{name: "resource-named App Cache overlay", key: "CACHE_CACHE_DRIVER", want: true},
		{name: "resource-prefixed App Cache overlay", key: "CACHE_READER_CACHE_REPORTS_DRIVER", want: true},
		{name: "resource-named App Events overlay", key: "CACHE_EVENTS_DRIVER", want: true},
		{name: "resource-prefixed App Storage overlay", key: "CACHE_READER_STORAGE_ARCHIVE_DRIVER", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := cacheIgnoredAppEnvironmentAssignment(test.key, ignoredApps); got != test.want {
				t.Fatalf("cacheIgnoredAppEnvironmentAssignment(%q) = %t, want %t", test.key, got, test.want)
			}
		})
	}
}

// TestCacheOwnerEnvironmentDependencyKeepsSharedCacheAcrossAppPrefixCollision prevents last-Cache cleanup from erasing owner configuration that merely shares an App prefix.
func TestCacheOwnerEnvironmentDependencyKeepsSharedCacheAcrossAppPrefixCollision(t *testing.T) {
	t.Chdir(t.TempDir())
	app := project.DefaultNamedApp("cache")
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			app.Name: {Components: project.Components{CLI: true, Cache: true}},
		},
	}
	writeCacheCleanupFixture(t, ".env", strings.Join([]string{
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory",
		"CACHE_REPORTS_DRIVER=redis",
		"",
		"# Cache",
		"CACHE_CACHE_DRIVER=memory",
		"",
	}, "\n"))

	path, key, err := projectRendererForTest(t, config).cacheOwnerEnvironmentDependency(
		project.Components{CLI: true},
		nil,
		[]project.App{app},
	)
	if err != nil {
		t.Fatalf("inspect Cache owner environment: %v", err)
	}
	if path != ".env" || key != "CACHE_REPORTS_DRIVER" {
		t.Fatalf("Cache owner environment dependency = %q, %q, want .env, CACHE_REPORTS_DRIVER", path, key)
	}
}

// TestNamedAppCacheDeselectionRemovesOnlyGeneratedDefaults verifies a named App can drop Cache without retaining framework environment residue.
func TestNamedAppCacheDeselectionRemovesOnlyGeneratedDefaults(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	enabled := project.Components{CLI: true, Cache: true}
	config := &project.Config{
		ProjectName:  "Named Cache Deselection",
		GoModuleName: "example.test/named-cache-deselection",
		Render:       project.RenderConfig{Components: enabled},
		Apps:         map[string]project.AppConfig{app.Name: {Components: enabled}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write Cache-enabled project config: %v", err)
	}
	environment := strings.Join([]string{
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory",
		"OWNER_SENTINEL=keep",
		"",
		"# Worker",
		"WORKER_CACHE_DRIVER=memory",
		"WORKER_OWNER_VALUE=keep",
		"",
	}, "\n")
	for _, path := range []string{".env", ".env.example"} {
		writePrimitiveRendererFile(t, path, environment)
	}

	renderer := unitProjectRenderer(t)
	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: project.Components{CLI: true}, SkipWire: true}); err != nil {
		t.Fatalf("disable Cache for named App: %v", err)
	}
	for _, path := range []string{".env", ".env.example"} {
		updated := readPrimitiveRendererFile(t, path)
		if strings.Contains(updated, "WORKER_CACHE_DRIVER=") {
			t.Fatalf("Cache deselection retained generated App default in %s:\n%s", path, updated)
		}
		for _, want := range []string{"OWNER_SENTINEL=keep", "WORKER_OWNER_VALUE=keep"} {
			if !strings.Contains(updated, want) {
				t.Fatalf("Cache deselection removed owner assignment %q from %s:\n%s", want, path, updated)
			}
		}
	}
	loaded, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("reload Cache-disabled App config: %v", err)
	}
	if loaded.Apps[app.Name].Components.Cache {
		t.Fatalf("named App still enables Cache: %#v", loaded.Apps[app.Name].Components)
	}
}

// TestNamedAppCacheDeselectionRejectsOwnerDependencies verifies every refusal happens before project files are mutated.
func TestNamedAppCacheDeselectionRejectsOwnerDependencies(t *testing.T) {
	tests := []struct {
		name        string
		ownerPath   string
		ownerSource string
		example     string
		want        string
	}{
		{
			name:        "App source import",
			ownerPath:   filepath.Join("app", "worker", "wire", "inject_services_app.go"),
			ownerSource: "package wire\n\nimport \"example.test/cache-owner/internal/caches\"\n\nvar ownerCache *caches.Manager\n",
			want:        "inject_services_app.go",
		},
		{
			name:        "loose App driver",
			ownerSource: "WORKER_CACHE_DRIVER=redis\n",
			want:        "WORKER_CACHE_DRIVER",
		},
		{
			name:    "named App cache",
			example: "# Worker\nWORKER_CACHE_REPORTS_DRIVER=redis\n",
			want:    "WORKER_CACHE_REPORTS_DRIVER",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("worker")
			enabled := project.Components{CLI: true, Cache: true}
			config := &project.Config{
				ProjectName:  "Cache Owner Preflight",
				GoModuleName: "example.test/cache-owner",
				Render:       project.RenderConfig{Components: enabled},
				Apps:         map[string]project.AppConfig{app.Name: {Components: enabled}},
			}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write Cache owner config: %v", err)
			}
			environment := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n"
			if test.ownerPath == "" {
				environment += test.ownerSource
			}
			environment += "\n# Worker\nWORKER_CACHE_DRIVER=memory\n"
			writePrimitiveRendererFile(t, ".env", environment)
			if test.example != "" {
				writePrimitiveRendererFile(t, ".env.example", test.example)
			}
			if test.ownerPath != "" {
				writePrimitiveRendererFile(t, test.ownerPath, test.ownerSource)
			}
			configBefore := readPrimitiveRendererFile(t, ".goforj.yml")
			environmentBefore := readPrimitiveRendererFile(t, ".env")

			renderer := unitProjectRenderer(t)
			err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: project.Components{CLI: true}, SkipWire: true})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Cache owner preflight error = %v, want %q", err, test.want)
			}
			if got := readPrimitiveRendererFile(t, ".goforj.yml"); got != configBefore {
				t.Fatal("rejected Cache transition rewrote project config")
			}
			if got := readPrimitiveRendererFile(t, ".env"); got != environmentBefore {
				t.Fatal("rejected Cache transition rewrote owner environment")
			}
			if test.ownerPath != "" && readPrimitiveRendererFile(t, test.ownerPath) != test.ownerSource {
				t.Fatalf("rejected Cache transition rewrote owner source %s", test.ownerPath)
			}
		})
	}
}

// TestLastCacheRemovalRejectsProjectOwnerDependencies verifies shared source and resource configuration block destructive cleanup.
func TestLastCacheRemovalRejectsProjectOwnerDependencies(t *testing.T) {
	tests := []struct {
		name        string
		ownerPath   string
		ownerSource string
		envExtra    string
		overlayPath string
		overlay     string
		want        string
	}{
		{
			name:        "shared source",
			ownerPath:   filepath.Join("internal", "billing", "owner.go"),
			ownerSource: "package billing\n\nimport \"example.test/cache-owner/app/wire\"\n\n// useCache proves owner code still calls the generated App Cache accessor.\nfunc useCache(app *wire.App) { _ = app.Cache() }\n",
			want:        filepath.Join("internal", "billing", "owner.go"),
		},
		{
			name:     "named cache configuration",
			envExtra: "CACHE_REPORTS_DRIVER=redis\n",
			want:     "CACHE_REPORTS_DRIVER",
		},
		{
			name:        "custom environment overlay",
			overlayPath: ".env.qa",
			overlay:     "CACHE_REPORTS_DRIVER=redis\n",
			want:        ".env.qa",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("worker")
			config := &project.Config{
				ProjectName:  "Last Cache Owner",
				GoModuleName: "example.test/cache-owner",
				Render:       project.RenderConfig{Components: project.Components{CLI: true}},
				Apps: map[string]project.AppConfig{
					app.Name: {Components: project.Components{CLI: true, Cache: true}},
				},
			}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write final Cache owner config: %v", err)
			}
			writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
			environment := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n" + test.envExtra + "\n# Worker\nWORKER_CACHE_DRIVER=memory\n"
			writePrimitiveRendererFile(t, ".env", environment)
			if test.overlayPath != "" {
				writePrimitiveRendererFile(t, test.overlayPath, test.overlay)
			}
			if test.ownerPath != "" {
				writePrimitiveRendererFile(t, test.ownerPath, test.ownerSource)
			}
			for _, artifact := range cacheGeneratedCleanupArtifacts() {
				writePrimitiveRendererFile(t, artifact.path, artifact.marker+"\n")
			}
			configBefore := readPrimitiveRendererFile(t, ".goforj.yml")

			result, err := unitProjectRenderer(t).RemoveApp(app)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("final Cache owner preflight error = %v, want %q", err, test.want)
			}
			if result.Changed() {
				t.Fatalf("rejected final Cache removal reported changes: %#v", result)
			}
			if got := readPrimitiveRendererFile(t, ".goforj.yml"); got != configBefore {
				t.Fatal("rejected final Cache removal rewrote project config")
			}
			if got := readPrimitiveRendererFile(t, ".env"); got != environment {
				t.Fatal("rejected final Cache removal rewrote owner environment")
			}
			if test.overlayPath != "" && readPrimitiveRendererFile(t, test.overlayPath) != test.overlay {
				t.Fatalf("rejected final Cache removal rewrote owner environment %s", test.overlayPath)
			}
			if _, err := os.Stat(app.Entrypoint); err != nil {
				t.Fatalf("rejected final Cache removal changed App source: %v", err)
			}
		})
	}
}

// writeCacheCleanupFixture writes one ownership fixture beneath the isolated test root.
func writeCacheCleanupFixture(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create Cache fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write Cache fixture %s: %v", path, err)
	}
}
