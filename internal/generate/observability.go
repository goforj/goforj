package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
)

// observabilityMetricsModeAuto and its related constants constrain target layouts before environment values reach planning logic.
const (
	observabilityMetricsModeAuto        = "auto"
	observabilityMetricsModeLocalSingle = "local-single"
	observabilityMetricsModeLocalMulti  = "local-multi"
	observabilityMetricsModeCompose     = "compose"
	observabilityMetricsModeDisabled    = "disabled"
	maxDeploymentMetadataLabelLength    = 128
)

var deploymentMetadataLabelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:+/@=-]*$`)

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
	RuntimeBase int
	Components  project.Components
}

// observabilityTargetPlan distinguishes an intentionally unmanaged target file from a managed plan with no active targets.
type observabilityTargetPlan struct {
	Entries []metricsTargetEntry
	Manage  bool
}

// observabilityTargetResolver keeps every environment lookup on the immutable snapshot used to plan one target file.
type observabilityTargetResolver struct {
	environment generationEnvironment
}

// observabilityEnvironmentMatch preserves the winning key so invalid values can produce actionable errors.
type observabilityEnvironmentMatch struct {
	key   string
	value string
}

// observabilityTargetIdentity holds the labels shared by every endpoint in one generated target file.
type observabilityTargetIdentity struct {
	service  string
	appEnv   string
	release  string
	revision string
}

// observabilityAppTargetPlan carries the shared labels and App layout used by local target builders.
type observabilityAppTargetPlan struct {
	identity     observabilityTargetIdentity
	host         string
	activeRoles  []metricsTargetRole
	applications []observabilityApp
}

// observabilityMetricRoles preserves a stable role order for deterministic generated target files.
var observabilityMetricRoles = []metricsTargetRole{
	{Name: "api", Path: filepath.Join("internal", "http"), PortEnv: "METRICS_API_PORT", Offset: 0, HostEnv: "OBSERVABILITY_API_METRICS_HOST"},
	{Name: "jobs", Path: filepath.Join("internal", "jobs"), PortEnv: "METRICS_JOBS_PORT", Offset: 2, HostEnv: "OBSERVABILITY_JOBS_METRICS_HOST"},
	{Name: "scheduler", Path: filepath.Join("internal", "schedules"), PortEnv: "METRICS_SCHEDULER_PORT", Offset: 1, HostEnv: "OBSERVABILITY_SCHEDULER_METRICS_HOST"},
}

// GenerateObservabilityFiles updates framework-owned scrape targets only when the project's observability mode delegates their management to Forj.
func GenerateObservabilityFiles(projectDir string) (int, error) {
	return generateObservabilityFiles(ambientGenerationInput(projectDir))
}

// generateObservabilityFiles renders scrape targets from the same captured environment as the other generator tasks.
func generateObservabilityFiles(input generationInput) (int, error) {
	plan, err := buildMetricsTargets(input)
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

	changed, err := writeObservabilityTargets(
		filepath.Join(input.projectDir, "containers", "observability", "vmagent", "metrics-targets.json"),
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

// writeObservabilityTargets publishes complete discovery documents with rename semantics so vmagent never observes a partially written target file.
func writeObservabilityTargets(path string, content []byte) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	temporary, err := os.CreateTemp(filepath.Dir(path), ".metrics-targets-*")
	if err != nil {
		return false, err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return false, err
	}
	return true, nil
}

// buildMetricsTargets translates project layout and target mode into a deterministic VictoriaMetrics scrape plan.
func buildMetricsTargets(input generationInput) (observabilityTargetPlan, error) {
	resolver := observabilityTargetResolver{environment: input.environment}
	service := resolver.envOrDefault("APP_NAME", filepath.Base(input.projectDir))
	environment := resolver.envOrDefault("APP_ENV", "local")
	config, err := loadObservabilityProjectConfig(input.projectDir)
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	activeRoles, err := discoverObservabilityMetricRoles(input.projectDir, config)
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	mode, err := resolver.resolveObservabilityMetricsMode()
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	if mode == observabilityMetricsModeDisabled {
		return observabilityTargetPlan{Manage: false}, nil
	}
	localHost := ""
	if mode == observabilityMetricsModeLocalSingle || mode == observabilityMetricsModeLocalMulti {
		var ok bool
		localHost, ok = resolver.resolveLocalMetricsHost()
		if !ok {
			return observabilityTargetPlan{Manage: false}, nil
		}
	}
	if len(activeRoles) == 0 {
		return observabilityTargetPlan{Manage: true, Entries: []metricsTargetEntry{}}, nil
	}
	release, err := resolver.optionalDeploymentMetadata("APP_VERSION")
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	revision, err := resolver.optionalDeploymentMetadata("APP_REVISION")
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	appNames, err := discoverObservabilityApps(input.projectDir, config, activeRoles)
	if err != nil {
		return observabilityTargetPlan{}, err
	}

	basePort, err := resolver.resolveMetricsBasePort()
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	appTargetPlan := observabilityAppTargetPlan{
		identity:     observabilityTargetIdentity{service: service, appEnv: environment, release: release, revision: revision},
		host:         localHost,
		activeRoles:  activeRoles,
		applications: appNames,
	}

	switch mode {
	case observabilityMetricsModeLocalSingle:
		entries, err := resolver.buildStandaloneTargets(appTargetPlan)
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	case observabilityMetricsModeLocalMulti:
		entries, err := resolver.buildAppRoleTargets(appTargetPlan)
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	case observabilityMetricsModeCompose:
		entries, err := buildRoleTargets(appTargetPlan.identity, activeRoles, func(role metricsTargetRole) (metricsTargetEndpoint, error) {
			host, ok := resolver.resolveComposeMetricsHost(role)
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
func (r observabilityTargetResolver) buildStandaloneTargets(plan observabilityAppTargetPlan) ([]metricsTargetEntry, error) {
	entries := make([]metricsTargetEntry, 0, len(plan.applications))
	for _, app := range plan.applications {
		port, err := r.resolveStandaloneMetricsPort(app)
		if err != nil {
			return nil, err
		}
		entries = append(entries, metricsTargetEntry{
			Targets: []string{plan.host + ":" + port},
			Labels:  observabilityTargetLabels(plan.identity, "app", app.Name),
		})
	}
	return entries, nil
}

// buildAppRoleTargets mirrors distributed local mode where each app/runtime pair can expose metrics independently.
func (r observabilityTargetResolver) buildAppRoleTargets(plan observabilityAppTargetPlan) ([]metricsTargetEntry, error) {
	entries := make([]metricsTargetEntry, 0, len(plan.activeRoles)*len(plan.applications))
	for _, app := range plan.applications {
		for _, role := range filterAppMetricRoles(plan.activeRoles, app) {
			port, err := r.resolveAppRolePort(role, app)
			if err != nil {
				return nil, err
			}
			entries = append(entries, metricsTargetEntry{
				Targets: []string{plan.host + ":" + port},
				Labels:  observabilityTargetLabels(plan.identity, role.Name, app.Name),
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
	discovery, err := projectlayout.Discover(projectDir)
	if err != nil {
		return nil, fmt.Errorf("discover observability apps: %w", err)
	}

	conventionalApps := discovery.ConventionalApps()
	apps := make([]observabilityApp, 0, len(conventionalApps))
	for index, app := range conventionalApps {
		apps = append(apps, observabilityApp{
			Name:        app.Name,
			Index:       index,
			EnvPrefix:   project.AppEnvironmentPrefix(app.Name),
			RuntimeBase: 10000 + index*10,
			Components:  observabilityAppComponents(config, app.Name, activeRoles),
		})
	}
	return apps, nil
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
func (r observabilityTargetResolver) resolveObservabilityMetricsMode() (string, error) {
	mode := strings.ToLower(r.envOrDefault("OBSERVABILITY_METRICS_TARGET_MODE", observabilityMetricsModeAuto))
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
func (r observabilityTargetResolver) resolveMetricsBasePort() (int, error) {
	value := r.envOrDefault("METRICS_PORT", "9100")
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid METRICS_PORT %q: %w", value, err)
	}
	return port, nil
}

// resolveRolePort honors explicit role ports while retaining deterministic offsets from the shared base port.
func (r observabilityTargetResolver) resolveRolePort(role metricsTargetRole, basePort int) (string, error) {
	if value, ok := r.lookupEnvTrimmed(role.PortEnv); ok {
		port, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("invalid %s %q: %w", role.PortEnv, value, err)
		}
		return strconv.Itoa(port), nil
	}
	return strconv.Itoa(basePort + role.Offset), nil
}

// resolveAppRolePort applies the same app-scoped override order used by generated runtime helpers.
func (r observabilityTargetResolver) resolveAppRolePort(role metricsTargetRole, app observabilityApp) (string, error) {
	envKeys := appRolePortEnvKeys(role, app)
	if match, ok := r.firstEnvTrimmed(envKeys); ok {
		port, err := strconv.Atoi(match.value)
		if err != nil {
			return "", fmt.Errorf("invalid %s %q: %w", match.key, match.value, err)
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

// observabilityAppEnvKeys prevents additional apps from consuming default-app globals.
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
func (r observabilityTargetResolver) resolveLocalMetricsHost() (string, bool) {
	if value, ok := r.lookupEnvTrimmed("OBSERVABILITY_METRICS_TARGET_HOST"); ok {
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "host.docker.internal", true
}

// resolveComposeMetricsHost defaults to stable Compose service names while allowing individual roles to be excluded.
func (r observabilityTargetResolver) resolveComposeMetricsHost(role metricsTargetRole) (string, bool) {
	if value, ok := r.lookupEnvTrimmed(role.HostEnv); ok {
		if value == "" {
			return "", false
		}
		return value, true
	}
	return role.Name, true
}

// resolveStandaloneMetricsPort keeps automated scrapes on the App's dedicated metrics listener instead of its public HTTP middleware path.
func (r observabilityTargetResolver) resolveStandaloneMetricsPort(app observabilityApp) (string, error) {
	return r.resolveAppRolePort(metricsTargetRole{Name: "api", Offset: 0}, app)
}

// buildRoleTargets applies one endpoint policy across roles so Compose and future layouts share labeling and failure behavior.
func buildRoleTargets(
	identity observabilityTargetIdentity,
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
			Labels:  observabilityTargetLabels(identity, role.Name, project.DefaultAppName),
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
func observabilityTargetLabels(identity observabilityTargetIdentity, process string, appName string) map[string]string {
	labels := map[string]string{
		"app":         appName,
		"environment": identity.appEnv,
		"process":     process,
		"service":     identity.service,
	}
	if identity.release != "" {
		labels["release"] = identity.release
	}
	if identity.revision != "" {
		labels["revision"] = identity.revision
	}
	return labels
}

// optionalDeploymentMetadata accepts only bounded, token-safe deployment identity values so target labels cannot become free-form fields.
func (r observabilityTargetResolver) optionalDeploymentMetadata(key string) (string, error) {
	value, ok := r.lookupEnvTrimmed(key)
	if !ok || value == "" {
		return "", nil
	}
	if len(value) > maxDeploymentMetadataLabelLength {
		return "", fmt.Errorf("invalid %s: value exceeds %d characters", key, maxDeploymentMetadataLabelLength)
	}
	if !deploymentMetadataLabelPattern.MatchString(value) {
		return "", fmt.Errorf("invalid %s %q: use a non-whitespace release or immutable revision token", key, value)
	}
	return value, nil
}

// firstEnvTrimmed returns the winning key as well as the value so validation errors can name it.
func (r observabilityTargetResolver) firstEnvTrimmed(keys []string) (observabilityEnvironmentMatch, bool) {
	for _, key := range keys {
		value, ok := r.lookupEnvTrimmed(key)
		if ok && value != "" {
			return observabilityEnvironmentMatch{key: key, value: value}, true
		}
	}
	return observabilityEnvironmentMatch{}, false
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
func (r observabilityTargetResolver) envOrDefault(key string, defaultValue string) string {
	if value, ok := r.lookupEnvTrimmed(key); ok {
		if value != "" {
			return value
		}
		return defaultValue
	}
	return strings.TrimSpace(defaultValue)
}

// lookupEnvTrimmed preserves whether a variable was explicitly set because empty values can disable optional targets.
func (r observabilityTargetResolver) lookupEnvTrimmed(key string) (string, bool) {
	value, ok := r.environment.Lookup(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}
