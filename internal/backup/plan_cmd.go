package backup

import (
	"encoding/json"
	"fmt"
)

// PlanCmd prints the native backup plan for configured database connections.
type PlanCmd struct {
	Resource string `help:"Optional database connection name"`
	JSON     bool   `help:"Print machine-readable JSON"`
}

// NewPlanCmd creates a backup plan command.
func NewPlanCmd() *PlanCmd { return &PlanCmd{} }

// Signature defines CLI metadata for the backup plan command.
func (*PlanCmd) Signature() string { return `name:"backup:plan" help:"Show the database backup plan"` }

// Run prints the configured native backup strategies.
func (c *PlanCmd) Run() error {
	plan, err := BuildPlan()
	if err != nil {
		return err
	}
	resourceName := normalizeResourceName(c.Resource)
	rows := []map[string]string{}
	for _, resource := range plan.Resources {
		if resourceName != "" && resourceName != resource.Connection.Name {
			continue
		}
		rows = append(rows, map[string]string{
			"id": "db." + resource.Connection.Name, "driver": resource.Connection.Driver,
			"strategy": resource.Strategy, "status": resource.Status,
		})
	}
	for _, resource := range plan.Storage {
		id := "storage." + resource.Name
		if c.Resource != "" && c.Resource != id && c.Resource != resource.Name {
			continue
		}
		strategy := "local-archive"
		if resource.Driver == "s3" {
			strategy = "object-manifest"
		}
		rows = append(rows, map[string]string{
			"id": id, "driver": resource.Driver, "strategy": strategy, "status": resource.Status,
		})
	}
	if len(rows) == 0 {
		return fmt.Errorf("backup resource %q was not found", c.Resource)
	}
	if c.JSON {
		data, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}
	for _, row := range rows {
		fmt.Printf("%s\t%s\t%s\n", row["id"], row["driver"], row["strategy"])
	}
	return nil
}
