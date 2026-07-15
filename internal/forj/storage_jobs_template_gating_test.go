package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestJobsStorageTemplatesUseProjectEnvelope verifies shared Jobs source includes Storage only when some App needs it.
func TestJobsStorageTemplatesUseProjectEnvelope(t *testing.T) {
	paths := map[string][]string{
		"internal/jobs/lighthouse.go.tmpl": {
			`"example.com/jobs-storage/internal/storages"`,
			"*storages.Manager",
		},
		"internal/jobs/lighthouse_benchmark.go.tmpl": {
			`"example.com/jobs-storage/internal/storages"`,
			"*storages.Manager",
		},
		"internal/jobs/benchmark_run_cmd.go.tmpl": {
			"c.storage",
		},
	}

	for _, test := range []struct {
		name              string
		componentsStorage bool
		projectStorage    bool
		wantStorage       bool
	}{
		{name: "project disabled even if App projection is stale", componentsStorage: true},
		{name: "named App widens project envelope", projectStorage: true, wantStorage: true},
		{name: "default App and project enabled", componentsStorage: true, projectStorage: true, wantStorage: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := templateRenderConfig{
				Config: &project.Config{GoModuleName: "example.com/jobs-storage"},
				Components: project.Components{
					CLI:     true,
					Cache:   true,
					Jobs:    true,
					Storage: test.componentsStorage,
				},
				ProjectComponents: project.Components{
					CLI:     true,
					Cache:   true,
					Jobs:    true,
					Storage: test.projectStorage,
				},
			}

			for path, markers := range paths {
				source := renderSharedTemplate(t, path, data)
				assertFormattedGoTemplate(t, path, source)
				for _, marker := range markers {
					assertTemplateMarker(t, path, source, marker, test.wantStorage)
				}
			}
		})
	}
}

// TestJobsWithoutStorageOmitsBenchmarkSurface verifies generated Jobs code has no Storage suite when the project envelope excludes it.
func TestJobsWithoutStorageOmitsBenchmarkSurface(t *testing.T) {
	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/jobs-storage-off"},
		Components:        project.Components{CLI: true, Cache: true, Jobs: true},
		ProjectComponents: project.Components{CLI: true, Cache: true, Jobs: true},
	}

	lighthouse := renderSharedTemplate(t, "internal/jobs/lighthouse.go.tmpl", data)
	benchmark := renderSharedTemplate(t, "internal/jobs/lighthouse_benchmark.go.tmpl", data)
	command := renderSharedTemplate(t, "internal/jobs/benchmark_run_cmd.go.tmpl", data)
	for path, source := range map[string]string{
		"internal/jobs/lighthouse.go.tmpl":           lighthouse,
		"internal/jobs/lighthouse_benchmark.go.tmpl": benchmark,
		"internal/jobs/benchmark_run_cmd.go.tmpl":    command,
	} {
		assertFormattedGoTemplate(t, path, source)
	}

	for _, marker := range []string{
		`"github.com/goforj/storage"`,
		"runStorageBenchmark",
		"NormalizeStorageDriver",
		`case "storage":`,
	} {
		assertTemplateMarker(t, "internal/jobs/lighthouse.go.tmpl", lighthouse, marker, false)
	}
	for _, marker := range []string{
		`"github.com/goforj/storage"`,
		"benchmarkStorageCatalog",
		"storageInstances",
		"runStorage",
		`catalog["storage"]`,
		`case "storage":`,
	} {
		assertTemplateMarker(t, "internal/jobs/lighthouse_benchmark.go.tmpl", benchmark, marker, false)
	}
	for _, marker := range []string{
		"cache,queue,storage",
		"c.storage",
		`case "storage":`,
		`suites = append(suites, "storage")`,
	} {
		assertTemplateMarker(t, "internal/jobs/benchmark_run_cmd.go.tmpl", command, marker, false)
	}
}

// TestMixedAppJobsStorageSurfaceUsesInjectedManager verifies shared Storage support remains unavailable to a non-participating App.
func TestMixedAppJobsStorageSurfaceUsesInjectedManager(t *testing.T) {
	data := templateRenderConfig{
		Config:            &project.Config{GoModuleName: "example.com/jobs-storage-mixed"},
		Components:        project.Components{CLI: true, Cache: true, Jobs: true},
		ProjectComponents: project.Components{CLI: true, Cache: true, Jobs: true, Storage: true},
	}

	lighthouse := renderSharedTemplate(t, "internal/jobs/lighthouse.go.tmpl", data)
	benchmark := renderSharedTemplate(t, "internal/jobs/lighthouse_benchmark.go.tmpl", data)
	command := renderSharedTemplate(t, "internal/jobs/benchmark_run_cmd.go.tmpl", data)
	for path, source := range map[string]string{
		"internal/jobs/lighthouse.go.tmpl":           lighthouse,
		"internal/jobs/lighthouse_benchmark.go.tmpl": benchmark,
		"internal/jobs/benchmark_run_cmd.go.tmpl":    command,
	} {
		assertFormattedGoTemplate(t, path, source)
	}

	for _, marker := range []string{
		"storage *storages.Manager",
		"storage,",
		"newBenchmarkRunner(caches, queues,",
		"runStorageBenchmark",
	} {
		assertTemplateMarker(t, "internal/jobs/lighthouse.go.tmpl", lighthouse, marker, true)
	}
	for _, marker := range []string{
		"storages *storages.Manager",
		"if r.storages != nil {",
		`catalog["storage"]`,
		`case "storage":`,
		"runStorage",
	} {
		assertTemplateMarker(t, "internal/jobs/lighthouse_benchmark.go.tmpl", benchmark, marker, true)
	}
	for _, marker := range []string{
		"storage *storages.Manager",
		"storageEnabled = c.storage != nil",
		"availableBenchmarkSuites(storageEnabled, databaseEnabled)",
		"if storageEnabled {",
		`storage benchmark unavailable: Storage is not enabled for this App`,
	} {
		assertTemplateMarker(t, "internal/jobs/benchmark_run_cmd.go.tmpl", command, marker, true)
	}

	for _, path := range []string{
		"internal/jobs/lighthouse.go.tmpl",
		"internal/jobs/lighthouse_benchmark.go.tmpl",
		"internal/jobs/benchmark_run_cmd.go.tmpl",
	} {
		content, err := templatesFS.ReadFile(path)
		if err != nil {
			t.Fatalf("read template %s: %v", path, err)
		}
		if strings.Contains(string(content), ".Components.Storage") {
			t.Fatalf("shared Jobs template %s uses App projection to shape shared Storage source", path)
		}
	}
}
