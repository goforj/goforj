package resources

import (
	"context"
	"net"
	"net/url"
	"sort"
	"strings"

	"github.com/goforj/str/v2"

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
	if r.Config != nil {
		components = r.Config.Render.Components
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
		resources = append(resources, Resource{ID: "lighthouse", Name: "Lighthouse", Category: "operator", URL: lighthouseURL, Description: "Operator UI and local runtime inspection surface.", Enabled: true, Priority: 10, Source: "env", App: project.DefaultAppName, Runtime: "operator", Owner: "goforj"})
	}

	if r.Config != nil {
		if components.HasDatabase() {
			resources = append(resources, Resource{ID: "database-default", Name: "default", Category: "database", Description: "Default database connection.", Enabled: true, Priority: 10, Source: "component", App: project.DefaultAppName, Owner: "goforj"})
		}
		resourceApps := projectResourceApps(r.Config)
		for _, app := range resourceApps {
			resources = append(resources, primitiveResourcesForApp(env, resourceApps, app)...)
		}
		if components.Docker && composeProfileEnabled(env, "mailpit") {
			resources = append(resources, Resource{ID: "mailpit", Name: "Mailpit", Category: "mail", URL: urlWithPort(env, "MAILPIT_HTTP_PORT", "8025"), Description: "Local development inbox.", Enabled: true, Priority: 10, Source: "profile", Service: "mailpit", Owner: "goforj"})
		}
		if components.Docker && (composeProfileEnabled(env, "victoriametrics") || composeProfileEnabled(env, "grafana")) {
			resources = append(resources, Resource{ID: "victoria-metrics", Name: "VictoriaMetrics", Category: "observability", URL: urlWithPort(env, "OBSERVABILITY_VM_PORT", "8428"), Description: "Local metrics database.", Enabled: true, Priority: 20, Source: "profile", Service: "victoriametrics", Runtime: "metrics", Owner: "goforj"})
		}
		if components.Docker && composeProfileEnabled(env, "grafana") {
			admin := strings.TrimSpace(envValue(env, "GRAFANA_ADMIN_USER"))
			if admin == "" {
				admin = "admin"
			}
			resources = append(resources, Resource{ID: "grafana", Name: "Grafana", Category: "observability", URL: urlWithPort(env, "GRAFANA_PORT", "13001"), Description: "Local Grafana dashboards.", Enabled: true, Priority: 30, Source: "profile", Service: "grafana", Runtime: "metrics", Auth: admin, Owner: "goforj"})
		}
	}

	return resources, nil
}

// resolveAPIURL keeps explicit custom origins while replacing local placeholders with an assigned host address.
func resolveAPIURL(env map[string]string) string {
	if raw := strings.TrimSpace(envValue(env, "APP_URL")); raw != "" {
		return replaceLocalURLHost(raw, resourceURLHost(env))
	}
	if port := firstNonEmpty(envValue(env, "API_HTTP_PORT"), envValue(env, "PORT")); port != "" {
		return urlForHostPort("http", resourceURLHost(env), port)
	}
	return urlForHostPort("http", resourceURLHost(env), "3000")
}

// resolveSwaggerURL derives the documentation path from the same externally reachable API origin.
func resolveSwaggerURL(env map[string]string) string {
	enabled := str.Of(envValue(env, "API_SWAGGER_ENABLED")).Trim().ToLower().String()
	if enabled == "" {
		enabled = str.Of(envValue(env, "SWAGGER_ENABLED")).Trim().ToLower().String()
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

// resolveLighthouseURL projects websocket configuration onto its browser-facing HTTP endpoint.
func resolveLighthouseURL(env map[string]string) string {
	enabled := str.Of(envValue(env, "LIGHTHOUSE_ENABLED")).Trim().ToLower().String()
	if enabled == "false" || enabled == "0" || enabled == "off" || enabled == "no" {
		return ""
	}

	raw := strings.TrimSpace(envValue(env, "LIGHTHOUSE_URL"))
	if raw == "" {
		return lighthouseURLFromAPI(env)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return lighthouseURLFromAPI(env)
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
	return replaceLocalURLHost(parsed.String(), resourceURLHost(env))
}

// lighthouseURLFromAPI keeps default operator links on the same assigned origin as the App.
func lighthouseURLFromAPI(env map[string]string) string {
	parsed, err := url.Parse(resolveAPIURL(env))
	if err != nil || parsed.Host == "" {
		return urlForHostPort("http", resourceURLHost(env), "3000") + "/lighthouse"
	}
	parsed.Path = "/lighthouse"
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

// urlWithPort creates a browser-facing tool URL on the assigned project address.
func urlWithPort(env map[string]string, key string, fallback string) string {
	port := firstNonEmpty(envValue(env, key), fallback)
	if port == "" {
		return ""
	}
	return urlForHostPort("http", resourceURLHost(env), port)
}

// resourceURLHost excludes wildcard bind addresses because they are not useful browser destinations.
func resourceURLHost(env map[string]string) string {
	raw := strings.Trim(strings.TrimSpace(envValue(env, "IP_ADDRESS")), "[]")
	address := net.ParseIP(raw)
	if address == nil || address.IsUnspecified() {
		return "localhost"
	}
	return address.String()
}

// urlForHostPort formats IPv4 and IPv6 destinations without allowing host text to alter URL structure.
func urlForHostPort(scheme string, host string, port string) string {
	return (&url.URL{Scheme: scheme, Host: net.JoinHostPort(host, port)}).String()
}

// replaceLocalURLHost preserves custom domains while replacing local placeholders with Harbor's assigned address.
func replaceLocalURLHost(rawURL string, assignedHost string) string {
	if assignedHost == "" || assignedHost == "localhost" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || !localURLPlaceholder(parsed.Hostname()) {
		return rawURL
	}
	if port := parsed.Port(); port != "" {
		parsed.Host = net.JoinHostPort(assignedHost, port)
	} else if strings.Contains(assignedHost, ":") {
		parsed.Host = "[" + assignedHost + "]"
	} else {
		parsed.Host = assignedHost
	}
	return parsed.String()
}

// localURLPlaceholder identifies origins that represent a bind location rather than an intentional custom domain.
func localURLPlaceholder(host string) bool {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	if host == "localhost" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && (address.IsLoopback() || address.IsUnspecified())
}

// envValue keeps missing maps and keys equivalent to an unset environment value.
func envValue(env map[string]string, key string) string {
	if env == nil {
		return ""
	}
	return env[key]
}

// composeProfileEnabled reports exact comma-delimited membership without matching neighboring owner profiles.
func composeProfileEnabled(env map[string]string, profile string) bool {
	for _, token := range strings.Split(envValue(env, "COMPOSE_PROFILES"), ",") {
		if strings.TrimSpace(token) == profile {
			return true
		}
	}
	return false
}

// namedResources derives logical names while allowing callers to exclude more specific environment namespaces.
func namedResources(env map[string]string, prefix string, excludedKeyPrefixes ...string) []string {
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
		excluded := false
		for _, excludedPrefix := range excludedKeyPrefixes {
			if strings.HasPrefix(key, excludedPrefix) {
				excluded = true
				break
			}
		}
		if excluded {
			continue
		}
		name := str.Of(key).TrimPrefix(prefix + "_").TrimSuffix("_DRIVER").String()
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

// primitiveResourcesForApp keeps component selection authoritative before interpreting shared or App-prefixed environment values.
func primitiveResourcesForApp(env map[string]string, apps []projectResourceApp, app projectResourceApp) []Resource {
	definitions := []struct {
		enabled     bool
		prefix      string
		category    string
		description string
		runtime     string
	}{
		{enabled: app.components.Jobs, prefix: "QUEUE", category: "queue", description: "Named queue resource.", runtime: "jobs"},
		{enabled: app.components.Cache, prefix: "CACHE", category: "cache", description: "Named cache resource."},
		{enabled: app.components.Storage, prefix: "STORAGE", category: "storage", description: "Named storage disk resource."},
		{enabled: app.components.Events, prefix: "EVENTS", category: "events", description: "Named event bus resource."},
	}
	resources := []Resource{}
	for _, definition := range definitions {
		if !definition.enabled {
			continue
		}
		for _, name := range namedResourcesForApp(env, apps, app.name, definition.prefix) {
			resources = append(resources, Resource{
				ID:          appResourceID(definition.category, app.name, name),
				Name:        name,
				Category:    definition.category,
				Description: definition.description,
				Enabled:     true,
				Priority:    10,
				Source:      "env",
				App:         app.name,
				Runtime:     definition.runtime,
				Owner:       "goforj",
			})
		}
	}
	return resources
}

// namedResourcesForApp keeps shared definitions available without attributing sibling App overlays to this App.
func namedResourcesForApp(env map[string]string, apps []projectResourceApp, appName string, prefix string) []string {
	definitions := project.ResourceCatalog()
	excludedKeyPrefixes := make([]string, 0, len(apps)*len(definitions))
	for _, app := range apps {
		if app.name == "" || app.name == project.DefaultAppName {
			continue
		}
		appPrefix := project.AppEnvironmentPrefix(app.name)
		if appPrefix == "" {
			continue
		}
		for _, definition := range definitions {
			excludedKeyPrefixes = append(excludedKeyPrefixes, appPrefix+"_"+definition.EnvironmentPrefix+"_")
		}
	}
	names := namedResources(env, prefix, excludedKeyPrefixes...)
	if appName == "" || appName == project.DefaultAppName {
		return names
	}
	appPrefix := project.AppEnvironmentPrefix(appName)
	if appPrefix == "" {
		return names
	}
	return uniqueSorted(append(names, namedResources(env, appPrefix+"_"+prefix)...))
}

// appResourceID preserves established default-App IDs while namespacing sibling Apps.
func appResourceID(category string, appName string, resourceName string) string {
	if appName == "" || appName == project.DefaultAppName {
		return category + "-" + resourceName
	}
	return category + "-" + strings.ToLower(appName) + "-" + resourceName
}

// firstNonEmpty returns the first nonblank value after applying environment whitespace rules.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

// uniqueSorted removes empty and duplicate resource names before stable projection.
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
