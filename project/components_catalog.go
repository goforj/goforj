package project

// ComponentKey identifies a renderable project component.
type ComponentKey string

const (
	// ComponentCLI enables generated CLI command support.
	ComponentCLI ComponentKey = "cli"
	// ComponentDemoApp enables the generated demo application surface.
	ComponentDemoApp ComponentKey = "demo_app"
	// ComponentMail enables outbound mail wiring.
	ComponentMail ComponentKey = "mail"
	// ComponentAuth enables user, session, and auth route scaffolding.
	ComponentAuth ComponentKey = "auth"
	// ComponentOAuth enables OAuth provider scaffolding layered on auth.
	ComponentOAuth ComponentKey = "oauth"
	// ComponentWebAPI enables HTTP API runtime support.
	ComponentWebAPI ComponentKey = "web_api"
	// ComponentWebUI enables frontend runtime and static asset support.
	ComponentWebUI ComponentKey = "web_ui"
	// ComponentMetrics enables framework metrics.
	ComponentMetrics ComponentKey = "metrics"
	// ComponentObservability enables local metrics collection services.
	ComponentObservability ComponentKey = "observability"
	// ComponentGrafana enables generated dashboards on top of observability.
	ComponentGrafana ComponentKey = "grafana"
	// ComponentDocker enables generated docker-compose dependencies.
	ComponentDocker ComponentKey = "docker"
	// ComponentDatabaseMySQL enables MySQL database support.
	ComponentDatabaseMySQL ComponentKey = "database_mysql"
	// ComponentDatabasePostgres enables Postgres database support.
	ComponentDatabasePostgres ComponentKey = "database_postgres"
	// ComponentDatabaseSQLite enables SQLite database support.
	ComponentDatabaseSQLite ComponentKey = "database_sqlite"
	// ComponentScheduler enables scheduled task runtime support.
	ComponentScheduler ComponentKey = "scheduler"
	// ComponentJobs enables queue worker runtime support.
	ComponentJobs ComponentKey = "jobs"
	// ComponentBackup enables application-owned backup and restore commands.
	ComponentBackup ComponentKey = "backup"
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
	{Key: ComponentCLI, Label: "CLI", Description: "Add commands to run and manage your app", DefaultSelected: true},
	{Key: ComponentDocker, Label: "Docker", Description: "Run databases and tools locally", DefaultSelected: true},
	{Key: ComponentMail, Label: "Mail", Description: "Send email from your app", DefaultSelected: true},
	{Key: ComponentAuth, Label: "Auth", Description: "User sign up, login, and sessions", DefaultSelected: true, Requires: []ComponentKey{ComponentMail}},
	{Key: ComponentOAuth, Label: "OAuth", Description: "Sign in with external providers", DefaultSelected: true, Requires: []ComponentKey{ComponentAuth}, Parent: ComponentAuth},
	{Key: ComponentWebAPI, Label: "Web API", Description: "Build endpoints for clients and frontends", DefaultSelected: true},
	{Key: ComponentWebUI, Label: "Web UI", Description: "Serve a browser frontend", DefaultSelected: true},
	{Key: ComponentMetrics, Label: "Metrics", Description: "Track app health and request stats", DefaultSelected: true, Requires: []ComponentKey{ComponentWebAPI}},
	{Key: ComponentObservability, Label: "Observability", Description: "Collect local app metrics", DefaultSelected: true, Requires: []ComponentKey{ComponentDocker, ComponentMetrics}},
	{Key: ComponentGrafana, Label: "Grafana", Description: "View metrics dashboards", DefaultSelected: true, Requires: []ComponentKey{ComponentObservability}, Parent: ComponentObservability},
	{Key: ComponentDatabaseMySQL, Label: "Database (MySQL)", Description: "Store app data in MySQL", DefaultSelected: true, ExclusiveGroup: "database"},
	{Key: ComponentDatabasePostgres, Label: "Database (Postgres)", Description: "Store app data in Postgres", ExclusiveGroup: "database"},
	{Key: ComponentDatabaseSQLite, Label: "Database (SQLite)", Description: "Store app data in SQLite", ExclusiveGroup: "database"},
	{Key: ComponentScheduler, Label: "Scheduler", Description: "Run tasks on a schedule", DefaultSelected: true},
	{Key: ComponentJobs, Label: "Jobs", Description: "Run background work", DefaultSelected: true},
	{Key: ComponentBackup, Label: "Backup", Description: "Back up and restore application data", DefaultSelected: false},
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
