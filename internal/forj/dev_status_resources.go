package forj

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/goforj/goforj/internal/forj/resources"
	"github.com/goforj/goforj/internal/managedenv"
	"github.com/goforj/goforj/project"
)

const (
	// devStatusMaximumResources bounds the host-level project catalog admitted into one observation.
	devStatusMaximumResources = 256
	// devStatusMaximumResourceURLBytes keeps launchable links within desktop and protocol limits.
	devStatusMaximumResourceURLBytes = 2048
)

// DevStatusResource describes one enabled secret-free project resource resolved in the host process.
type DevStatusResource struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	URL         string `json:"url,omitempty"`
	Description string `json:"description,omitempty"`
	App         string `json:"app,omitempty"`
	Service     string `json:"service,omitempty"`
	Runtime     string `json:"runtime,omitempty"`
	Health      string `json:"health,omitempty"`
	Owner       string `json:"owner,omitempty"`
}

// devStatusProjectContext carries one config load and the environment snapshot derived from the same precedence pass.
type devStatusProjectContext struct {
	config      *project.Config
	environment map[string]string
	tasks       []project.DevTask
}

// devStatusProjectLoader resolves all host-owned status inputs without starting a generated App.
type devStatusProjectLoader func() (devStatusProjectContext, error)

// devStatusResourceResolver resolves the project registry in-process so generated commands are never executed.
type devStatusResourceResolver func(context.Context, *project.Config, map[string]string) ([]resources.Resource, error)

// loadDevStatusProject applies development precedence once before loading config, tasks, and host resources.
func loadDevStatusProject() (devStatusProjectContext, error) {
	managedEnvironment, err := managedenv.Capture()
	if err != nil {
		return devStatusProjectContext{}, fmt.Errorf("capture managed environment: %w", err)
	}
	if err := loadDevEnvironment(false, managedEnvironment); err != nil {
		return devStatusProjectContext{}, fmt.Errorf("load development environment: %w", err)
	}
	config, err := project.LoadProjectConfig()
	if err != nil {
		return devStatusProjectContext{}, err
	}
	return devStatusProjectContext{
		config:      config,
		environment: snapshotProcessEnv(),
		tasks:       effectiveDevPreTasks(config),
	}, nil
}

// resolveDevStatusResources reads the host-owned registry directly instead of invoking generated App code.
func resolveDevStatusResources(ctx context.Context, config *project.Config, environment map[string]string) ([]resources.Resource, error) {
	return resources.RegistryForProject(config, environment).List(ctx)
}

// projectDevStatusResources admits only enabled, bounded fields from the host-owned registry.
func projectDevStatusResources(source []resources.Resource) ([]DevStatusResource, error) {
	projected := make([]DevStatusResource, 0, min(len(source), devStatusMaximumResources))
	seen := make(map[string]struct{}, min(len(source), devStatusMaximumResources))
	var joined error
	for index, resource := range source {
		if !resource.Enabled {
			continue
		}
		if len(projected) >= devStatusMaximumResources {
			joined = errors.Join(joined, fmt.Errorf("resource catalog exceeds %d enabled entries", devStatusMaximumResources))
			break
		}
		normalized, err := normalizeDevStatusResource(resource)
		if err != nil {
			joined = errors.Join(joined, fmt.Errorf("resource %d: %w", index+1, err))
			continue
		}
		if _, exists := seen[normalized.ID]; exists {
			joined = errors.Join(joined, fmt.Errorf("resource %d: duplicate ID %q", index+1, normalized.ID))
			continue
		}
		seen[normalized.ID] = struct{}{}
		projected = append(projected, normalized)
	}
	sort.Slice(projected, func(left int, right int) bool {
		if projected[left].Kind != projected[right].Kind {
			return projected[left].Kind < projected[right].Kind
		}
		if projected[left].ID != projected[right].ID {
			return projected[left].ID < projected[right].ID
		}
		return projected[left].Name < projected[right].Name
	})
	return projected, joined
}

// normalizeDevStatusResource converts registry categories into Harbor-facing kinds without copying credential fields.
func normalizeDevStatusResource(resource resources.Resource) (DevStatusResource, error) {
	if !validDevStatusIdentifier(resource.ID, 128) {
		return DevStatusResource{}, fmt.Errorf("ID is missing or unsafe")
	}
	if err := validateDevStatusText("name", resource.Name, 512, true); err != nil {
		return DevStatusResource{}, err
	}
	if !validDevStatusIdentifier(resource.Category, 128) {
		return DevStatusResource{}, fmt.Errorf("kind is missing or unsafe")
	}
	if err := validateDevStatusResourceURL("URL", resource.URL); err != nil {
		return DevStatusResource{}, err
	}
	if err := validateDevStatusText("description", resource.Description, 1024, false); err != nil {
		return DevStatusResource{}, err
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "App", value: resource.App},
		{name: "service", value: resource.Service},
		{name: "runtime", value: resource.Runtime},
		{name: "owner", value: resource.Owner},
	} {
		if field.value != "" && !validDevStatusIdentifier(field.value, 128) {
			return DevStatusResource{}, fmt.Errorf("%s is unsafe", field.name)
		}
	}
	if resource.App != "" && resource.Service != "" {
		return DevStatusResource{}, fmt.Errorf("App and service ownership are mutually exclusive")
	}
	if err := validateDevStatusResourceURL("health URL", resource.Health); err != nil {
		return DevStatusResource{}, err
	}
	return DevStatusResource{
		ID:          resource.ID,
		Name:        resource.Name,
		Kind:        resource.Category,
		URL:         resource.URL,
		Description: resource.Description,
		App:         resource.App,
		Service:     resource.Service,
		Runtime:     resource.Runtime,
		Health:      resource.Health,
		Owner:       resource.Owner,
	}, nil
}

// validateDevStatusText permits bounded human-readable fields while excluding ambiguous control characters.
func validateDevStatusText(name string, value string, maximum int, required bool) error {
	if value == "" {
		if required {
			return fmt.Errorf("%s is missing", name)
		}
		return nil
	}
	if !utf8.ValidString(value) || strings.TrimSpace(value) != value || len(value) > maximum {
		return fmt.Errorf("%s is unsafe or exceeds %d bytes", name, maximum)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

// validateDevStatusResourceURL limits desktop-facing links to bounded absolute HTTP URLs without embedded credentials.
func validateDevStatusResourceURL(name string, rawURL string) error {
	if rawURL == "" {
		return nil
	}
	if len(rawURL) > devStatusMaximumResourceURLBytes || !utf8.ValidString(rawURL) || strings.TrimSpace(rawURL) != rawURL {
		return fmt.Errorf("%s is unsafe or exceeds %d bytes", name, devStatusMaximumResourceURLBytes)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("%s must be an absolute HTTP URL without user information", name)
	}
	return nil
}
