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
		if projectComponents.Jobs {
			for _, app := range projectResourceApps(r.Config) {
				if !app.components.Jobs {
					continue
				}
				for _, name := range namedResourcesForApp(env, app.name, "QUEUE") {
					resources = append(resources, Resource{ID: queueResourceID(app.name, name), Name: name, Category: "queue", Description: "Named queue resource.", Enabled: true, Priority: 10, Source: "env", App: app.name, Runtime: "jobs", Owner: "goforj"})
				}
			}
		}
		if projectComponents.Cache {
			for _, name := range namedResources(env, "CACHE") {
				resources = append(resources, Resource{ID: "cache-" + name, Name: name, Category: "cache", Description: "Named cache resource.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Owner: "goforj"})
			}
		}
		if projectComponents.Storage {
			for _, name := range namedResources(env, "STORAGE") {
				resources = append(resources, Resource{ID: "storage-" + name, Name: name, Category: "storage", Description: "Named storage disk resource.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Owner: "goforj"})
			}
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
		if key == prefix+"_DRIVER" {
			continue
		}
		name := strings.TrimSuffix(strings.TrimPrefix(key, prefix+"_"), "_DRIVER")
		if name != "" {
			names = append(names, strings.ToLower(name))
		}
	}
	return uniqueSorted(names)
}

// projectResourceApp binds one App name to the component projection that owns its resource overlays.
type projectResourceApp struct {
	name       string
	components project.Components
}

// projectResourceApps returns deterministic App-local component projections for resource ownership.
func projectResourceApps(config *project.Config) []projectResourceApp {
	apps := []projectResourceApp{{
		name:       project.DefaultAppName,
		components: config.Render.Components.WithResolvedDependencies(),
	}}
	names := make([]string, 0, len(config.Apps))
	for name := range config.Apps {
		if name == "" || name == project.DefaultAppName {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		apps = append(apps, projectResourceApp{
			name:       name,
			components: project.NormalizeConfiguredAppComponents(config, config.Apps[name].Components),
		})
	}
	return apps
}

// namedResourcesForApp combines shared queue definitions with one participating App's overrides.
func namedResourcesForApp(env map[string]string, appName string, prefix string) []string {
	names := namedResources(env, prefix)
	if appName == "" || appName == project.DefaultAppName {
		return names
	}
	appPrefix := resourceAppEnvPrefix(appName)
	if appPrefix == "" {
		return names
	}
	return uniqueSorted(append(names, namedResources(env, appPrefix+"_"+prefix)...))
}

// queueResourceID preserves the established default-App IDs while namespacing sibling Apps.
func queueResourceID(appName string, queueName string) string {
	if appName == "" || appName == project.DefaultAppName {
		return "queue-" + queueName
	}
	return "queue-" + strings.ToLower(appName) + "-" + queueName
}

// resourceAppEnvPrefix mirrors the generated runtime convention for App environment overlays.
func resourceAppEnvPrefix(appName string) string {
	parts := strings.FieldsFunc(strings.TrimSpace(appName), func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '.'
	})
	for i, part := range parts {
		parts[i] = strings.ToUpper(part)
	}
	return strings.Join(parts, "_")
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
