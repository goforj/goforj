package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/forj/makeapp"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestPrimitiveRendererPreflight verifies unsafe primitive transitions fail before mutating project state.
func TestPrimitiveRendererPreflight(t *testing.T) {
	t.Run("residue", testPrimitiveResiduePreflight)
	t.Run("non-source artifacts", testPrimitiveNonSourceArtifacts)
	t.Run("App surface syntax", testPrimitiveAppSurfaceSyntax)
	t.Run("full render before writes", testJobsFullRenderBeforeWrites)
	t.Run("App-only removal before writes", testPrimitiveAppOnlyRemovalBeforeWrites)
}

// testPrimitiveResiduePreflight verifies every declared generated artifact blocks unsupported removal without mutation.
func testPrimitiveResiduePreflight(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			for _, residuePath := range primitiveResiduePaths(contract.key) {
				residuePath := residuePath
				t.Run(strings.ReplaceAll(residuePath, string(filepath.Separator), "_"), func(t *testing.T) {
					usePrimitiveRendererRoot(t)
					fixturePath := primitiveResidueFixturePath(residuePath)
					contents := "fixture\n"
					if filepath.Ext(fixturePath) == ".go" {
						contents = "package fixture\n"
					}
					writePrimitiveRendererFile(t, fixturePath, contents)

					components := primitiveRendererBaseComponents()
					renderer := projectRendererForTest(t, primitiveRendererConfig(components))
					err := validatePrimitiveTransition(renderer, contract.key, components)
					if err == nil || !strings.Contains(err.Error(), residuePath) {
						t.Fatalf("%s transition error = %v, want residue %s", contract.name, err, residuePath)
					}
					if got := readPrimitiveRendererFile(t, fixturePath); got != contents {
						t.Fatalf("%s preflight changed %s: %q", contract.name, fixturePath, got)
					}
				})
			}
		})
	}
}

// testPrimitiveNonSourceArtifacts keeps runtime data and empty historical shells outside removal ownership checks.
func testPrimitiveNonSourceArtifacts(t *testing.T) {
	tests := []struct {
		name        string
		key         project.ComponentKey
		directories []string
		files       map[string]string
	}{
		{
			name:  "Storage runtime data",
			key:   project.ComponentStorage,
			files: map[string]string{filepath.Join("storage", "app", "private", "upload.txt"): "owner data\n"},
		},
		{
			name: "Jobs runtime data and empty shells",
			key:  project.ComponentJobs,
			directories: []string{
				filepath.Join("internal", "jobs", "archive"),
				filepath.Join("internal", "queues", "stale", "nested"),
			},
			files: map[string]string{filepath.Join("_data", "queues", "default.db"): "runtime data\n"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			for _, path := range test.directories {
				if err := os.MkdirAll(path, 0o755); err != nil {
					t.Fatalf("create non-source directory %s: %v", path, err)
				}
			}
			for path, contents := range test.files {
				writePrimitiveRendererFile(t, path, contents)
			}
			components := primitiveRendererBaseComponents()
			renderer := projectRendererForTest(t, primitiveRendererConfig(components))
			if err := validatePrimitiveTransition(renderer, test.key, components); err != nil {
				t.Fatalf("%s non-source artifact blocked removal: %v", test.name, err)
			}
			for _, path := range test.directories {
				info, err := os.Stat(path)
				if err != nil || !info.IsDir() {
					t.Fatalf("%s preflight changed artifact directory %s: info=%v err=%v", test.name, path, info, err)
				}
			}
			for path, contents := range test.files {
				if got := readPrimitiveRendererFile(t, path); got != contents {
					t.Fatalf("non-source artifact %s changed: %q", path, got)
				}
			}
		})
	}
}

// testPrimitiveAppSurfaceSyntax verifies comments, strings, fields, free functions, and unrelated receivers cannot block removal.
func testPrimitiveAppSurfaceSyntax(t *testing.T) {
	tests := []struct {
		name   string
		key    project.ComponentKey
		source string
		want   bool
	}{
		{name: "Storage receiver", key: project.ComponentStorage, source: "package wire\ntype App struct{}\nfunc (a *App) Storage() any { return nil }\n", want: true},
		{name: "Storage free function", key: project.ComponentStorage, source: "package wire\ntype App struct{ Storage string }\nfunc Storage() any { return nil }\n"},
		{name: "Jobs Queue receiver", key: project.ComponentJobs, source: "package wire\ntype App struct{}\nfunc (a *App) Queue() any { return nil }\n", want: true},
		{name: "Jobs Queues receiver", key: project.ComponentJobs, source: "package wire\ntype App struct{}\nfunc (a App) Queues() any { return nil }\n", want: true},
		{
			name: "Jobs comments strings fields free functions and unrelated receivers",
			key:  project.ComponentJobs,
			source: `package wire

// func (a *App) Queue() any is retained as migration documentation only.
const migrationNote = "func (a *App) Queues()"

type App struct { Queue string }
type Worker struct{}

func Queue() any { return nil }
func (w *Worker) Queues() any { return nil }
`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "app.go")
			writePrimitiveRendererFile(t, path, test.source)
			workspace := currentProjectRenderWorkspace(t)
			var got bool
			var err error
			switch test.key {
			case project.ComponentStorage:
				got, err = workspace.appStorageSurfaceExists(path)
			case project.ComponentJobs:
				got, err = workspace.appJobsSurfaceExists(path)
			default:
				t.Fatalf("unsupported primitive App surface: %s", test.key)
			}
			if err != nil {
				t.Fatalf("inspect %s App surface: %v", test.name, err)
			}
			if got != test.want {
				t.Fatalf("%s App surface presence = %t, want %t", test.name, got, test.want)
			}
		})
	}
}

// testJobsFullRenderBeforeWrites verifies full-render Jobs preflight runs before any project mutation.
func testJobsFullRenderBeforeWrites(t *testing.T) {
	usePrimitiveRendererRoot(t)
	components := primitiveRendererBaseComponents()
	config := primitiveRendererConfig(components)
	config.ProjectName = "Jobs Removal"
	config.GoModuleName = "example.test/jobs-removal"
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	configBefore := readPrimitiveRendererFile(t, ".goforj.yml")
	environment := "OWNER_SENTINEL=unchanged\n"
	writePrimitiveRendererFile(t, ".env", environment)
	jobPath := filepath.Join("internal", "jobs", "custom_job.go")
	jobContents := "package jobs\n\nvar ownerSentinel = true\n"
	writePrimitiveRendererFile(t, jobPath, jobContents)
	legacyOwnerPath := filepath.Join("app", "wire", "inject_controllers_app.go")
	currentOwnerPath := filepath.Join("app", "wire", "inject_http_controllers_app.go")
	legacyOwnerContents := "package wire\n\nvar ownerSentinel = true\n"
	writePrimitiveRendererFile(t, legacyOwnerPath, legacyOwnerContents)

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	err := renderer.Render(ComponentRenderInput{renderAll: true})
	if err == nil || !strings.Contains(err.Error(), filepath.Join("internal", "jobs")) {
		t.Fatalf("full Jobs removal error = %v, want internal/jobs residue", err)
	}
	for path, want := range map[string]string{
		".goforj.yml":   configBefore,
		".env":          environment,
		jobPath:         jobContents,
		legacyOwnerPath: legacyOwnerContents,
	} {
		if got := readPrimitiveRendererFile(t, path); got != want {
			t.Fatalf("failed Jobs preflight changed %s", path)
		}
	}
	for _, path := range []string{currentOwnerPath, "go.mod"} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("failed Jobs preflight created %s: %v", path, statErr)
		}
	}
}

// testPrimitiveAppOnlyRemovalBeforeWrites verifies prospective App state is validated before config, environment, migration, or scaffold writes.
func testPrimitiveAppOnlyRemovalBeforeWrites(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("worker")
			components := primitiveRendererBaseComponents()
			appComponents := components
			appComponents.SetEnabled(contract.key, true)
			config := primitiveRendererConfig(components)
			config.ProjectName = contract.name + " Removal"
			config.GoModuleName = "example.test/primitive-removal"
			config.Apps = map[string]project.AppConfig{app.Name: {Components: appComponents}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			configBefore := readPrimitiveRendererFile(t, ".goforj.yml")
			environment := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n" + primitiveRootEnvironment(contract.key)
			writePrimitiveRendererFile(t, ".env", environment)
			surfacePath, surfaceSource := primitiveRemovalSurface(app, contract.key)
			writePrimitiveRendererFile(t, surfacePath, surfaceSource)
			legacyPath := filepath.Join("app", "wire", "inject_controllers_app.go")
			currentPath := filepath.Join("app", "wire", "inject_http_controllers_app.go")
			legacyContents := "package wire\n\nvar ownerSentinel = true\n"
			writePrimitiveRendererFile(t, legacyPath, legacyContents)

			renderer := NewProjectRenderer(logger.NewSilentLogger())
			err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: components, SkipWire: true})
			if err == nil || !strings.Contains(err.Error(), surfacePath) {
				t.Fatalf("%s removal error = %v, want %s", contract.name, err, surfacePath)
			}
			if got := readPrimitiveRendererFile(t, ".goforj.yml"); got != configBefore {
				t.Fatalf("%s failed preflight rewrote project config", contract.name)
			}
			if got := readPrimitiveRendererFile(t, ".env"); got != environment {
				t.Fatalf("%s failed preflight rewrote environment: %q", contract.name, got)
			}
			if got := readPrimitiveRendererFile(t, legacyPath); got != legacyContents {
				t.Fatalf("%s failed preflight changed owner file", contract.name)
			}
			if _, err := os.Stat(currentPath); !os.IsNotExist(err) {
				t.Fatalf("%s failed preflight created migrated owner: %v", contract.name, err)
			}
			if _, err := os.Stat(app.Entrypoint); !os.IsNotExist(err) {
				t.Fatalf("%s failed preflight rendered entrypoint: %v", contract.name, err)
			}
		})
	}
}
