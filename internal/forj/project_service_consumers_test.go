package forj

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestEffectiveResourceConsumersDiscoverArbitraryNamedRedis verifies owner-authored named scopes join the local default endpoint.
func TestEffectiveResourceConsumersDiscoverArbitraryNamedRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Events: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_REPORTS_DRIVER=redis\nCACHE_REPORTS_ADDR=redis:6379\nREDIS_HOST=redis\nREDIS_PORT=6379\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, nil)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal), consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	requirement, exists := resolved.Requirement(project.ServiceRedis)
	if !exists || requirement.State != project.ServiceStateActiveLocal {
		t.Fatalf("Redis requirement = %#v exists=%t, want active local", requirement, exists)
	}
	if !reflect.DeepEqual(requirement.ActiveConsumers, []string{"cache:reports"}) {
		t.Fatalf("Redis consumers = %#v, want named reports cache", requirement.ActiveConsumers)
	}
}

// TestEffectiveResourceConsumersSeparateExternalRedisEndpoints verifies root resource overrides do not collapse by driver name.
func TestEffectiveResourceConsumersSeparateExternalRedisEndpoints(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Events: true}
	plan := redisResourcePlanForTest(t, components)
	source := []byte("CACHE_DRIVER=redis\nCACHE_ADDR=cache.redis.example:6379\nEVENTS_DRIVER=redis\nEVENTS_ADDR=events.redis.example:6379\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, nil)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal), consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	external := resolved.RequirementsInState(project.ServiceStateExternalRequired)
	if len(external) != 2 {
		t.Fatalf("external requirements = %#v, want two Redis endpoints", external)
	}
	if external[0].EndpointAffinity == external[1].EndpointAffinity {
		t.Fatalf("external endpoints share affinity: %#v", external)
	}
	consumerSets := map[string]bool{}
	for _, requirement := range external {
		if len(requirement.ActiveConsumers) == 1 {
			consumerSets[requirement.ActiveConsumers[0]] = true
		}
	}
	if !consumerSets["cache"] || !consumerSets["events"] {
		t.Fatalf("external consumers = %#v, want cache and events separated", external)
	}
}

// TestEffectiveResourceConsumersApplyNamedAppOverrides verifies runtime App overlay conventions participate in project-level discovery.
func TestEffectiveResourceConsumersApplyNamedAppOverrides(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("BILLING_CACHE_DRIVER=redis\nBILLING_CACHE_ADDR=billing.redis.example:6379\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, []string{"billing"})
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal), consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	external := resolved.RequirementsInState(project.ServiceStateExternalRequired)
	if len(external) != 1 || !reflect.DeepEqual(external[0].ActiveConsumers, []string{"billing:cache"}) {
		t.Fatalf("named App external requirement = %#v", external)
	}
}

// TestEffectiveResourceConsumersRespectConfiguredAppComponents keeps stale overlays from inventing App capabilities.
func TestEffectiveResourceConsumersRespectConfiguredAppComponents(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Docker: true, DatabaseMySQL: true}},
		Apps: map[string]project.AppConfig{
			"billing": {Components: project.Components{CLI: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan := defaultResourcePlanForTest(t, projectComponents)
	database, _ := plan.Selection(project.ResourceDatabase)
	database.Supported = append(database.Supported, "postgres")
	plan, err := plan.WithSelection(project.ResourceDatabase, database).Normalized(projectComponents)
	if err != nil {
		t.Fatalf("normalize resource plan: %v", err)
	}
	source := []byte("DB_DRIVER=mysql\nDB_HOST=mysql\nDB_PORT=3306\nBILLING_DB_DRIVER=postgres\nBILLING_DB_HOST=postgres.billing.example\n")

	consumers, err := effectiveResourceConsumersFromProjectConfig(source, plan, projectComponents, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	for _, consumer := range consumers {
		if consumer.Consumer == "billing:database" || strings.HasPrefix(consumer.Consumer, "billing:database:") {
			t.Fatalf("database-disabled billing App gained a consumer from stale environment: %#v", consumers)
		}
	}
}

// TestEffectiveResourceConsumersIgnoreStaleDisabledEvents verifies root and App overlays cannot invent Events participation.
func TestEffectiveResourceConsumersIgnoreStaleDisabledEvents(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Docker: true}},
		Apps: map[string]project.AppConfig{
			"billing": {Components: project.Components{CLI: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan := defaultResourcePlanForTest(t, projectComponents)
	source := []byte("EVENTS_DRIVER=redis\nEVENTS_ADDR=events.example:6379\nBILLING_EVENTS_DRIVER=redis\nBILLING_EVENTS_ADDR=billing-events.example:6379\n")

	consumers, err := effectiveResourceConsumersFromProjectConfig(source, plan, projectComponents, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	for _, consumer := range consumers {
		if consumer.Resource == project.ResourceEvents {
			t.Fatalf("stale owner env resurrected Events consumer: %#v", consumers)
		}
	}
}

// TestEffectiveResourceConsumersIgnoreStaleDisabledStorage verifies root and App overlays cannot invent Storage participation.
func TestEffectiveResourceConsumersIgnoreStaleDisabledStorage(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Docker: true}},
		Apps: map[string]project.AppConfig{
			"billing": {Components: project.Components{CLI: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan := defaultResourcePlanForTest(t, projectComponents)
	source := []byte("STORAGE_DRIVER=redis\nSTORAGE_ADDR=storage.example:6379\nBILLING_STORAGE_DRIVER=redis\nBILLING_STORAGE_ADDR=billing-storage.example:6379\n")

	consumers, err := effectiveResourceConsumersFromProjectConfig(source, plan, projectComponents, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	for _, consumer := range consumers {
		if consumer.Resource == project.ResourceStorage {
			t.Fatalf("stale owner env resurrected Storage consumer: %#v", consumers)
		}
	}
}

// TestEffectiveResourceConsumersUseNamedAppStorageEnvelope verifies App-local Storage participates in project service discovery.
func TestEffectiveResourceConsumersUseNamedAppStorageEnvelope(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
		Apps: map[string]project.AppConfig{
			"files": {Components: project.Components{CLI: true, Storage: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan := defaultResourcePlanForTest(t, projectComponents)
	storage, ok := plan.Selection(project.ResourceStorage)
	if !ok {
		t.Fatal("project Storage envelope did not create a Storage selection")
	}
	storage.Supported = []string{"local", "redis", "s3"}
	var err error
	plan, err = plan.WithSelection(project.ResourceStorage, storage).Normalized(projectComponents)
	if err != nil {
		t.Fatalf("normalize Storage resource plan: %v", err)
	}
	source := []byte("STORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local,redis,s3\nFILES_STORAGE_DRIVER=redis\nFILES_STORAGE_ADDR=files.redis.example:6379\nAPI_STORAGE_DRIVER=s3\nAPI_STORAGE_BUCKET=stale-api\n")

	consumers, err := effectiveResourceConsumersFromProjectConfig(source, plan, projectComponents, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	drivers := map[string]string{}
	for _, consumer := range consumers {
		if consumer.Resource != project.ResourceStorage {
			continue
		}
		drivers[consumer.Consumer] = consumer.Driver
		if !strings.HasPrefix(consumer.Consumer, "files:storage") {
			t.Fatalf("Storage-disabled default App gained a consumer: %#v", consumers)
		}
	}
	if drivers["files:storage"] != "redis" {
		t.Fatalf("named Storage App consumers = %#v, want files root using Redis", drivers)
	}
	if config.Render.Components.Storage {
		t.Fatal("consumer discovery widened the default App Storage selection")
	}
}

// TestEffectiveResourceConsumersUseEachConfiguredAppDatabase verifies disjoint App database participation shares one build contract.
func TestEffectiveResourceConsumersUseEachConfiguredAppDatabase(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Docker: true, DatabaseMySQL: true}},
		Apps: map[string]project.AppConfig{
			"reporting": {Components: project.Components{CLI: true, DatabasePostgres: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan := defaultResourcePlanForTest(t, projectComponents)
	plan, err := withProjectDatabaseCapabilities(plan, config.Render.Components, projectComponents)
	if err != nil {
		t.Fatalf("build project database plan: %v", err)
	}
	source := []byte("DB_DRIVER=mysql\nDB_HOST=mysql\nDB_PORT=3306\nREPORTING_DB_DRIVER=postgres\nREPORTING_DB_HOST=postgres.reporting.example\n")

	consumers, err := effectiveResourceConsumersFromProjectConfig(source, plan, projectComponents, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	drivers := map[string]string{}
	for _, consumer := range consumers {
		drivers[consumer.Consumer] = consumer.Driver
	}
	if drivers["database"] != "mysql" || drivers["reporting:database"] != "postgres" {
		t.Fatalf("App database consumers = %#v, want mysql root and postgres reporting", drivers)
	}
}

// TestEffectiveResourceConsumersKeepImplicitAppDatabaseStableAcrossSiblings verifies discovery uses the same App projection as rendering.
func TestEffectiveResourceConsumersKeepImplicitAppDatabaseStableAcrossSiblings(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Docker: true}},
		Apps: map[string]project.AppConfig{
			"accounts":  {Components: project.Components{CLI: true, Auth: true}},
			"reporting": {Components: project.Components{CLI: true, DatabasePostgres: true}},
		},
	}
	projectComponents := project.ProjectComponents(config)
	plan := defaultResourcePlanForTest(t, projectComponents)
	plan, err := withProjectDatabaseCapabilities(plan, config.Render.Components, projectComponents)
	if err != nil {
		t.Fatalf("build project database plan: %v", err)
	}
	source := []byte("ACCOUNTS_DB_DRIVER=mysql\nACCOUNTS_DB_HOST=mysql.accounts.example\nREPORTING_DB_DRIVER=postgres\nREPORTING_DB_HOST=postgres.reporting.example\n")

	consumers, err := effectiveResourceConsumersFromProjectConfig(source, plan, projectComponents, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	drivers := map[string]string{}
	for _, consumer := range consumers {
		drivers[consumer.Consumer] = consumer.Driver
	}
	if drivers["accounts:database"] != "mysql" || drivers["reporting:database"] != "postgres" {
		t.Fatalf("App database consumers = %#v, want stable mysql accounts and postgres reporting", drivers)
	}
	if _, exists := drivers["database"]; exists {
		t.Fatalf("database-disabled default App gained a consumer: %#v", drivers)
	}
}

// TestEffectiveResourceConsumersUseRuntimeDefaultsForBlankAppRootDrivers keeps App overlays aligned with generated manager fallbacks.
func TestEffectiveResourceConsumersUseRuntimeDefaultsForBlankAppRootDrivers(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Docker: true, Jobs: true}
	plan := redisResourcePlanForTest(t, components)
	database, _ := plan.Selection(project.ResourceDatabase)
	database.Supported = append(database.Supported, "sqlite")
	plan, err := plan.WithSelection(project.ResourceDatabase, database).Normalized(components)
	if err != nil {
		t.Fatalf("normalize resource plan: %v", err)
	}

	source := []byte("DB_DRIVER=mysql\nQUEUE_DRIVER=redis\nBILLING_DB_DRIVER=\nBILLING_QUEUE_DRIVER=\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, []string{"billing"})
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}

	drivers := map[string]string{}
	for _, consumer := range consumers {
		drivers[consumer.Consumer] = consumer.Driver
	}
	if got := drivers["billing:database"]; got != "sqlite" {
		t.Fatalf("blank App database driver = %q, want runtime SQLite fallback", got)
	}
	if got := drivers["billing:queue"]; got != "workerpool" {
		t.Fatalf("blank App queue driver = %q, want runtime workerpool fallback", got)
	}
}

// TestEffectiveResourceConsumersCanonicalizeDatabaseAliases keeps root, named, and App overlays compatible with direct generation.
func TestEffectiveResourceConsumersCanonicalizeDatabaseAliases(t *testing.T) {
	components := project.Components{DatabasePostgres: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("DB_DRIVER=postgresql\nDB_REPORTS_DRIVER=sqlite3\nBILLING_DB_DRIVER=mariadb\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, []string{"billing"})
	if err != nil {
		t.Fatalf("discover aliased database consumers: %v", err)
	}
	drivers := map[string]string{}
	for _, consumer := range consumers {
		drivers[consumer.Consumer] = consumer.Driver
	}
	for consumer, want := range map[string]string{
		"database":                 "postgres",
		"database:reports":         "sqlite",
		"billing:database":         "mysql",
		"billing:database:reports": "sqlite",
	} {
		if got := drivers[consumer]; got != want {
			t.Fatalf("%s driver = %q, want %q; consumers=%#v", consumer, got, want, consumers)
		}
	}
}

// TestEffectiveResourceConsumersResolveDotenvReferences keeps endpoint planning on the generator's full-file parsing contract.
func TestEffectiveResourceConsumersResolveDotenvReferences(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := redisResourcePlanForTest(t, components)
	source := []byte("CACHE_BACKEND=redis\nCACHE_DRIVER=${CACHE_BACKEND}\nCACHE_ADDR=cache.redis.example:6379\n")

	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, nil)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	var cache project.EffectiveResourceConsumer
	for _, consumer := range consumers {
		if consumer.Consumer == "cache" {
			cache = consumer
			break
		}
	}
	if cache.Driver != "redis" || cache.EndpointAffinity == "" || cache.LocalService {
		t.Fatalf("interpolated cache consumer = %#v, want external Redis", cache)
	}
}

// TestResourceAppPrefixesRequireTopologyEvidence prevents unrelated feature flags from inventing named Apps.
func TestResourceAppPrefixesRequireTopologyEvidence(t *testing.T) {
	prefixes := resourceAppPrefixes(map[string]string{
		"METRICS_CACHE_ENABLED":    "true",
		"BILLING_MAIL_SMTP_HOST":   "smtp.billing.example",
		"REPORTING_CACHE_ADDR":     "reporting.redis.example:6379",
		"WAREHOUSE_DB_HOST":        "warehouse.db.example",
		"OBSERVABILITY_DB_ENABLED": "true",
	}, nil)
	want := []resourceAppPrefix{
		{name: "billing", prefix: "BILLING"},
		{name: "reporting", prefix: "REPORTING"},
		{name: "warehouse", prefix: "WAREHOUSE"},
	}
	if !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("resource App prefixes = %#v, want %#v", prefixes, want)
	}
}

// TestEffectiveResourceConsumersKeepResourceFirstNamesOutOfAppInference protects names containing another resource marker.
func TestEffectiveResourceConsumersKeepResourceFirstNamesOutOfAppInference(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_REPORTING_DB_DRIVER=redis\nCACHE_REPORTING_DB_ADDR=redis:6379\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, nil)
	if err != nil {
		t.Fatalf("discover resource-first named consumer: %v", err)
	}
	found := false
	for _, consumer := range consumers {
		if consumer.Consumer == "cache:reporting_db" && consumer.Driver == "redis" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("consumers = %#v, want cache:reporting_db Redis consumer", consumers)
	}
}

// TestResourceAppPrefixesUseTheFirstResourceMarker prevents one App-named resource key from inventing a second nested App.
func TestResourceAppPrefixesUseTheFirstResourceMarker(t *testing.T) {
	prefixes := resourceAppPrefixes(map[string]string{
		"FOO_CACHE_DB_DRIVER": "redis",
	}, nil)
	want := []resourceAppPrefix{{name: "foo", prefix: "FOO"}}
	if !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("resource App prefixes = %#v, want %#v", prefixes, want)
	}
}

// TestEffectiveResourceConsumersRejectAlternateDatabaseWithoutEndpoint prevents plans for Compose services that are not rendered.
func TestEffectiveResourceConsumersRejectAlternateDatabaseWithoutEndpoint(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	database, _ := plan.Selection(project.ResourceDatabase)
	database.Supported = append(database.Supported, "mysql")
	plan = plan.WithSelection(project.ResourceDatabase, database)

	_, err := effectiveResourceConsumersFromEnvironment([]byte("DB_DRIVER=sqlite\nBILLING_DB_DRIVER=mysql\n"), plan, components, []string{"billing"})
	if err == nil || !strings.Contains(err.Error(), "BILLING_DB_HOST") {
		t.Fatalf("alternate database error = %v, want explicit billing endpoint validation", err)
	}
}

// TestEffectiveResourceConsumersSeparateAlternateAppDatabase keeps an explicitly external engine out of the local Compose slice.
func TestEffectiveResourceConsumersSeparateAlternateAppDatabase(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	database, _ := plan.Selection(project.ResourceDatabase)
	database.Supported = append(database.Supported, "mysql")
	plan = plan.WithSelection(project.ResourceDatabase, database)
	source := []byte("DB_DRIVER=sqlite\nBILLING_DB_DRIVER=mysql\nBILLING_DB_HOST=mysql.billing.example\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, []string{"billing"})
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	requirements := resolved.RequirementsFor(project.ServiceMySQL)
	if len(requirements) != 1 || requirements[0].State != project.ServiceStateExternalRequired || !reflect.DeepEqual(requirements[0].ActiveConsumers, []string{"billing:database"}) {
		t.Fatalf("MySQL requirements = %#v, want billing external only", requirements)
	}
	_, compose := renderResourceTemplatesWithConsumers(t, components, plan, project.LocalServiceIntent{}, consumers)
	if strings.Contains(compose, "\n  mysql:\n") {
		t.Fatalf("SQLite root emitted an unowned MySQL Compose service:\n%s", compose)
	}
}

// TestEffectiveResourceConsumersDeduplicateInheritedRootDatabase keeps same-engine App scopes on the root Compose service.
func TestEffectiveResourceConsumersDeduplicateInheritedRootDatabase(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("DB_DRIVER=mysql\nDB_HOST=mysql\nDB_PORT=3306\nBILLING_DB_DRIVER=mysql\nBILLING_DB_DATABASE=billing\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, []string{"billing"})
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	requirements := resolved.RequirementsFor(project.ServiceMySQL)
	if len(requirements) != 1 || requirements[0].State != project.ServiceStateActiveLocal {
		t.Fatalf("MySQL requirements = %#v, want one active local service", requirements)
	}
	wantConsumers := []string{"database", "billing:database"}
	if !reflect.DeepEqual(requirements[0].ActiveConsumers, wantConsumers) {
		t.Fatalf("MySQL consumers = %#v, want %#v", requirements[0].ActiveConsumers, wantConsumers)
	}
}

// TestEffectiveResourceConsumersDiscoverEndpointOnlyNamedDatabase preserves named scopes that inherit the root driver.
func TestEffectiveResourceConsumersDiscoverEndpointOnlyNamedDatabase(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("DB_DRIVER=mysql\nDB_HOST=mysql\nDB_PORT=3306\nDB_REPORTING_HOST=reporting.mysql.example\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, nil)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	requirements := resolved.RequirementsFor(project.ServiceMySQL)
	if len(requirements) != 2 {
		t.Fatalf("MySQL requirements = %#v, want local root and external reporting endpoints", requirements)
	}
	if requirements[0].State != project.ServiceStateActiveLocal || !reflect.DeepEqual(requirements[0].ActiveConsumers, []string{"database"}) {
		t.Fatalf("root MySQL requirement = %#v, want active local database", requirements[0])
	}
	if requirements[1].State != project.ServiceStateExternalRequired || !reflect.DeepEqual(requirements[1].ActiveConsumers, []string{"database:reporting"}) {
		t.Fatalf("reporting MySQL requirement = %#v, want endpoint-only named external database", requirements[1])
	}
}

// TestEffectiveResourceConsumersSeparateExternalAppSMTP keeps Mailpit local while reporting an App-owned SMTP endpoint.
func TestEffectiveResourceConsumersSeparateExternalAppSMTP(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Mail: true}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("MAIL_DRIVER=smtp\nMAIL_SMTP_HOST=mailpit\nMAIL_SMTP_PORT=1025\nBILLING_MAIL_SMTP_HOST=smtp.billing.example\nBILLING_MAIL_SMTP_PORT=2525\n")
	consumers, err := effectiveResourceConsumersFromEnvironment(source, plan, components, []string{"billing"})
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
	if err != nil {
		t.Fatalf("resolve service plan: %v", err)
	}
	requirements := resolved.RequirementsFor(project.ServiceMailSMTP)
	if len(requirements) != 1 || requirements[0].State != project.ServiceStateExternalRequired || requirements[0].EndpointAffinity == "" {
		t.Fatalf("SMTP requirements = %#v, want one external endpoint", requirements)
	}
	if !reflect.DeepEqual(requirements[0].ActiveConsumers, []string{"billing:mail"}) {
		t.Fatalf("SMTP consumers = %#v, want billing mail only", requirements[0].ActiveConsumers)
	}
}
