package rendercheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestValidateRenderedComponentContractsAcceptsProjectAndAppSelections verifies the compact contract handles disabled, enabled, dashboard, and named-App projections.
func TestValidateRenderedComponentContractsAcceptsProjectAndAppSelections(t *testing.T) {
	tests := []struct {
		name       string
		components project.Components
		apps       map[string]project.AppConfig
	}{
		{name: "all disabled", components: project.Components{CLI: true}},
		{name: "cache", components: project.Components{CLI: true, Cache: true}},
		{name: "events", components: project.Components{CLI: true, Events: true}},
		{name: "storage", components: project.Components{CLI: true, Storage: true}},
		{name: "jobs", components: project.Components{CLI: true, Jobs: true}},
		{name: "all enabled with dashboards", components: project.Components{CLI: true, Cache: true, Events: true, Storage: true, Jobs: true, Observability: true, Grafana: true}},
		{
			name:       "named App only",
			components: project.Components{CLI: true},
			apps: map[string]project.AppConfig{
				"worker": {Components: project.Components{CLI: true, Jobs: true}},
			},
		},
		{
			name:       "named App primitive with project dashboards",
			components: project.Components{CLI: true, Observability: true, Grafana: true},
			apps: map[string]project.AppConfig{
				"worker": {Components: project.Components{CLI: true, Jobs: true}},
			},
		},
		{
			name:       "named App primitive with disabled sibling",
			components: project.Components{CLI: true},
			apps: map[string]project.AppConfig{
				"billing-worker": {Components: project.Components{CLI: true, Events: true}},
				"observer":       {Components: project.Components{CLI: true}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, config, apps := writeRenderedContractFixture(t, test.components, test.apps)
			if err := validateRenderedComponentContracts(root, config, apps); err != nil {
				t.Fatalf("validateRenderedComponentContracts() error: %v", err)
			}
		})
	}
}

// TestValidateRenderedComponentContractsChecksNamedAppEnvironmentOwnership verifies exact App prefixes follow each primitive selection independently.
func TestValidateRenderedComponentContractsChecksNamedAppEnvironmentOwnership(t *testing.T) {
	for _, contract := range renderedPrimitiveContracts {
		contract := contract
		t.Run(contract.label, func(t *testing.T) {
			enabledComponents := project.Components{CLI: true}
			enabledComponents.SetEnabled(contract.key, true)
			configuredApps := map[string]project.AppConfig{
				"billing-worker": {Components: enabledComponents},
				"observer":       {Components: project.Components{CLI: true}},
			}

			for _, environmentPath := range []string{".env", ".env.example"} {
				environmentPath := environmentPath
				t.Run("missing enabled key in "+environmentPath, func(t *testing.T) {
					root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true}, configuredApps)
					key := "BILLING_WORKER_" + contract.environmentPrefix + "_DRIVER"
					path := filepath.Join(root, environmentPath)
					source, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("read %s: %v", environmentPath, err)
					}
					writeRenderedContractFile(t, root, environmentPath, strings.Replace(string(source), key+"=default\n", "", 1))

					err = validateRenderedComponentContracts(root, config, apps)
					if err == nil || !strings.Contains(err.Error(), "App billing-worker requires "+key+" in "+environmentPath) {
						t.Fatalf("validateRenderedComponentContracts() error = %v, want missing named-App %s key", err, contract.label)
					}
				})

				t.Run("disabled sibling key in "+environmentPath, func(t *testing.T) {
					root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true}, configuredApps)
					key := "OBSERVER_" + contract.environmentPrefix + "_DRIVER"
					path := filepath.Join(root, environmentPath)
					source, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("read %s: %v", environmentPath, err)
					}
					writeRenderedContractFile(t, root, environmentPath, string(source)+key+"=default\n")

					err = validateRenderedComponentContracts(root, config, apps)
					if err == nil || !strings.Contains(err.Error(), "App observer is disabled but "+environmentPath+" defines "+key) {
						t.Fatalf("validateRenderedComponentContracts() error = %v, want disabled sibling %s key failure", err, contract.label)
					}
				})
			}
		})
	}
}

// TestValidateRenderedComponentContractsRequiresPairedAppAccessors verifies default-instance and manager accessors remain one generated surface.
func TestValidateRenderedComponentContractsRequiresPairedAppAccessors(t *testing.T) {
	tests := []struct {
		name       string
		components project.Components
		markers    []string
	}{
		{name: "Cache", components: project.Components{CLI: true, Cache: true}, markers: []string{"func (a *App) Cache()", "func (a *App) Caches()"}},
		{name: "Events", components: project.Components{CLI: true, Events: true}, markers: []string{"func (a *App) Bus()", "func (a *App) Events()"}},
		{name: "Jobs", components: project.Components{CLI: true, Jobs: true}, markers: []string{"func (a *App) Queue()", "func (a *App) Queues()"}},
	}
	for _, test := range tests {
		for _, marker := range test.markers {
			marker := marker
			t.Run(test.name+"/"+marker, func(t *testing.T) {
				root, config, apps := writeRenderedContractFixture(t, test.components, nil)
				path := filepath.Join(apps[0].WireDir, "app.go")
				source, err := os.ReadFile(filepath.Join(root, path))
				if err != nil {
					t.Fatalf("read %s: %v", path, err)
				}
				writeRenderedContractFile(t, root, path, strings.Replace(string(source), marker+" {}\n", "", 1))

				err = validateRenderedComponentContracts(root, config, apps)
				if err == nil || !strings.Contains(err.Error(), "requires marker \""+marker+"\"") {
					t.Fatalf("validateRenderedComponentContracts() error = %v, want missing paired accessor", err)
				}
			})
		}
	}
}

// TestValidateRenderedComponentContractsRejectsPrimitiveSubmodules verifies deselection excludes nested modules as well as each primitive's base module.
func TestValidateRenderedComponentContractsRejectsPrimitiveSubmodules(t *testing.T) {
	for _, contract := range renderedPrimitiveContracts {
		contract := contract
		t.Run(contract.label, func(t *testing.T) {
			root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true}, nil)
			modulePath := contract.modulePath + "/driver/test"
			writeRenderedContractFile(t, root, "go.mod", "module example.com/render\n\ngo 1.26\n\nrequire "+modulePath+" v0.1.0\n")

			err := validateRenderedComponentContracts(root, config, apps)
			if err == nil || !strings.Contains(err.Error(), "is disabled but directly requires module "+modulePath) {
				t.Fatalf("validateRenderedComponentContracts() error = %v, want nested %s module failure", err, contract.label)
			}
		})
	}
}

// TestValidateRenderedComponentContractsRejectsUnownedDashboardAndScopedEnvironment verifies stale cross-App surfaces cannot survive deselection.
func TestValidateRenderedComponentContractsRejectsUnownedDashboardAndScopedEnvironment(t *testing.T) {
	t.Run("dashboard without Grafana", func(t *testing.T) {
		root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true, Cache: true}, nil)
		path := "containers/observability/grafana/dashboards/cache-overview.json"
		writeRenderedContractFile(t, root, path, "{}\n")

		err := validateRenderedComponentContracts(root, config, apps)
		if err == nil || !strings.Contains(err.Error(), "Grafana is disabled") {
			t.Fatalf("validateRenderedComponentContracts() error = %v, want stale Cache dashboard failure", err)
		}
	})

	t.Run("scoped environment after deselection", func(t *testing.T) {
		root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true}, nil)
		writeRenderedContractFile(t, root, ".env", "API_CACHE_SESSIONS_DRIVER=redis\n")

		err := validateRenderedComponentContracts(root, config, apps)
		if err == nil || !strings.Contains(err.Error(), ".env defines API_CACHE_SESSIONS_DRIVER") {
			t.Fatalf("validateRenderedComponentContracts() error = %v, want scoped Cache environment failure", err)
		}
	})
}

// TestValidateRenderedComponentContractsReportsHighSignalLeaks verifies one validation pass reports every user-visible contract category.
func TestValidateRenderedComponentContractsReportsHighSignalLeaks(t *testing.T) {
	root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true}, nil)
	writeRenderedContractFile(t, root, "internal/events/leaked.go", "package events\n")
	writeRenderedContractFile(t, root, "containers/observability/grafana/dashboards/events-overview.json", "{}\n")
	writeRenderedContractFile(t, root, ".env", "EVENTS_DRIVER=inproc\n")
	writeRenderedContractFile(t, root, ".env.example", "EVENTS_DRIVER=inproc\n")
	writeRenderedContractFile(t, root, "go.mod", "module example.com/render\n\ngo 1.26\n\nrequire github.com/goforj/events v0.1.0\n")
	writeRenderedContractFile(t, root, "internal/runtime/discovery.go", "package runtime\n\nfunc DiscoverEventInstances() {}\n")
	writeRenderedContractFile(t, root, filepath.Join(apps[0].WireDir, "app.go"), "package wire\n\nfunc (a *App) Events() {}\n")

	err := validateRenderedComponentContracts(root, config, apps)
	if err == nil {
		t.Fatal("validateRenderedComponentContracts() error = nil, want disabled Events violations")
	}
	for _, want := range []string{
		"internal/events",
		"dashboard",
		".env defines EVENTS_DRIVER",
		"directly requires module github.com/goforj/events",
		"runtime marker",
		"App app is disabled but retains marker",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error missing %q:\n%s", want, err)
		}
	}
}

// TestValidateRenderedComponentContractsReportsMissingEnabledSurface verifies enabled components must carry core code, docs, environment, modules, and runtime accessors.
func TestValidateRenderedComponentContractsReportsMissingEnabledSurface(t *testing.T) {
	root, config, apps := writeRenderedContractFixture(t, project.Components{CLI: true, Cache: true}, nil)
	for _, path := range []string{
		"internal/caches/manager_gen.go",
		"internal/caches/README.md",
		"internal/runtime/discovery.go",
		filepath.Join(apps[0].WireDir, "app.go"),
	} {
		if err := os.Remove(pathJoin(root, path)); err != nil {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
	writeRenderedContractFile(t, root, ".env", "APP_ENV=local\n")
	writeRenderedContractFile(t, root, ".env.example", "APP_ENV=local\n")
	writeRenderedContractFile(t, root, "go.mod", "module example.com/render\n\ngo 1.26\n")

	err := validateRenderedComponentContracts(root, config, apps)
	if err == nil {
		t.Fatal("validateRenderedComponentContracts() error = nil, want missing Cache violations")
	}
	for _, want := range []string{
		"requires file internal/caches/manager_gen.go",
		"requires documentation internal/caches/README.md",
		"requires CACHE_DRIVER in .env",
		"requires direct module github.com/goforj/cache",
		"cannot inspect internal/runtime/discovery.go",
		"App app cannot inspect app/wire/app.go",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error missing %q:\n%s", want, err)
		}
	}
}

// TestRenderedDirectModuleRequirementsIgnoresIndirectModules protects disabled checks from transitive module graph noise.
func TestRenderedDirectModuleRequirementsIgnoresIndirectModules(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go.mod")
	writeRenderedContractFile(t, filepath.Dir(path), filepath.Base(path), `module example.com/render

go 1.26

require github.com/goforj/cache v0.3.0

require (
	github.com/goforj/events v0.1.0
	github.com/goforj/storage v0.4.6 // indirect
)
`)
	direct, err := renderedDirectModuleRequirements(path)
	if err != nil {
		t.Fatalf("renderedDirectModuleRequirements() error: %v", err)
	}
	if !direct["github.com/goforj/cache"] || !direct["github.com/goforj/events"] {
		t.Fatalf("direct requirements = %#v, want cache and events", direct)
	}
	if direct["github.com/goforj/storage"] {
		t.Fatalf("direct requirements = %#v, want indirect storage omitted", direct)
	}
}

// writeRenderedContractFixture keeps contract cases table-driven without reproducing full project renders in unit tests.
func writeRenderedContractFixture(t *testing.T, components project.Components, configuredApps map[string]project.AppConfig) (string, *project.Config, []project.App) {
	t.Helper()
	root := t.TempDir()
	config := &project.Config{
		GoModuleName: "example.com/render",
		Render:       project.RenderConfig{Components: components},
		Apps:         configuredApps,
	}
	apps, err := renderComboApps(renderCombo{apps: configuredApps})
	if err != nil {
		t.Fatalf("renderComboApps() error: %v", err)
	}
	projectComponents := project.ProjectComponents(config)

	environmentLines := []string{"APP_ENV=local"}
	moduleLines := []string{"module example.com/render", "", "go 1.26", "", "require ("}
	runtimeLines := map[string][]string{}
	for _, contract := range renderedPrimitiveContracts {
		if !projectComponents.Enabled(contract.key) {
			continue
		}
		for _, path := range append(append([]string{}, contract.corePaths...), contract.supportPaths...) {
			writeRenderedContractFile(t, root, path, "generated\n")
		}
		writeRenderedContractFile(t, root, contract.documentationPath, "generated documentation\n")
		if projectComponents.Grafana {
			writeRenderedContractFile(t, root, contract.dashboardPath, "{}\n")
		}
		environmentLines = append(environmentLines,
			contract.environmentPrefix+"_DRIVER=default",
			contract.environmentPrefix+"_SUPPORTED_DRIVERS=default",
		)
		moduleLines = append(moduleLines, "\t"+contract.modulePath+" v0.1.0")
		for _, marker := range contract.runtimeMarkers {
			if len(runtimeLines[marker.path]) == 0 {
				runtimeLines[marker.path] = []string{"package runtime", ""}
			}
			runtimeLines[marker.path] = append(runtimeLines[marker.path], marker.marker+" {}", "")
		}
	}
	for _, app := range apps {
		prefix := project.AppEnvironmentPrefix(app.Name)
		if prefix == "" {
			continue
		}
		appComponents := renderedAppComponents(config, app)
		for _, contract := range renderedPrimitiveContracts {
			if appComponents.Enabled(contract.key) {
				environmentLines = append(environmentLines, prefix+"_"+contract.environmentPrefix+"_DRIVER=default")
			}
		}
	}
	moduleLines = append(moduleLines, ")")
	writeRenderedContractFile(t, root, ".env", strings.Join(environmentLines, "\n")+"\n")
	writeRenderedContractFile(t, root, ".env.example", strings.Join(environmentLines, "\n")+"\n")
	writeRenderedContractFile(t, root, ".env.host", "APP_ENV=local\n")
	writeRenderedContractFile(t, root, "go.mod", strings.Join(moduleLines, "\n")+"\n")
	for _, path := range []string{"internal/runtime/discovery.go", "internal/runtime/timeouts.go"} {
		lines := runtimeLines[path]
		if len(lines) == 0 {
			lines = []string{"package runtime", ""}
		}
		writeRenderedContractFile(t, root, path, strings.Join(lines, "\n"))
	}
	for _, app := range apps {
		appComponents := renderedAppComponents(config, app)
		appLines := []string{"package wire", ""}
		for _, contract := range renderedPrimitiveContracts {
			if appComponents.Enabled(contract.key) {
				for _, marker := range contract.appMarkers {
					appLines = append(appLines, marker+" {}", "")
				}
			}
		}
		writeRenderedContractFile(t, root, filepath.Join(app.WireDir, "app.go"), strings.Join(appLines, "\n"))
	}
	return root, config, apps
}

// writeRenderedContractFile keeps fixture publication failures attached to the logical artifact under test.
func writeRenderedContractFile(t *testing.T, root string, path string, content string) {
	t.Helper()
	fullPath := pathJoin(root, path)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("create parent for %s: %v", path, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// pathJoin prevents fixture paths from accidentally escaping the temporary test workspace.
func pathJoin(root string, path string) string {
	return filepath.Join(root, filepath.FromSlash(path))
}
