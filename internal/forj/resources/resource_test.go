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
	config.Render.Components.Cache = true
	config.Render.Components.Events = true
	config.Render.Components.Storage = true
	config.Render.Components.Jobs = true
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
		"LIGHTHOUSE_ENABLED":    "false",
		"SWAGGER_ENABLED":       "false",
		"CACHE_SESSIONS_DRIVER": "redis",
		"EVENTS_AUDIT_DRIVER":   "redis",
		"STORAGE_PUBLIC_DRIVER": "s3",
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
	if _, ok := resourceByID(resources, "cache-sessions"); ok {
		t.Fatalf("stale Cache env resurrected a disabled resource in %#v", resources)
	}
	if _, ok := resourceByID(resources, "storage-public"); ok {
		t.Fatalf("stale Storage env resurrected a disabled resource in %#v", resources)
	}
	if app, ok := resourceByID(resources, "app"); !ok || app.URL != "http://localhost:3000" {
		t.Fatalf("app = %#v ok=%v", app, ok)
	}
}

// TestRegistryKeepsPrimitiveResourcesAppLocal verifies each participating App owns shared and prefixed primitive resources.
func TestRegistryKeepsPrimitiveResourcesAppLocal(t *testing.T) {
	type expectedResource struct {
		id       string
		name     string
		category string
		app      string
	}
	tests := []struct {
		name   string
		config project.Config
		env    map[string]string
		want   []expectedResource
	}{
		{
			name: "default App only",
			config: project.Config{
				Render: project.RenderConfig{Components: project.Components{CLI: true, Cache: true, Events: true, Storage: true}},
				Apps: map[string]project.AppConfig{
					"api": {Components: project.Components{CLI: true}},
				},
			},
			env: map[string]string{
				"CACHE_SESSIONS_DRIVER":      "memory",
				"EVENTS_AUDIT_DRIVER":        "inproc",
				"STORAGE_PUBLIC_DRIVER":      "local",
				"API_CACHE_PRIVATE_DRIVER":   "redis",
				"API_EVENTS_PRIVATE_DRIVER":  "redis",
				"API_STORAGE_PRIVATE_DRIVER": "s3",
			},
			want: []expectedResource{
				{id: "cache-sessions", name: "sessions", category: "cache", app: project.DefaultAppName},
				{id: "events-audit", name: "audit", category: "events", app: project.DefaultAppName},
				{id: "storage-public", name: "public", category: "storage", app: project.DefaultAppName},
			},
		},
		{
			name: "named App only",
			config: project.Config{
				Render: project.RenderConfig{Components: project.Components{CLI: true}},
				Apps: map[string]project.AppConfig{
					"worker": {Components: project.Components{CLI: true, Cache: true, Events: true, Storage: true}},
				},
			},
			env: map[string]string{
				"CACHE_DRIVER":                 "memory",
				"WORKER_CACHE_SESSIONS_DRIVER": "redis",
				"EVENTS_DRIVER":                "inproc",
				"WORKER_EVENTS_AUDIT_DRIVER":   "redis",
				"STORAGE_DRIVER":               "local",
				"WORKER_STORAGE_PUBLIC_DRIVER": "s3",
			},
			want: []expectedResource{
				{id: "cache-worker-default", name: "default", category: "cache", app: "worker"},
				{id: "cache-worker-sessions", name: "sessions", category: "cache", app: "worker"},
				{id: "events-worker-audit", name: "audit", category: "events", app: "worker"},
				{id: "events-worker-default", name: "default", category: "events", app: "worker"},
				{id: "storage-worker-default", name: "default", category: "storage", app: "worker"},
				{id: "storage-worker-public", name: "public", category: "storage", app: "worker"},
			},
		},
		{
			name: "disjoint and multi-App ownership",
			config: project.Config{
				Render: project.RenderConfig{Components: project.Components{CLI: true}},
				Apps: map[string]project.AppConfig{
					"api":          {Components: project.Components{CLI: true}},
					"cache-reader": {Components: project.Components{CLI: true, Cache: true}},
					"cache-writer": {Components: project.Components{CLI: true, Cache: true}},
					"event-worker": {Components: project.Components{CLI: true, Events: true}},
					"files":        {Components: project.Components{CLI: true, Storage: true}},
				},
			},
			env: map[string]string{
				"CACHE_DRIVER":                        "memory",
				"CACHE_READER_CACHE_SESSIONS_DRIVER":  "redis",
				"CACHE_WRITER_CACHE_RESULTS_DRIVER":   "redis",
				"EVENTS_AUDIT_DRIVER":                 "inproc",
				"EVENT_WORKER_EVENTS_INTERNAL_DRIVER": "redis",
				"STORAGE_PUBLIC_DRIVER":               "local",
				"FILES_STORAGE_ARCHIVE_DRIVER":        "s3",
				"API_CACHE_STALE_DRIVER":              "redis",
				"CACHE_READER_EVENTS_STALE_DRIVER":    "redis",
				"EVENT_WORKER_STORAGE_STALE_DRIVER":   "s3",
			},
			want: []expectedResource{
				{id: "cache-cache-reader-default", name: "default", category: "cache", app: "cache-reader"},
				{id: "cache-cache-reader-sessions", name: "sessions", category: "cache", app: "cache-reader"},
				{id: "cache-cache-writer-default", name: "default", category: "cache", app: "cache-writer"},
				{id: "cache-cache-writer-results", name: "results", category: "cache", app: "cache-writer"},
				{id: "events-event-worker-audit", name: "audit", category: "events", app: "event-worker"},
				{id: "events-event-worker-internal", name: "internal", category: "events", app: "event-worker"},
				{id: "storage-files-archive", name: "archive", category: "storage", app: "files"},
				{id: "storage-files-public", name: "public", category: "storage", app: "files"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuredDefaultComponents := test.config.Render.Components
			resources, err := RegistryForProject(&test.config, test.env).List(t.Context())
			if err != nil {
				t.Fatalf("list resources: %v", err)
			}
			primitiveResources := append(Filter(resources, Category("cache")), Filter(resources, Category("events"))...)
			primitiveResources = append(primitiveResources, Filter(resources, Category("storage"))...)
			if len(primitiveResources) != len(test.want) {
				t.Fatalf("primitive resources = %#v, want %#v", primitiveResources, test.want)
			}
			for _, want := range test.want {
				resource, ok := resourceByID(resources, want.id)
				if !ok || resource.Name != want.name || resource.Category != want.category || resource.App != want.app {
					t.Errorf("resource %s = %#v ok=%v, want name=%q category=%q App=%q", want.id, resource, ok, want.name, want.category, want.app)
				}
			}
			if test.config.Render.Components != configuredDefaultComponents {
				t.Fatal("resource discovery changed the configured default App components")
			}
		})
	}
}

// TestRegistryIgnoresStaleDisabledQueueResources verifies Queue env cannot invent Background Jobs participation.
func TestRegistryIgnoresStaleDisabledQueueResources(t *testing.T) {
	config := &project.Config{Render: project.RenderConfig{Components: project.Components{CLI: true}}}
	resources, err := RegistryForProject(config, map[string]string{
		"QUEUE_DRIVER":           "redis",
		"QUEUE_REPORTS_DRIVER":   "redis",
		"WORKER_QUEUE_DRIVER":    "redis",
		"WORKER_QUEUE_FAST_NAME": "fast",
	}).List(t.Context())
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}
	if queues := Filter(resources, Category("queue")); len(queues) != 0 {
		t.Fatalf("stale Queue env resurrected disabled Background Jobs resources: %#v", queues)
	}
}

// TestRegistryKeepsQueueResourcesAppLocal verifies named Jobs Apps own their logical queues without widening siblings.
func TestRegistryKeepsQueueResourcesAppLocal(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"api":    {Components: project.Components{CLI: true, WebAPI: true}},
			"worker": {Components: project.Components{CLI: true, Jobs: true}},
		},
	}
	resources, err := RegistryForProject(config, map[string]string{
		"QUEUE_DRIVER":               "workerpool",
		"QUEUE_REPORTS_DRIVER":       "workerpool",
		"API_QUEUE_EXPORTS_DRIVER":   "redis",
		"WORKER_QUEUE_EMAILS_DRIVER": "redis",
	}).List(t.Context())
	if err != nil {
		t.Fatalf("list resources: %v", err)
	}

	for _, id := range []string{"queue-worker-default", "queue-worker-emails", "queue-worker-reports"} {
		resource, ok := resourceByID(resources, id)
		if !ok || resource.App != "worker" || resource.Runtime != "jobs" {
			t.Fatalf("named App queue %s = %#v ok=%v resources=%#v", id, resource, ok, resources)
		}
	}
	for _, id := range []string{"queue-default", "queue-reports", "queue-api-exports"} {
		if _, ok := resourceByID(resources, id); ok {
			t.Fatalf("Queue resource %s leaked onto a disabled App: %#v", id, resources)
		}
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
