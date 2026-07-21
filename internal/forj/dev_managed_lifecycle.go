package forj

import (
	"fmt"

	"github.com/goforj/goforj/project"
)

// validateManagedDevLifecycle validates the task contract Harbor relies on without mutating the loaded project.
func validateManagedDevLifecycle(config *project.Config) error {
	if config == nil {
		return nil
	}

	// Render migrations are deliberately read-only here: managed admission may happen before a user chooses to persist a render.
	normalized := *config
	normalized.Dev = config.Dev
	normalized.Dev.Pre = append([]project.DevTask(nil), config.Dev.Pre...)
	normalized.Dev.Down = append([]project.DevTask(nil), config.Dev.Down...)
	normalizeDockerComposeUpTask(&normalized.Dev.Pre, normalized.Render.Components)
	normalizeGeneratedDatabaseWaitTask(&normalized.Dev.Pre)
	normalizeDockerComposeDownTask(&normalized.Dev.Down)
	migrateGeneratedDevFrontendInstallTasks(&normalized)

	if err := normalized.Dev.ValidateManagedTaskPhases(); err != nil {
		return fmt.Errorf("managed development lifecycle requires explicit task phases; run forj render or phase custom tasks: %w", err)
	}
	return nil
}
