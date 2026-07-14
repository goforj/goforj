package resources

import (
	"context"
	"testing"

	"github.com/goforj/goforj/project"
)

func TestRegistryForProjectResolvesBaseResources(t *testing.T) {
	config := &project.Config{}
	config.Render.Components.WebAPI = true
	config.Render.Components.WebUI = true
	config.Render.Components.DatabaseSQLite = true
	config.Render.Components.Events = true
	config.Render.Components.Mail = true
	config.Render.Components.Docker = true
	config.Render.Components.Observability = true
	config.Render.Components.Grafana = true

	env := map[string]string{
		"APP_URL":               "http://127.0.0.1:8080",
		"LIGHTHOUSE_URL":        "ws://127.0.0.1:7777/lighthouse/ws/agent",
		"API_SWAGGER_ENABLED":   "true",
		"MAILPIT_HTTP_PORT":     "18025",
		"OBSERVABILITY_VM_PORT": "18428",
		"GRAFANA_PORT":          "13001",
		"GRAFANA_ADMIN_USER":    "ops",
		"QUEUE_REPORTS_DRIVER":  "redis",
		"CACHE_SESSIONS_DRIVER": "memory",
		"STORAGE_PUBLIC_DRIVER": "local",
		"EVENTS_AUDIT_DRIVER":   "inproc",
	}

	resources, err := RegistryForProject(config, env).List(t.Context())
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}

	wantURLs := map[string]string{
		"app":              "http://127.0.0.1:8080",
		"api":              "http://127.0.0.1:8080",
		"swagger":          "http://127.0.0.1:8080/swagger",
		"mailpit":          "http://localhost:18025",
		"lighthouse":       "http://127.0.0.1:7777/lighthouse",
		"victoria-metrics": "http://localhost:18428",
		"grafana":          "http://localhost:13001",
	}
	for id, url := range wantURLs {
		resource, ok := resourceByID(resources, id)
		if !ok {
			t.Fatalf("missing resource %s in %#v", id, resources)
		}
		if resource.URL != url {
			t.Fatalf("%s URL = %q, want %q", id, resource.URL, url)
		}
	}
	api, ok := resourceByID(resources, "api")
	if !ok || api.Category != "api" || api.App != project.DefaultAppName || api.Runtime != "http" || api.Health != "http://127.0.0.1:8080/health" {
		t.Fatalf("api = %#v ok=%v", api, ok)
	}
	grafana, ok := resourceByID(resources, "grafana")
	if !ok || grafana.Auth != "ops / admin" || grafana.Owner != "goforj" {
		t.Fatalf("grafana = %#v ok=%v", grafana, ok)
	}
	for _, id := range []string{"database-default", "queue-reports", "cache-sessions", "storage-public", "events-audit"} {
		if _, ok := resourceByID(resources, id); !ok {
			t.Fatalf("missing named resource %s in %#v", id, resources)
		}
	}
	if got := resourceIDs(Filter(resources, Category("queue"), Runtime("jobs"))); !equalStrings(got, []string{"queue-reports"}) {
		t.Fatalf("queue resources = %#v", got)
	}
}

func TestRegistryHandlesDisabledAndMissingOptionalResources(t *testing.T) {
	config := &project.Config{}
	config.Render.Components.WebAPI = true
	env := map[string]string{
		"LIGHTHOUSE_ENABLED":  "false",
		"SWAGGER_ENABLED":     "false",
		"EVENTS_AUDIT_DRIVER": "redis",
	}

	resources, err := RegistryForProject(config, env).List(t.Context())
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if _, ok := resourceByID(resources, "lighthouse"); ok {
		t.Fatalf("disabled lighthouse present in %#v", resources)
	}
	if _, ok := resourceByID(resources, "swagger"); ok {
		t.Fatalf("disabled swagger present in %#v", resources)
	}
	if _, ok := resourceByID(resources, "events-audit"); ok {
		t.Fatalf("stale Events env resurrected a disabled resource in %#v", resources)
	}
	if app, ok := resourceByID(resources, "app"); !ok || app.URL != "http://localhost:3000" {
		t.Fatalf("app = %#v ok=%v", app, ok)
	}
}

// TestRegistryUsesNamedAppEventsEnvelope verifies shared Events resources remain visible when only a named App participates.
func TestRegistryUsesNamedAppEventsEnvelope(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"events-worker": {Components: project.Components{CLI: true, Events: true}},
		},
	}
	resources, err := RegistryForProject(config, map[string]string{"EVENTS_AUDIT_DRIVER": "inproc"}).List(t.Context())
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if _, ok := resourceByID(resources, "events-audit"); !ok {
		t.Fatalf("named Events App did not expose project Events resources: %#v", resources)
	}
	if config.Render.Components.Events {
		t.Fatal("resource discovery widened the default App Events selection")
	}
}

func TestRegistryOrdersAndFiltersResources(t *testing.T) {
	registry := NewRegistry(ResolverFunc(func(context.Context) ([]Resource, error) {
		return []Resource{
			{ID: "z", Name: "Zed", Category: "observability", Enabled: true, Priority: 30},
			{ID: "a", Name: "App", Category: "app", Enabled: true, Priority: 20},
			{ID: "hidden", Name: "Hidden", Category: "app", Enabled: false, Priority: 10},
			{ID: "api", Name: "API", Category: "app", Enabled: true, Priority: 10},
		}, nil
	}))

	resources, err := registry.List(t.Context())
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if got := resourceIDs(resources); !equalStrings(got, []string{"api", "a", "z"}) {
		t.Fatalf("ids = %#v", got)
	}
	if got := resourceIDs(Filter(resources, Category("app"))); !equalStrings(got, []string{"api", "a"}) {
		t.Fatalf("filtered ids = %#v", got)
	}
	resource, ok, err := registry.ByID(t.Context(), "z")
	if err != nil {
		t.Fatalf("by id: %v", err)
	}
	if !ok || resource.Name != "Zed" {
		t.Fatalf("resource = %#v ok=%v", resource, ok)
	}
}

func resourceByID(resources []Resource, id string) (Resource, bool) {
	for _, resource := range resources {
		if resource.ID == id {
			return resource, true
		}
	}
	return Resource{}, false
}

func resourceIDs(resources []Resource) []string {
	ids := make([]string, 0, len(resources))
	for _, resource := range resources {
		ids = append(ids, resource.ID)
	}
	return ids
}

func equalStrings(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
