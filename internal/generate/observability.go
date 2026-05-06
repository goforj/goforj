package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	observabilityMetricsModeAuto        = "auto"
	observabilityMetricsModeLocalSingle = "local-single"
	observabilityMetricsModeLocalMulti  = "local-multi"
	observabilityMetricsModeCompose     = "compose"
	observabilityMetricsModeDisabled    = "disabled"
)

type metricsTargetEntry struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels,omitempty"`
}

type metricsTargetRole struct {
	Name    string
	Path    string
	PortEnv string
	Offset  int
	HostEnv string
}

type observabilityTargetPlan struct {
	Entries []metricsTargetEntry
	Manage  bool
}

var observabilityMetricRoles = []metricsTargetRole{
	{Name: "api", Path: filepath.Join("internal", "http"), PortEnv: "METRICS_API_PORT", Offset: 0, HostEnv: "OBSERVABILITY_API_METRICS_HOST"},
	{Name: "jobs", Path: filepath.Join("internal", "jobs"), PortEnv: "METRICS_JOBS_PORT", Offset: 1, HostEnv: "OBSERVABILITY_JOBS_METRICS_HOST"},
	{Name: "scheduler", Path: filepath.Join("internal", "scheduler"), PortEnv: "METRICS_SCHEDULER_PORT", Offset: 2, HostEnv: "OBSERVABILITY_SCHEDULER_METRICS_HOST"},
}

func GenerateObservabilityFiles(projectDir string) (int, error) {
	plan, err := buildMetricsTargets(projectDir)
	if err != nil {
		return 0, err
	}
	if !plan.Manage {
		return 0, nil
	}
	if len(plan.Entries) == 0 {
		return 0, nil
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

func buildMetricsTargets(projectDir string) (observabilityTargetPlan, error) {
	service := envOrDefault("APP_NAME", filepath.Base(projectDir))
	environment := envOrDefault("APP_ENV", "local")
	activeRoles, err := discoverObservabilityMetricRoles(projectDir)
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	if len(activeRoles) == 0 {
		return observabilityTargetPlan{Manage: true}, nil
	}

	mode, err := resolveObservabilityMetricsMode(activeRoles)
	if err != nil {
		return observabilityTargetPlan{}, err
	}
	if mode == observabilityMetricsModeDisabled {
		return observabilityTargetPlan{Manage: false}, nil
	}

	basePort, err := resolveMetricsBasePort()
	if err != nil {
		return observabilityTargetPlan{}, err
	}

	switch mode {
	case observabilityMetricsModeLocalSingle:
		host, ok := resolveLocalMetricsHost()
		if !ok {
			return observabilityTargetPlan{Manage: false}, nil
		}
		targetPort, err := resolveStandaloneMetricsPort(activeRoles)
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{
			Manage: true,
			Entries: []metricsTargetEntry{
				{
					Targets: []string{fmt.Sprintf("%s:%s", host, targetPort)},
					Labels: map[string]string{
						"environment": environment,
						"process":     "app",
						"service":     service,
					},
				},
			},
		}, nil
	case observabilityMetricsModeLocalMulti:
		host, ok := resolveLocalMetricsHost()
		if !ok {
			return observabilityTargetPlan{Manage: false}, nil
		}
		entries, err := buildRoleTargets(service, environment, activeRoles, func(role metricsTargetRole) (string, string, bool, error) {
			port, err := resolveRolePort(role, basePort)
			if err != nil {
				return "", "", false, err
			}
			return host, port, true, nil
		})
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	case observabilityMetricsModeCompose:
		entries, err := buildRoleTargets(service, environment, activeRoles, func(role metricsTargetRole) (string, string, bool, error) {
			host, ok := resolveComposeMetricsHost(role)
			if !ok {
				return "", "", false, nil
			}
			return host, strconv.Itoa(basePort), true, nil
		})
		if err != nil {
			return observabilityTargetPlan{}, err
		}
		if len(entries) == 0 {
			return observabilityTargetPlan{Manage: false}, nil
		}
		return observabilityTargetPlan{Manage: true, Entries: entries}, nil
	default:
		return observabilityTargetPlan{}, fmt.Errorf("unsupported OBSERVABILITY_METRICS_TARGET_MODE %q", mode)
	}
}

func discoverObservabilityMetricRoles(projectDir string) ([]metricsTargetRole, error) {
	roles := make([]metricsTargetRole, 0, len(observabilityMetricRoles))
	for _, role := range observabilityMetricRoles {
		if _, err := os.Stat(filepath.Join(projectDir, role.Path)); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, nil
}

func resolveObservabilityMetricsMode(activeRoles []metricsTargetRole) (string, error) {
	mode := strings.ToLower(envOrDefault("OBSERVABILITY_METRICS_TARGET_MODE", observabilityMetricsModeAuto))
	switch mode {
	case "", observabilityMetricsModeAuto:
		runtimeMode := strings.ToLower(envOrDefault("RUNTIME_MODE", "standalone"))
		switch runtimeMode {
		case "", "standalone":
			return observabilityMetricsModeLocalSingle, nil
		case "distributed":
			if len(activeRoles) <= 1 {
				return observabilityMetricsModeLocalSingle, nil
			}
			return observabilityMetricsModeLocalMulti, nil
		default:
			return "", fmt.Errorf("unknown RUNTIME_MODE %q", runtimeMode)
		}
	case observabilityMetricsModeLocalSingle, observabilityMetricsModeLocalMulti, observabilityMetricsModeCompose, observabilityMetricsModeDisabled:
		return mode, nil
	default:
		return "", fmt.Errorf("unknown OBSERVABILITY_METRICS_TARGET_MODE %q", mode)
	}
}

func resolveMetricsBasePort() (int, error) {
	value := envOrDefault("METRICS_PORT", "9100")
	port, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid METRICS_PORT %q: %w", value, err)
	}
	return port, nil
}

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

func resolveLocalMetricsHost() (string, bool) {
	if value, ok := lookupEnvTrimmed("OBSERVABILITY_METRICS_TARGET_HOST"); ok {
		if value == "" {
			return "", false
		}
		return value, true
	}
	return "host.docker.internal", true
}

func resolveComposeMetricsHost(role metricsTargetRole) (string, bool) {
	if value, ok := lookupEnvTrimmed(role.HostEnv); ok {
		if value == "" {
			return "", false
		}
		return value, true
	}
	return role.Name, true
}

func resolveStandaloneMetricsPort(activeRoles []metricsTargetRole) (string, error) {
	if containsObservabilityRole(activeRoles, "api") {
		return envOrDefault("API_HTTP_PORT", "3000"), nil
	}
	basePort, err := resolveMetricsBasePort()
	if err != nil {
		return "", err
	}
	return strconv.Itoa(basePort), nil
}

func buildRoleTargets(
	service string,
	environment string,
	roles []metricsTargetRole,
	resolve func(role metricsTargetRole) (host string, port string, ok bool, err error),
) ([]metricsTargetEntry, error) {
	entries := make([]metricsTargetEntry, 0, len(roles))
	for _, role := range roles {
		host, port, ok, err := resolve(role)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, nil
		}
		entries = append(entries, metricsTargetEntry{
			Targets: []string{host + ":" + port},
			Labels: map[string]string{
				"environment": environment,
				"process":     role.Name,
				"service":     service,
			},
		})
	}
	return entries, nil
}

func containsObservabilityRole(roles []metricsTargetRole, name string) bool {
	for _, role := range roles {
		if role.Name == name {
			return true
		}
	}
	return false
}

func envOrDefault(key string, defaultValue string) string {
	if value, ok := lookupEnvTrimmed(key); ok {
		if value != "" {
			return value
		}
		return defaultValue
	}
	return strings.TrimSpace(defaultValue)
}

func lookupEnvTrimmed(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(value), true
}
