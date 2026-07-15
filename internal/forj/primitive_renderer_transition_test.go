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
	key         project.ComponentKey
	fallback    string
	validDriver string
	validEnv    string
	excludedEnv string
}

// TestPrimitiveRendererTransitions covers additive enablement, safe removal, and driver inheritance for every primitive.
func TestPrimitiveRendererTransitions(t *testing.T) {
	t.Run("additive enablement", testPrimitiveAdditiveEnablement)
	t.Run("last App removal", testPrimitiveLastAppRemoval)
	t.Run("accessor-only last App removal", testJobsAccessorOnlyLastAppRemoval)
	t.Run("driver environment", testPrimitiveDriverEnvironment)
}

// primitiveRendererContracts returns the transition contract shared by Events, Storage, and Jobs.
func primitiveRendererContracts() []primitiveRendererContract {
	return []primitiveRendererContract{
		{
			name:        "Events",
			key:         project.ComponentEvents,
			fallback:    "inproc",
			validDriver: "redis",
			validEnv:    "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n",
			excludedEnv: "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc\n",
		},
		{
			name:        "Storage",
			key:         project.ComponentStorage,
			fallback:    "local",
			validDriver: "s3",
			validEnv:    "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local,s3\n",
			excludedEnv: "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local\n",
		},
		{
			name:        "Jobs",
			key:         project.ComponentJobs,
			fallback:    "workerpool",
			validDriver: "redis",
			validEnv:    "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n",
			excludedEnv: "QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool\n",
		},
	}
}

// testPrimitiveAdditiveEnablement verifies new component boundaries can be added without rewriting any existing App-owned file.
func testPrimitiveAdditiveEnablement(t *testing.T) {
	for _, contract := range primitiveRendererContracts() {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			usePrimitiveRendererRoot(t)
			app := project.DefaultNamedApp("api")
			config := primitiveAdditiveConfig(contract.key, app)
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			writePrimitiveRendererFile(t, ".env", primitiveAdditiveEnvironment(contract.key))

			initialRenderer := NewProjectRenderer(logger.NewSilentLogger())
			initialRenderer.config = config
			initialRenderer.resources.plan = defaultResourcePlanForTest(t, project.ProjectComponents(config))
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
			enabledComponents.SetEnabled(contract.key, true)
			renderer := NewProjectRenderer(logger.NewSilentLogger())
			projectedComponents := project.ProjectComponents(config)
			projectedComponents.SetEnabled(contract.key, true)
			renderer.resources.plan = defaultResourcePlanForTest(t, projectedComponents)
			if err := renderer.RenderAppOnly(app, makeapp.RenderOptions{Components: enabledComponents, SkipWire: true}); err != nil {
				t.Fatalf("add %s to existing App: %v", contract.name, err)
			}
			gotHashes := primitiveRendererFileHashes(t, ownerPaths)
			for _, path := range ownerPaths {
				if gotHashes[path] != wantHashes[path] {
					t.Fatalf("additive %s render changed App-owned file %s", contract.name, path)
				}
			}
			for _, path := range primitiveAdditiveExpectedPaths(app, contract.key) {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("additive %s render omitted %s: %v", contract.name, path, err)
				}
			}
			assertPrimitiveAdditiveSurface(t, app, contract.key)
			environment := readPrimitiveRendererFile(t, ".env")
			if want := primitiveNamedEnvironment(contract.key); !strings.Contains(environment, want) {
				t.Fatalf("additive %s render omitted %q:\n%s", contract.name, want, environment)
			}
			if contract.key == project.ComponentJobs {
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
			if !loaded.Apps[app.Name].Components.Enabled(contract.key) {
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
				app.Name: {Components: primitiveComponentsWith(project.ComponentJobs)},
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
			renderer := projectRendererForTest(t, config)
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
			appComponents.SetEnabled(contract.key, true)
			config := primitiveRendererConfig(components)
			config.Apps = map[string]project.AppConfig{app.Name: {Components: appComponents}}
			if err := writeProjectConfig(".goforj.yml", config); err != nil {
				t.Fatalf("write project config: %v", err)
			}
			configBefore := readPrimitiveRendererFile(t, ".goforj.yml")
			appSentinel := filepath.Join(app.AppDir, "owner.go")
			writePrimitiveRendererFile(t, appSentinel, "package workerapp\n\nvar ownerSentinel = true\n")
			writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
			writePrimitiveRendererFile(t, ".env", primitiveRootEnvironment(contract.key))
			residuePath, residueSource := primitiveLastOwnerResidue(contract.key)
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
			workspace := currentProjectRenderWorkspace(t)
			tests := []struct {
				name        string
				environment string
				want        string
				wantError   string
			}{
				{name: "missing uses fallback", want: contract.fallback},
				{name: "valid owner contract", environment: contract.validEnv, want: contract.validDriver},
				{name: "unknown active driver", environment: primitiveUnknownEnvironment(contract.key), wantError: "unsupported driver"},
				{name: "active driver excluded", environment: contract.excludedEnv, wantError: "excludes active"},
			}
			if contract.key == project.ComponentJobs {
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
					got, err := workspace.primitiveDriverDefaultFromEnv(contract.key, path)
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

// primitiveRendererBaseComponents returns the explicit durable baseline used by transition fixtures.
func primitiveRendererBaseComponents() project.Components {
	return project.Components{CLI: true, Cache: true}
}

// primitiveComponentsWith returns the renderer baseline with the requested optional primitives enabled.
func primitiveComponentsWith(keys ...project.ComponentKey) project.Components {
	components := primitiveRendererBaseComponents()
	for _, key := range keys {
		components.SetEnabled(key, true)
	}
	return components
}

// primitiveRendererConfig creates a canonical selection fixture so omissions remain disabled.
func primitiveRendererConfig(components project.Components) *project.Config {
	return &project.Config{Render: project.RenderConfig{
		Components: components,
	}}
}

// primitiveResiduePaths returns the production-owned residue inventory for one primitive.
func primitiveResiduePaths(key project.ComponentKey) []string {
	switch key {
	case project.ComponentEvents:
		return projectEventsResiduePaths()
	case project.ComponentStorage:
		return projectStorageResiduePaths()
	case project.ComponentJobs:
		return projectJobsResiduePaths()
	default:
		panic("unsupported primitive component: " + string(key))
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
func validatePrimitiveTransition(renderer *ProjectRenderer, key project.ComponentKey, components project.Components) error {
	switch key {
	case project.ComponentEvents:
		return renderer.validateEventsRenderTransition(components)
	case project.ComponentStorage:
		return renderer.validateStorageRenderTransition(components)
	case project.ComponentJobs:
		return renderer.validateJobsRenderTransition(components)
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveRemovalSurface returns the App-local artifact that proves removal is unsafe.
func primitiveRemovalSurface(app project.App, key project.ComponentKey) (string, string) {
	switch key {
	case project.ComponentEvents:
		return filepath.Join(app.AppDir, "event_commands.go"), "package workerapp\n"
	case project.ComponentStorage:
		return filepath.Join(app.WireDir, "app.go"), "package workerapp\ntype App struct{}\nfunc (a *App) Storage() any { return nil }\n"
	case project.ComponentJobs:
		return filepath.Join(app.WireDir, "app.go"), "package workerapp\ntype App struct{}\nfunc (a *App) Queue() any { return nil }\n"
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveRootEnvironment returns a valid root contract for one primitive.
func primitiveRootEnvironment(key project.ComponentKey) string {
	switch key {
	case project.ComponentEvents:
		return "EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n"
	case project.ComponentStorage:
		return "STORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local,memory\n"
	case project.ComponentJobs:
		return "QUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis\n"
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveAdditiveConfig creates the smallest existing-project shape supported by each incremental transition.
func primitiveAdditiveConfig(key project.ComponentKey, app project.App) *project.Config {
	config := primitiveRendererConfig(primitiveRendererBaseComponents())
	config.ProjectName = "Additive " + string(key)
	config.GoModuleName = "example.test/additive-primitive"
	config.Apps = map[string]project.AppConfig{app.Name: {Components: primitiveRendererBaseComponents()}}
	switch key {
	case project.ComponentEvents:
		config.Render.Components = primitiveComponentsWith(project.ComponentEvents)
	case project.ComponentStorage:
		config.Apps["storage-worker"] = project.AppConfig{Components: primitiveComponentsWith(project.ComponentStorage)}
	case project.ComponentJobs:
	default:
		panic("unsupported primitive component: " + string(key))
	}
	return config
}

// primitiveAdditiveEnvironment returns the owner contract used before adding a primitive to one App.
func primitiveAdditiveEnvironment(key project.ComponentKey) string {
	base := "CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n"
	switch key {
	case project.ComponentEvents:
		return base + "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n"
	case project.ComponentStorage:
		return base + "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=local,s3\n"
	case project.ComponentJobs:
		return base
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveAdditiveExpectedPaths returns the new boundaries an incremental App render must create.
func primitiveAdditiveExpectedPaths(app project.App, key project.ComponentKey) []string {
	switch key {
	case project.ComponentEvents:
		return []string{filepath.Join(app.AppDir, "event_commands.go"), filepath.Join(app.WireDir, "inject_subscribers_app.go")}
	case project.ComponentStorage:
		return []string{filepath.Join(app.WireDir, "app.go")}
	case project.ComponentJobs:
		return []string{
			filepath.Join(app.WireDir, "inject_jobs.go"),
			filepath.Join(app.WireDir, "inject_jobs_app.go"),
			filepath.Join("internal", "jobs", "runtime.go"),
			filepath.Join("internal", "queues", "manager_gen.go"),
		}
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// assertPrimitiveAdditiveSurface verifies the generated App API for an incrementally enabled primitive.
func assertPrimitiveAdditiveSurface(t *testing.T, app project.App, key project.ComponentKey) {
	t.Helper()
	if key == project.ComponentEvents {
		body := readPrimitiveRendererFile(t, filepath.Join(app.AppDir, "event_commands.go"))
		if !strings.Contains(body, "GeneratedEventCommands") {
			t.Fatalf("additive Events render omitted generated command boundary:\n%s", body)
		}
		return
	}
	workspace := currentProjectRenderWorkspace(t)
	appPath := filepath.Join(app.WireDir, "app.go")
	var exists bool
	var err error
	switch key {
	case project.ComponentStorage:
		exists, err = workspace.appStorageSurfaceExists(appPath)
	case project.ComponentJobs:
		exists, err = workspace.appJobsSurfaceExists(appPath)
	default:
		panic("unsupported primitive App surface: " + string(key))
	}
	if err != nil {
		t.Fatalf("inspect additive %s App surface: %v", key, err)
	}
	if !exists {
		t.Fatalf("additive render did not generate the App %s surface", key)
	}
}

// primitiveNamedEnvironment returns the named-App driver assignment expected after additive enablement.
func primitiveNamedEnvironment(key project.ComponentKey) string {
	switch key {
	case project.ComponentEvents:
		return "API_EVENTS_DRIVER=redis"
	case project.ComponentStorage:
		return "API_STORAGE_DRIVER=s3"
	case project.ComponentJobs:
		return "API_QUEUE_DRIVER=workerpool"
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveLastOwnerResidue returns one representative shared artifact for last-owner removal preflight.
func primitiveLastOwnerResidue(key project.ComponentKey) (string, string) {
	switch key {
	case project.ComponentEvents:
		return filepath.Join("internal", "events", "event.go"), "package events\n"
	case project.ComponentStorage:
		return filepath.Join("internal", "storages", "manager_gen.go"), "package storages\n"
	case project.ComponentJobs:
		return filepath.Join("internal", "jobs", "custom_job.go"), "package jobs\n"
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveDriverDefaultFromEnv resolves one primitive's owner-controlled root driver inside the test workspace.
func (w projectRenderWorkspace) primitiveDriverDefaultFromEnv(key project.ComponentKey, path string) (string, error) {
	switch key {
	case project.ComponentEvents:
		return w.eventDriverDefaultFromEnv(path)
	case project.ComponentStorage:
		return w.storageDriverDefaultFromEnv(path)
	case project.ComponentJobs:
		return w.queueDriverDefaultFromEnv(path)
	default:
		panic("unsupported primitive component: " + string(key))
	}
}

// primitiveUnknownEnvironment returns an invalid active driver contract for one primitive.
func primitiveUnknownEnvironment(key project.ComponentKey) string {
	prefix := strings.ToUpper(string(key))
	if key == project.ComponentJobs {
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
