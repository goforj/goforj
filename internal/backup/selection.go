package backup

import (
	"context"
	"fmt"
	"strings"
)

// CreateOptions controls explicit resource selection without changing legacy create defaults.
type CreateOptions struct {
	Resources   []string
	CopyObjects bool
	MaxBytes    int64
}

// CreateSelected creates one framework backup set for an exact resource selection.
func (s *Service) CreateSelected(ctx context.Context, root string, options CreateOptions) (CreatedBackup, error) {
	if len(options.Resources) == 0 {
		return CreatedBackup{}, fmt.Errorf("at least one backup resource is required")
	}
	if options.MaxBytes < 0 {
		return CreatedBackup{}, fmt.Errorf("backup size limit cannot be negative")
	}
	seen := map[string]bool{}
	for _, id := range options.Resources {
		if !strings.HasPrefix(id, "db.") && !strings.HasPrefix(id, "storage.") {
			return CreatedBackup{}, fmt.Errorf("backup resource must use db. or storage. prefix")
		}
		if seen[id] {
			return CreatedBackup{}, fmt.Errorf("duplicate backup resource %s", id)
		}
		seen[id] = true
	}
	return s.create(ctx, root, "", &options)
}

// selectBackupPlan validates all requested resources before any capture starts.
func selectBackupPlan(plan Plan, options *CreateOptions) (Plan, error) {
	selected := Plan{}
	found := map[string]bool{}
	for _, id := range options.Resources {
		for _, resource := range plan.Resources {
			if id == "db."+resource.Connection.Name {
				selected.Resources = append(selected.Resources, resource)
				found[id] = true
			}
		}
		for _, resource := range plan.Storage {
			if id == "storage."+resource.Name {
				if resource.Status != "backupable" {
					return Plan{}, fmt.Errorf("selected storage %s is not backupable", id)
				}
				selected.Storage = append(selected.Storage, resource)
				found[id] = true
			}
		}
		if !found[id] {
			return Plan{}, fmt.Errorf("selected backup resource %s was not found", id)
		}
	}
	return selected, nil
}
