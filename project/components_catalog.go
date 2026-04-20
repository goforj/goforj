package project

// ComponentKey identifies a renderable project component.
type ComponentKey string

const (
	ComponentCLI              ComponentKey = "cli"
	ComponentDemoApp          ComponentKey = "demo_app"
	ComponentMail             ComponentKey = "mail"
	ComponentAuth             ComponentKey = "auth"
	ComponentOAuth            ComponentKey = "oauth"
	ComponentWebAPI           ComponentKey = "web_api"
	ComponentWebUI            ComponentKey = "web_ui"
	ComponentDocker           ComponentKey = "docker"
	ComponentDatabaseMySQL    ComponentKey = "database_mysql"
	ComponentDatabasePostgres ComponentKey = "database_postgres"
	ComponentDatabaseSQLite   ComponentKey = "database_sqlite"
	ComponentScheduler        ComponentKey = "scheduler"
	ComponentJobs             ComponentKey = "jobs"
	ComponentStressTest       ComponentKey = "stress_test"
)

// ComponentDefinition describes a project component, its wizard display, and its dependency contract.
type ComponentDefinition struct {
	Key             ComponentKey
	Label           string
	Description     string
	DefaultSelected bool
	Requires        []ComponentKey
	Parent          ComponentKey
	ExclusiveGroup  string
}

var componentCatalog = []ComponentDefinition{
	{Key: ComponentCLI, Label: "CLI", DefaultSelected: true},
	{Key: ComponentDocker, Label: "Docker", Description: "Builds docker-compose.yml dependencies for your app", DefaultSelected: true},
	{Key: ComponentMail, Label: "Mail", Description: "Outbound email delivery and generated mail wiring", DefaultSelected: true},
	{Key: ComponentAuth, Label: "Auth", Description: "Users, sessions, and generated authentication scaffolding", DefaultSelected: true, Requires: []ComponentKey{ComponentMail}},
	{Key: ComponentOAuth, Label: "OAuth", Description: "Optional OAuth providers layered on top of auth", DefaultSelected: true, Requires: []ComponentKey{ComponentAuth}, Parent: ComponentAuth},
	{Key: ComponentWebAPI, Label: "Web API", DefaultSelected: true},
	{Key: ComponentWebUI, Label: "Web UI", DefaultSelected: true},
	{Key: ComponentDatabaseMySQL, Label: "Database (MySQL)", DefaultSelected: true, ExclusiveGroup: "database"},
	{Key: ComponentDatabasePostgres, Label: "Database (Postgres)", ExclusiveGroup: "database"},
	{Key: ComponentDatabaseSQLite, Label: "Database (SQLite)", ExclusiveGroup: "database"},
	{Key: ComponentScheduler, Label: "Scheduler", Description: "Cron jobs and scheduled tasks. go-cron with fluent support", DefaultSelected: true},
	{Key: ComponentJobs, Label: "Jobs", Description: "Asynq", DefaultSelected: true},
	{Key: ComponentStressTest, Label: "Stress Test", Description: "Synthetic queue stress jobs and scheduler tick command", DefaultSelected: true, Requires: []ComponentKey{ComponentJobs}, Parent: ComponentJobs},
}

// ComponentCatalog returns the canonical component definitions.
func ComponentCatalog() []ComponentDefinition {
	out := make([]ComponentDefinition, len(componentCatalog))
	copy(out, componentCatalog)
	return out
}

// ComponentDefinitionByKey returns the definition for a component key.
func ComponentDefinitionByKey(key ComponentKey) (ComponentDefinition, bool) {
	for _, definition := range componentCatalog {
		if definition.Key == key {
			return definition, true
		}
	}
	return ComponentDefinition{}, false
}

// ComponentDefinitionByLabel returns the definition for a wizard label.
func ComponentDefinitionByLabel(label string) (ComponentDefinition, bool) {
	for _, definition := range componentCatalog {
		if definition.Label == label {
			return definition, true
		}
	}
	return ComponentDefinition{}, false
}

// DefaultSelectedComponents returns the default component selection derived from the catalog.
func DefaultSelectedComponents() Components {
	var components Components
	for _, definition := range componentCatalog {
		if !definition.DefaultSelected {
			continue
		}
		components.SetEnabled(definition.Key, true)
	}
	return components.WithResolvedDependencies()
}
