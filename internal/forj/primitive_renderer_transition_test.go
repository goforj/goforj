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

// primitiveRendererContract describes the owner boundary and environment defaults for one optional primitive.
type primitiveRendererContract struct {
	name        string
	fallback    string
	validDriver string
	validEnv    string
	excludedEnv string
}

// TestPrimitiveRendererTransitions covers residue rejection, owner preservation, additive enablement, and safe removal for every primitive.
func TestPrimitiveRendererTransitions(t *testing.T) {
	t.Run("residue preflight", testPrimitiveResiduePreflight)
	t.Run("non-source artifacts", testPrimitiveNonSourceArtifacts)
	t.Run("App surface syntax", testPrimitiveAppSurfaceSyntax)
	t.Run("full render before writes", testJobsFullRenderBeforeWrites)
	t.Run("App-only removal before writes", testPrimitiveAppOnlyRemovalBeforeWrites)
	t.Run("additive enablement", testPrimitiveAdditiveEnablement)
	t.Run("last App removal", testPrimitiveLastAppRemoval)
	t.Run("accessor-only last App removal", testJobsAccessorOnlyLastAppRemoval)
	t.Run("driver environment", testPrimitiveDriverEnvironment)
	t.Run("legacy owners", testPrimitiveLegacyOwners)
}

// primitiveRendererContracts returns the transition contract shared by Events, Storage, and Jobs.
func primitiveRendererContracts() []primitiveRendererContract {
	return []primitiveRendererContract{
		{
			name:        "Events",
			fallback:    "inproc",
			validDriver: "redis",
			validEnv:    "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n",
			excludedEnv: "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc\n",
		},
		{
			name:        "Storage",
			fallback:    "local",
			validDriver: "s3",
			validEnv:    "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local,s3\n",
			excludedEnv: "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local\n",
		},
		{
			name:        "Jobs",
			fallback:    "workerpool",
			validDriver: "redis",
			validEnv:    "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n",
			excludedEnv: "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool\n",
		},
	}
}

// testPrimitiveResiduePreflight verifies every declared generated artifact blocks unsupported removal without mutation.
func testPrimitiveResiduePreflight(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			for _, residuePath := range primitiveResiduePaths(contract.name) {
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
					renderer := &ProjectRenderer{config: primitiveRendererConfig(components)}
					err := validatePrimitiveTransition(renderer, contract.name, components)
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
		directories []string
		files       map[string]string
	}{
		{
			name:  "Storage runtime data",
			files: map[string]string{filepath.Join("storage", "app", "private", "upload.txt"): "owner data\n"},
		},
		{
			name: "Jobs runtime data and empty shells",
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
			renderer := &ProjectRenderer{config: primitiveRendererConfig(components)}
			name := strings.Fields(test.name)[0]
			if err := validatePrimitiveTransition(renderer, name, components); err != nil {
				t.Fatalf("%s non-source artifact blocked removal: %v", name, err)
			}
			for _, path := range test.directories {
				info, err := os.Stat(path)
				if err != nil || !info.IsDir() {
					t.Fatalf("%s preflight changed artifact directory %s: info=%v err=%v", name, path, info, err)
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
		source string
		want   bool
	}{
		{name: "Storage receiver", source: "package wire\ntype App struct{}\nfunc (a *App) Storage() any { return nil }\n", want: true},
		{name: "Storage free function", source: "package wire\ntype App struct{ Storage string }\nfunc Storage() any { return nil }\n"},
		{name: "Jobs Queue receiver", source: "package wire\ntype App struct{}\nfunc (a *App) Queue() any { return nil }\n", want: true},
		{name: "Jobs Queues receiver", source: "package wire\ntype App struct{}\nfunc (a App) Queues() any { return nil }\n", want: true},
		{
			name: "Jobs comments strings fields free functions and unrelated receivers",
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
			name := strings.Fields(test.name)[0]
			got, err := primitiveAppSurfaceExists(name, path)
			if err != nil {
				t.Fatalf("inspect %s App surface: %v", name, err)
			}
			if got != test.want {
				t.Fatalf("%s App surface presence = %t, want %t", name, got, test.want)
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
			setPrimitiveComponent(&appComponents, contract.name, true)
			config := primitiveRendererConfig(components)
			config.ProjectName = contract.name + " Removal"
			config.GoModuleName = "example.test/primitive-removal"
			config.Apps = map[string]project.AppConfig{app.Name: {Components: appComponents}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			configBefore := readPrimitiveRendererFile(t, ".goforj.yml")
			environment := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n" + primitiveRootEnvironment(contract.name)
			writePrimitiveRendererFile(t, ".env", environment)
			surfacePath, surfaceSource := primitiveRemovalSurface(app, contract.name)
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

// testPrimitiveAdditiveEnablement verifies new component boundaries can be added without rewriting any existing App-owned file.
func testPrimitiveAdditiveEnablement(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("api")
			config := primitiveAdditiveConfig(contract.name, app)
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			writePrimitiveRendererFile(t, ".env", primitiveAdditiveEnvironment(contract.name))

			initialRenderer := NewProjectRenderer(logger.NewSilentLogger())
			initialRenderer.config = config
			if err := initialRenderer.renderApp(app); err != nil {
				t.Fatalf("render %s-disabled App fixture: %v", contract.name, err)
			}
			ownerPaths := make([]string, 0)
			for _, mapping := range initialRenderer.appOwnedMappings(app) {
				ownerPaths = append(ownerPaths, mapping.dest)
				contents := readPrimitiveRendererFile(t, mapping.dest)
				writePrimitiveRendererFile(t, mapping.dest, contents+"\n// OwnerSentinel proves additive renders preserve this file.\n")
			}
			wantHashes := primitiveRendererFileHashes(t, ownerPaths)

			enabledComponents := primitiveRendererBaseComponents()
			setPrimitiveComponent(&enabledComponents, contract.name, true)
			renderer := NewProjectRenderer(logger.NewSilentLogger())
			if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: enabledComponents, SkipWire: true}); err != nil {
				t.Fatalf("add %s to existing App: %v", contract.name, err)
			}
			gotHashes := primitiveRendererFileHashes(t, ownerPaths)
			for _, path := range ownerPaths {
				if gotHashes[path] != wantHashes[path] {
					t.Fatalf("additive %s render changed App-owned file %s", contract.name, path)
				}
			}
			for _, path := range primitiveAdditiveExpectedPaths(app, contract.name) {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("additive %s render omitted %s: %v", contract.name, path, err)
				}
			}
			assertPrimitiveAdditiveSurface(t, app, contract.name)
			environment := readPrimitiveRendererFile(t, ".env")
			if want := primitiveNamedEnvironment(contract.name); !strings.Contains(environment, want) {
				t.Fatalf("additive %s render omitted %q:\n%s", contract.name, want, environment)
			}
			if contract.name == "Jobs" {
				for _, want := range []string{"QUEUE_DRIVER=workerpool", "QUEUE_SUPPORTED_DRIVERS=workerpool,redis"} {
					if !strings.Contains(environment, want) {
						t.Fatalf("additive Jobs render omitted root contract %q:\n%s", want, environment)
					}
				}
			}
			loaded, err := project.LoadProjectConfig()
			if err != nil {
				t.Fatalf("reload additive %s config: %v", contract.name, err)
			}
			if !primitiveComponentEnabled(loaded.Apps[app.Name].Components, contract.name) {
				t.Fatalf("additive render did not persist %s participation", contract.name)
			}
		})
	}
}

// testJobsAccessorOnlyLastAppRemoval verifies the App Queue API alone protects the final Jobs owner.
func testJobsAccessorOnlyLastAppRemoval(t *testing.T) {
	tests := []struct {
		name      string
		removeApp bool
	}{
		{name: "transition preflight"},
		{name: "RemoveApp", removeApp: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("worker")
			config := primitiveRendererConfig(primitiveRendererBaseComponents())
			config.Apps = map[string]project.AppConfig{
				app.Name: {Components: primitiveComponentsWith("Jobs")},
			}
			if test.removeApp {
				if err := writeProjectConfig(".goforj.yml", config); err != nil {
					t.Fatalf("write project config: %v", err)
				}
			}
			appPath := filepath.Join(app.WireDir, "app.go")
			appContents := `package workerapp

type App struct{}

func (a *App) Queues() any { return nil }
`
			writePrimitiveRendererFile(t, appPath, appContents)
			renderer := &ProjectRenderer{config: config}
			if test.removeApp {
				renderer = NewProjectRenderer(logger.NewSilentLogger())
				result, err := renderer.RemoveApp(app)
				if result.Changed() {
					t.Fatalf("failed accessor-only Jobs removal reported changes: %#v", result)
				}
				assertJobsAccessorRemovalError(t, err, appPath)
			} else {
				assertJobsAccessorRemovalError(t, renderer.validateRemoveAppTransition(app), appPath)
			}
			if got := readPrimitiveRendererFile(t, appPath); got != appContents {
				t.Fatal("accessor-only Jobs removal changed the App owner")
			}
		})
	}
}

// assertJobsAccessorRemovalError verifies the final-owner diagnostic identifies the App Queue API.
func assertJobsAccessorRemovalError(t *testing.T, err error, appPath string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "last App using Jobs") || !strings.Contains(err.Error(), appPath) {
		t.Fatalf("last Jobs App accessor error = %v", err)
	}
}

// testPrimitiveLastAppRemoval verifies removing the final owner cannot delete App files or strand shared component source.
func testPrimitiveLastAppRemoval(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("worker")
			components := primitiveRendererBaseComponents()
			appComponents := components
			setPrimitiveComponent(&appComponents, contract.name, true)
			config := primitiveRendererConfig(components)
			config.Apps = map[string]project.AppConfig{app.Name: {Components: appComponents}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			configBefore := readPrimitiveRendererFile(t, ".goforj.yml")
			appSentinel := filepath.Join(app.AppDir, "owner.go")
			writePrimitiveRendererFile(t, appSentinel, "package workerapp\n\nvar ownerSentinel = true\n")
			writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
			writePrimitiveRendererFile(t, ".env", primitiveRootEnvironment(contract.name))
			residuePath, residueSource := primitiveLastOwnerResidue(contract.name)
			writePrimitiveRendererFile(t, residuePath, residueSource)

			renderer := NewProjectRenderer(logger.NewSilentLogger())
			result, err := renderer.RemoveApp(app)
			if err == nil || !strings.Contains(err.Error(), "last App using "+contract.name) || !strings.Contains(err.Error(), filepath.Dir(residuePath)) {
				t.Fatalf("remove last %s App error = %v", contract.name, err)
			}
			if result.Changed() {
				t.Fatalf("failed %s App removal reported changes: %#v", contract.name, result)
			}
			for _, path := range []string{appSentinel, app.Entrypoint, ".env", residuePath} {
				if _, statErr := os.Stat(path); statErr != nil {
					t.Fatalf("failed %s App removal changed %s: %v", contract.name, path, statErr)
				}
			}
			if got := readPrimitiveRendererFile(t, ".goforj.yml"); got != configBefore {
				t.Fatalf("failed %s App removal rewrote project config", contract.name)
			}
		})
	}
}

// testPrimitiveDriverEnvironment verifies incremental Apps inherit only valid owner-controlled root selections.
func testPrimitiveDriverEnvironment(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			tests := []struct {
				name        string
				environment string
				want        string
				wantError   string
			}{
				{name: "missing uses fallback", want: contract.fallback},
				{name: "valid owner contract", environment: contract.validEnv, want: contract.validDriver},
				{name: "unknown active driver", environment: primitiveUnknownEnvironment(contract.name), wantError: "unsupported driver"},
				{name: "active driver excluded", environment: contract.excludedEnv, wantError: "excludes active"},
			}
			if contract.name == "Jobs" {
				tests = append(tests, struct {
					name        string
					environment string
					want        string
					wantError   string
				}{name: "fallback driver excluded", environment: "QUEUE_SUPPORTED_DRIVERS=redis\n", wantError: "excludes active"})
			}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					path := filepath.Join(t.TempDir(), ".env")
					if test.environment != "" {
						writePrimitiveRendererFile(t, path, test.environment)
					}
					got, err := primitiveDriverDefaultFromEnv(contract.name, path)
					if test.wantError != "" {
						if err == nil || !strings.Contains(err.Error(), test.wantError) {
							t.Fatalf("%s driver error = %v, want %q", contract.name, err, test.wantError)
						}
						return
					}
					if err != nil {
						t.Fatalf("resolve %s driver: %v", contract.name, err)
					}
					if got != test.want {
						t.Fatalf("%s driver = %q, want %q", contract.name, got, test.want)
					}
				})
			}
		})
	}
}

// testPrimitiveLegacyOwners retains byte-for-byte migrations, collision preflights, and surgical Events compatibility repairs.
func testPrimitiveLegacyOwners(t *testing.T) {
	t.Run("preserved migrations", testPrimitivePreservedOwnerMigrations)
	t.Run("collision preflight", testPrimitiveOwnerMigrationCollisions)
	t.Run("Events subscriber set rename", testEventSubscriberSetRename)
	t.Run("Events structured compatibility", testEventStructuredCompatibility)
	t.Run("Jobs framework cleanup", testJobsLegacyFrameworkCleanup)
}

// testPrimitivePreservedOwnerMigrations verifies historical owner paths move without content changes.
func testPrimitivePreservedOwnerMigrations(t *testing.T) {
	tests := []struct {
		name   string
		config *project.Config
		source string
		target string
	}{
		{
			name:   "Events default App",
			config: primitiveRendererConfig(primitiveComponentsWith("Events")),
			source: filepath.Join("wire", "inject_event_subscribers.go"),
			target: eventSubscriberOwnerPath(project.DefaultApp()),
		},
		{
			name: "Events named App",
			config: &project.Config{
				Render: project.RenderConfig{Components: primitiveRendererBaseComponents(), ComponentContractVersion: project.CurrentComponentContractVersion},
				Apps:   map[string]project.AppConfig{"worker": {Components: primitiveComponentsWith("Events")}},
			},
			source: filepath.Join(project.DefaultNamedApp("worker").WireDir, "inject_event_subscribers.go"),
			target: eventSubscriberOwnerPath(project.DefaultNamedApp("worker")),
		},
		{
			name:   "Jobs default App",
			config: primitiveRendererConfig(primitiveComponentsWith("Jobs")),
			source: filepath.Join("wire", "inject_jobs_app.go"),
			target: filepath.Join(project.DefaultApp().WireDir, "inject_jobs_app.go"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			contents := "package wire\n\n// OwnerSentinel must survive migration.\nvar ownerSentinel = true\n"
			writePrimitiveRendererFile(t, test.source, contents)
			renderer := &ProjectRenderer{config: test.config}
			if err := renderer.migrateAppOwnedWireFilenames(); err != nil {
				t.Fatalf("migrate %s owner: %v", test.name, err)
			}
			if _, err := os.Stat(test.source); !os.IsNotExist(err) {
				t.Fatalf("legacy owner %s remains: %v", test.source, err)
			}
			if got := readPrimitiveRendererFile(t, test.target); got != contents {
				t.Fatalf("migrated owner %s changed", test.target)
			}
		})
	}
}

// testPrimitiveOwnerMigrationCollisions verifies competing owner files are discovered before either file moves.
func testPrimitiveOwnerMigrationCollisions(t *testing.T) {
	tests := []struct {
		name            string
		config          *project.Config
		source          string
		target          string
		unrelatedSource string
		unrelatedTarget string
	}{
		{
			name:   "Events",
			config: primitiveRendererConfig(primitiveComponentsWith("Events")),
			source: filepath.Join("wire", "inject_event_subscribers.go"),
			target: eventSubscriberOwnerPath(project.DefaultApp()),
		},
		{
			name:            "Jobs",
			config:          primitiveRendererConfig(primitiveComponentsWith("Events", "Jobs")),
			source:          filepath.Join("wire", "inject_jobs_app.go"),
			target:          filepath.Join(project.DefaultApp().WireDir, "inject_jobs_app.go"),
			unrelatedSource: filepath.Join(project.DefaultApp().WireDir, "inject_event_subscribers.go"),
			unrelatedTarget: eventSubscriberOwnerPath(project.DefaultApp()),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			sourceContents := "package wire\n\nvar legacyOwner = true\n"
			targetContents := "package wire\n\nvar currentOwner = true\n"
			unrelatedContents := "package wire\n\nvar unrelatedOwner = true\n"
			writePrimitiveRendererFile(t, test.source, sourceContents)
			writePrimitiveRendererFile(t, test.target, targetContents)
			if test.unrelatedSource != "" {
				writePrimitiveRendererFile(t, test.unrelatedSource, unrelatedContents)
			}
			renderer := &ProjectRenderer{config: test.config}
			err := renderer.migrateAppOwnedWireFilenames()
			if err == nil || !strings.Contains(err.Error(), test.source) || !strings.Contains(err.Error(), test.target) {
				t.Fatalf("%s collision error = %v", test.name, err)
			}
			if got := readPrimitiveRendererFile(t, test.source); got != sourceContents {
				t.Fatalf("%s collision changed legacy owner", test.name)
			}
			if got := readPrimitiveRendererFile(t, test.target); got != targetContents {
				t.Fatalf("%s collision changed current owner", test.name)
			}
			if test.unrelatedSource != "" {
				if got := readPrimitiveRendererFile(t, test.unrelatedSource); got != unrelatedContents {
					t.Fatalf("%s collision changed unrelated Events owner", test.name)
				}
				if _, err := os.Stat(test.unrelatedTarget); !os.IsNotExist(err) {
					t.Fatalf("%s collision created unrelated Events owner: %v", test.name, err)
				}
			}
		})
	}
}

// testEventSubscriberSetRename verifies identifier repair changes code references without rewriting comments or strings.
func testEventSubscriberSetRename(t *testing.T) {
	usePrimitiveRendererRoot(t)
	path := eventSubscriberOwnerPath(project.DefaultApp())
	contents := `package wire

// eventSubscriberSet remains here as migration documentation.
var eventSubscriberSet = wire.NewSet(NewCustomSubscriber)
var historicalName = "eventSubscriberSet"

func currentSubscriberSet() any { return eventSubscriberSet }
`
	writePrimitiveRendererFile(t, path, contents)
	renderer := &ProjectRenderer{config: primitiveRendererConfig(primitiveComponentsWith("Events"))}
	if err := renderer.migrateAppOwnedWireFilenames(); err != nil {
		t.Fatalf("repair Events subscriber set: %v", err)
	}
	got := readPrimitiveRendererFile(t, path)
	for _, want := range []string{"var appSubscriberSet =", "return appSubscriberSet", "// eventSubscriberSet", `"eventSubscriberSet"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("repaired Events owner omitted %q:\n%s", want, got)
		}
	}
}

// testEventStructuredCompatibility verifies real declarations are detected while ambiguous App-owned providers remain untouched.
func testEventStructuredCompatibility(t *testing.T) {
	t.Run("detect declarations", func(t *testing.T) {
		usePrimitiveRendererRoot(t)
		app := project.DefaultApp()
		commandsPath := filepath.Join(app.AppDir, "commands.go")
		wiringPath := filepath.Join(app.WireDir, "inject_cmd_app.go")
		writePrimitiveRendererFile(t, commandsPath, `package app

import "example.test/internal/cmd"

// cmd.TestEventPipelineCmd is documentation only.
type Commands struct { Pipeline cmd.TestEventPipelineCmd }
`)
		writePrimitiveRendererFile(t, wiringPath, `package wire

var appCommandSet = wire.NewSet(cmd.NewTestEventPipelineCmd)
`)
		if !legacyEventPipelineField(app) || !legacyEventPipelineProvider(app) {
			t.Fatal("structured Events compatibility declarations were not detected")
		}
	})

	t.Run("preserve ambiguous providers", func(t *testing.T) {
		usePrimitiveRendererRoot(t)
		app := project.DefaultApp()
		servicePath := filepath.Join(app.WireDir, "inject_services_app.go")
		serviceContents := `package wire

func CustomProvider() any { return nil }
func provideSharedRedisClient() any { return nil }
`
		writePrimitiveRendererFile(t, servicePath, serviceContents)
		config := &project.Config{
			Render: project.RenderConfig{Components: primitiveRendererBaseComponents(), ComponentContractVersion: project.CurrentComponentContractVersion},
			Apps:   map[string]project.AppConfig{"worker": {Components: primitiveComponentsWith("Events")}},
		}
		renderer := &ProjectRenderer{config: config}
		err := renderer.validateEventsRenderTransition(project.ProjectComponents(config))
		if err == nil || !strings.Contains(err.Error(), servicePath) {
			t.Fatalf("ambiguous Events provider error = %v", err)
		}
		if got := readPrimitiveRendererFile(t, servicePath); got != serviceContents {
			t.Fatal("Events compatibility preflight changed App-owned providers")
		}
	})
}

// testJobsLegacyFrameworkCleanup verifies obsolete generated files remain cleanup-owned while custom Jobs source survives.
func testJobsLegacyFrameworkCleanup(t *testing.T) {
	usePrimitiveRendererRoot(t)
	for _, path := range legacyJobsFrameworkPaths() {
		writePrimitiveRendererFile(t, path, "package jobs\n")
	}
	customPath := filepath.Join("internal", "jobs", "reports", "custom_job.go")
	customContents := "package reports\n\nvar customJobSentinel = true\n"
	writePrimitiveRendererFile(t, customPath, customContents)
	writePrimitiveRendererFile(t, filepath.Join("project", "config.go"), "package project\n")
	writePrimitiveRendererFile(t, filepath.Join("internal", "lighthouse", "project_config_patch.go"), "package lighthouse\n")
	components := primitiveComponentsWith("Jobs")
	renderer := &ProjectRenderer{config: primitiveRendererConfig(components)}
	if err := renderer.validateJobsRenderTransition(components); err != nil {
		t.Fatalf("legacy Jobs framework files blocked enabled rerender: %v", err)
	}
	if err := renderer.cleanupLegacyGeneratedFiles(); err != nil {
		t.Fatalf("clean legacy Jobs framework files: %v", err)
	}
	for _, path := range legacyJobsFrameworkPaths() {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("legacy Jobs framework file %s remains: %v", path, err)
		}
	}
	if got := readPrimitiveRendererFile(t, customPath); got != customContents {
		t.Fatalf("custom Jobs source changed: %q", got)
	}
}

// primitiveRendererBaseComponents returns the explicit durable baseline used by transition fixtures.
func primitiveRendererBaseComponents() project.Components {
	return project.Components{CLI: true, Cache: true}
}

// primitiveComponentsWith returns the renderer baseline with the requested optional primitives enabled.
func primitiveComponentsWith(names ...string) project.Components {
	components := primitiveRendererBaseComponents()
	for _, name := range names {
		setPrimitiveComponent(&components, name, true)
	}
	return components
}

// primitiveRendererConfig creates an explicit-contract fixture so omissions remain disabled.
func primitiveRendererConfig(components project.Components) *project.Config {
	return &project.Config{Render: project.RenderConfig{
		Components:               components,
		ComponentContractVersion: project.CurrentComponentContractVersion,
	}}
}

// primitiveResiduePaths returns the production-owned residue inventory for one primitive.
func primitiveResiduePaths(name string) []string {
	switch name {
	case "Events":
		return projectEventsResiduePaths()
	case "Storage":
		return projectStorageResiduePaths()
	case "Jobs":
		return projectJobsResiduePaths()
	default:
		return nil
	}
}

// primitiveResidueFixturePath turns component directories into meaningful source fixtures.
func primitiveResidueFixturePath(path string) string {
	for _, directory := range []string{
		filepath.Join("internal", "events"),
		filepath.Join("internal", "storages"),
		filepath.Join("internal", "jobs"),
		filepath.Join("internal", "queues"),
	} {
		if path == directory {
			return filepath.Join(path, "owner.go")
		}
	}
	return path
}

// validatePrimitiveTransition calls the production preflight for one primitive.
func validatePrimitiveTransition(renderer *ProjectRenderer, name string, components project.Components) error {
	switch name {
	case "Events":
		return renderer.validateEventsRenderTransition(components)
	case "Storage":
		return renderer.validateStorageRenderTransition(components)
	case "Jobs":
		return renderer.validateJobsRenderTransition(components)
	default:
		return nil
	}
}

// primitiveAppSurfaceExists calls the syntax-aware App API detector for one primitive.
func primitiveAppSurfaceExists(name string, path string) (bool, error) {
	switch name {
	case "Storage":
		return appStorageSurfaceExists(path)
	case "Jobs":
		return appJobsSurfaceExists(path)
	default:
		return false, nil
	}
}

// primitiveRemovalSurface returns the App-local artifact that proves removal is unsafe.
func primitiveRemovalSurface(app project.App, name string) (string, string) {
	switch name {
	case "Events":
		return filepath.Join(app.AppDir, "event_commands.go"), "package workerapp\n"
	case "Storage":
		return filepath.Join(app.WireDir, "app.go"), "package workerapp\ntype App struct{}\nfunc (a *App) Storage() any { return nil }\n"
	case "Jobs":
		return filepath.Join(app.WireDir, "app.go"), "package workerapp\ntype App struct{}\nfunc (a *App) Queue() any { return nil }\n"
	default:
		return "", ""
	}
}

// primitiveRootEnvironment returns a valid root contract for one primitive.
func primitiveRootEnvironment(name string) string {
	switch name {
	case "Events":
		return "EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n"
	case "Storage":
		return "STORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local,memory\n"
	case "Jobs":
		return "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n"
	default:
		return ""
	}
}

// primitiveAdditiveConfig creates the smallest existing-project shape supported by each incremental transition.
func primitiveAdditiveConfig(name string, app project.App) *project.Config {
	config := primitiveRendererConfig(primitiveRendererBaseComponents())
	config.ProjectName = "Additive " + name
	config.GoModuleName = "example.test/additive-primitive"
	config.Apps = map[string]project.AppConfig{app.Name: {Components: primitiveRendererBaseComponents()}}
	switch name {
	case "Events":
		config.Render.Components = primitiveComponentsWith("Events")
	case "Storage":
		config.Apps["storage-worker"] = project.AppConfig{Components: primitiveComponentsWith("Storage")}
	}
	return config
}

// primitiveAdditiveEnvironment returns the owner contract used before adding a primitive to one App.
func primitiveAdditiveEnvironment(name string) string {
	base := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n"
	switch name {
	case "Events":
		return base + "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n"
	case "Storage":
		return base + "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local,s3\n"
	default:
		return base
	}
}

// primitiveAdditiveExpectedPaths returns the new boundaries an incremental App render must create.
func primitiveAdditiveExpectedPaths(app project.App, name string) []string {
	switch name {
	case "Events":
		return []string{filepath.Join(app.AppDir, "event_commands.go"), filepath.Join(app.WireDir, "inject_subscribers_app.go")}
	case "Storage":
		return []string{filepath.Join(app.WireDir, "app.go")}
	case "Jobs":
		return []string{
			filepath.Join(app.WireDir, "inject_jobs.go"),
			filepath.Join(app.WireDir, "inject_jobs_app.go"),
			filepath.Join("internal", "jobs", "runtime.go"),
			filepath.Join("internal", "queues", "manager_gen.go"),
		}
	default:
		return nil
	}
}

// assertPrimitiveAdditiveSurface verifies the generated App API for an incrementally enabled primitive.
func assertPrimitiveAdditiveSurface(t *testing.T, app project.App, name string) {
	t.Helper()
	if name == "Events" {
		body := readPrimitiveRendererFile(t, filepath.Join(app.AppDir, "event_commands.go"))
		if !strings.Contains(body, "GeneratedEventCommands") {
			t.Fatalf("additive Events render omitted generated command boundary:\n%s", body)
		}
		return
	}
	exists, err := primitiveAppSurfaceExists(name, filepath.Join(app.WireDir, "app.go"))
	if err != nil {
		t.Fatalf("inspect additive %s App surface: %v", name, err)
	}
	if !exists {
		t.Fatalf("additive render did not generate the App %s surface", name)
	}
}

// primitiveNamedEnvironment returns the named-App driver assignment expected after additive enablement.
func primitiveNamedEnvironment(name string) string {
	switch name {
	case "Events":
		return "API_EVENTS_DRIVER=redis"
	case "Storage":
		return "API_STORAGE_DRIVER=s3"
	case "Jobs":
		return "API_QUEUE_DRIVER=workerpool"
	default:
		return ""
	}
}

// primitiveLastOwnerResidue returns one representative shared artifact for last-owner removal preflight.
func primitiveLastOwnerResidue(name string) (string, string) {
	switch name {
	case "Events":
		return filepath.Join("internal", "events", "event.go"), "package events\n"
	case "Storage":
		return filepath.Join("internal", "storages", "manager_gen.go"), "package storages\n"
	case "Jobs":
		return filepath.Join("internal", "jobs", "custom_job.go"), "package jobs\n"
	default:
		return "", ""
	}
}

// primitiveDriverDefaultFromEnv resolves one primitive's owner-controlled root driver.
func primitiveDriverDefaultFromEnv(name string, path string) (string, error) {
	switch name {
	case "Events":
		return eventDriverDefaultFromEnv(path)
	case "Storage":
		return storageDriverDefaultFromEnv(path)
	case "Jobs":
		return queueDriverDefaultFromEnv(path)
	default:
		return "", nil
	}
}

// primitiveUnknownEnvironment returns an invalid active driver contract for one primitive.
func primitiveUnknownEnvironment(name string) string {
	prefix := strings.ToUpper(name)
	if name == "Jobs" {
		prefix = "QUEUE"
	}
	return prefix + "_DRIVER=unknown\n" + prefix + "_SUPPORTED_DRIVERS=unknown\n"
}

// usePrimitiveRendererRoot changes into an isolated render root for the duration of one test.
func usePrimitiveRendererRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to primitive render root: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	return root
}

// writePrimitiveRendererFile writes one isolated fixture with its conventional parent directories.
func writePrimitiveRendererFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// readPrimitiveRendererFile reads one ownership-sensitive fixture.
func readPrimitiveRendererFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(contents)
}

// primitiveRendererFileHashes captures byte-level identities for App-owned files before an additive render.
func primitiveRendererFileHashes(t *testing.T, paths []string) map[string][sha256.Size]byte {
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
