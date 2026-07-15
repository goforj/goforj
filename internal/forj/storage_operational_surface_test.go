package forj

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestRenderedStorageComponentOwnsItsCompleteSurface verifies the Storage selection controls generated code, dependencies, runtime reporting, documentation, and dashboards together.
func TestRenderedStorageComponentOwnsItsCompleteSurface(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			root := renderStorageContractProject(t, enabled)

			for _, path := range []string{
				"internal/storages/README.md",
				"internal/storages/manager_gen.go",
				"internal/observability/storage_observer.go",
				"internal/observability/storage_observer_test.go",
				"containers/observability/grafana/dashboards/storage-overview.json",
			} {
				assertStorageContractPath(t, root, path, enabled)
			}

			environment := readStorageContractFile(t, root, ".env")
			for _, token := range []string{
				"METRICS_STORAGE_ENABLED=",
				"STORAGE_DRIVER=",
				"STORAGE_SUPPORTED_DRIVERS=",
				"STORAGE_ROOT=",
				"STORAGE_PUBLIC_DRIVER=",
			} {
				assertStorageContractToken(t, ".env", environment, token, enabled)
			}

			goMod := readStorageContractFile(t, root, "go.mod")
			assertStorageContractToken(t, "go.mod", goMod, "github.com/goforj/storage", enabled)

			app := readStorageContractFile(t, root, "app/wire/app.go")
			for _, token := range []string{
				`internal/storages`,
				"storage *storages.Manager",
				"func (a *App) Storage() *storages.Manager",
				"func logStorageWarnings(",
			} {
				assertStorageContractToken(t, "app/wire/app.go", app, token, enabled)
			}

			managerWiring := readStorageContractFile(t, root, "app/wire/inject_managers.go")
			for _, token := range []string{
				`internal/storages`,
				"provideStorageManager",
				"observability.StorageObserver(",
			} {
				assertStorageContractToken(t, "app/wire/inject_managers.go", managerWiring, token, enabled)
			}

			about := readStorageContractFile(t, root, "internal/runtime/about.go")
			for _, token := range []string{
				"aboutStorageRootKeys",
				"type AboutStorage struct",
				"[]AboutStorage",
				"appComponents.Storage",
				"func aboutStorageReports(",
			} {
				assertStorageContractToken(t, "internal/runtime/about.go", about, token, enabled)
			}

			discovery := readStorageContractFile(t, root, "internal/runtime/discovery.go")
			for _, token := range []string{"func DiscoverStorageInstances(", "func NormalizeStorageDriver("} {
				assertStorageContractToken(t, "internal/runtime/discovery.go", discovery, token, enabled)
			}

			runtimeApps := readStorageContractFile(t, root, "internal/runtime/apps.go")
			assertStorageContractToken(t, "internal/runtime/apps.go", runtimeApps, "Storage          bool", enabled)

			metrics := readStorageContractFile(t, root, "internal/metrics/manager.go")
			for _, token := range []string{
				`github.com/goforj/storage`,
				`internal/storages`,
				"storageOperations",
				"func (m *Manager) StorageEnabled() bool",
				"func (m *Manager) RecordStorageOperation(",
			} {
				assertStorageContractToken(t, "internal/metrics/manager.go", metrics, token, enabled)
			}
			databaseTest := readStorageContractFile(t, root, "internal/database/connections_test.go")
			assertStorageContractToken(t, "internal/database/connections_test.go", databaseTest, "Storage:  true", enabled)
			assertStorageContractToken(t, "internal/database/connections_test.go", databaseTest, "Events:   true", false)

			for path, tokens := range map[string][]string{
				"internal/metrics/README.md": {
					"METRICS_STORAGE_ENABLED",
					"storage_operations_total",
					"### Storage",
				},
				"internal/observability/README.md": {
					"Storage Overview",
					"filtered to one disk",
				},
				"internal/inspects/README.md": {
					"- `storage`",
					"- `RecordStorageEvent`",
				},
			} {
				body := readStorageContractFile(t, root, path)
				for _, token := range tokens {
					assertStorageContractToken(t, path, body, token, enabled)
				}
			}

			seed := readStorageContractFile(t, root, "containers/observability/grafana/seed-dashboards.sh")
			assertStorageContractToken(t, "containers/observability/grafana/seed-dashboards.sh", seed, "goforj-storage-overview", enabled)

			platform := readStorageContractFile(t, root, "containers/observability/grafana/dashboards/platform-overview.json")
			assertStorageContractToken(t, "containers/observability/grafana/dashboards/platform-overview.json", platform, "storage_", enabled)
			assertStorageDashboardJSON(t, root)
		})
	}
}

// TestMixedAppStorageWiringKeepsParticipationAppLocal verifies either App can own Storage while shared constructors retain the project-wide shape.
func TestMixedAppStorageWiringKeepsParticipationAppLocal(t *testing.T) {
	for _, test := range []struct {
		name           string
		defaultStorage bool
		workerStorage  bool
	}{
		{name: "named App only", workerStorage: true},
		{name: "default App only", defaultStorage: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := storageOperationalProjectionConfig(test.defaultStorage, test.workerStorage)
			apps := []struct {
				app     project.App
				enabled bool
			}{
				{app: project.DefaultApp(), enabled: test.defaultStorage},
				{app: project.DefaultNamedApp("worker"), enabled: test.workerStorage},
			}

			for _, target := range apps {
				t.Run(target.app.Name, func(t *testing.T) {
					components := appRenderComponents(config, target.app)
					data := appTemplateDataForProjectionTest(config, target.app, components)
					data.HelpFormatterFunc = "FrameworkFormatter"

					app := renderSharedTemplate(t, "wire/app.go.tmpl", data)
					managers := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", data)
					httpWiring := renderSharedTemplate(t, "wire/inject_http.go.tmpl", data)
					for path, source := range map[string]string{
						"wire/app.go.tmpl":             app,
						"wire/inject_managers.go.tmpl": managers,
						"wire/inject_http.go.tmpl":     httpWiring,
					} {
						assertFormattedGoTemplate(t, path, source)
					}

					assertTemplateMarker(t, "wire/app.go.tmpl", app, `internal/storages`, target.enabled)
					assertTemplateMarker(t, "wire/app.go.tmpl", app, "func (a *App) Storage() *storages.Manager", target.enabled)
					assertTemplateMarker(t, "wire/app.go.tmpl", app, "func logStorageWarnings(", target.enabled)
					assertTemplateMarker(t, "wire/inject_managers.go.tmpl", managers, "provideStorageManager", target.enabled)
					assertTemplateMarker(t, "wire/inject_managers.go.tmpl", managers, "provideDisabledStorageManager", !target.enabled)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", httpWiring, `internal/storages`, true)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", httpWiring, "storage *storages.Manager", true)
				})
			}
		})
	}
}

// TestStorageManagerMetricsParticipationIsAppLocal verifies project Metrics support does not inject Metrics into an App that omitted it.
func TestStorageManagerMetricsParticipationIsAppLocal(t *testing.T) {
	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/storage-metrics"},
		Components:        project.Components{CLI: true, Cache: true, Storage: true},
		ProjectComponents: project.Components{CLI: true, Cache: true, Metrics: true, Storage: true},
	}
	wiring := renderSharedTemplate(t, "wire/inject_managers.go.tmpl", data)
	assertFormattedGoTemplate(t, "wire/inject_managers.go.tmpl", wiring)
	start := strings.Index(wiring, "func provideStorageManager(")
	if start < 0 {
		t.Fatalf("rendered wiring omitted the Storage manager provider\n%s", wiring)
	}
	end := strings.Index(wiring[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("rendered wiring omitted the Storage manager provider terminator\n%s", wiring)
	}
	provider := wiring[start : start+end+3]
	if strings.Contains(provider, "metricsManager *metrics.Manager") {
		t.Fatalf("Storage provider inherited an App-local Metrics parameter from the project envelope\n%s", provider)
	}
	if !strings.Contains(provider, "(*metrics.Manager)(nil)") {
		t.Fatalf("Storage provider did not project the intentional no-Metrics observer seam\n%s", provider)
	}
	observer := renderSharedTemplate(t, "internal/observability/storage_observer.go.tmpl", data)
	assertFormattedGoTemplate(t, "internal/observability/storage_observer.go.tmpl", observer)
	if !strings.Contains(observer, "if metricsManager != nil {") {
		t.Fatalf("shared Storage observer does not tolerate the intentional no-Metrics seam\n%s", observer)
	}
}

// TestStorageObserverTestTemplateFollowsMetricsEnvelope verifies its runtime nil-seam fixture does not leave unused imports when project Metrics is absent.
func TestStorageObserverTestTemplateFollowsMetricsEnvelope(t *testing.T) {
	for _, metricsEnabled := range []bool{false, true} {
		data := templateRenderConfig{
			Config:            &project.Config{GoModuleName: "example.com/storage-observer-test"},
			Components:        project.Components{CLI: true, Storage: true, Metrics: metricsEnabled},
			ProjectComponents: project.Components{CLI: true, Storage: true, Metrics: metricsEnabled},
		}
		source := renderSharedTemplate(t, "internal/observability/storage_observer_test.go.tmpl", data)
		assertFormattedGoTemplate(t, "internal/observability/storage_observer_test.go.tmpl", source)
		if got := strings.Contains(source, "TestStorageObserverAllowsAppMetricsOptOut"); got != metricsEnabled {
			t.Fatalf("metricsEnabled=%t nil-seam fixture presence=%t\n%s", metricsEnabled, got, source)
		}
	}
}

// renderStorageContractProject renders a maximal project under the test process's /tmp root.
func renderStorageContractProject(t *testing.T, storageEnabled bool) string {
	t.Helper()
	root := t.TempDir()
	cleanTemp := filepath.Clean(os.TempDir()) + string(os.PathSeparator)
	if !strings.HasPrefix(filepath.Clean(root), cleanTemp) {
		t.Fatalf("Storage contract render root %s is outside %s", root, os.TempDir())
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to Storage contract render root: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()

	components := project.Components{
		CLI:            true,
		WebAPI:         true,
		Metrics:        true,
		Docker:         true,
		Observability:  true,
		Grafana:        true,
		DatabaseSQLite: true,
		Cache:          true,
		Jobs:           true,
		Storage:        storageEnabled,
	}
	config := project.Config{
		ProjectName:  "Storage Contract",
		GoModuleName: "example.com/storage-contract",
		Render: project.RenderConfig{
			Components:               components,
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
	}
	if err := WriteYAML(".goforj.yml", config); err != nil {
		t.Fatalf("write Storage contract config: %v", err)
	}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("render Storage contract project: %v", err)
	}
	return root
}

// storageOperationalProjectionConfig creates two HTTP Apps with independently selected Storage participation.
func storageOperationalProjectionConfig(defaultStorage bool, workerStorage bool) *project.Config {
	return &project.Config{
		GoModuleName: "example.com/storage-projection",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, WebAPI: true, Cache: true, Metrics: true, Storage: defaultStorage,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{
				CLI: true, WebAPI: true, Cache: true, Metrics: true, Storage: workerStorage,
			}},
		},
	}
}

// assertStorageContractPath verifies one generated artifact follows the Storage component contract.
func assertStorageContractPath(t *testing.T, root string, path string, want bool) {
	t.Helper()
	_, err := os.Stat(filepath.Join(root, path))
	got := err == nil
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("stat rendered %s: %v", path, err)
	}
	if got != want {
		t.Fatalf("rendered path %s presence = %t, want %t", path, got, want)
	}
}

// assertStorageContractToken verifies one generated source marker follows the Storage component contract.
func assertStorageContractToken(t *testing.T, path string, source string, token string, want bool) {
	t.Helper()
	if got := strings.Contains(source, token); got != want {
		t.Fatalf("rendered %s token %q presence = %t, want %t\n%s", path, token, got, want, source)
	}
}

// assertStorageDashboardJSON verifies every rendered Grafana dashboard is valid JSON.
func assertStorageDashboardJSON(t *testing.T, root string) {
	t.Helper()
	dashboardRoot := filepath.Join(root, "containers", "observability", "grafana", "dashboards")
	err := filepath.WalkDir(dashboardRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var decoded any
		if err := json.Unmarshal(content, &decoded); err != nil {
			t.Errorf("rendered dashboard %s is invalid JSON: %v\n%s", filepath.Base(path), err, content)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk rendered dashboards: %v", err)
	}
}

// readStorageContractFile reads one generated artifact from an isolated Storage render.
func readStorageContractFile(t *testing.T, root string, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read rendered %s: %v", path, err)
	}
	return string(content)
}
