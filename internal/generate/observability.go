package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/goforj/goforj/project"
)

// observabilityMetricsModeAuto and its related constants constrain target layouts before environment values reach planning logic.
const (
	observabilityMetricsModeAuto        = "auto"
	observabilityMetricsModeLocalSingle = "local-single"
	observabilityMetricsModeLocalMulti  = "local-multi"
	observabilityMetricsModeCompose     = "compose"
	observabilityMetricsModeDisabled    = "disabled"
)

// metricsTargetEntry mirrors the VictoriaMetrics file-discovery shape so generation remains schema-specific at the boundary.
type metricsTargetEntry struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

// metricsTargetRole keeps each runtime's discovery and environment conventions together to prevent role-specific drift.
type metricsTargetRole struct {
	Name    string
	Path    string
	PortEnv string
	Offset  int
	HostEnv string
}

// metricsTargetEndpoint makes per-role exclusion explicit without coupling it to an empty host or port sentinel.
type metricsTargetEndpoint struct {
	Host    string
	Port    string
	Include bool
}

// observabilityApp carries the derived identity and port defaults needed to build per-App scrape targets.
type observabilityApp struct {
	Name        string
	Index       int
	EnvPrefix   string
	HTTPPort    int
	RuntimeBase int
	Components  project.Components
}

// observabilityTargetPlan distinguishes an intentionally unmanaged target file from a managed plan with no active targets.
type observabilityTargetPlan struct {
	Entries []metricsTargetEntry
	Manage  bool
}

// observabilityMetricRoles preserves a stable role order for deterministic generated target files.
var observabilityMetricRoles = []metricsTargetRole{
	{Name: "api", Path: filepath.Join("internal", "http"), PortEnv: "METRICS_API_PORT", Offset: 0, HostEnv: "OBSERVABILITY_API_METRICS_HOST"},
	{Name: "jobs", Path: filepath.Join("internal", "jobs"), PortEnv: "METRICS_JOBS_PORT", Offset: 2, HostEnv: "OBSERVABILITY_JOBS_METRICS_HOST"},
	{Name: "scheduler", Path: filepath.Join("internal", "schedules"), PortEnv: "METRICS_SCHEDULER_PORT", Offset: 1, HostEnv: "OBSERVABILITY_SCHEDULER_METRICS_HOST"},
}

// GenerateObservabilityFiles updates framework-owned scrape targets only when the project's observability mode delegates their management to Forj.
func GenerateObservabilityFiles(projectDir string) (int, error) {
	plan, err := buildMetricsTargets(projectDir)
	if err != nil {
		return 0, err
	}
	if !plan.Manage {
		return 0, nil
	}
	if plan.Entries == nil {
		plan.Entries = []metricsTargetEntry{}
	}

	content, err := json.MarshalIndent(plan.Entries, "", "  ")
	if err != nil {
		return 0, err
	}
	content = append(content, '\n')

	changed, err := writeGeneratedSource(
		filepath.Join(projectDir, "containers", "observability", "vmagent", "metrics-targets.json"),
		content,
	)
	if err != nil {
		return 0, err
	}
	if !changed {
		return 0, nil
	}
	return 1, nil
}

// buildMetricsTargets translates project layout and target mode into a deterministic VictoriaMetrics scrape plan.
func buildMetricsTargets(projectDir string) (observabilityTargetPlan, error) {
	service := envOrDefault("APP_NAME", filepath.Base(projectDir))
	environment := envOrDefault("APP_ENV", "local")
	config, err := loadObservabilityProjectConfig(projectDir)
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	activeRoles, err := discoverObservabilityMetricRoles(projectDir, config)
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	mode, err := resolveObservabilityMetricsMode()
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	if mode == observabilityMetricsModeDisabled {
		return observabilityTargetPlan{Manage: false}, nil
	}
	localHost := ""
	if mode == observabilityMetricsModeLocalSingle || mode == observabilityMetricsModeLocalMulti {
		var ok bool
		localHost, ok = resolveLocalMetricsHost()
		if !ok {
			return observabilityTargetPlan{Manage: false}, nil
		}
	}
	if len(activeRoles) == 0 {
		return observabilityTargetPlan{Manage: true, Entries: []metricsTargetEntry{}}, nil
	}
	appNames, err := discoverObservabilityApps(projectDir, config, activeRoles)
	if err != nil {
		return observabilityTargetPlan{}, err
	}

	basePort, err := resolveMetricsBasePort()
	if err != nil {
		return observabilityTargetPlan{}, err
	}

	switch mode {
	case observabilityMetricsModeLocalSingle:
		entries, err := buildStandaloneTargets(service, environment, localHost, activeRoles, appNames)
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	case observabilityMetricsModeLocalMulti:
		entries, err := buildAppRoleTargets(service, environment, localHost, activeRoles, appNames)
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	case observabilityMetricsModeCompose:
		entries, err := buildRoleTargets(service, environment, activeRoles, func(role metricsTargetRole) (metricsTargetEndpoint, error) {
			host, ok := resolveComposeMetricsHost(role)
			if !ok {
				return metricsTargetEndpoint{}, nil
			}
			return metricsTargetEndpoint{Host: host, Port: strconv.Itoa(basePort), Include: true}, nil
		})
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	default:
		return observabilityTargetPlan{}, fmt.Errorf("unsupported OBSERVABILITY_METRICS_TARGET_MODE %q", mode)
	}
}

// buildStandaloneTargets mirrors app run/dev mode where each app owns one host process.
func buildStandaloneTargets(
	service string,
	environment string,
	host string,
	activeRoles []metricsTargetRole,
	appNames []observabilityApp,
) ([]metricsTargetEntry, error) {
	entries := make([]metricsTargetEntry, 0, len(appNames))
	for _, app := range appNames {
		port, err := resolveStandaloneMetricsPort(app, activeRoles)
		if err != nil {
			return nil, err
		}
		entries = append(entries, metricsTargetEntry{
			Targets: []string{host + ":" + port},
			Labels:  observabilityTargetLabels(service, environment, "app", app.Name),
		})
	}
	return entries, nil
}

// buildAppRoleTargets mirrors distributed local mode where each app/runtime pair can expose metrics independently.
func buildAppRoleTargets(
	service string,
	environment string,
	host string,
	activeRoles []metricsTargetRole,
	appNames []observabilityApp,
) ([]metricsTargetEntry, error) {
	entries := make([]metricsTargetEntry, 0, len(activeRoles)*len(appNames))
	for _, app := range appNames {
		for _, role := range filterAppMetricRoles(activeRoles, app) {
			port, err := resolveAppRolePort(role, app)
			if err != nil {
				return nil, err
			}
			entries = append(entries, metricsTargetEntry{
				Targets: []string{host + ":" + port},
				Labels:  observabilityTargetLabels(service, environment, role.Name, app.Name),
			})
		}
	}
	return entries, nil
}

// discoverObservabilityMetricRoles keeps role discovery tied to rendered framework packages without letting stale Jobs output override project intent.
func discoverObservabilityMetricRoles(projectDir string, config *project.Config) ([]metricsTargetRole, error) {
	var configuredComponents *project.Components
	if config != nil {
		components := project.ProjectComponents(config)
		configuredComponents = &components
	}

	roles := make([]metricsTargetRole, 0, len(observabilityMetricRoles))
	for _, role := range observabilityMetricRoles {
		if role.Name == "jobs" && configuredComponents != nil && !configuredComponents.Jobs {
			continue
		}
		if _, err := os.Stat(filepath.Join(projectDir, role.Path)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("inspect observability %s role: %w", role.Name, err)
		}
		roles = append(roles, role)
	}
	return roles, nil
}

// discoverObservabilityApps derives app identity from layout conventions so generation does not depend on dev config.
func discoverObservabilityApps(projectDir string, config *project.Config, activeRoles []metricsTargetRole) ([]observabilityApp, error) {
	names := map[string]bool{project.DefaultAppName: true}
	conventionalApps, err := discoverConventionalObservabilityApps(projectDir)
	if err != nil {
		return nil, err
	}
	for _, app := range conventionalApps {
		names[app.Name] = true
	}

	orderedNames := make([]string, 0, len(names))
	for name := range names {
		if name != project.DefaultAppName {
			orderedNames = append(orderedNames, name)
		}
	}
	sort.Strings(orderedNames)
	orderedNames = append([]string{project.DefaultAppName}, orderedNames...)

	apps := make([]observabilityApp, 0, len(orderedNames))
	for index, name := range orderedNames {
		apps = append(apps, observabilityApp{
			Name:        name,
			Index:       index,
			EnvPrefix:   project.AppEnvironmentPrefix(name),
			HTTPPort:    3000 + index,
			RuntimeBase: 10000 + index*10,
			Components:  observabilityAppComponents(config, name, activeRoles),
		})
	}
	return apps, nil
}

// discoverConventionalObservabilityApps treats cmd/<app> and app/<app> as app ownership markers.
func discoverConventionalObservabilityApps(projectDir string) ([]project.App, error) {
	names := make(map[string]bool)
	cmdDir := filepath.Join(projectDir, "cmd")
	entries, err := os.ReadDir(cmdDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("discover observability apps in %s: %w", cmdDir, err)
	}
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppName || !project.IsSafeAppName(name) {
				continue
			}
			marker := filepath.Join(projectDir, "cmd", name, "main.go")
			if _, err := os.Stat(marker); err == nil {
				names[name] = true
			} else if !os.IsNotExist(err) {
				return nil, fmt.Errorf("inspect observability app marker %s: %w", marker, err)
			}
		}
	}
	appRoot := filepath.Join(projectDir, "app")
	entries, err = os.ReadDir(appRoot)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("discover observability apps in %s: %w", appRoot, err)
	}
	if err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			if name == project.DefaultAppName || !project.IsSafeAppName(name) || project.IsReservedAppName(name) {
				continue
			}
			hasFiles, err := hasConventionalObservabilityAppFiles(filepath.Join(projectDir, "app", name))
			if err != nil {
				return nil, err
			}
			if hasFiles {
				names[name] = true
			}
		}
	}

	apps := make([]project.App, 0, len(names))
	for name := range names {
		apps = append(apps, project.DefaultNamedApp(name))
	}
	sort.Slice(apps, func(i, j int) bool {
		return apps[i].Name < apps[j].Name
	})
	return apps, nil
}

// hasConventionalObservabilityAppFiles avoids treating arbitrary app subpackages as apps.
func hasConventionalObservabilityAppFiles(appDir string) (bool, error) {
	for _, path := range []string{
		filepath.Join(appDir, "wire"),
		filepath.Join(appDir, "commands.go"),
		filepath.Join(appDir, "root_cmd.go"),
		filepath.Join(appDir, "routes.go"),
		filepath.Join(appDir, "schedules.go"),
		filepath.Join(appDir, "lifecycle.go"),
	} {
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("inspect observability app marker %s: %w", path, err)
		}
	}
	return false, nil
}

// loadObservabilityProjectConfig allows convention-only legacy projects while rejecting unreadable or malformed configuration.
func loadObservabilityProjectConfig(projectDir string) (*project.Config, error) {
	config, err := project.LoadProjectConfigAt(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("load project observability config: %w", err)
	}
	return config, nil
}

// observabilityAppComponents uses config only to filter roles for a known conventional app.
func observabilityAppComponents(cfg *project.Config, appName string, activeRoles []metricsTargetRole) project.Components {
	if cfg == nil {
		return componentsFromMetricRoles(activeRoles)
	}
	if appName == "" || appName == project.DefaultAppName {
		return cfg.Render.Components
	}
	components := cfg.Render.Components
	if appConfig, ok := cfg.Apps[appName]; ok {
		components = project.NormalizeAppComponents(cfg.Render.Components, appConfig.Components)
	}
	return components
}

// componentsFromMetricRoles preserves legacy behavior when no project config exists in tests or hand-built fixtures.
func componentsFromMetricRoles(activeRoles []metricsTargetRole) project.Components {
	var components project.Components
	components.Metrics = len(activeRoles) > 0
	for _, role := range activeRoles {
		switch role.Name {
		case "api":
			components.WebAPI = true
		case "jobs":
			components.Jobs = true
		case "scheduler":
			components.Scheduler = true
		}
	}
	return components
}

// resolveObservabilityMetricsMode defaults to the combined App target because split roles are selected by explicit commands.
func resolveObservabilityMetricsMode() (string, error) {
	mode := strings.ToLower(envOrDefault("OBSERVABILITY_METRICS_TARGET_MODE", observabilityMetricsModeAuto))
	switch mode {
	case "", observabilityMetricsModeAuto:
		return observabilityMetricsModeLocalSingle, nil
	case observabilityMetricsModeLocalSingle, observabilityMetricsModeLocalMulti, observabilityMetricsModeCompose, observabilityMetricsModeDisabled:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown OBSERVABILITY_METRICS_TARGET_MODE %q", mode)
	}
}

// resolveMetricsBasePort validates the shared port before role offsets are applied so invalid configuration fails generation early.
func resolveMetricsBasePort() (int, error) {
	value := envOrDefault("METRICS_PORT", "9100")
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid METRICS_PORT %q: %w", value, err)
	}
	return port, nil
}

// resolveRolePort honors explicit role ports while retaining deterministic offsets from the shared base port.
func resolveRolePort(role metricsTargetRole, basePort int) (string, error) {
	if value, ok := lookupEnvTrimmed(role.PortEnv); ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("invalid %s %q: %w", role.PortEnv, value, err)
		}
		return strconv.Itoa(port), nil
	}
	return strconv.Itoa(basePort + role.Offset), nil
}

// resolveAppRolePort applies the same app-scoped override order used by generated runtime helpers.
func resolveAppRolePort(role metricsTargetRole, app observabilityApp) (string, error) {
	envKeys := appRolePortEnvKeys(role, app)
	if value, key, ok := firstEnvTrimmed(envKeys); ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("invalid %s %q: %w", key, value, err)
		}
		return strconv.Itoa(port), nil
	}
	return strconv.Itoa(app.RuntimeBase + role.Offset), nil
}

// appRolePortEnvKeys lists the accepted env aliases for one metrics source.
func appRolePortEnvKeys(role metricsTargetRole, app observabilityApp) []string {
	switch role.Name {
	case "api":
		return observabilityAppEnvKeys(app, "METRICS_PORT", "API_METRICS_PORT", "METRICS_API_PORT")
	case "scheduler":
		return observabilityAppEnvKeys(app, "SCHEDULER_METRICS_PORT", "METRICS_SCHEDULER_PORT", "METRICS_PORT")
	case "jobs":
		return observabilityAppEnvKeys(app, "WORKER_METRICS_PORT", "JOBS_METRICS_PORT", "METRICS_JOBS_PORT", "METRICS_PORT")
	default:
		return observabilityAppEnvKeys(app, role.PortEnv)
	}
}

// observabilityAppEnvKeys prevents named apps from consuming default app globals.
func observabilityAppEnvKeys(app observabilityApp, suffixes ...string) []string {
	if app.Name == project.DefaultAppName || app.EnvPrefix == "" {
		return suffixes
	}
	keys := make([]string, 0, len(suffixes))
	for _, suffix := range suffixes {
		if suffix = strings.TrimSpace(suffix); suffix != "" {
			keys = append(keys, app.EnvPrefix+"_"+suffix)
		}
	}
	return keys
}

// resolveLocalMetricsHost treats an explicitly empty host as an opt-out while retaining the Docker-to-host default.
func resolveLocalMetricsHost() (string, bool) {
	if value, ok := lookupEnvTrimmed("OBSERVABILITY_METRICS_TARGET_HOST"); ok {
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "host.docker.internal", true
}

// resolveComposeMetricsHost defaults to stable Compose service names while allowing individual roles to be excluded.
func resolveComposeMetricsHost(role metricsTargetRole) (string, bool) {
	if value, ok := lookupEnvTrimmed(role.HostEnv); ok {
		if value == "" {
			return "", false
		}
		return value, true
	}
	return role.Name, true
}

// resolveStandaloneMetricsPort prefers the HTTP listener because app run exposes /metrics there when HTTP is present.
func resolveStandaloneMetricsPort(app observabilityApp, activeRoles []metricsTargetRole) (string, error) {
	if containsObservabilityRole(activeRoles, "api") {
		if value, key, ok := firstEnvTrimmed(observabilityAppEnvKeys(app, "PORT", "API_HTTP_PORT")); ok {
			port, err := strconv.Atoi(value)
			if err != nil {
				return "", fmt.Errorf("invalid %s %q: %w", key, value, err)
			}
			return strconv.Itoa(port), nil
		}
		return strconv.Itoa(app.HTTPPort), nil
	}
	return resolveAppRolePort(metricsTargetRole{Name: "api", Offset: 0}, app)
}

// buildRoleTargets applies one endpoint policy across roles so Compose and future layouts share labeling and failure behavior.
func buildRoleTargets(
	service string,
	environment string,
	roles []metricsTargetRole,
	resolve func(role metricsTargetRole) (metricsTargetEndpoint, error),
) ([]metricsTargetEntry, error) {
	entries := make([]metricsTargetEntry, 0, len(roles))
	for _, role := range roles {
		endpoint, err := resolve(role)
		if err != nil {
			return nil, err
		}
		if !endpoint.Include {
			continue
		}
		entries = append(entries, metricsTargetEntry{
			Targets: []string{endpoint.Host + ":" + endpoint.Port},
			Labels:  observabilityTargetLabels(service, environment, role.Name, project.DefaultAppName),
		})
	}
	return entries, nil
}

// filterAppMetricRoles keeps local-multi scraping aligned to the app's selected runtime surfaces.
func filterAppMetricRoles(activeRoles []metricsTargetRole, app observabilityApp) []metricsTargetRole {
	out := make([]metricsTargetRole, 0, len(activeRoles))
	for _, role := range activeRoles {
		if appHasMetricRole(app, role) {
			out = append(out, role)
		}
	}
	return out
}

// appHasMetricRole maps source roles onto app component participation.
func appHasMetricRole(app observabilityApp, role metricsTargetRole) bool {
	switch role.Name {
	case "api":
		return app.Components.WebAPI || app.Components.WebUI
	case "jobs":
		return app.Components.Jobs
	case "scheduler":
		return app.Components.Scheduler
	default:
		return true
	}
}

// observabilityTargetLabels keeps vmagent labels consistent with emitted framework metric labels.
func observabilityTargetLabels(service string, environment string, process string, appName string) map[string]string {
	return map[string]string{
		"app":         appName,
		"environment": environment,
		"process":     process,
		"service":     service,
	}
}

// firstEnvTrimmed returns the winning key as well as the value so validation errors can name it.
func firstEnvTrimmed(keys []string) (string, string, bool) {
	for _, key := range keys {
		value, ok := lookupEnvTrimmed(key)
		if ok && value != "" {
			return value, key, true
		}
	}
	return "", "", false
}

// containsObservabilityRole keeps role-presence checks independent of the discovery order.
func containsObservabilityRole(roles []metricsTargetRole, name string) bool {
	for _, role := range roles {
		if role.Name == name {
			return true
		}
	}
	return false
}

// envOrDefault normalizes blank environment values to defaults so whitespace cannot create malformed endpoints.
func envOrDefault(key string, defaultValue string) string {
	if value, ok := lookupEnvTrimmed(key); ok {
		if value != "" {
			return value
		}
		return defaultValue
	}
	return strings.TrimSpace(defaultValue)
}

// lookupEnvTrimmed preserves whether a variable was explicitly set because empty values can disable optional targets.
func lookupEnvTrimmed(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}
