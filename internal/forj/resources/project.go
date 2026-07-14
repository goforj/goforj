package resources

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
)

// RegistryForProject returns a base registry for config and env-derived resources.
func RegistryForProject(config *project.Config, env map[string]string) Registry {
	return NewRegistry(ProjectResolver{Config: config, Env: env})
}

// ProjectResolver resolves static and config-derived project resources.
type ProjectResolver struct {
	Config *project.Config
	Env    map[string]string
}

// Resolve returns project resources derived from configuration and environment.
func (r ProjectResolver) Resolve(context.Context) ([]Resource, error) {
	env := r.Env
	components := project.Components{}
	projectComponents := project.Components{}
	if r.Config != nil {
		components = r.Config.Render.Components
		projectComponents = project.ProjectComponents(r.Config)
	}
	webEnabled := r.Config == nil || components.WebAPI || components.WebUI
	resources := []Resource{}

	if webEnabled {
		apiURL := resolveAPIURL(env)
		healthURL := strings.TrimRight(apiURL, "/") + "/health"
		resources = append(resources,
			Resource{ID: "app", Name: "App", Category: "app", URL: apiURL, Description: "Primary local app URL.", Enabled: apiURL != "", Priority: 10, Source: "config", App: project.DefaultAppName, Runtime: "http", Health: healthURL, Owner: "goforj"},
			Resource{ID: "api", Name: "API", Category: "api", URL: apiURL, Description: "Local HTTP API URL.", Enabled: apiURL != "" && components.WebAPI, Priority: 20, Source: "config", App: project.DefaultAppName, Runtime: "http", Health: healthURL, Owner: "goforj"},
		)
		if swaggerURL := resolveSwaggerURL(env); swaggerURL != "" {
			resources = append(resources, Resource{ID: "swagger", Name: "Swagger", Category: "docs", URL: swaggerURL, Description: "Generated Swagger UI for the local OpenAPI document.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Runtime: "http", Owner: "goforj"})
		}
	}

	if lighthouseURL := resolveLighthouseURL(env); lighthouseURL != "" {
		resources = append(resources, Resource{ID: "lighthouse", Name: "Lighthouse", Category: "operator", URL: lighthouseURL, Description: "Operator UI and local runtime inspection surface.", Enabled: true, Priority: 10, Source: "env", Runtime: "operator", Owner: "goforj"})
	}

	if r.Config != nil {
		if components.HasDatabase() {
			resources = append(resources, Resource{ID: "database-default", Name: "default", Category: "database", Description: "Default database connection.", Enabled: true, Priority: 10, Source: "component", App: project.DefaultAppName, Owner: "goforj"})
		}
		for _, name := range namedResources(env, "QUEUE") {
			resources = append(resources, Resource{ID: "queue-" + name, Name: name, Category: "queue", Description: "Named queue resource.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Runtime: "jobs", Owner: "goforj"})
		}
		for _, name := range namedResources(env, "CACHE") {
			resources = append(resources, Resource{ID: "cache-" + name, Name: name, Category: "cache", Description: "Named cache resource.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Owner: "goforj"})
		}
		for _, name := range namedResources(env, "STORAGE") {
			resources = append(resources, Resource{ID: "storage-" + name, Name: name, Category: "storage", Description: "Named storage disk resource.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Owner: "goforj"})
		}
		if projectComponents.Events {
			for _, name := range namedResources(env, "EVENTS") {
				resources = append(resources, Resource{ID: "events-" + name, Name: name, Category: "events", Description: "Named event bus resource.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Owner: "goforj"})
			}
		}
		if components.Mail && components.Docker {
			resources = append(resources, Resource{ID: "mailpit", Name: "Mailpit", Category: "mail", URL: urlWithPort(env, "MAILPIT_HTTP_PORT", "8025"), Description: "Local development inbox.", Enabled: true, Priority: 10, Source: "component", Owner: "goforj"})
		}
		if components.Observability {
			resources = append(resources, Resource{ID: "victoria-metrics", Name: "VictoriaMetrics", Category: "observability", URL: urlWithPort(env, "OBSERVABILITY_VM_PORT", "8428"), Description: "Local metrics database.", Enabled: true, Priority: 20, Source: "component", Runtime: "metrics", Owner: "goforj"})
		}
		if components.Grafana {
			admin := strings.TrimSpace(envValue(env, "GRAFANA_ADMIN_USER"))
			if admin == "" {
				admin = "admin"
			}
			resources = append(resources, Resource{ID: "grafana", Name: "Grafana", Category: "observability", URL: urlWithPort(env, "GRAFANA_PORT", "13001"), Description: fmt.Sprintf("Local Grafana dashboards. Default login: %s / admin.", admin), Enabled: true, Priority: 30, Source: "component", Runtime: "metrics", Auth: fmt.Sprintf("%s / admin", admin), Owner: "goforj"})
		}
	}

	return resources, nil
}

func resolveAPIURL(env map[string]string) string {
	if raw := strings.TrimSpace(envValue(env, "APP_URL")); raw != "" {
		return raw
	}
	if port := firstNonEmpty(envValue(env, "API_HTTP_PORT"), envValue(env, "PORT")); port != "" {
		return "http://localhost:" + port
	}
	return "http://localhost:3000"
}

func resolveSwaggerURL(env map[string]string) string {
	enabled := strings.ToLower(strings.TrimSpace(envValue(env, "API_SWAGGER_ENABLED")))
	if enabled == "" {
		enabled = strings.ToLower(strings.TrimSpace(envValue(env, "SWAGGER_ENABLED")))
	}
	if enabled == "false" || enabled == "0" || enabled == "off" || enabled == "no" {
		return ""
	}

	apiURL := strings.TrimSpace(resolveAPIURL(env))
	if apiURL == "" {
		return ""
	}
	return strings.TrimRight(apiURL, "/") + "/swagger"
}

func resolveLighthouseURL(env map[string]string) string {
	enabled := strings.ToLower(strings.TrimSpace(envValue(env, "LIGHTHOUSE_ENABLED")))
	if enabled == "false" || enabled == "0" || enabled == "off" || enabled == "no" {
		return ""
	}

	raw := strings.TrimSpace(envValue(env, "LIGHTHOUSE_URL"))
	if raw == "" {
		return "http://localhost:3000/lighthouse"
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "http://localhost:3000/lighthouse"
	}
	switch strings.ToLower(parsed.Scheme) {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	case "":
		parsed.Scheme = "http"
	}
	parsed.Path = "/lighthouse"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func urlWithPort(env map[string]string, key string, fallback string) string {
	port := firstNonEmpty(envValue(env, key), fallback)
	if port == "" {
		return ""
	}
	return "http://localhost:" + port
}

func envValue(env map[string]string, key string) string {
	if env == nil {
		return ""
	}
	return env[key]
}

func namedResources(env map[string]string, prefix string) []string {
	names := []string{}
	if strings.TrimSpace(envValue(env, prefix+"_DRIVER")) != "" {
		names = append(names, "default")
	}
	for key := range env {
		if !strings.HasPrefix(key, prefix+"_") || !strings.HasSuffix(key, "_DRIVER") {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, prefix+"_"), "_DRIVER")
		if name != "" {
			names = append(names, strings.ToLower(name))
		}
	}
	return uniqueSorted(names)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
