package forj

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestValidateStorageRenderTransitionRejectsProjectResidue verifies every generated project artifact blocks an unsupported Storage removal.
func TestValidateStorageRenderTransitionRejectsProjectResidue(t *testing.T) {
	for _, residuePath := range projectStorageResiduePaths() {
		residuePath := residuePath
		t.Run(strings.ReplaceAll(residuePath, string(filepath.Separator), "_"), func(t *testing.T) {
			useStorageRendererRoot(t)
			fixturePath := residuePath
			if residuePath == filepath.Join("internal", "storages") {
				fixturePath = filepath.Join(residuePath, "manager_gen.go")
			}
			contents := "fixture\n"
			if filepath.Ext(fixturePath) == ".go" {
				contents = "package fixture\n"
			}
			writeStorageRendererFile(t, fixturePath, contents)

			components := project.Components{CLI: true}
			renderer := &ProjectRenderer{config: storageRendererConfig(components)}
			err := renderer.validateStorageRenderTransition(components)
			if err == nil || !strings.Contains(err.Error(), residuePath) {
				t.Fatalf("transition error = %v, want residue path %s", err, residuePath)
			}
			if _, statErr := os.Stat(fixturePath); statErr != nil {
				t.Fatalf("Storage preflight changed residue %s: %v", fixturePath, statErr)
			}
		})
	}
}

// TestValidateStorageRenderTransitionIgnoresRuntimeStorageData verifies application files are not mistaken for removable generated code.
func TestValidateStorageRenderTransitionIgnoresRuntimeStorageData(t *testing.T) {
	useStorageRendererRoot(t)
	runtimePath := filepath.Join("storage", "app", "private", "upload.txt")
	writeStorageRendererFile(t, runtimePath, "owner data\n")

	components := project.Components{CLI: true}
	renderer := &ProjectRenderer{config: storageRendererConfig(components)}
	if err := renderer.validateStorageRenderTransition(components); err != nil {
		t.Fatalf("runtime Storage data blocked component opt-out: %v", err)
	}
	if contents, err := os.ReadFile(runtimePath); err != nil {
		t.Fatalf("read preserved runtime Storage data: %v", err)
	} else if string(contents) != "owner data\n" {
		t.Fatalf("runtime Storage data changed: %q", contents)
	}
}

// TestAppStorageSurfaceExistsUsesAccessorSyntax verifies only a receiver method named Storage proves the generated App API exists.
func TestAppStorageSurfaceExistsUsesAccessorSyntax(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     bool
	}{
		{
			name: "generated accessor",
			contents: `package wire

type App struct{}

func (a *App) Storage() any { return nil }
`,
			want: true,
		},
		{
			name: "comments fields strings and free functions",
			contents: `package wire

// func (a *App) Storage() any is retained as migration documentation only.
const migrationNote = "func (a *App) Storage()"

type App struct {
	Storage string
}

func Storage() any { return nil }
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.go")
			writeStorageRendererFile(t, path, test.contents)
			got, err := appStorageSurfaceExists(path)
			if err != nil {
				t.Fatalf("inspect App Storage surface: %v", err)
			}
			if got != test.want {
				t.Fatalf("Storage surface presence = %t, want %t", got, test.want)
			}
		})
	}
}

// TestRenderAppOnlyRejectsStorageRemovalBeforeWrites verifies prospective App removal fails before configuration, environment, owner, or migration writes.
func TestRenderAppOnlyRejectsStorageRemovalBeforeWrites(t *testing.T) {
	useStorageRendererRoot(t)
	app := project.DefaultNamedApp("storage-worker")
	config := storageRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Storage Transition"
	config.GoModuleName = "example.test/storage-transition"
	config.Apps = map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true, Storage: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore := readStorageRendererFile(t, ".goforj.yml")

	appSurfacePath := filepath.Join(app.WireDir, "app.go")
	writeStorageRendererFile(t, appSurfacePath, `package storageworkerapp

type App struct{}

func (a *App) Storage() any { return nil }
`)
	legacyOwnerPath := filepath.Join("app", "wire", "inject_controllers_app.go")
	currentOwnerPath := filepath.Join("app", "wire", "inject_http_controllers_app.go")
	legacyOwnerContents := "package wire\n\nvar ownerSentinel = true\n"
	writeStorageRendererFile(t, legacyOwnerPath, legacyOwnerContents)
	writeStorageRendererFile(t, ".env", "OWNER_SENTINEL=unchanged\n")

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	err := renderer.RenderAppOnly(app, makeapp.RenderOptions{
		Components: project.Components{CLI: true},
		SkipWire:   true,
	})
	if err == nil || !strings.Contains(err.Error(), appSurfacePath) {
		t.Fatalf("Storage removal error = %v, want generated App API path %s", err, appSurfacePath)
	}
	if got := readStorageRendererFile(t, ".goforj.yml"); got != configBefore {
		t.Fatalf("failed Storage preflight rewrote project config\nbefore:\n%s\nafter:\n%s", configBefore, got)
	}
	if got := readStorageRendererFile(t, ".env"); got != "OWNER_SENTINEL=unchanged\n" {
		t.Fatalf("failed Storage preflight rewrote environment: %q", got)
	}
	if got := readStorageRendererFile(t, legacyOwnerPath); got != legacyOwnerContents {
		t.Fatalf("failed Storage preflight changed legacy owner %s", legacyOwnerPath)
	}
	if _, statErr := os.Stat(currentOwnerPath); !os.IsNotExist(statErr) {
		t.Fatalf("failed Storage preflight created migrated owner %s: %v", currentOwnerPath, statErr)
	}
	if _, statErr := os.Stat(app.Entrypoint); !os.IsNotExist(statErr) {
		t.Fatalf("failed Storage preflight rendered App entrypoint %s: %v", app.Entrypoint, statErr)
	}
}

// TestRemoveAppRejectsLastStorageOwnerBeforeWrites verifies App removal cannot strand a shared Storage surface after deleting owner files.
func TestRemoveAppRejectsLastStorageOwnerBeforeWrites(t *testing.T) {
	useStorageRendererRoot(t)
	app := project.DefaultNamedApp("files")
	config := storageRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Storage App Removal"
	config.GoModuleName = "example.test/storage-app-removal"
	config.Apps = map[string]project.AppConfig{
		app.Name: {Components: project.Components{CLI: true, Storage: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore := readStorageRendererFile(t, ".goforj.yml")
	appSentinel := filepath.Join(app.AppDir, "owner.go")
	writeStorageRendererFile(t, appSentinel, "package filesapp\n\nvar ownerSentinel = true\n")
	writeStorageRendererFile(t, app.Entrypoint, "package main\n")
	writeStorageRendererFile(t, ".env", "FILES_STORAGE_DRIVER=local\n")
	writeStorageRendererFile(t, filepath.Join("internal", "storages", "manager_gen.go"), "package storages\n")

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	result, err := renderer.RemoveApp(app)
	if err == nil || !strings.Contains(err.Error(), "last App using Storage") || !strings.Contains(err.Error(), filepath.Join("internal", "storages")) {
		t.Fatalf("remove last Storage App error = %v", err)
	}
	if result.Changed() {
		t.Fatalf("failed Storage App removal reported changes: %#v", result)
	}
	for _, path := range []string{appSentinel, app.Entrypoint, ".env", filepath.Join("internal", "storages", "manager_gen.go")} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("failed Storage App removal changed %s: %v", path, statErr)
		}
	}
	if got := readStorageRendererFile(t, ".goforj.yml"); got != configBefore {
		t.Fatalf("failed Storage App removal rewrote project config\nbefore:\n%s\nafter:\n%s", configBefore, got)
	}
}

// TestRenderAppOnlyAddsStorageWithoutChangingAppOwnedFiles verifies an App can opt in while a sibling keeps shared Storage support available.
func TestRenderAppOnlyAddsStorageWithoutChangingAppOwnedFiles(t *testing.T) {
	useStorageRendererRoot(t)
	app := project.DefaultNamedApp("api")
	config := storageRendererConfig(project.Components{CLI: true})
	config.ProjectName = "Additive Storage"
	config.GoModuleName = "example.test/additive-storage"
	config.Apps = map[string]project.AppConfig{
		app.Name:         {Components: project.Components{CLI: true}},
		"storage-worker": {Components: project.Components{CLI: true, Storage: true}},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	writeStorageRendererFile(t, ".env", "STORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local,memory\n")

	initialRenderer := NewProjectRenderer(logger.NewSilentLogger())
	initialRenderer.config = config
	if err := initialRenderer.renderApp(app); err != nil {
		t.Fatalf("render Storage-disabled App fixture: %v", err)
	}
	ownerPaths := make([]string, 0)
	for _, mapping := range initialRenderer.appOwnedMappings(app) {
		ownerPaths = append(ownerPaths, mapping.dest)
		contents := readStorageRendererFile(t, mapping.dest)
		writeStorageRendererFile(t, mapping.dest, contents+"\n// OwnerSentinel proves additive renders preserve this file.\n")
	}
	wantHashes := storageRendererFileHashes(t, ownerPaths)

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{
		Components: project.Components{CLI: true, Storage: true},
		SkipWire:   true,
	}); err != nil {
		t.Fatalf("add Storage to existing App: %v", err)
	}
	gotHashes := storageRendererFileHashes(t, ownerPaths)
	for _, path := range ownerPaths {
		if gotHashes[path] != wantHashes[path] {
			t.Fatalf("additive Storage render changed App-owned file %s", path)
		}
	}
	if exists, err := appStorageSurfaceExists(filepath.Join(app.WireDir, "app.go")); err != nil {
		t.Fatalf("inspect additive App Storage surface: %v", err)
	} else if !exists {
		t.Fatal("additive render did not generate the App Storage accessor")
	}
	loaded, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("reload additive Storage config: %v", err)
	}
	if !loaded.Apps[app.Name].Components.Storage {
		t.Fatalf("additive render did not persist App Storage participation: %#v", loaded.Apps[app.Name].Components)
	}
}

// TestStorageDriverDefaultFromEnvValidatesOwnerContract prevents incremental Apps from copying an invalid project Storage selection.
func TestStorageDriverDefaultFromEnvValidatesOwnerContract(t *testing.T) {
	tests := []struct {
		name        string
		environment string
		want        string
		wantError   string
	}{
		{name: "missing environment uses local", want: "local"},
		{name: "valid cloud driver", environment: "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local,s3\n", want: "s3"},
		{name: "unknown active driver", environment: "STORAGE_DRIVER=unknown\nSTORAGE_SUPPORTED_DRIVERS=unknown\n", wantError: "unsupported driver"},
		{name: "active driver excluded", environment: "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local\n", wantError: "excludes active"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if test.environment != "" {
				writeStorageRendererFile(t, path, test.environment)
			}
			got, err := storageDriverDefaultFromEnv(path)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("Storage driver error = %v, want %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve Storage driver: %v", err)
			}
			if got != test.want {
				t.Fatalf("Storage driver = %q, want %q", got, test.want)
			}
		})
	}
}

// storageRendererConfig returns an explicit-contract fixture so omitted primitive components remain disabled.
func storageRendererConfig(components project.Components) *project.Config {
	return &project.Config{
		Render: project.RenderConfig{
			Components:               components,
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
	}
}

// useStorageRendererRoot changes into an isolated render directory for the duration of one test.
func useStorageRendererRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to Storage render directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return root
}

// writeStorageRendererFile writes one isolated fixture with its conventional parent directories.
func writeStorageRendererFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// readStorageRendererFile reads one ownership-sensitive fixture.
func readStorageRendererFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(contents)
}

// storageRendererFileHashes captures byte-level identities for App-owned files before an additive render.
func storageRendererFileHashes(t *testing.T, paths []string) map[string][sha256.Size]byte {
	t.Helper()
	hashes := make(map[string][sha256.Size]byte, len(paths))
	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read App-owned file %s: %v", path, err)
		}
		hashes[path] = sha256.Sum256(contents)
	}
	return hashes
}
