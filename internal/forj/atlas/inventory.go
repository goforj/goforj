package atlas

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goforj/atlas/diagnostics"
	"github.com/goforj/atlas/mcp"
	atlasproject "github.com/goforj/atlas/project"
	"github.com/goforj/atlas/workflows"
	"github.com/goforj/goforj/internal/forj/resources"
	"github.com/goforj/goforj/project"
)

var scheduleNamePattern = regexp.MustCompile(`\.Name\("([^"]+)"\)`)
var routeGroupPattern = regexp.MustCompile(`web\.NewRouteGroup\("([^"]*)"`)
var commandFieldPattern = regexp.MustCompile(`(?m)^\s*([A-Za-z0-9_]+)\s+[\w.]+\s+` + "`" + `cmd:""` + "`")

var cacheEnvKeys = []string{
	"DRIVER", "DEFAULT_TTL_SECONDS", "PREFIX", "MEMORY_CLEANUP_SECONDS", "FILE_DIR", "ADDR",
	"ADDRESSES", "USERNAME", "PASSWORD", "DB", "DSN", "TABLE", "ENDPOINT", "REGION", "TLS",
	"INSECURE_SKIP_VERIFY", "URL", "BUCKET", "BUCKET_TTL", "BUCKET_TTL_SECONDS", "DESCRIPTION",
	"HISTORY", "MAX_BYTES", "MAX_VALUE_SIZE", "REPLICAS", "STORAGE", "COMPRESSED", "COMPRESSION",
	"MAX_VALUE_BYTES", "ENCRYPTION_KEY",
}

var storageEnvKeys = []string{
	"DRIVER", "ROOT", "PREFIX", "ADDR", "USERNAME", "PASSWORD", "DB", "HOST", "PORT", "USER",
	"TLS", "INSECURE_SKIP_VERIFY", "KEY_PATH", "KNOWN_HOSTS_PATH", "INSECURE_IGNORE_HOST_KEY",
	"BUCKET", "ENDPOINT", "REGION", "ACCESS_KEY_ID", "SECRET_ACCESS_KEY", "USE_PATH_STYLE",
	"UNSIGNED_PAYLOAD", "CREDENTIALS_JSON", "TOKEN", "REMOTE", "RCLONE_CONFIG_PATH", "RCLONE_CONFIG_DATA",
}

var eventEnvKeys = []string{
	"DRIVER", "ADDR", "REDIS_CHANNEL_PREFIX", "URL", "BROKERS", "PROJECT_ID", "URI", "REGION",
	"ENDPOINT", "TOPIC_NAME_PREFIX", "QUEUE_NAME_PREFIX", "WAIT_TIME_SECONDS", "VISIBILITY_TIMEOUT_SECONDS",
	"SUBJECT_PREFIX", "STREAM_NAME_PREFIX", "INACTIVE_THRESHOLD_SECONDS", "ACK_WAIT_SECONDS",
	"FETCH_MAX_WAIT_MS", "STORAGE", "INPROC_WORKERS", "INPROC_BUFFER",
}

var dbEnvKeys = []string{
	"DEFAULT", "DRIVER", "DSN", "HOST", "DATABASE", "USERNAME", "PASSWORD", "PORT", "QUERY_LOGGING",
	"MAX_IDLE_CONNECTIONS", "MAX_OPEN_CONNECTIONS", "CONN_MAX_LIFETIME_MINUTES", "ROOT_PASSWORD",
}

// Inventory discovers safe project resource names for the Atlas MCP server.
func Inventory(root string) mcp.Inventory {
	root = firstNonEmpty(root, ".")
	cfg, _ := loadProjectConfig(root)
	env := loadAtlasEnv(root)
	atlasProject := Project(root).WithDiscoveredDefaults()
	apps := atlasProject.Apps
	projectComponents := project.Components{}
	if cfg != nil {
		projectComponents = project.ProjectComponents(cfg)
	}
	disks := []string(nil)
	if projectComponents.Storage {
		disks = resourceNames(env, "STORAGE", storageEnvKeys)
	}
	eventBuses := []string(nil)
	if projectComponents.Events {
		eventBuses = resourceNames(env, "EVENTS", eventEnvKeys)
	}
	links := resourceLinks(cfg, env)
	return mcp.Inventory{
		Routes:     discoverAppSymbols(root, apps, "routes.go", routeGroupPattern, routeLabel),
		Schedules:  discoverAppSymbols(root, apps, "schedules.go", scheduleNamePattern, identityLabel),
		Commands:   discoverAppSymbols(root, apps, "commands.go", commandFieldPattern, identityLabel),
		Queues:     resourceLinkLabels(links, "queue"),
		Caches:     resourceNames(env, "CACHE", cacheEnvKeys),
		Disks:      disks,
		EventBuses: eventBuses,
		Resources:  links,
	}
}

// Diagnostics discovers safe read-only diagnostics metadata for Atlas tools.
func Diagnostics(root string) diagnostics.Provider {
	root = firstNonEmpty(root, ".")
	cfg, _ := loadProjectConfig(root)
	env := loadAtlasEnv(root)
	project := Project(root).WithDiscoveredDefaults()
	return diagnostics.StaticProvider{
		Connections: databaseConnections(project.Apps, cfg, env),
		BaseURLs:    appBaseURLs(project.Apps, cfg, env),
		Metrics:     metricsMetadata(project.Apps, cfg, env),
	}
}

func atlasAppsFromConfig(root string, cfg *project.Config, discovered []atlasproject.App) []atlasproject.App {
	if cfg == nil {
		return discovered
	}

	seen := map[string]struct{}{}
	apps := make([]atlasproject.App, 0, len(discovered)+len(cfg.Apps)+1)
	add := func(name string, defaultApp bool) {
		if strings.TrimSpace(name) == "" {
			name = project.DefaultAppName
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		apps = append(apps, atlasproject.App{
			Name:     name,
			Default:  defaultApp || name == project.DefaultAppName,
			Runtimes: runtimeNames(appComponents(cfg, name)),
		})
	}

	add(project.DefaultAppName, true)
	for _, app := range discovered {
		add(app.Name, app.Default)
	}
	names := make([]string, 0, len(cfg.Apps))
	for name := range cfg.Apps {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if appExists(root, name) {
			add(name, false)
		}
	}
	return apps
}

func appComponents(cfg *project.Config, appName string) project.Components {
	if cfg == nil {
		return project.Components{}
	}
	if appName != "" && appName != project.DefaultAppName {
		if app, ok := cfg.Apps[appName]; ok {
			return project.NormalizeAppComponents(cfg.Render.Components, app.Components)
		}
	}
	return cfg.Render.Components.WithResolvedDependencies()
}

func runtimeNames(components project.Components) []string {
	runtimes := []string{}
	if components.WebAPI || components.WebUI {
		runtimes = append(runtimes, "http")
	}
	if components.Jobs {
		runtimes = append(runtimes, "jobs")
	}
	if components.Scheduler {
		runtimes = append(runtimes, "scheduler")
	}
	if components.CLI {
		runtimes = append(runtimes, "cli")
	}
	if len(runtimes) == 0 {
		return []string{"cli"}
	}
	return runtimes
}

func appExists(root string, name string) bool {
	if name == "" || name == project.DefaultAppName {
		return true
	}
	_, cmdErr := os.Stat(filepath.Join(root, "cmd", name, "main.go"))
	_, appErr := os.Stat(filepath.Join(root, "app", name))
	return cmdErr == nil && appErr == nil
}

func loadAtlasEnv(root string) map[string]string {
	env := map[string]string{}
	for _, kv := range os.Environ() {
		key, value, ok := strings.Cut(kv, "=")
		if ok {
			env[key] = value
		}
	}
	for _, name := range []string{".env", ".env.local"} {
		loadEnvFile(filepath.Join(root, name), env)
	}
	return env
}

func loadEnvFile(path string, env map[string]string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if before, _, ok := strings.Cut(value, " #"); ok {
			value = strings.TrimSpace(before)
		}
		value = strings.Trim(value, `"'`)
		if key != "" {
			env[key] = value
		}
	}
}

func discoverAppSymbols(root string, apps []atlasproject.App, filename string, pattern *regexp.Regexp, label func(string) string) map[string][]string {
	out := map[string][]string{}
	for _, app := range apps {
		path := filepath.Join(root, appDir(app.Name), filename)
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		values := []string{}
		for _, matches := range pattern.FindAllStringSubmatch(string(content), -1) {
			if len(matches) < 2 {
				continue
			}
			value := label(matches[1])
			if value != "" {
				values = append(values, value)
			}
		}
		if len(values) > 0 {
			out[app.Name] = uniqueSorted(values)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appDir(name string) string {
	if name == "" || name == project.DefaultAppName {
		return "app"
	}
	return filepath.Join("app", name)
}

func routeLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "group /"
	}
	return "group " + strings.TrimSpace(value)
}

func identityLabel(value string) string {
	return strings.TrimSpace(value)
}

func resourceNames(env map[string]string, prefix string, rootKeys []string) []string {
	names := []string{"default"}
	keySet := map[string]struct{}{}
	for _, key := range rootKeys {
		keySet[key] = struct{}{}
	}
	for key := range env {
		if !strings.HasPrefix(key, prefix+"_") {
			continue
		}
		suffix := strings.TrimPrefix(key, prefix+"_")
		for rootKey := range keySet {
			if suffix == rootKey {
				continue
			}
			if strings.HasSuffix(suffix, "_"+rootKey) {
				name := strings.TrimSuffix(suffix, "_"+rootKey)
				if name != "" {
					names = append(names, strings.ToLower(name))
				}
				break
			}
		}
	}
	return uniqueSorted(names)
}

func databaseConnections(apps []atlasproject.App, cfg *project.Config, env map[string]string) []diagnostics.DatabaseConnection {
	connections := []diagnostics.DatabaseConnection{}
	for _, app := range apps {
		components := appComponents(cfg, app.Name)
		if !components.HasDatabase() {
			continue
		}
		driver := firstNonEmpty(env[appScopedKey(app.Name, "DB_DRIVER")], components.DatabaseDriver())
		if driver == "" {
			continue
		}
		connections = append(connections, diagnostics.DatabaseConnection{
			Name:     app.Name,
			Driver:   driver,
			Database: firstNonEmpty(env[appScopedKey(app.Name, "DB_DATABASE")], projectName(cfg), app.Name),
			App:      app.Name,
		})
	}
	return connections
}

func projectName(cfg *project.Config) string {
	if cfg == nil {
		return ""
	}
	return cfg.ProjectName
}

func appBaseURLs(apps []atlasproject.App, cfg *project.Config, env map[string]string) map[string]string {
	baseURLs := map[string]string{}
	for _, app := range apps {
		components := appComponents(cfg, app.Name)
		if !components.WebAPI && !components.WebUI {
			continue
		}
		url := env[appScopedKey(app.Name, "APP_URL")]
		if url == "" {
			port := firstNonEmpty(env[appScopedKey(app.Name, "API_HTTP_PORT")], env[appScopedKey(app.Name, "PORT")])
			if port != "" {
				url = "http://localhost:" + port
			}
		}
		if url != "" {
			baseURLs[app.Name] = url
		}
	}
	if len(baseURLs) == 0 {
		return nil
	}
	return baseURLs
}

func metricsMetadata(apps []atlasproject.App, cfg *project.Config, env map[string]string) map[string]diagnostics.MetricsMetadata {
	metadata := map[string]diagnostics.MetricsMetadata{}
	for _, app := range apps {
		components := appComponents(cfg, app.Name)
		if !components.Metrics {
			continue
		}
		for _, runtime := range runtimeNames(components) {
			if runtime == "cli" {
				continue
			}
			key := app.Name + "/" + runtime
			metadata[key] = diagnostics.MetricsMetadata{
				App:     app.Name,
				Runtime: runtime,
				Labels:  map[string]string{"app": app.Name, "runtime": runtime},
				Targets: []string{
					metricsTarget(app.Name, runtime, env),
				},
				Counters: metricsCounters(components),
			}
		}
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

// metricsCounters advertises only metric families owned by the App's selected runtime surfaces.
func metricsCounters(components project.Components) []string {
	counters := []string{
		"http_requests_total",
		"cache_operations_total",
	}
	if components.Jobs {
		counters = append(counters, "queue_jobs_total")
	}
	return counters
}

func metricsTarget(appName string, runtime string, env map[string]string) string {
	envKey := map[string]string{
		"http":      "METRICS_API_PORT",
		"jobs":      "METRICS_JOBS_PORT",
		"scheduler": "METRICS_SCHEDULER_PORT",
	}[runtime]
	port := firstNonEmpty(env[appScopedKey(appName, envKey)], env[appScopedKey(appName, "METRICS_PORT")])
	if port == "" {
		return ""
	}
	return "http://localhost:" + port + "/metrics"
}

func resourceLinks(cfg *project.Config, env map[string]string) []workflows.ResourceLink {
	registryResources, err := resources.RegistryForProject(cfg, env).List(context.Background())
	if err != nil && len(registryResources) == 0 {
		return nil
	}
	links := make([]workflows.ResourceLink, 0, len(registryResources))
	for _, resource := range registryResources {
		links = append(links, workflows.ResourceLink{
			ID:       resource.ID,
			Label:    resource.Name,
			URL:      resource.URL,
			Category: resource.Category,
			Source:   resource.Source,
			App:      resource.App,
			Runtime:  resource.Runtime,
			Health:   resource.Health,
			Auth:     resource.Auth,
			Owner:    resource.Owner,
		})
	}
	return links
}

// resourceLinkLabels returns the unique labels exposed by one resource category.
func resourceLinkLabels(links []workflows.ResourceLink, category string) []string {
	labels := make([]string, 0, len(links))
	for _, link := range links {
		if link.Category == category {
			labels = append(labels, link.Label)
		}
	}
	return uniqueSorted(labels)
}

func appScopedKey(appName string, key string) string {
	if appName == "" || appName == project.DefaultAppName {
		return key
	}
	prefix := strings.ToUpper(strings.ReplaceAll(appName, "-", "_"))
	return prefix + "_" + key
}

func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
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

var _ diagnostics.Provider = diagnostics.StaticProvider{}
