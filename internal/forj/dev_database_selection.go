package forj

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/goforj/goforj/internal/projectlayout"
	"github.com/goforj/goforj/project"
	"github.com/goforj/str/v2"
)

type devDatabase struct {
	App    string
	Driver string
	Name   string
}

// devDatabasesForApps discovers server databases whose Compose services participate in the current dev session.
func devDatabasesForApps(config *project.Config, apps []project.App) ([]devDatabase, error) {
	seen := map[string]bool{}
	databases := make([]devDatabase, 0, len(apps))
	for _, app := range apps {
		app = projectlayout.NormalizeApp(app)
		components := appRenderComponents(config, app)
		if !components.HasDatabase() {
			continue
		}
		driver := normalizeDevDatabaseDriver(appScopedEnvValue(app, "DB_DRIVER"))
		if driver == "" {
			driver = normalizeDevDatabaseDriver(components.DatabaseDriver())
		}
		if driver == "" || driver == "sqlite" || composeServiceDisabled(driver) {
			continue
		}
		name := appScopedEnvValue(app, "DB_DATABASE")
		if name == "" {
			return nil, fmt.Errorf("missing %s for %s database app %s", appScopedEnvKey(app, "DB_DATABASE"), driver, app.Name)
		}
		if err := validateDevDatabaseName(name); err != nil {
			return nil, fmt.Errorf("%s for app %s: %w", appScopedEnvKey(app, "DB_DATABASE"), app.Name, err)
		}
		key := driver + "\x00" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		databases = append(databases, devDatabase{App: app.Name, Driver: driver, Name: name})
	}
	sort.Slice(databases, func(left, right int) bool {
		if databases[left].Driver != databases[right].Driver {
			return databases[left].Driver < databases[right].Driver
		}
		return databases[left].Name < databases[right].Name
	})
	return databases, nil
}

// normalizeDevDatabaseDriver maps common aliases to the compose service driver names.
func normalizeDevDatabaseDriver(driver string) string {
	switch str.Of(driver).Trim().ToLower().String() {
	case "mysql", "mariadb":
		return "mysql"
	case "postgres", "postgresql":
		return "postgres"
	case "sqlite", "sqlite3":
		return "sqlite"
	default:
		return str.Of(driver).Trim().ToLower().String()
	}
}

// appScopedEnvValue reads app-specific env with the same fallback shape used by app runtime.
func appScopedEnvValue(app project.App, key string) string {
	if app.Name != "" && app.Name != project.DefaultAppName {
		if value := strings.TrimSpace(os.Getenv(appScopedEnvKey(app, key))); value != "" {
			return value
		}
	}
	return strings.TrimSpace(os.Getenv(key))
}

// appScopedEnvKey returns the app-prefixed env key that overrides a base env value.
func appScopedEnvKey(app project.App, key string) string {
	if app.Name == "" || app.Name == project.DefaultAppName {
		return key
	}
	prefix := project.AppEnvironmentPrefix(app.Name)
	if prefix == "" {
		return key
	}
	return prefix + "_" + key
}

// validateDevDatabaseName keeps local database creation scripts simple and injection-safe.
func validateDevDatabaseName(name string) error {
	if name == "" {
		return fmt.Errorf("database name is empty")
	}
	for _, char := range name {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '_' {
			continue
		}
		return fmt.Errorf("database name %q must contain only letters, numbers, and underscores", name)
	}
	return nil
}
