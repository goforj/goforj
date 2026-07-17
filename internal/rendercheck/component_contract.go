package rendercheck

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goforj/str"

	"github.com/goforj/goforj/project"
)

// renderedPrimitiveContract captures one primitive's high-signal generated surface without mirroring every template.
type renderedPrimitiveContract struct {
	key               project.ComponentKey
	label             string
	packageRoots      []string
	corePaths         []string
	supportPaths      []string
	documentationPath string
	dashboardPath     string
	environmentPrefix string
	environmentKeys   []string
	modulePath        string
	runtimeMarkers    []renderedContentMarker
	appMarkers        []string
}

// renderedContentMarker keeps shared runtime files testable without snapshotting their unrelated contents.
type renderedContentMarker struct {
	path   string
	marker string
}

var renderedPrimitiveContracts = []renderedPrimitiveContract{
	{
		key:          project.ComponentCache,
		label:        "Cache",
		packageRoots: []string{"internal/caches"},
		corePaths:    []string{"internal/caches/manager_gen.go"},
		supportPaths: []string{
			"internal/cmd/cache_shell_cmd.go",
			"internal/observability/cache_observer.go",
		},
		documentationPath: "internal/caches/README.md",
		dashboardPath:     "containers/observability/grafana/dashboards/cache-overview.json",
		environmentPrefix: "CACHE",
		environmentKeys:   []string{"METRICS_CACHE_ENABLED"},
		modulePath:        "github.com/goforj/cache",
		runtimeMarkers: []renderedContentMarker{
			{path: "internal/runtime/discovery.go", marker: "func DiscoverCacheInstances()"},
		},
		appMarkers: []string{"func (a *App) Cache()", "func (a *App) Caches()"},
	},
	{
		key:          project.ComponentEvents,
		label:        "Events",
		packageRoots: []string{"internal/events"},
		corePaths:    []string{"internal/events/manager_gen.go"},
		supportPaths: []string{
			"internal/cmd/test_event_pipeline_cmd.go",
			"internal/makecmd/event.tmpl",
			"internal/makecmd/make_event_cmd.go",
			"internal/makecmd/make_subscriber_cmd.go",
			"internal/makecmd/subscriber.tmpl",
			"internal/observability/event_observer.go",
		},
		documentationPath: "internal/events/README.md",
		dashboardPath:     "containers/observability/grafana/dashboards/events-overview.json",
		environmentPrefix: "EVENTS",
		environmentKeys:   []string{"METRICS_EVENTS_ENABLED"},
		modulePath:        "github.com/goforj/events",
		runtimeMarkers: []renderedContentMarker{
			{path: "internal/runtime/discovery.go", marker: "func DiscoverEventInstances()"},
		},
		appMarkers: []string{"func (a *App) Bus()", "func (a *App) Events()"},
	},
	{
		key:               project.ComponentStorage,
		label:             "Storage",
		packageRoots:      []string{"internal/storages"},
		corePaths:         []string{"internal/storages/manager_gen.go"},
		supportPaths:      []string{"internal/observability/storage_observer.go"},
		documentationPath: "internal/storages/README.md",
		dashboardPath:     "containers/observability/grafana/dashboards/storage-overview.json",
		environmentPrefix: "STORAGE",
		environmentKeys:   []string{"METRICS_STORAGE_ENABLED"},
		modulePath:        "github.com/goforj/storage",
		runtimeMarkers: []renderedContentMarker{
			{path: "internal/runtime/discovery.go", marker: "func DiscoverStorageInstances()"},
		},
		appMarkers: []string{"func (a *App) Storage()"},
	},
	{
		key:          project.ComponentJobs,
		label:        "Background Jobs",
		packageRoots: []string{"internal/queues", "internal/jobs"},
		corePaths:    []string{"internal/queues/manager_gen.go", "internal/jobs/worker.go"},
		supportPaths: []string{
			"internal/makecmd/job.tmpl",
			"internal/makecmd/make_job_cmd.go",
			"internal/makecmd/make_queue_cmd.go",
			"internal/observability/queue_observer.go",
		},
		documentationPath: "internal/queues/README.md",
		dashboardPath:     "containers/observability/grafana/dashboards/queue-overview.json",
		environmentPrefix: "QUEUE",
		environmentKeys:   []string{"METRICS_JOBS_PORT", "METRICS_QUEUE_ENABLED"},
		modulePath:        "github.com/goforj/queue",
		runtimeMarkers: []renderedContentMarker{
			{path: "internal/runtime/discovery.go", marker: "func DiscoverQueueInstances()"},
			{path: "internal/runtime/timeouts.go", marker: "func (s *Timeouts) QueueShutdownTimeout()"},
		},
		appMarkers: []string{"func (a *App) Queue()", "func (a *App) Queues()"},
	},
}

// validateRenderedComponentContracts proves real renders honor the selected project and per-App primitive boundaries.
func validateRenderedComponentContracts(root string, config *project.Config, apps []project.App) error {
	projectComponents := project.ProjectComponents(config)
	directModules, err := renderedDirectModuleRequirements(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	environments, err := renderedEnvironmentAssignments(root)
	if err != nil {
		return err
	}

	violations := make([]string, 0)
	for _, contract := range renderedPrimitiveContracts {
		enabled := projectComponents.Enabled(contract.key)
		violations = append(violations, contract.validateProject(root, enabled, projectComponents.Grafana, directModules, environments)...)
		for _, app := range apps {
			components := renderedAppComponents(config, app)
			enabled := components.Enabled(contract.key)
			violations = append(violations, contract.validateApp(root, app, enabled)...)
			violations = append(violations, contract.validateAppEnvironment(app, enabled, environments)...)
		}
	}
	if len(violations) == 0 {
		return nil
	}
	sort.Strings(violations)
	return fmt.Errorf("rendered component contract violations:\n- %s", strings.Join(violations, "\n- "))
}

// validateProject checks the shared package, environment, module, documentation, dashboard, and runtime surfaces for one primitive.
func (contract renderedPrimitiveContract) validateProject(root string, enabled bool, grafanaEnabled bool, directModules map[string]bool, environments map[string]map[string]bool) []string {
	violations := make([]string, 0)
	checkPath := func(path string, required bool, category string) {
		exists, err := renderedPathExists(root, path)
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s cannot inspect %s %s: %v", contract.label, category, path, err))
			return
		}
		if required && !exists {
			violations = append(violations, fmt.Sprintf("%s requires %s %s", contract.label, category, path))
		}
		if !required && exists {
			violations = append(violations, fmt.Sprintf("%s is disabled but %s %s exists", contract.label, category, path))
		}
	}
	ownedPaths := append([]string{}, contract.corePaths...)
	ownedPaths = append(ownedPaths, contract.supportPaths...)
	if enabled {
		for _, path := range ownedPaths {
			checkPath(path, true, "file")
		}
		checkPath(contract.documentationPath, true, "documentation")
	} else {
		for _, path := range append(append([]string{}, contract.packageRoots...), contract.supportPaths...) {
			checkPath(path, false, "path")
		}
		checkPath(contract.documentationPath, false, "documentation")
	}

	dashboardRequired := enabled && grafanaEnabled
	dashboardExists, err := renderedPathExists(root, contract.dashboardPath)
	if err != nil {
		violations = append(violations, fmt.Sprintf("%s cannot inspect dashboard %s: %v", contract.label, contract.dashboardPath, err))
	} else if dashboardRequired && !dashboardExists {
		violations = append(violations, fmt.Sprintf("%s requires dashboard %s", contract.label, contract.dashboardPath))
	} else if !dashboardRequired && dashboardExists {
		reason := "the component is disabled"
		if enabled {
			reason = "Grafana is disabled"
		}
		violations = append(violations, fmt.Sprintf("%s dashboard %s exists while %s", contract.label, contract.dashboardPath, reason))
	}

	for path, assignments := range environments {
		if enabled {
			if path != ".env.host" {
				for _, key := range []string{contract.environmentPrefix + "_DRIVER", contract.environmentPrefix + "_SUPPORTED_DRIVERS"} {
					if !assignments[key] {
						violations = append(violations, fmt.Sprintf("%s requires %s in %s", contract.label, key, path))
					}
				}
			}
			continue
		}
		for key := range assignments {
			if contract.ownsEnvironmentKey(key) {
				violations = append(violations, fmt.Sprintf("%s is disabled but %s defines %s", contract.label, path, key))
			}
		}
	}

	if enabled && !directModules[contract.modulePath] {
		violations = append(violations, fmt.Sprintf("%s requires direct module %s", contract.label, contract.modulePath))
	}
	if !enabled {
		for modulePath := range directModules {
			if modulePath == contract.modulePath || strings.HasPrefix(modulePath, contract.modulePath+"/") {
				violations = append(violations, fmt.Sprintf("%s is disabled but directly requires module %s", contract.label, modulePath))
			}
		}
	}

	for _, runtimeMarker := range contract.runtimeMarkers {
		runtimeSource, err := os.ReadFile(filepath.Join(root, runtimeMarker.path))
		if err != nil {
			violations = append(violations, fmt.Sprintf("%s cannot inspect %s: %v", contract.label, runtimeMarker.path, err))
		} else if strings.Contains(string(runtimeSource), runtimeMarker.marker) != enabled {
			state := "requires"
			if !enabled {
				state = "is disabled but retains"
			}
			violations = append(violations, fmt.Sprintf("%s %s runtime marker %q in %s", contract.label, state, runtimeMarker.marker, runtimeMarker.path))
		}
	}
	return violations
}

// validateApp checks that shared project capability never promotes the primitive into an App that did not select it.
func (contract renderedPrimitiveContract) validateApp(root string, app project.App, enabled bool) []string {
	path := filepath.Join(app.WireDir, "app.go")
	source, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return []string{fmt.Sprintf("%s App %s cannot inspect %s: %v", contract.label, app.Name, path, err)}
	}
	violations := make([]string, 0)
	for _, marker := range contract.appMarkers {
		if strings.Contains(string(source), marker) == enabled {
			continue
		}
		state := "requires"
		if !enabled {
			state = "is disabled but retains"
		}
		violations = append(violations, fmt.Sprintf("%s App %s %s marker %q in %s", contract.label, app.Name, state, marker, path))
	}
	return violations
}

// validateAppEnvironment checks the generated driver overlay for one named App without confusing shared root defaults with default-App ownership.
func (contract renderedPrimitiveContract) validateAppEnvironment(app project.App, enabled bool, environments map[string]map[string]bool) []string {
	prefix := project.AppEnvironmentPrefix(app.Name)
	if prefix == "" {
		return nil
	}
	key := prefix + "_" + contract.environmentPrefix + "_DRIVER"
	violations := make([]string, 0)
	for path, assignments := range environments {
		required := enabled && path != ".env.host"
		if assignments[key] == required {
			continue
		}
		if required {
			violations = append(violations, fmt.Sprintf("%s App %s requires %s in %s", contract.label, app.Name, key, path))
			continue
		}
		if enabled {
			violations = append(violations, fmt.Sprintf("%s App %s must not define %s in %s", contract.label, app.Name, key, path))
			continue
		}
		violations = append(violations, fmt.Sprintf("%s App %s is disabled but %s defines %s", contract.label, app.Name, path, key))
	}
	return violations
}

// ownsEnvironmentKey recognizes framework-owned root, metrics, and named-App driver assignments for one primitive.
func (contract renderedPrimitiveContract) ownsEnvironmentKey(key string) bool {
	prefix := contract.environmentPrefix + "_"
	if strings.HasPrefix(key, prefix) || strings.Contains(key, "_"+prefix) {
		return true
	}
	for _, owned := range contract.environmentKeys {
		if key == owned {
			return true
		}
	}
	return false
}

// renderedAppComponents resolves named Apps against the same stable default capability set used by the renderer.
func renderedAppComponents(config *project.Config, app project.App) project.Components {
	if app.Name == project.DefaultAppName {
		return config.Render.Components.WithResolvedDependencies()
	}
	configured, ok := config.Apps[app.Name]
	if !ok {
		return project.Components{}
	}
	return project.NormalizeConfiguredAppComponents(config, configured.Components)
}

// renderedPathExists distinguishes absence from workspace failures so disabled contracts cannot hide unreadable residue.
func renderedPathExists(root string, path string) (bool, error) {
	_, err := os.Stat(filepath.Join(root, path))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// renderedEnvironmentAssignments parses only active dotenv assignments so commented driver hints remain harmless.
func renderedEnvironmentAssignments(root string) (map[string]map[string]bool, error) {
	environments := map[string]map[string]bool{}
	for _, path := range []string{".env", ".env.example", ".env.host"} {
		source, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return nil, fmt.Errorf("inspect rendered %s: %w", path, err)
		}
		assignments := map[string]bool{}
		scanner := bufio.NewScanner(strings.NewReader(string(source)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, _, ok := strings.Cut(line, "=")
			if ok {
				assignments[strings.TrimSpace(key)] = true
			}
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("scan rendered %s: %w", path, err)
		}
		environments[path] = assignments
	}
	return environments, nil
}

// renderedDirectModuleRequirements reads direct requirements without treating transitive module graph entries as selected API surface.
func renderedDirectModuleRequirements(path string) (map[string]bool, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("inspect rendered go.mod: %w", err)
	}
	direct := map[string]bool{}
	inRequireBlock := false
	scanner := bufio.NewScanner(strings.NewReader(string(source)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case line == "require (":
			inRequireBlock = true
			continue
		case inRequireBlock && line == ")":
			inRequireBlock = false
			continue
		case strings.HasPrefix(line, "require "):
			line = str.Of(line).ChopStart("require ").TrimSpace().String()
		case !inRequireBlock:
			continue
		}
		if line == "" || strings.Contains(line, "// indirect") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			direct[fields[0]] = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan rendered go.mod: %w", err)
	}
	return direct, nil
}
