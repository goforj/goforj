package forj

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestStorageHTTPSharedSurfaceUsesProjectEnvelope verifies a named App can widen shared HTTP support without forcing Storage onto its sibling.
func TestStorageHTTPSharedSurfaceUsesProjectEnvelope(t *testing.T) {
	config := storageHTTPProjectionConfig(false, true)
	data := templateRenderConfig{
		Config:            config,
		Components:        config.Render.Components,
		ProjectComponents: project.ProjectComponents(config),
	}
	sources := renderStorageHTTPSharedSources(t, data)
	for path, source := range sources {
		assertFormattedGoTemplate(t, path, source)
	}

	for path, markers := range map[string][]string{
		"internal/http/lighthouse.go.tmpl": {
			`"github.com/goforj/storage"`,
			`"example.com/storage-http/internal/storages"`,
			"storage *storages.Manager",
			"if len(r.storageDisks()) > 0 {",
			`RegisterCommand("storage:list"`,
		},
		"internal/http/server.go.tmpl": {
			`"example.com/storage-http/internal/storages"`,
			"storage *storages.Manager",
			"if s.storage != nil {",
			"StorageDownload: storageDownload",
		},
		"internal/http/readiness_checks.go.tmpl": {
			`"example.com/storage-http/internal/storages"`,
			"storage *storages.Manager",
			"for _, check := range storage.ReadinessChecks()",
		},
		"internal/http/health.go.tmpl": {
			`case "storage":`,
			"func storageDriver(",
		},
		"internal/lighthouse/server.go.tmpl": {
			"StorageDownload func(",
			"if config.StorageDownload != nil {",
			`apiGroup.GET("/storage/download"`,
		},
	} {
		for _, marker := range markers {
			assertTemplateMarker(t, path, sources[path], marker, true)
		}
	}

	offConfig := storageHTTPProjectionConfig(false, false)
	offData := templateRenderConfig{
		Config:            offConfig,
		Components:        offConfig.Render.Components,
		ProjectComponents: project.ProjectComponents(offConfig),
	}
	offSources := renderStorageHTTPSharedSources(t, offData)
	for path, source := range offSources {
		assertFormattedGoTemplate(t, path, source)
	}

	for path, markers := range map[string][]string{
		"internal/http/lighthouse.go.tmpl": {
			`"github.com/goforj/storage"`,
			`internal/storages`,
			"storage *storages.Manager",
			`RegisterCommand("storage:list"`,
			"storageDisks(",
		},
		"internal/http/server.go.tmpl": {
			`internal/storages`,
			"storage *storages.Manager",
			"StorageDownload:",
			"lighthouseStorageDownload",
		},
		"internal/http/readiness_checks.go.tmpl": {
			`internal/storages`,
			"storage *storages.Manager",
			"storage.ReadinessChecks()",
		},
		"internal/http/health.go.tmpl": {
			`case "storage":`,
			"func storageDriver(",
		},
		"internal/lighthouse/server.go.tmpl": {
			"StorageDownload",
			`/storage/download`,
		},
	} {
		for _, marker := range markers {
			assertTemplateMarker(t, path, offSources[path], marker, false)
		}
	}
}

// TestStorageHTTPServerCallsFollowProjectConstructorShape verifies generated HTTP tests omit the Storage dependency when the shared server does.
func TestStorageHTTPServerCallsFollowProjectConstructorShape(t *testing.T) {
	paths := []string{
		"internal/http/health_test.go.tmpl",
		"internal/http/metrics_test.go.tmpl",
		"internal/http/inspects_bench_test.go.tmpl",
		"internal/http/inspect_child_event_test.go.tmpl",
		"internal/http/runtime_bench_test.go.tmpl",
		"internal/http/swagger_test.go.tmpl",
	}

	for _, storageEnabled := range []bool{false, true} {
		components := project.Components{
			CLI:     true,
			WebAPI:  true,
			Metrics: true,
			Cache:   true,
			Storage: storageEnabled,
		}
		config := &project.Config{
			GoModuleName: "example.com/storage-http",
			Render:       project.RenderConfig{Components: components},
		}
		data := templateRenderConfig{
			Config:            config,
			Components:        components,
			ProjectComponents: project.ProjectComponents(config),
		}
		wantArguments := 7
		if storageEnabled {
			wantArguments++
		}

		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				source := renderSharedTemplate(t, path, data)
				assertFormattedGoTemplate(t, path, source)
				assertNewServerArgumentCount(t, path, source, wantArguments)
			})
		}
	}
}

// storageHTTPProjectionConfig creates two HTTP Apps with independently selected Storage participation.
func storageHTTPProjectionConfig(defaultStorage bool, workerStorage bool) *project.Config {
	return &project.Config{
		GoModuleName: "example.com/storage-http",
		Render: project.RenderConfig{Components: project.Components{
			CLI: true, WebAPI: true, Cache: true, Storage: defaultStorage,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{
				CLI: true, WebAPI: true, Cache: true, Storage: workerStorage,
			}},
		},
	}
}

// renderStorageHTTPSharedSources renders the shared templates whose compile shape depends on the project Storage envelope.
func renderStorageHTTPSharedSources(t *testing.T, data templateRenderConfig) map[string]string {
	t.Helper()
	paths := []string{
		"internal/http/lighthouse.go.tmpl",
		"internal/http/server.go.tmpl",
		"internal/http/readiness_checks.go.tmpl",
		"internal/http/health.go.tmpl",
		"internal/lighthouse/server.go.tmpl",
	}
	sources := make(map[string]string, len(paths))
	for _, path := range paths {
		sources[path] = renderSharedTemplate(t, path, data)
	}
	return sources
}

// assertNewServerArgumentCount verifies every generated HTTP server constructor call follows the shared constructor signature.
func assertNewServerArgumentCount(t *testing.T, path string, source string, want int) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("parse rendered template %s: %v\n%s", path, err, source)
	}
	calls := 0
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		name, ok := call.Fun.(*ast.Ident)
		if !ok || name.Name != "NewServer" {
			return true
		}
		calls++
		if len(call.Args) != want {
			t.Errorf("template %s NewServer argument count = %d, want %d", path, len(call.Args), want)
		}
		return true
	})
	if calls == 0 {
		t.Fatalf("template %s rendered no NewServer calls", path)
	}
}
