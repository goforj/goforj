package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestRedisShellForAppFollowsAppCapabilities verifies sibling Apps do not inherit a Redis command solely from the project envelope.
func TestRedisShellForAppFollowsAppCapabilities(t *testing.T) {
	cacheComponents := project.Components{Cache: true}.WithResolvedDependencies()
	plan, err := project.DefaultResourcePlan(cacheComponents)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	if !redisShellForApp(plan, cacheComponents, false, resourceRenderValues{}) {
		t.Fatal("Redis shell was not projected for a Redis-capable Cache App")
	}
	if redisShellForApp(plan, project.Components{CLI: true}, false, resourceRenderValues{}) {
		t.Fatal("Redis shell leaked into a sibling App without a Redis-capable resource")
	}
	if redisShellForApp(plan, project.Components{CLI: true}, true, resourceRenderValues{RedisLocal: true}) {
		t.Fatal("Redis shell leaked into the default App from a named App's active local service")
	}
}

// TestRedisShellCommandWiringFollowsProjection verifies generated App fields and providers use the same per-App decision.
func TestRedisShellCommandWiringFollowsProjection(t *testing.T) {
	base := templateRenderConfig{
		Config:         &project.Config{GoModuleName: "example.com/redis-shell"},
		Components:     project.Components{CLI: true},
		App:            project.DefaultApp(),
		AppPackageName: "app",
		AppImportPath:  "app",
		WireImportPath: "app/wire",
	}
	for _, enabled := range []bool{false, true} {
		data := base
		data.RedisShell = enabled
		root := renderSharedTemplate(t, "app/root_cmd.go.tmpl", data)
		wire := renderSharedTemplate(t, "wire/inject_cmd.go.tmpl", data)
		assertFormattedGoTemplate(t, "app/root_cmd.go.tmpl", root)
		assertFormattedGoTemplate(t, "wire/inject_cmd.go.tmpl", wire)
		if got := strings.Contains(root, "RedisShellCmd"); got != enabled {
			t.Fatalf("RedisShell=%t root command marker = %t", enabled, got)
		}
		if got := strings.Contains(wire, "cmd.NewRedisShellCmd,"); got != enabled {
			t.Fatalf("RedisShell=%t Wire provider marker = %t", enabled, got)
		}
	}
}

// TestRedisShellPreparedDataDoesNotLeakNamedAppService verifies project-level Redis activity is narrowed before App command rendering.
func TestRedisShellPreparedDataDoesNotLeakNamedAppService(t *testing.T) {
	config := &project.Config{
		GoModuleName: "example.com/redis-shell",
		Render: project.RenderConfig{Components: project.Components{
			CLI:    true,
			Docker: true,
		}},
		Apps: map[string]project.AppConfig{
			"worker": {Components: project.Components{CLI: true, Cache: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan, err := project.DefaultResourcePlan(projectComponents)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	renderer := projectRendererForTest(t, config)
	renderer.resources.plan = plan
	renderer.resources.serviceIntent = project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	renderer.resources.serviceConsumers = []project.EffectiveResourceConsumer{
		{
			Resource:     project.ResourceCache,
			Consumer:     "worker:cache",
			Driver:       "redis",
			LocalService: true,
		},
	}

	for _, target := range []struct {
		app  project.App
		want bool
	}{
		{app: project.DefaultApp(), want: false},
		{app: project.DefaultNamedApp("worker"), want: true},
	} {
		data := renderer.workspace.templateDataForApp(config, target.app)
		prepared, err := renderer.prepareTemplateData(data)
		if err != nil {
			t.Fatalf("prepareTemplateData(%s) returned error: %v", target.app.Name, err)
		}
		got := prepared.(templateRenderConfig).RedisShell
		if got != target.want {
			t.Fatalf("App %s RedisShell = %t, want %t", target.app.Name, got, target.want)
		}
	}
}

// TestRedisShellForAppKeepsExplicitLocalServiceOnDefaultApp verifies owner-requested Redis remains reachable without leaking across App binaries.
func TestRedisShellForAppKeepsExplicitLocalServiceOnDefaultApp(t *testing.T) {
	resources := resourceRenderValues{RedisLocal: true, RedisLocalRequestedUnused: true}
	if !redisShellForApp(project.ResourcePlan{}, project.Components{}, true, resources) {
		t.Fatal("Redis shell was not projected for the default App's explicit local service")
	}
	if redisShellForApp(project.ResourcePlan{}, project.Components{}, false, resources) {
		t.Fatal("Redis shell leaked into a named App without a Redis-capable resource")
	}
}
