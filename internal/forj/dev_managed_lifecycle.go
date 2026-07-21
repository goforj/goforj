package forj

import (
	"context"
	"fmt"
	"io"

	"github.com/goforj/goforj/project"
)

// managedDevLifecyclePlan groups startup tasks by their explicit managed phase.
type managedDevLifecyclePlan struct {
	preCompose  []project.DevTask
	compose     []project.DevTask
	postCompose []project.DevTask
	postMigrate []project.DevTask
}

// managedDevTeardownPlan groups teardown tasks by their explicit managed phase.
type managedDevTeardownPlan struct {
	preComposeDown  []project.DevTask
	composeDown     []project.DevTask
	postComposeDown []project.DevTask
}

// validateManagedDevLifecycle validates the task contract Harbor relies on without mutating the loaded project.
func validateManagedDevLifecycle(config *project.Config) error {
	if config == nil {
		return nil
	}

	normalized := normalizedManagedDevConfig(config)

	if err := normalized.Dev.ValidateManagedTaskPhases(); err != nil {
		return fmt.Errorf("managed development lifecycle requires explicit task phases; run forj render or phase custom tasks: %w", err)
	}
	return nil
}

// normalizedManagedDevConfig returns a read-only migration view for managed startup planning.
func normalizedManagedDevConfig(config *project.Config) project.Config {
	normalized := *config
	normalized.Dev = config.Dev
	normalized.Dev.Pre = append([]project.DevTask(nil), config.Dev.Pre...)
	normalized.Dev.Down = append([]project.DevTask(nil), config.Dev.Down...)
	normalizeDockerComposeUpTask(&normalized.Dev.Pre, normalized.Render.Components)
	normalizeGeneratedDatabaseWaitTask(&normalized.Dev.Pre)
	normalizeDockerComposeDownTask(&normalized.Dev.Down)
	migrateGeneratedDevFrontendInstallTasks(&normalized)
	return normalized
}

// planManagedDevLifecycle returns the effective startup tasks in their explicit phase order.
func planManagedDevLifecycle(config *project.Config) (managedDevLifecyclePlan, error) {
	if config == nil {
		return managedDevLifecyclePlan{}, nil
	}
	normalized := normalizedManagedDevConfig(config)
	normalized.Dev.Pre = effectiveDevPreTasks(&normalized)
	if err := normalized.Dev.ValidateManagedTaskPhases(); err != nil {
		return managedDevLifecyclePlan{}, fmt.Errorf("managed development lifecycle requires explicit task phases; run forj render or phase custom tasks: %w", err)
	}
	plan := managedDevLifecyclePlan{}
	for _, task := range normalized.Dev.Pre {
		switch task.Phase {
		case project.DevTaskPhasePreCompose:
			plan.preCompose = append(plan.preCompose, task)
		case project.DevTaskPhaseCompose:
			plan.compose = append(plan.compose, task)
		case project.DevTaskPhasePostCompose:
			plan.postCompose = append(plan.postCompose, task)
		case project.DevTaskPhasePostMigrate:
			plan.postMigrate = append(plan.postMigrate, task)
		default:
			return managedDevLifecyclePlan{}, fmt.Errorf("managed development startup task %q has unsupported phase %q", task.Name, task.Phase)
		}
	}
	return plan, nil
}

// runManagedDevInitialLifecycle executes explicit startup phases around Harbor's authoritative Compose barrier.
func runManagedDevInitialLifecycle(
	config *project.Config,
	outWriter io.Writer,
	errWriter io.Writer,
	ctx context.Context,
	barrier func(context.Context) error,
) error {
	return runManagedDevInitialLifecycleWithPlan(config, outWriter, errWriter, ctx, barrier, nil)
}

// runManagedDevInitialLifecycleWithPlan inserts one optional trusted runtime overlay after Compose is admitted.
func runManagedDevInitialLifecycleWithPlan(
	config *project.Config,
	outWriter io.Writer,
	errWriter io.Writer,
	ctx context.Context,
	barrier func(context.Context) error,
	runtimePlan func(context.Context) error,
) error {
	plan, err := planManagedDevLifecycle(config)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	spaBuilt := false
	if config.Dev.UsesStructuredApps() {
		if err := runDevTasks("Running pre-compose setup", plan.preCompose); err != nil {
			return err
		}
		spaBuilt, err = runDevInitialSPABuilds(config, outWriter, errWriter)
		if err != nil {
			return err
		}
		writeDevAppBuildLine(outWriter, activeDevAppsForConfig(config))
		if err := runDevInitialBuild(config, outWriter, errWriter); err != nil {
			return err
		}
	} else {
		writeDevAppBuildLine(outWriter, activeDevAppsForConfig(config))
		if err := runDevInitialBuild(config, outWriter, errWriter); err != nil {
			return err
		}
		if err := runDevTasks("Running pre-compose setup", plan.preCompose); err != nil {
			return err
		}
		spaBuilt, err = runDevInitialSPABuilds(config, outWriter, errWriter)
		if err != nil {
			return err
		}
	}

	if err := runDevTasks("Running Compose", plan.compose); err != nil {
		return err
	}
	if barrier != nil {
		if err := barrier(ctx); err != nil {
			return fmt.Errorf("managed Compose barrier failed: %w", err)
		}
	}
	if runtimePlan != nil {
		if err := runtimePlan(ctx); err != nil {
			return fmt.Errorf("managed runtime plan failed: %w", err)
		}
	}
	if err := runDevTasks("Running post-compose setup", plan.postCompose); err != nil {
		return err
	}
	if err := runDevAppSetup(config, outWriter, errWriter); err != nil {
		return err
	}
	if err := runDevTasks("Running post-migrate setup", plan.postMigrate); err != nil {
		return err
	}
	if spaBuilt || len(plan.postMigrate) > 0 {
		if err := runDevBuild(config, outWriter, errWriter); err != nil {
			return fmt.Errorf("post-setup forj build failed: %w", err)
		}
	}
	return nil
}

// planManagedDevTeardown returns effective teardown tasks in their explicit phase order.
func planManagedDevTeardown(config *project.Config) (managedDevTeardownPlan, error) {
	if config == nil {
		return managedDevTeardownPlan{}, nil
	}
	normalized := normalizedManagedDevConfig(config)
	normalized.Dev.Down = effectiveDevDownTasks(&normalized)
	if err := normalized.Dev.ValidateManagedTaskPhases(); err != nil {
		return managedDevTeardownPlan{}, fmt.Errorf("managed development teardown requires explicit task phases; run forj render or phase custom tasks: %w", err)
	}
	plan := managedDevTeardownPlan{}
	for _, task := range normalized.Dev.Down {
		switch task.Phase {
		case project.DevTaskPhasePreComposeDown:
			plan.preComposeDown = append(plan.preComposeDown, task)
		case project.DevTaskPhaseComposeDown:
			plan.composeDown = append(plan.composeDown, task)
		case project.DevTaskPhasePostComposeDown:
			plan.postComposeDown = append(plan.postComposeDown, task)
		default:
			return managedDevTeardownPlan{}, fmt.Errorf("managed development teardown task %q has unsupported phase %q", task.Name, task.Phase)
		}
	}
	return plan, nil
}

// runManagedDevDownTasks executes Harbor-owned teardown in the explicit typed phase order.
func runManagedDevDownTasks(config *project.Config) error {
	plan, err := planManagedDevTeardown(config)
	if err != nil {
		return err
	}
	tasks := make([]project.DevTask, 0, len(plan.preComposeDown)+len(plan.composeDown)+len(plan.postComposeDown))
	tasks = append(tasks, plan.preComposeDown...)
	tasks = append(tasks, plan.composeDown...)
	tasks = append(tasks, plan.postComposeDown...)
	return runDevDownTasks(tasks)
}
