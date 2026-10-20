package forj

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestPlanNewProjectServiceTasksSkipsAvailableRedis verifies a support-only profiled definition does not create an empty Compose startup.
func TestPlanNewProjectServiceTasksSkipsAvailableRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	_, servicePlan := resolveNewProjectServiceTestPlans(t, components, project.LocalServiceIntent{}, false)
	requirement, exists := servicePlan.Requirement(project.ServiceRedis)
	if !exists || requirement.State != project.ServiceStateAvailableLocal {
		t.Fatalf("Redis requirement = %#v, exists %v; want available local", requirement, exists)
	}

	tasks := planNewProjectServiceTasks(servicePlan, components)
	if len(tasks.Pre) != 0 || len(tasks.Down) != 0 {
		t.Fatalf("service tasks = %#v, want no Compose lifecycle", tasks)
	}
}

// TestPlanNewProjectServiceTasksStartsActiveRedis verifies a locally managed active Redis resource starts one Compose lifecycle.
func TestPlanNewProjectServiceTasksStartsActiveRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	_, servicePlan := resolveNewProjectServiceTestPlans(t, components, intent, true)

	tasks := planNewProjectServiceTasks(servicePlan, components)
	wantPre := []project.DevTask{{Name: "Run Docker Compose", Cmd: "docker-compose up -d"}}
	wantDown := []project.DevTask{{Name: "Docker Compose Down", Cmd: dockerComposeDownDevCommand()}}
	if !reflect.DeepEqual(tasks.Pre, wantPre) {
		t.Fatalf("pre tasks = %#v, want %#v", tasks.Pre, wantPre)
	}
	if !reflect.DeepEqual(tasks.Down, wantDown) {
		t.Fatalf("down tasks = %#v, want %#v", tasks.Down, wantDown)
	}
}

// TestPlanNewProjectServiceTasksHonorsRetainedUnusedRedis verifies explicit owner intent remains an actual Compose lifecycle.
func TestPlanNewProjectServiceTasksHonorsRetainedUnusedRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	_, servicePlan := resolveNewProjectServiceTestPlans(t, components, intent, false)
	requirement, exists := servicePlan.Requirement(project.ServiceRedis)
	if !exists || requirement.State != project.ServiceStateLocalRequestedUnused {
		t.Fatalf("Redis requirement = %#v, exists %v; want retained unused local service", requirement, exists)
	}

	tasks := planNewProjectServiceTasks(servicePlan, components)
	if len(tasks.Pre) != 1 || tasks.Pre[0].Name != "Run Docker Compose" || len(tasks.Down) != 1 {
		t.Fatalf("retained unused Redis lifecycle = %#v", tasks)
	}
}

// TestPlanNewProjectServiceTasksSkipsExternalRedis verifies Docker capability alone does not start an externally placed active resource.
func TestPlanNewProjectServiceTasksSkipsExternalRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal)
	_, servicePlan := resolveNewProjectServiceTestPlans(t, components, intent, true)

	tasks := planNewProjectServiceTasks(servicePlan, components)
	if len(tasks.Pre) != 0 || len(tasks.Down) != 0 {
		t.Fatalf("service tasks = %#v, want no Compose lifecycle", tasks)
	}
}

// TestPlanNewProjectServiceTasksStartsDockerBackedTools verifies each current Compose development-tool gate retains startup independently of App services.
func TestPlanNewProjectServiceTasksStartsDockerBackedTools(t *testing.T) {
	tests := []struct {
		name       string
		components project.Components
	}{
		{name: "Mailpit", components: project.Components{DatabaseSQLite: true, Docker: true, Mail: true}},
		{name: "VictoriaMetrics", components: project.Components{DatabaseSQLite: true, Docker: true, Observability: true}},
		{name: "Grafana", components: project.Components{DatabaseSQLite: true, Docker: true, Grafana: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, servicePlan := resolveNewProjectServiceTestPlans(t, test.components, project.LocalServiceIntent{}, false)
			tasks := planNewProjectServiceTasks(servicePlan, test.components)
			if len(tasks.Pre) != 1 || tasks.Pre[0].Name != "Run Docker Compose" {
				t.Fatalf("pre tasks = %#v, want one Compose startup", tasks.Pre)
			}
			if len(tasks.Down) != 1 || tasks.Down[0].Name != "Docker Compose Down" {
				t.Fatalf("down tasks = %#v, want one Compose teardown", tasks.Down)
			}
		})
	}
}

// TestNewProjectDatabaseLifecycleOmitsReadinessHooks keeps framework preparation out of project-owned hooks.
func TestNewProjectDatabaseLifecycleOmitsReadinessHooks(t *testing.T) {
	for _, components := range []project.Components{{Docker: true, DatabaseMySQL: true}, {Docker: true, DatabasePostgres: true}} {
		_, servicePlan := resolveNewProjectServiceTestPlans(t, components, project.LocalServiceIntent{}, false)
		tasks := planNewProjectServiceTasks(servicePlan, components)
		if len(tasks.Pre) != 1 || tasks.Pre[0].Name != "Run Docker Compose" {
			t.Fatalf("pre tasks = %#v, want Compose only", tasks.Pre)
		}
	}
}

// TestPlanNewProjectServiceTasksSkipsExternalDatabaseWait verifies a remote SQL service never receives a local Compose readiness command.
func TestPlanNewProjectServiceTasksSkipsExternalDatabaseWait(t *testing.T) {
	components := project.Components{DatabaseMySQL: true}
	_, servicePlan := resolveNewProjectServiceTestPlans(t, components, project.LocalServiceIntent{}, false)

	tasks := planNewProjectServiceTasks(servicePlan, components)
	if len(tasks.Pre) != 0 || len(tasks.Down) != 0 {
		t.Fatalf("service tasks = %#v, want no local lifecycle", tasks)
	}
}

// resolveNewProjectServiceTestPlans creates validated concrete plans so task tests exercise only lifecycle policy.
func resolveNewProjectServiceTestPlans(t *testing.T, components project.Components, intent project.LocalServiceIntent, redisActive bool) (project.ResourcePlan, project.ServicePlan) {
	t.Helper()
	resourcePlan := defaultResourcePlanForTest(t, components)
	if redisActive {
		resourcePlan = redisResourcePlanForTest(t, components)
	}
	servicePlan, err := project.ResolveServicePlan(resourcePlan, components, intent)
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}
	return resourcePlan, servicePlan
}
