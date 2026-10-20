package forj

import "github.com/goforj/goforj/project"

// newProjectServiceTaskPlan groups generated development lifecycle tasks without persisting transient resource topology.
type newProjectServiceTaskPlan struct {
	Pre  []project.DevTask
	Down []project.DevTask
}

// planNewProjectServiceTasks derives Compose lifecycle work from active local services and generated development tools.
func planNewProjectServiceTasks(servicePlan project.ServicePlan, components project.Components) newProjectServiceTaskPlan {
	if !newProjectNeedsDockerCompose(servicePlan, components) {
		return newProjectServiceTaskPlan{}
	}

	tasks := newProjectServiceTaskPlan{
		Pre: []project.DevTask{{
			Name: "Run Docker Compose",
			Cmd:  dockerComposeUpDevCommand(components),
		}},
		Down: []project.DevTask{{
			Name: "Docker Compose Down",
			Cmd:  dockerComposeDownDevCommand(),
		}},
	}
	return tasks
}

// newProjectNeedsDockerCompose keeps inactive profiled services from creating an empty startup while retaining selected development tools.
func newProjectNeedsDockerCompose(servicePlan project.ServicePlan, components project.Components) bool {
	if !components.Docker {
		return false
	}
	return servicePlan.HasActiveLocal() || newProjectHasDockerComposeTools(components)
}

// newProjectHasDockerComposeTools mirrors the Mailpit, VictoriaMetrics, and Grafana service gates in the generated Compose template.
func newProjectHasDockerComposeTools(components project.Components) bool {
	return components.Mail || components.Observability || components.Grafana
}
