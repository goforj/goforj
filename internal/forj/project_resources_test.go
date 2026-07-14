package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestReconcileResourceEnvironmentPreservesOwners verifies full resource ownership and portable initialization.
func TestReconcileResourceEnvironmentPreservesOwners(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Mail: true, Docker: true, Jobs: true, Auth: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\nCOMPOSE_PROFILES=metrics\n")
	updated, effective, changed, err := reconcileResourceEnvironment(source, seed, components, true)
	if err != nil {
		t.Fatalf("reconcile resource environment: %v", err)
	}
	if !changed {
		t.Fatal("expected missing resource keys to be initialized")
	}
	cache, _ := effective.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("owner cache driver = %q, want redis", cache.Active)
	}
	text := string(updated)
	for _, want := range []string{
		"CACHE_DRIVER=redis\n",
		"CACHE_SUPPORTED_DRIVERS=memory,redis\n",
		"QUEUE_DRIVER=workerpool\n",
		"QUEUE_SUPPORTED_DRIVERS=workerpool,redis\n",
		"EVENTS_SUPPORTED_DRIVERS=inproc,redis\n",
		"MAIL_SUPPORTED_DRIVERS=log,smtp\n",
		"CACHE_SESSIONS_DRIVER=memory\n",
		"COMPOSE_PROFILES=metrics\n",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("reconciled environment omitted %q:\n%s", want, text)
		}
	}
}

// TestReconcileResourceEnvironmentAcceptsDotenvSyntax keeps preview, rerender, and generation on one parsing contract.
func TestReconcileResourceEnvironmentAcceptsDotenvSyntax(t *testing.T) {
	components := project.Components{DatabaseSQLite: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("export CACHE_DRIVER=\"redis\"\nCACHE_SUPPORTED_DRIVERS='memory,redis' # portable pair\nEVENTS_DRIVER=\"inproc\"\nEVENTS_SUPPORTED_DRIVERS=\"inproc,redis\"\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\nDB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite\nCACHE_PREFIX=\"owner#prefix\"\n")
	updated, effective, _, err := reconcileResourceEnvironment(source, seed, components, true)
	if err != nil {
		t.Fatalf("reconcile resource environment: %v", err)
	}
	cache, _ := effective.Selection(project.ResourceCache)
	if cache.Active != "redis" || strings.Join(cache.Supported, ",") != "memory,redis" {
		t.Fatalf("dotenv values were not normalized: %#v", cache)
	}
	if !strings.Contains(string(updated), "CACHE_PREFIX=\"owner#prefix\"") {
		t.Fatalf("line-aware reconciliation damaged a quoted hash:\n%s", updated)
	}
}

// TestReconcileResourceEnvironmentResolvesDotenvReferences prevents owner indirection from being mistaken for a missing value.
func TestReconcileResourceEnvironmentResolvesDotenvReferences(t *testing.T) {
	components := project.Components{DatabaseSQLite: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_BACKEND=redis\nCACHE_DRIVER=${CACHE_BACKEND}\nCACHE_SUPPORTED_DRIVERS=memory,redis\nEVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\nDB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite\n")

	updated, effective, _, err := reconcileResourceEnvironment(source, seed, components, true)
	if err != nil {
		t.Fatalf("reconcile resource environment: %v", err)
	}
	cache, _ := effective.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("interpolated cache driver = %q, want redis", cache.Active)
	}
	if !strings.Contains(string(updated), "CACHE_DRIVER=${CACHE_BACKEND}") {
		t.Fatalf("reconciliation replaced owner interpolation:\n%s", updated)
	}
}

// TestReconcileResourceEnvironmentPreservesDatabaseAliases keeps owner syntax while planning with canonical driver identities.
func TestReconcileResourceEnvironmentPreservesDatabaseAliases(t *testing.T) {
	components := project.Components{DatabasePostgres: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("DB_DRIVER=postgresql\nDB_SUPPORTED_DRIVERS=postgres\n")
	updated, effective, _, err := reconcileResourceEnvironment(source, seed, components, true)
	if err != nil {
		t.Fatalf("reconcile database aliases: %v", err)
	}
	database, _ := effective.Selection(project.ResourceDatabase)
	if database.Active != "postgres" || strings.Join(database.Supported, ",") != "postgres" {
		t.Fatalf("canonical database selection = %#v, want Postgres", database)
	}
	if !strings.Contains(string(updated), "DB_DRIVER=postgresql") {
		t.Fatalf("reconciliation replaced owner alias:\n%s", updated)
	}
}

// TestCompatibilityResourcePlanPreservesDemoSQLiteFallback keeps legacy render entry points on the Demo build invariant.
func TestCompatibilityResourcePlanPreservesDemoSQLiteFallback(t *testing.T) {
	components := project.Components{DemoApp: true, DatabaseMySQL: true}
	plan, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	database, _ := plan.Selection(project.ResourceDatabase)
	if database.Active != "mysql" || strings.Join(database.Supported, ",") != "sqlite,mysql" {
		t.Fatalf("Demo compatibility database = %#v, want SQLite fallback and active MySQL", database)
	}
}

// TestPrepareResourceEnvironmentUsesCommittedFallback verifies a clean checkout keeps its portable build contract before .env is recreated.
func TestPrepareResourceEnvironmentUsesCommittedFallback(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	example := []byte(strings.Join([]string{
		"CACHE_DRIVER=memory",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"EVENTS_DRIVER=inproc",
		"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
		"STORAGE_DRIVER=local",
		"STORAGE_SUPPORTED_DRIVERS=local",
		"COMPOSE_PROFILES=",
		"",
	}, "\n"))
	if err := os.WriteFile(".env.example", example, 0o644); err != nil {
		t.Fatalf("write environment example: %v", err)
	}
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	renderer := &ProjectRenderer{config: &project.Config{Render: project.RenderConfig{Components: components}}, resourcePlan: plan}
	if err := renderer.prepareResourceEnvironment(); err != nil {
		t.Fatalf("prepare resource environment: %v", err)
	}

	cache, _ := renderer.resourcePlan.Selection(project.ResourceCache)
	events, _ := renderer.resourcePlan.Selection(project.ResourceEvents)
	if strings.Join(cache.Supported, ",") != "memory,redis" || strings.Join(events.Supported, ",") != "inproc,redis" {
		t.Fatalf("committed fallback was narrowed: cache=%#v events=%#v", cache, events)
	}
	if renderer.pendingEnvironmentWrite || renderer.pendingEnvironment != nil {
		t.Fatal("safe fallback must remain read-only until the owner environment is rendered")
	}
	after, err := os.ReadFile(".env.example")
	if err != nil {
		t.Fatalf("read environment example: %v", err)
	}
	if string(after) != string(example) {
		t.Fatalf("prepare changed environment example\nwant: %q\ngot:  %q", example, after)
	}
}

// TestPrepareResourceEnvironmentKeepsExplicitPlanAboveCommittedFallback enforces the new-project precedence contract.
func TestPrepareResourceEnvironmentKeepsExplicitPlanAboveCommittedFallback(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	if err := os.WriteFile(".env.example", []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis\nEVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\nCOMPOSE_PROFILES=\n"), 0o644); err != nil {
		t.Fatalf("write environment example: %v", err)
	}
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	renderer := &ProjectRenderer{
		config:               &project.Config{Render: project.RenderConfig{Components: components}},
		resourcePlan:         plan,
		localServiceIntent:   intent,
		explicitResourcePlan: true,
	}
	if err := renderer.prepareResourceEnvironment(); err != nil {
		t.Fatalf("prepare resource environment: %v", err)
	}
	cache, _ := renderer.resourcePlan.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("safe fallback replaced explicit cache driver with %q", cache.Active)
	}
	mode, _ := renderer.localServiceIntent.Mode(project.ServiceRedis)
	if mode != project.LocalServiceModeLocal {
		t.Fatalf("safe fallback replaced explicit Redis placement with %q", mode)
	}
}

// TestPrepareResourceEnvironmentCarriesNamedAndAppConsumers verifies rerender service validation sees owner scopes outside the root plan.
func TestPrepareResourceEnvironmentCarriesNamedAndAppConsumers(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	source := []byte("CACHE_SUPPORTED_DRIVERS=memory,redis\nCACHE_REPORTS_DRIVER=redis\nCACHE_REPORTS_ADDR=redis:6379\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\nBILLING_EVENTS_DRIVER=redis\nBILLING_EVENTS_ADDR=events.redis.example:6379\nCOMPOSE_PROFILES=redis\n")
	if err := os.WriteFile(".env", source, 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	renderer := &ProjectRenderer{
		config: &project.Config{
			Render: project.RenderConfig{Components: components},
			Apps:   map[string]project.AppConfig{"billing": {Components: components}},
		},
		resourcePlan:       plan,
		localServiceIntent: project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal),
	}
	if err := renderer.prepareResourceEnvironment(); err != nil {
		t.Fatalf("prepare resource environment: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(renderer.resourcePlan, components, renderer.localServiceIntent, renderer.serviceConsumers)
	if err != nil {
		t.Fatalf("resolve renderer service plan: %v", err)
	}
	if got := len(resolved.RequirementsInState(project.ServiceStateActiveLocal)); got != 1 {
		t.Fatalf("active local requirements = %d, want named local Redis", got)
	}
	external := resolved.RequirementsInState(project.ServiceStateExternalRequired)
	if len(external) != 1 || !strings.Contains(strings.Join(external[0].ActiveConsumers, ","), "billing:events") {
		t.Fatalf("renderer external requirements = %#v", external)
	}
}

// TestComposeRedisServiceWithoutProfile distinguishes legacy local intent from the optional profile bridge.
func TestComposeRedisServiceWithoutProfile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "docker-compose.yml")
	legacy := "volumes:\n  redis:\n    driver: local\nservices:\n  redis:\n    image: redis:7.4\n  api:\n    image: app\n"
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("write legacy Compose file: %v", err)
	}
	if !composeRedisServiceWithoutProfile(path) {
		t.Fatal("legacy unprofiled Redis service was not detected")
	}
	profiled := strings.Replace(legacy, "  redis:\n    image", "  redis:\n    profiles: [redis]\n    image", 1)
	if err := os.WriteFile(path, []byte(profiled), 0o644); err != nil {
		t.Fatalf("write profiled Compose file: %v", err)
	}
	if composeRedisServiceWithoutProfile(path) {
		t.Fatal("profiled Redis service was mistaken for legacy local intent")
	}
}

// TestReconcileResourceEnvironmentRejectsOwnerMismatch verifies validation happens before any file write.
func TestReconcileResourceEnvironmentRejectsOwnerMismatch(t *testing.T) {
	components := project.Components{DatabaseMySQL: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory\n")
	updated, _, changed, err := reconcileResourceEnvironment(source, seed, components, true)
	if err == nil || !strings.Contains(err.Error(), "excludes active") {
		t.Fatalf("reconcile error = %v, want active-subset failure", err)
	}
	if updated != nil || changed {
		t.Fatalf("invalid contract returned a rewrite: changed=%t data=%q", changed, updated)
	}
}

// TestSeedComposeProfilesPreservesTokens verifies local Redis activation edits one exact profile token.
func TestSeedComposeProfilesPreservesTokens(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true}
	plan := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	updated, changed := seedComposeProfiles([]byte("COMPOSE_PROFILES=metrics,redis-debug\n"), plan, components, intent)
	if !changed || string(updated) != "COMPOSE_PROFILES=metrics,redis-debug,redis\n" {
		t.Fatalf("seeded profiles = %q changed=%t", updated, changed)
	}
	second, changed := seedComposeProfiles(updated, plan, components, intent)
	if changed || string(second) != string(updated) {
		t.Fatalf("profile seeding is not idempotent: %q changed=%t", second, changed)
	}
}

// TestSeedComposeProfilesHonorsRetainedUnusedRedis keeps explicit owner lifecycle intent independent of the active driver.
func TestSeedComposeProfilesHonorsRetainedUnusedRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	updated, changed := seedComposeProfiles([]byte("COMPOSE_PROFILES=metrics\n"), plan, components, intent)
	if !changed || string(updated) != "COMPOSE_PROFILES=metrics,redis\n" {
		t.Fatalf("seeded retained profile = %q changed=%t", updated, changed)
	}
}

// TestLocalServiceIntentFromEnvironmentTreatsMissingRedisProfileAsExternal protects explicit placement semantics.
func TestLocalServiceIntentFromEnvironmentTreatsMissingRedisProfileAsExternal(t *testing.T) {
	fallback := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	intent := localServiceIntentFromEnvironment([]byte("COMPOSE_PROFILES=metrics,redis-debug\n"), fallback)
	mode, ok := intent.Mode(project.ServiceRedis)
	if !ok || mode != project.LocalServiceModeExternal {
		t.Fatalf("Redis mode = %q selected=%t, want external", mode, ok)
	}
	intent = localServiceIntentFromEnvironment([]byte("COMPOSE_PROFILES=metrics,redis\n"), fallback)
	mode, ok = intent.Mode(project.ServiceRedis)
	if !ok || mode != project.LocalServiceModeLocal {
		t.Fatalf("Redis mode = %q selected=%t, want local", mode, ok)
	}
	intent = localServiceIntentFromEnvironment([]byte("APP_NAME=Existing\n"), project.LocalServiceIntent{})
	if mode, ok = intent.Mode(project.ServiceRedis); ok {
		t.Fatalf("ordinary environment invented Redis placement %q", mode)
	}
}

// TestResourceRenderValuesIncludeNamedRedis verifies Auth sessions participate in Redis discovery.
func TestResourceRenderValuesIncludeNamedRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Auth: true}
	plan := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	values := resourceRenderValuesForPlan(plan, components, intent)
	if values.CacheSessionsDriver != "redis" || !values.RedisActive || !values.RedisSupported || !values.RedisLocal {
		t.Fatalf("render values = %#v", values)
	}
}

// TestSyncProjectConfigDoesNotWidenDefaultAppForNamedDatabaseSupport verifies shared build support remains derived from App selections.
func TestSyncProjectConfigDoesNotWidenDefaultAppForNamedDatabaseSupport(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	configured := project.Components{CLI: true, DatabaseMySQL: true}
	renderer := &ProjectRenderer{
		config: &project.Config{
			ProjectName:  "Example",
			GoModuleName: "example.com/app",
			Dev:          project.DevConfig{WirePaths: []string{"wire"}},
			Render:       project.RenderConfig{Components: configured},
			Apps: map[string]project.AppConfig{
				"reporting": {Components: project.Components{CLI: true, DatabasePostgres: true}},
			},
		},
	}
	envelope := project.ProjectComponents(renderer.config)
	if !envelope.DatabaseMySQL || !envelope.DatabasePostgres {
		t.Fatalf("project envelope = %#v, want MySQL and Postgres support", envelope)
	}
	if err := renderer.syncProjectConfigForRender(configured); err != nil {
		t.Fatalf("sync project config: %v", err)
	}
	persisted, err := project.LoadProjectConfig()
	if err != nil {
		t.Fatalf("load project config: %v", err)
	}
	if !persisted.Render.Components.DatabaseMySQL || persisted.Render.Components.DatabasePostgres {
		t.Fatalf("persisted default App components widened to shared support: %#v", persisted.Render.Components)
	}
	reporting := persisted.Apps["reporting"].Components
	if !reporting.DatabasePostgres || reporting.DatabaseMySQL {
		t.Fatalf("persisted reporting components = %#v, want Postgres only", reporting)
	}
}

// TestWithProjectDatabaseCapabilitiesKeepsDefaultDriverActive verifies shared driver support cannot replace the root App choice.
func TestWithProjectDatabaseCapabilitiesKeepsDefaultDriverActive(t *testing.T) {
	defaultComponents := project.Components{DatabaseMySQL: true}
	projectComponents := project.Components{DatabaseMySQL: true, DatabasePostgres: true}
	plan, err := compatibilityResourcePlan(projectComponents, "workerpool")
	if err != nil {
		t.Fatalf("compatibilityResourcePlan returned error: %v", err)
	}

	plan, err = withProjectDatabaseCapabilities(plan, defaultComponents, projectComponents)
	if err != nil {
		t.Fatalf("withProjectDatabaseCapabilities returned error: %v", err)
	}
	selection, ok := plan.Selection(project.ResourceDatabase)
	if !ok {
		t.Fatal("project database selection is missing")
	}
	if selection.Active != "mysql" {
		t.Fatalf("active database = %q, want default App mysql", selection.Active)
	}
	wantSupported := []string{"mysql", "postgres"}
	if !reflect.DeepEqual(selection.Supported, wantSupported) {
		t.Fatalf("supported databases = %#v, want %#v", selection.Supported, wantSupported)
	}
}

// TestWithProjectDatabaseCapabilitiesPreservesDemoMySQLDriver verifies preserved alternate database choices cannot override Demo's runtime contract.
func TestWithProjectDatabaseCapabilitiesPreservesDemoMySQLDriver(t *testing.T) {
	components := project.Components{DemoApp: true, DatabasePostgres: true}.WithResolvedDependencies()
	plan, err := compatibilityResourcePlan(components, "workerpool")
	if err != nil {
		t.Fatalf("compatibilityResourcePlan returned error: %v", err)
	}

	plan, err = withProjectDatabaseCapabilities(plan, components, components)
	if err != nil {
		t.Fatalf("withProjectDatabaseCapabilities returned error: %v", err)
	}
	selection, ok := plan.Selection(project.ResourceDatabase)
	if !ok {
		t.Fatal("Demo database selection is missing")
	}
	if selection.Active != "mysql" {
		t.Fatalf("Demo active database = %q, want mysql", selection.Active)
	}
	for _, driver := range []string{"sqlite", "mysql", "postgres"} {
		if !stringSliceContainsFold(selection.Supported, driver) {
			t.Fatalf("Demo supported databases = %#v, want %q included", selection.Supported, driver)
		}
	}
}
