package scenarios

import "fmt"

// scenarioPlan is the validated five-stage program consumed by full and live-prefix execution.
type scenarioPlan struct {
	spec                  ScenarioSpec
	dependencySteps       []plannedScenarioStep
	preparationSteps      []plannedScenarioStep
	startingChecks        []ScenarioCommand
	targetSteps           []plannedScenarioStep
	finalChecks           []ScenarioCommand
	dependencyScenarioIDs []string
}

// plannedScenarioStep retains the owning scenario for module expansion and actionable diagnostics.
type plannedScenarioStep struct {
	spec ScenarioSpec
	step ScenarioStep
}

// compileScenarioPlans resolves every dependency diamond once before any execution mutates a workspace.
func compileScenarioPlans(specs []ScenarioSpec, byID map[string]ScenarioSpec) (map[string]scenarioPlan, error) {
	plans := make(map[string]scenarioPlan, len(specs))
	for _, spec := range specs {
		plan, err := compileScenarioPlan(spec, byID)
		if err != nil {
			return nil, err
		}
		plans[spec.ID] = plan
	}
	return plans, nil
}

// compileScenarioPlan flattens dependency completion while preserving declaration and topological order.
func compileScenarioPlan(spec ScenarioSpec, byID map[string]ScenarioSpec) (scenarioPlan, error) {
	plan := scenarioPlan{
		spec:           spec,
		startingChecks: cloneScenarioCommands(spec.Prepare.Checks),
		finalChecks:    cloneScenarioCommands(spec.Verify.Commands),
	}
	for _, step := range spec.Prepare.Steps {
		plan.preparationSteps = append(plan.preparationSteps, plannedScenarioStep{spec: spec, step: step})
	}
	for _, step := range spec.Steps {
		plan.targetSteps = append(plan.targetSteps, plannedScenarioStep{spec: spec, step: step})
	}

	applied := map[string]bool{}
	var appendDependency func(string) error
	appendDependency = func(id string) error {
		dependency, ok := byID[id]
		if !ok {
			return fmt.Errorf("%s depends on unknown scenario %q", spec.ID, id)
		}
		for _, ancestorID := range dependency.DependsOn {
			if err := appendDependency(ancestorID); err != nil {
				return err
			}
		}
		if applied[id] {
			return nil
		}
		for _, step := range dependency.Prepare.Steps {
			plan.dependencySteps = append(plan.dependencySteps, plannedScenarioStep{spec: dependency, step: step})
		}
		for _, step := range dependency.Steps {
			plan.dependencySteps = append(plan.dependencySteps, plannedScenarioStep{spec: dependency, step: step})
		}
		plan.dependencyScenarioIDs = append(plan.dependencyScenarioIDs, dependency.ID)
		applied[id] = true
		return nil
	}
	for _, dependencyID := range spec.DependsOn {
		if err := appendDependency(dependencyID); err != nil {
			return scenarioPlan{}, err
		}
	}
	return plan, nil
}

// cloneScenarioCommands prevents callers from mutating catalog-owned command argument slices through a compiled plan.
func cloneScenarioCommands(commands []ScenarioCommand) []ScenarioCommand {
	cloned := make([]ScenarioCommand, 0, len(commands))
	for _, command := range commands {
		cloned = append(cloned, ScenarioCommand{
			Run:      append([]string(nil), command.Run...),
			Contains: append([]string(nil), command.Contains...),
		})
	}
	return cloned
}
