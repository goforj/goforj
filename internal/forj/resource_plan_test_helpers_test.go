package forj

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// defaultResourcePlanForTest builds the same concrete driver plan used by a new project.
func defaultResourcePlanForTest(t *testing.T, components project.Components) project.ResourcePlan {
	t.Helper()
	plan, err := project.DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	return plan
}

// initializeDefaultResourceStateForTest gives direct renderer helper tests the validated state normally prepared by Render.
func initializeDefaultResourceStateForTest(t *testing.T, renderer *ProjectRenderer) {
	t.Helper()
	renderer.resources.plan = defaultResourcePlanForTest(t, project.ProjectComponents(renderer.config))
}

// redisResourcePlanForTest derives an explicit Redis-active plan without reintroducing a persisted preset identity.
func redisResourcePlanForTest(t *testing.T, components project.Components) project.ResourcePlan {
	t.Helper()
	plan := defaultResourcePlanForTest(t, components)
	for _, key := range []project.ResourceKey{project.ResourceCache, project.ResourceQueue, project.ResourceEvents} {
		selection, ok := plan.Selection(key)
		if !ok {
			continue
		}
		selection.Active = "redis"
		plan = plan.WithSelection(key, selection)
	}
	if components.Auth {
		plan = plan.WithNamedSelection("CACHE_SESSIONS_DRIVER", "redis")
	}
	normalized, err := plan.Normalized(components)
	if err != nil {
		t.Fatalf("normalize Redis resource plan: %v", err)
	}
	return normalized
}

// queueDriverResourcePlanForTest replaces the Queue selection without relying on retired renderer-only state.
func queueDriverResourcePlanForTest(t *testing.T, components project.Components, driver string) project.ResourcePlan {
	t.Helper()
	plan := defaultResourcePlanForTest(t, components)
	plan = plan.WithSelection(project.ResourceQueue, project.DriverSelection{
		Active:    driver,
		Supported: []string{driver},
	})
	normalized, err := plan.Normalized(components)
	if err != nil {
		t.Fatalf("normalize %s Queue resource plan: %v", driver, err)
	}
	return normalized
}
