package forj

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/goforj/goforj/internal/managedsession"
	"github.com/goforj/goforj/project"
)

var managedRuntimeOverlay = struct {
	sync.RWMutex
	values map[string]string
}{}

// managedRuntimePlanRequest projects the active GoForj development Apps into the exact Harbor plan fence.
func managedRuntimePlanRequest(config *project.Config, registration managedsession.RegisterResponse) (managedsession.RuntimePlanRequest, error) {
	if config == nil {
		return managedsession.RuntimePlanRequest{}, errors.New("managed runtime plan requires a project configuration")
	}
	if err := registration.Validate(); err != nil {
		return managedsession.RuntimePlanRequest{}, fmt.Errorf("validate managed runtime plan registration: %w", err)
	}
	activeApps := activeDevAppsForConfig(config)
	requestApps := make([]managedsession.ActiveApp, 0, len(activeApps))
	seen := make(map[string]struct{}, len(activeApps))
	for _, app := range activeApps {
		if app.Name == "" {
			return managedsession.RuntimePlanRequest{}, errors.New("managed runtime plan App name must not be empty")
		}
		if _, duplicate := seen[app.Name]; duplicate {
			return managedsession.RuntimePlanRequest{}, fmt.Errorf("managed runtime plan App %q is duplicated", app.Name)
		}
		seen[app.Name] = struct{}{}
		runtimeIDs := []string{}
		components := projectDescribeAppComponents(config, app.Name)
		if components.WebAPI || components.WebUI {
			runtimeIDs = append(runtimeIDs, "http")
		}
		requestApps = append(requestApps, managedsession.ActiveApp{ID: app.Name, RuntimeIDs: runtimeIDs})
	}
	sort.Slice(requestApps, func(left, right int) bool { return requestApps[left].ID < requestApps[right].ID })
	request := managedsession.RuntimePlanRequest{
		SchemaVersion: managedsession.SchemaVersion,
		Fence:         registration.Fence,
		ActiveApps:    requestApps,
	}
	if err := request.Validate(); err != nil {
		return managedsession.RuntimePlanRequest{}, fmt.Errorf("validate managed runtime plan request: %w", err)
	}
	return request, nil
}

// installManagedRuntimeOverlay installs one process-local assignment layer and returns an idempotent restoration closure.
func installManagedRuntimeOverlay(plan managedsession.RuntimePlan) (func(), error) {
	values, err := managedRuntimePlanEnvironment(plan)
	if err != nil {
		return nil, err
	}
	managedRuntimeOverlay.Lock()
	previous := cloneEnvironmentMap(managedRuntimeOverlay.values)
	managedRuntimeOverlay.values = values
	managedRuntimeOverlay.Unlock()
	var restoreOnce sync.Once
	return func() {
		restoreOnce.Do(func() {
			managedRuntimeOverlay.Lock()
			managedRuntimeOverlay.values = previous
			managedRuntimeOverlay.Unlock()
		})
	}, nil
}

// managedRuntimePlanEnvironment translates the v1 HTTP assignment into the generated App environment keys.
func managedRuntimePlanEnvironment(plan managedsession.RuntimePlan) (map[string]string, error) {
	if err := plan.Validate(); err != nil {
		return nil, fmt.Errorf("validate managed runtime overlay plan: %w", err)
	}
	values := make(map[string]string)
	for _, app := range plan.Apps {
		for _, runtime := range app.Runtimes {
			if runtime.ID != "http" {
				return nil, fmt.Errorf("managed runtime overlay does not support runtime %q", runtime.ID)
			}
			if err := setManagedRuntimeOverlayValue(values, "API_HTTP_HOST", runtime.BindHost); err != nil {
				return nil, err
			}
			prefix := project.AppEnvironmentPrefix(app.ID)
			port := strconv.Itoa(int(runtime.BindPort))
			keys := []string{"API_HTTP_PORT", "PORT"}
			if prefix != "" {
				keys = []string{prefix + "_API_HTTP_PORT", prefix + "_PORT"}
			}
			for _, key := range keys {
				if err := setManagedRuntimeOverlayValue(values, key, port); err != nil {
					return nil, err
				}
			}
		}
	}
	return values, nil
}

// setManagedRuntimeOverlayValue rejects contradictory assignments before they reach child processes.
func setManagedRuntimeOverlayValue(values map[string]string, key string, value string) error {
	if key == "" || value == "" {
		return errors.New("managed runtime overlay assignments must have non-empty keys and values")
	}
	if previous, exists := values[key]; exists && previous != value {
		return fmt.Errorf("managed runtime overlay key %q has conflicting assignments %q and %q", key, previous, value)
	}
	values[key] = value
	return nil
}

// currentManagedRuntimeOverlay returns a defensive snapshot for one child-process environment merge.
func currentManagedRuntimeOverlay() map[string]string {
	managedRuntimeOverlay.RLock()
	defer managedRuntimeOverlay.RUnlock()
	return cloneEnvironmentMap(managedRuntimeOverlay.values)
}

// cloneEnvironmentMap keeps process-local overlay state independent from callers and restoration closures.
func cloneEnvironmentMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
