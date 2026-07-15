package forj

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestRenderedJobsFreeProjectOmitsOperationalSurface verifies a full rerender cannot recreate a disabled Jobs or Queue surface.
func TestRenderedJobsFreeProjectOmitsOperationalSurface(t *testing.T) {
	components := project.Components{
		CLI: true, WebAPI: true, Metrics: true, Docker: true, Observability: true, Grafana: true,
		DatabaseSQLite: true, Cache: true,
	}
	root := renderJobsContractProject(t, &project.Config{
		ProjectName:  "Jobs Free",
		GoModuleName: "example.com/jobs-free",
		Render: project.RenderConfig{
			Components:               components,
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
	})
	rerenderJobsContractProject(t, root)

	for _, path := range []string{
		"internal/jobs",
		"internal/queues",
		"internal/observability/queue_observer.go",
		"internal/makecmd/make_job_cmd.go",
		"internal/makecmd/make_queue_cmd.go",
		"internal/makecmd/job.tmpl",
		"app/wire/inject_jobs.go",
		"app/wire/inject_jobs_app.go",
		"containers/observability/grafana/dashboards/queue-overview.json",
	} {
		assertJobsContractPath(t, root, path, false)
	}

	environment := readJobsContractFile(t, root, ".env")
	for _, token := range []string{
		"QUEUE_",
		"METRICS_QUEUE_ENABLED=",
		"METRICS_JOBS_PORT=",
		"OBSERVABILITY_JOBS_METRICS_HOST",
	} {
		if strings.Contains(environment, token) {
			t.Fatalf("Jobs-free environment retained %q\n%s", token, environment)
		}
	}

	goMod := readJobsContractFile(t, root, "go.mod")
	if strings.Contains(goMod, "github.com/goforj/queue") {
		t.Fatalf("Jobs-free dependency graph retained Queue modules\n%s", goMod)
	}

	for path, tokens := range map[string][]string{
		"app/root_cmd.go": {
			"jobs.WorkerCmd",
			"jobs.BenchmarkRunCmd",
		},
		"app/wire/app.go": {
			"func (a *App) Queue()",
			"func (a *App) Queues()",
		},
		"app/wire/inject_cmd.go": {
			"jobsRuntime *jobs.Runtime",
		},
		"app/wire/wire.go": {
			"appJobSet",
			"jobSet",
		},
		"internal/metrics/manager.go": {
			"github.com/goforj/queue",
			"RecordQueueEvent",
			"queueJobsEnqueued",
		},
		"internal/makecmd/README.md": {
			"`make:job`",
			"`make:queue`",
		},
		"internal/metrics/README.md": {
			"METRICS_QUEUE_ENABLED",
			"queue_jobs_enqueued_total",
		},
		"internal/observability/README.md": {
			"Queue Overview",
			"OBSERVABILITY_JOBS_METRICS_HOST",
		},
	} {
		body := readJobsContractFile(t, root, path)
		for _, token := range tokens {
			if strings.Contains(body, token) {
				t.Fatalf("Jobs-free render %s retained %q\n%s", path, token, body)
			}
		}
	}

	seed := readJobsContractFile(t, root, "containers/observability/grafana/seed-dashboards.sh")
	if strings.Contains(seed, "goforj-queue-overview") {
		t.Fatalf("Jobs-free Grafana seed retained the Queue dashboard\n%s", seed)
	}
	platform := readJobsContractFile(t, root, "containers/observability/grafana/dashboards/platform-overview.json")
	var decoded any
	if err := json.Unmarshal([]byte(platform), &decoded); err != nil {
		t.Fatalf("Jobs-free Platform Overview is invalid JSON: %v\n%s", err, platform)
	}
	if strings.Contains(platform, "queue_jobs_") {
		t.Fatalf("Jobs-free Platform Overview retained Queue queries\n%s", platform)
	}
}

// TestRenderedMixedAppJobsSurfaceFollowsAppParticipation verifies shared Jobs support does not widen either App direction.
func TestRenderedMixedAppJobsSurfaceFollowsAppParticipation(t *testing.T) {
	tests := []struct {
		name      string
		config    *project.Config
		appJobs   map[string]bool
		envPrefix map[string]string
	}{
		{
			name: "named App only",
			config: &project.Config{
				ProjectName:  "Named Jobs",
				GoModuleName: "example.com/named-jobs",
				Render: project.RenderConfig{
					Components: project.Components{
						CLI: true, WebAPI: true, DatabaseSQLite: true, Storage: true,
					},
					ComponentContractVersion: project.CurrentComponentContractVersion,
				},
				Apps: map[string]project.AppConfig{
					"metrics-api": {Components: project.Components{CLI: true, WebAPI: true, Metrics: true}},
					"worker":      {Components: project.Components{CLI: true, Jobs: true}},
				},
			},
			appJobs: map[string]bool{
				project.DefaultAppName: false,
				"metrics-api":          false,
				"worker":               true,
			},
			envPrefix: map[string]string{"metrics-api": "METRICS_API", "worker": "WORKER"},
		},
		{
			name: "default App only",
			config: &project.Config{
				ProjectName:  "Default Jobs",
				GoModuleName: "example.com/default-jobs",
				Render: project.RenderConfig{
					Components: project.Components{
						CLI: true, WebAPI: true, Metrics: true, DatabaseSQLite: true, Storage: true, Jobs: true,
					},
					ComponentContractVersion: project.CurrentComponentContractVersion,
				},
				Apps: map[string]project.AppConfig{
					"api": {Components: project.Components{CLI: true, WebAPI: true}},
				},
			},
			appJobs: map[string]bool{
				project.DefaultAppName: true,
				"api":                  false,
			},
			envPrefix: map[string]string{"api": "API"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := renderJobsContractProject(t, test.config)
			for _, path := range []string{
				"internal/jobs/runtime.go",
				"internal/queues/manager_gen.go",
				"internal/observability/queue_observer.go",
			} {
				assertJobsContractPath(t, root, path, true)
			}

			for name, wantJobs := range test.appJobs {
				app := project.DefaultNamedApp(name)
				if name == project.DefaultAppName {
					app = project.DefaultApp()
				}
				appSource := readJobsContractFile(t, root, filepath.Join(app.WireDir, "app.go"))
				if got := strings.Contains(appSource, "func (a *App) Queue()"); got != wantJobs {
					t.Fatalf("App %s Queue API presence = %t, want %t\n%s", name, got, wantJobs, appSource)
				}
				assertJobsContractPath(t, root, filepath.Join(app.WireDir, "inject_jobs.go"), wantJobs)
				assertJobsContractPath(t, root, filepath.Join(app.WireDir, "inject_jobs_app.go"), wantJobs)
			}

			environment := readJobsContractFile(t, root, ".env")
			for name, prefix := range test.envPrefix {
				wantJobs := test.appJobs[name]
				token := prefix + "_QUEUE_DRIVER="
				if got := strings.Contains(environment, token); got != wantJobs {
					t.Fatalf("App %s Queue environment presence = %t, want %t\n%s", name, got, wantJobs, environment)
				}
			}
			for _, token := range []string{"QUEUE_DRIVER=workerpool", "QUEUE_SUPPORTED_DRIVERS=workerpool,redis"} {
				if !strings.Contains(environment, token) {
					t.Fatalf("mixed-App Jobs environment omitted %q\n%s", token, environment)
				}
			}
		})
	}
}

// renderJobsContractProject renders a full fixture beneath /tmp so ownership checks exercise real output selection.
func renderJobsContractProject(t *testing.T, config *project.Config) string {
	t.Helper()
	root, err := os.MkdirTemp("/tmp", "goforj-jobs-contract-")
	if err != nil {
		t.Fatalf("create Jobs contract render root: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	withJobsContractRoot(t, root, func() {
		if err := WriteYAML(".goforj.yml", *config); err != nil {
			t.Fatalf("write Jobs contract config: %v", err)
		}
		for name := range config.Apps {
			app := project.DefaultNamedApp(name)
			if err := os.MkdirAll(app.WireDir, 0o755); err != nil {
				t.Fatalf("create configured App layout for %s: %v", name, err)
			}
		}
		renderer := NewProjectRenderer(logger.NewSilentLogger())
		if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
			t.Fatalf("render Jobs contract project: %v", err)
		}
	})
	return root
}

// rerenderJobsContractProject exercises steady-state rendering against the first generated output.
func rerenderJobsContractProject(t *testing.T, root string) {
	t.Helper()
	withJobsContractRoot(t, root, func() {
		renderer := NewProjectRenderer(logger.NewSilentLogger())
		if err := renderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
			t.Fatalf("rerender Jobs contract project: %v", err)
		}
	})
}

// withJobsContractRoot scopes process-relative renderer operations to one isolated fixture.
func withJobsContractRoot(t *testing.T, root string, run func()) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to Jobs contract render root: %v", err)
	}
	defer func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()
	run()
}

// assertJobsContractPath verifies one generated artifact follows Jobs participation.
func assertJobsContractPath(t *testing.T, root string, path string, want bool) {
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

// readJobsContractFile reads one generated artifact from an isolated Jobs render.
func readJobsContractFile(t *testing.T, root string, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		t.Fatalf("read rendered %s: %v", path, err)
	}
	return string(content)
}
