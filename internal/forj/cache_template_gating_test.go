package forj

import (
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestCacheTemplatesFollowAppAndProjectParticipation verifies App APIs and shared runtime source use their respective component envelopes.
func TestCacheTemplatesFollowAppAndProjectParticipation(t *testing.T) {
	tests := []struct {
		name         string
		defaultCache bool
		workerCache  bool
	}{
		{name: "all Apps disabled"},
		{name: "named App only", workerCache: true},
		{name: "default App only", defaultCache: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := &project.Config{
				GoModuleName: "example.com/cache-projection",
				Render: project.RenderConfig{Components: project.Components{
					CLI: true, Cache: test.defaultCache,
				}},
				Apps: map[string]project.AppConfig{
					"worker": {Components: project.Components{CLI: true, Cache: test.workerCache}},
				},
			}
			apps := []struct {
				app     project.App
				enabled bool
			}{
				{app: project.DefaultApp(), enabled: test.defaultCache},
				{app: project.DefaultNamedApp("worker"), enabled: test.workerCache},
			}

			for _, target := range apps {
				t.Run(target.app.Name, func(t *testing.T) {
					components := appRenderComponents(config, target.app)
					data := appTemplateDataForProjectionTest(config, target.app, components)
					data.HelpFormatterFunc = "FrameworkFormatter"
					sources := map[string]string{
						"app/commands.go.tmpl":                            renderSharedTemplate(t, "app/commands.go.tmpl", data),
						"app/root_cmd.go.tmpl":                            renderSharedTemplate(t, "app/root_cmd.go.tmpl", data),
						"internal/http/lighthouse.go.tmpl":                renderSharedTemplate(t, "internal/http/lighthouse.go.tmpl", data),
						"internal/http/readiness_checks.go.tmpl":          renderSharedTemplate(t, "internal/http/readiness_checks.go.tmpl", data),
						"internal/jobs/benchmark_run_cmd.go.tmpl":         renderSharedTemplate(t, "internal/jobs/benchmark_run_cmd.go.tmpl", data),
						"internal/jobs/lighthouse.go.tmpl":                renderSharedTemplate(t, "internal/jobs/lighthouse.go.tmpl", data),
						"internal/jobs/lighthouse_benchmark.go.tmpl":      renderSharedTemplate(t, "internal/jobs/lighthouse_benchmark.go.tmpl", data),
						"internal/metrics/cache_metrics_gen.go.tmpl":      renderSharedTemplate(t, "internal/metrics/cache_metrics_gen.go.tmpl", data),
						"internal/metrics/cache_metrics_gen_test.go.tmpl": renderSharedTemplate(t, "internal/metrics/cache_metrics_gen_test.go.tmpl", data),
						"internal/metrics/manager.go.tmpl":                renderSharedTemplate(t, "internal/metrics/manager.go.tmpl", data),
						"internal/metrics/manager_test.go.tmpl":           renderSharedTemplate(t, "internal/metrics/manager_test.go.tmpl", data),
						"wire/app.go.tmpl":                                renderSharedTemplate(t, "wire/app.go.tmpl", data),
						"wire/inject_cmd.go.tmpl":                         renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data),
						"wire/inject_http.go.tmpl":                        renderSharedTemplate(t, "wire/inject_http.go.tmpl", data),
						"wire/inject_jobs.go.tmpl":                        renderSharedTemplate(t, "wire/inject_jobs.go.tmpl", data),
					}
					for path, source := range sources {
						assertFormattedGoTemplate(t, path, source)
					}

					for _, marker := range []string{
						`"github.com/goforj/cache"`,
						`"example.com/cache-projection/internal/caches"`,
						"func (a *App) Cache() *cache.Cache",
						"func (a *App) Caches() *caches.Manager",
						"cacheManager *caches.Manager",
					} {
						assertTemplateMarker(t, "wire/app.go.tmpl", sources["wire/app.go.tmpl"], marker, target.enabled)
					}
					assertTemplateMarker(t, "app/root_cmd.go.tmpl", sources["app/root_cmd.go.tmpl"], "CacheShellCmd", target.enabled)
					assertTemplateMarker(t, "wire/inject_cmd.go.tmpl", sources["wire/inject_cmd.go.tmpl"], "cmd.NewCacheShellCmd", target.enabled)
					if strings.Contains(sources["app/commands.go.tmpl"], "CacheShellCmd") {
						t.Fatalf("app-owned commands retained Cache wiring:\n%s", sources["app/commands.go.tmpl"])
					}

					projectCache := test.defaultCache || test.workerCache
					assertTemplateMarker(t, "internal/http/lighthouse.go.tmpl", sources["internal/http/lighthouse.go.tmpl"], `"example.com/cache-projection/internal/caches"`, projectCache)
					assertTemplateMarker(t, "internal/http/lighthouse.go.tmpl", sources["internal/http/lighthouse.go.tmpl"], `"github.com/goforj/cache"`, projectCache)
					assertTemplateMarker(t, "internal/http/lighthouse.go.tmpl", sources["internal/http/lighthouse.go.tmpl"], `RegisterCommand("cache:list"`, projectCache)
					assertTemplateMarker(t, "internal/http/lighthouse.go.tmpl", sources["internal/http/lighthouse.go.tmpl"], "func NewCachedLighthouseRuntime", projectCache)
					assertTemplateMarker(t, "internal/http/readiness_checks.go.tmpl", sources["internal/http/readiness_checks.go.tmpl"], "internal/caches", false)
					assertTemplateMarker(t, "internal/metrics/manager.go.tmpl", sources["internal/metrics/manager.go.tmpl"], "func (m *Manager) RecordCacheOperation", false)
					assertTemplateMarker(t, "internal/metrics/cache_metrics_gen.go.tmpl", sources["internal/metrics/cache_metrics_gen.go.tmpl"], "func (m *Manager) RecordCacheOperation", true)
					assertTemplateMarker(t, "internal/metrics/manager.go.tmpl", sources["internal/metrics/manager.go.tmpl"], `Name: "cache.operations"`, projectCache)
					assertTemplateMarker(t, "internal/metrics/manager_test.go.tmpl", sources["internal/metrics/manager_test.go.tmpl"], "func TestRecordCacheOperationTracksLabeledSeries", false)
					assertTemplateMarker(t, "internal/metrics/cache_metrics_gen_test.go.tmpl", sources["internal/metrics/cache_metrics_gen_test.go.tmpl"], "func TestRecordCacheOperationTracksLabeledSeries", true)

					assertTemplateMarker(t, "wire/inject_http.go.tmpl", sources["wire/inject_http.go.tmpl"], "http.NewCachedLighthouseRuntime", target.enabled)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", sources["wire/inject_http.go.tmpl"], "cacheManager *caches.Manager", target.enabled)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", sources["wire/inject_http.go.tmpl"], "for _, check := range cacheManager.ReadinessChecks()", target.enabled)
					assertTemplateMarker(t, "wire/inject_http.go.tmpl", sources["wire/inject_http.go.tmpl"], "(*caches.Manager)(nil)", false)
					assertTemplateMarker(t, "wire/inject_jobs.go.tmpl", sources["wire/inject_jobs.go.tmpl"], "jobs.NewCachedBenchmarkRunCmd", target.enabled)
					assertTemplateMarker(t, "wire/inject_jobs.go.tmpl", sources["wire/inject_jobs.go.tmpl"], "jobs.NewCachedLighthouseRuntime", target.enabled)
					assertTemplateMarker(t, "wire/inject_jobs.go.tmpl", sources["wire/inject_jobs.go.tmpl"], "(*caches.Manager)(nil)", false)
					assertTemplateMarker(t, "internal/jobs/benchmark_run_cmd.go.tmpl", sources["internal/jobs/benchmark_run_cmd.go.tmpl"], "func NewCachedBenchmarkRunCmd", projectCache)
					assertTemplateMarker(t, "internal/jobs/lighthouse.go.tmpl", sources["internal/jobs/lighthouse.go.tmpl"], "func NewCachedLighthouseRuntime", projectCache)
					cacheFallback := "if value == \"\" {\n\t\tif r.caches != nil {\n\t\t\treturn \"cache\"\n\t\t}\n\t\treturn \"queue\"\n\t}"
					assertTemplateMarker(t, "internal/jobs/lighthouse_benchmark.go.tmpl", sources["internal/jobs/lighthouse_benchmark.go.tmpl"], cacheFallback, projectCache)
					queueFallback := "if value == \"\" {\n\t\treturn \"queue\"\n\t}"
					assertTemplateMarker(t, "internal/jobs/lighthouse_benchmark.go.tmpl", sources["internal/jobs/lighthouse_benchmark.go.tmpl"], queueFallback, !projectCache)
				})
			}

			projectEnabled := test.defaultCache || test.workerCache
			sharedData := appTemplateDataForProjectionTest(config, project.DefaultApp(), config.Render.Components)
			runtimeSources := map[string]string{
				"internal/runtime/about.go.tmpl":     renderSharedTemplate(t, "internal/runtime/about.go.tmpl", sharedData),
				"internal/runtime/discovery.go.tmpl": renderSharedTemplate(t, "internal/runtime/discovery.go.tmpl", sharedData),
			}
			for path, source := range runtimeSources {
				assertFormattedGoTemplate(t, path, source)
			}
			for _, marker := range []string{
				"func DiscoverCacheInstances(",
				"if !CurrentApp().Components.Cache",
				"func NormalizeCacheDriver(",
			} {
				assertTemplateMarker(t, "internal/runtime/discovery.go.tmpl", runtimeSources["internal/runtime/discovery.go.tmpl"], marker, projectEnabled)
			}
			for _, marker := range []string{
				"aboutCacheRootKeys = []string",
				"type AboutCache struct",
				`json:"caches,omitempty"`,
				"report.Caches = aboutCacheReports()",
				`Title: "Caches"`,
				"func aboutCacheConnections(",
				`components = append(components, "cache")`,
				"func aboutCacheReports(",
				`case "CACHE":`,
				"func aboutCacheDetails(",
			} {
				assertTemplateMarker(t, "internal/runtime/about.go.tmpl", runtimeSources["internal/runtime/about.go.tmpl"], marker, projectEnabled)
			}
		})
	}
}

// TestRemoveLastCacheAppReconcilesSharedSurface verifies supported Cache removal clears generated code and framework environment assignments.
func TestRemoveLastCacheAppReconcilesSharedSurface(t *testing.T) {
	usePrimitiveRendererRoot(t)
	app := project.DefaultNamedApp("worker")
	config := &project.Config{
		ProjectName:  "Cache Removal",
		GoModuleName: "example.test/cache-removal",
		Render: project.RenderConfig{
			Components:               project.Components{CLI: true},
			ComponentContractVersion: project.CurrentComponentContractVersion,
		},
		Apps: map[string]project.AppConfig{
			app.Name: {Components: project.Components{CLI: true, Cache: true}},
		},
	}
	if err := writeProjectConfig(".goforj.yml", config); err != nil {
		t.Fatalf("write Cache removal config: %v", err)
	}
	writePrimitiveRendererFile(t, app.Entrypoint, "package main\n")
	writePrimitiveRendererFile(t, filepath.Join(app.AppDir, "owner.go"), "package workerapp\n")
	environment := strings.Join([]string{
		"OWNER_SENTINEL=keep",
		"METRICS_CACHE_ENABLED=true",
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"CACHE_PREFIX=app",
		"CACHE_DEFAULT_TTL_SECONDS=300",
		"CACHE_MEMORY_CLEANUP_SECONDS=600",
		"WORKER_CACHE_DRIVER=memory",
		"CACHE_REPORTS_DRIVER=redis",
		"",
	}, "\n")
	writePrimitiveRendererFile(t, ".env", environment)
	writePrimitiveRendererFile(t, ".env.example", environment)
	cacheArtifacts := []string{
		filepath.Join("internal", "caches", "README.md"),
		filepath.Join("internal", "caches", "accessors_gen.go"),
		filepath.Join("internal", "caches", "manager_gen.go"),
		filepath.Join("internal", "cmd", "cache_shell_cmd.go"),
		filepath.Join("internal", "metrics", "cache_metrics_gen.go"),
		filepath.Join("containers", "observability", "grafana", "dashboards", "cache-overview.json"),
	}
	for _, path := range cacheArtifacts {
		writePrimitiveRendererFile(t, path, "generated Cache artifact\n")
	}

	renderer := NewProjectRenderer(logger.NewSilentLogger())
	result, err := renderer.RemoveApp(app)
	if err != nil {
		t.Fatalf("remove final Cache App: %v", err)
	}
	if !result.Changed() {
		t.Fatal("final Cache App removal reported no changes")
	}
	for _, path := range cacheArtifacts {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("generated Cache artifact %s remains: %v", path, err)
		}
	}
	loaded, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("reload Cache removal config: %v", err)
	}
	if project.ProjectComponents(loaded).Cache {
		t.Fatalf("removed App still contributes Cache: %#v", loaded.Apps)
	}
	for _, path := range []string{".env", ".env.example"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read reconciled %s: %v", path, err)
		}
		text := string(source)
		for _, removed := range []string{"METRICS_CACHE_ENABLED=", "CACHE_DRIVER=", "CACHE_SUPPORTED_DRIVERS=", "CACHE_PREFIX=", "CACHE_DEFAULT_TTL_SECONDS=", "CACHE_MEMORY_CLEANUP_SECONDS=", "WORKER_CACHE_DRIVER="} {
			if strings.Contains(text, removed) {
				t.Fatalf("reconciled %s retained %q:\n%s", path, removed, text)
			}
		}
		for _, preserved := range []string{"OWNER_SENTINEL=keep", "CACHE_REPORTS_DRIVER=redis"} {
			if !strings.Contains(text, preserved) {
				t.Fatalf("reconciled %s removed owner assignment %q:\n%s", path, preserved, text)
			}
		}
	}
}

// TestRemoveLegacyCacheShellCommandSourceMigratesOnlyTheGeneratedShape protects customized owner code during the command ownership move.
func TestRemoveLegacyCacheShellCommandSourceMigratesOnlyTheGeneratedShape(t *testing.T) {
	generated := `package app

type Commands struct {
	AboutCmd cmd.AboutCmd ` + "`cmd:\"\"`" + `
	CacheShellCmd cmd.CacheShellCmd ` + "`cmd:\"\"`" + `
}

func NewCommands(
	aboutCmd *cmd.AboutCmd,
	cacheShellCmd *cmd.CacheShellCmd,
) *Commands {
	return &Commands{
		AboutCmd: *aboutCmd,
		CacheShellCmd: *cacheShellCmd,
	}
}

`
	formattedGenerated, err := format.Source([]byte(generated))
	if err != nil {
		t.Fatalf("format generated Cache command owner: %v", err)
	}
	tests := []struct {
		name        string
		source      string
		wantChanged bool
		wantError   bool
	}{
		{name: "generated owner", source: generated, wantChanged: true},
		{name: "gofmt-aligned generated owner", source: string(formattedGenerated), wantChanged: true},
		{name: "already neutral", source: "package app\n\ntype Commands struct{}\n"},
		{name: "customized field", source: "package app\n\ntype Commands struct {\n\tLegacyCache cmd.CacheShellCmd `cmd:\"\"`\n}\n", wantError: true},
		{name: "owner comment", source: strings.Replace(generated, "CacheShellCmd cmd.CacheShellCmd", "CacheShellCmd cmd.CacheShellCmd // keep this owner note", 1), wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updated, changed, err := removeLegacyCacheShellCommandSource("commands.go", []byte(test.source))
			if got := err != nil; got != test.wantError {
				t.Fatalf("migration error presence = %t, want %t: %v", got, test.wantError, err)
			}
			if test.wantError {
				return
			}
			if changed != test.wantChanged {
				t.Fatalf("migration changed = %t, want %t", changed, test.wantChanged)
			}
			if test.wantChanged && (strings.Contains(string(updated), "CacheShellCmd") || !strings.Contains(string(updated), "AboutCmd")) {
				t.Fatalf("migration changed more than the generated Cache lines:\n%s", updated)
			}
		})
	}
}
