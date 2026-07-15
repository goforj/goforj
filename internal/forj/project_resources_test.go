package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/resourceenv"
	"github.com/goforj/goforj/project"
)

// TestReconcileResourceEnvironmentSeedsStorageOnFirstEnablement verifies config-only activation initializes the Storage contract.
func TestReconcileResourceEnvironmentSeedsStorageOnFirstEnablement(t *testing.T) {
	components := project.Components{CLI: true, Storage: true}
	seed, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	source := []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")

	reconciled, err := resourceenv.Reconcile(source, seed, components, false)
	if err != nil {
		t.Fatalf("reconcile first Storage enablement: %v", err)
	}
	if !reconciled.Changed {
		t.Fatal("first Storage enablement did not initialize owner environment keys")
	}
	text := string(reconciled.Source)
	for _, want := range []string{"STORAGE_DRIVER=local\n", "STORAGE_SUPPORTED_DRIVERS=local\n", "STORAGE_PUBLIC_DRIVER=local\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("first Storage enablement omitted %q:\n%s", want, text)
		}
	}
	storage, ok := reconciled.EffectivePlan.Selection(project.ResourceStorage)
	if !ok || storage.Active != "local" || strings.Join(storage.Supported, ",") != "local" {
		t.Fatalf("effective first Storage selection = %#v selected=%t", storage, ok)
	}
}

// TestReconcileResourceEnvironmentSeedsPortableEventsOnFirstEnablement verifies config-only activation compiles the common infrastructure transition.
func TestReconcileResourceEnvironmentSeedsPortableEventsOnFirstEnablement(t *testing.T) {
	components := project.Components{CLI: true, Events: true}
	seed, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	source := []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\n")

	reconciled, err := resourceenv.Reconcile(source, seed, components, false)
	if err != nil {
		t.Fatalf("reconcile first Events enablement: %v", err)
	}
	if !reconciled.Changed {
		t.Fatal("first Events enablement did not initialize owner environment keys")
	}
	if !strings.Contains(string(reconciled.Source), "EVENTS_DRIVER=inproc\n") || !strings.Contains(string(reconciled.Source), "EVENTS_SUPPORTED_DRIVERS=inproc,redis\n") {
		t.Fatalf("first Events enablement omitted portable defaults:\n%s", reconciled.Source)
	}
	events, ok := reconciled.EffectivePlan.Selection(project.ResourceEvents)
	if !ok || events.Active != "inproc" || strings.Join(events.Supported, ",") != "inproc,redis" {
		t.Fatalf("effective first Events selection = %#v selected=%t", events, ok)
	}
}

// TestReconcileResourceEnvironmentSeedsPortableQueueOnFirstEnablement verifies every render path starts with the local-to-Redis transition compiled.
func TestReconcileResourceEnvironmentSeedsPortableQueueOnFirstEnablement(t *testing.T) {
	components := project.Components{CLI: true, Jobs: true}
	seed, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	source := []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\n")

	reconciled, err := resourceenv.Reconcile(source, seed, components, false)
	if err != nil {
		t.Fatalf("reconcile first Jobs enablement: %v", err)
	}
	if !reconciled.Changed {
		t.Fatal("first Jobs enablement did not initialize owner environment keys")
	}
	if !strings.Contains(string(reconciled.Source), "QUEUE_DRIVER=workerpool\n") || !strings.Contains(string(reconciled.Source), "QUEUE_SUPPORTED_DRIVERS=workerpool,redis\n") {
		t.Fatalf("first Jobs enablement omitted portable Queue defaults:\n%s", reconciled.Source)
	}
	queue, ok := reconciled.EffectivePlan.Selection(project.ResourceQueue)
	if !ok || queue.Active != "workerpool" || strings.Join(queue.Supported, ",") != "workerpool,redis" {
		t.Fatalf("effective first Queue selection = %#v selected=%t", queue, ok)
	}
}

// TestReconcileResourceEnvironmentKeepsExistingEventsCompatibility verifies an owner key retains the legacy active-only build contract.
func TestReconcileResourceEnvironmentKeepsExistingEventsCompatibility(t *testing.T) {
	components := project.Components{CLI: true, Events: true}
	seed, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	source := []byte("CACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\nEVENTS_DRIVER=redis\n")

	reconciled, err := resourceenv.Reconcile(source, seed, components, false)
	if err != nil {
		t.Fatalf("reconcile owner Events selection: %v", err)
	}
	if !strings.Contains(string(reconciled.Source), "EVENTS_DRIVER=redis\n") || !strings.Contains(string(reconciled.Source), "EVENTS_SUPPORTED_DRIVERS=redis\n") {
		t.Fatalf("owner Events selection was widened or replaced:\n%s", reconciled.Source)
	}
	events, ok := reconciled.EffectivePlan.Selection(project.ResourceEvents)
	if !ok || events.Active != "redis" || strings.Join(events.Supported, ",") != "redis" {
		t.Fatalf("effective owner Events selection = %#v selected=%t", events, ok)
	}
}

// TestCompatibilityResourcePlanKeepsPortableEventsDefaults verifies renders without an environment file retain additive Events support.
func TestCompatibilityResourcePlanKeepsPortableEventsDefaults(t *testing.T) {
	plan, err := compatibilityResourcePlan(project.Components{CLI: true, Events: true}, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	events, ok := plan.Selection(project.ResourceEvents)
	if !ok || events.Active != "inproc" || strings.Join(events.Supported, ",") != "inproc,redis" {
		t.Fatalf("Events compatibility defaults = %#v selected=%t", events, ok)
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
	components := project.Components{DatabaseSQLite: true, Docker: true, Events: true, Cache: true}
	plan, err := compatibilityResourcePlan(components, "")
	if err != nil {
		t.Fatalf("resolve compatibility plan: %v", err)
	}
	renderer := &ProjectRenderer{
		workspace: currentProjectRenderWorkspace(t),
		config:    &project.Config{Render: project.RenderConfig{Components: components}},
		resources: resourceRenderState{plan: plan},
	}
	if err := renderer.prepareResourceEnvironment(); err != nil {
		t.Fatalf("prepare resource environment: %v", err)
	}

	cache, _ := renderer.resources.plan.Selection(project.ResourceCache)
	events, _ := renderer.resources.plan.Selection(project.ResourceEvents)
	if strings.Join(cache.Supported, ",") != "memory,redis" || strings.Join(events.Supported, ",") != "inproc,redis" {
		t.Fatalf("committed fallback was narrowed: cache=%#v events=%#v", cache, events)
	}
	if renderer.resources.pendingEnvironmentWrite || renderer.resources.pendingEnvironment != nil {
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
	components := project.Components{DatabaseSQLite: true, Docker: true, Events: true, Cache: true}
	plan := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	renderer := &ProjectRenderer{
		workspace: currentProjectRenderWorkspace(t),
		config:    &project.Config{Render: project.RenderConfig{Components: components}},
		resources: resourceRenderState{
			plan:          plan,
			serviceIntent: intent,
			explicitPlan:  true,
		},
	}
	if err := renderer.prepareResourceEnvironment(); err != nil {
		t.Fatalf("prepare resource environment: %v", err)
	}
	cache, _ := renderer.resources.plan.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("safe fallback replaced explicit cache driver with %q", cache.Active)
	}
	mode, _ := renderer.resources.serviceIntent.Mode(project.ServiceRedis)
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
	components := project.Components{DatabaseSQLite: true, Docker: true, Events: true, Cache: true}
	plan := defaultResourcePlanForTest(t, components)
	renderer := &ProjectRenderer{
		workspace: currentProjectRenderWorkspace(t),
		config: &project.Config{
			Render: project.RenderConfig{Components: components},
			Apps:   map[string]project.AppConfig{"billing": {Components: components}},
		},
		resources: resourceRenderState{
			plan:          plan,
			serviceIntent: project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal),
		},
	}
	if err := renderer.prepareResourceEnvironment(); err != nil {
		t.Fatalf("prepare resource environment: %v", err)
	}
	resolved, err := project.ResolveServicePlanWithConsumers(renderer.resources.plan, components, renderer.resources.serviceIntent, renderer.resources.serviceConsumers)
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
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	plan := defaultResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	updated, changed := seedComposeProfiles([]byte("COMPOSE_PROFILES=metrics\n"), plan, components, intent)
	if !changed || string(updated) != "COMPOSE_PROFILES=metrics,redis\n" {
		t.Fatalf("seeded retained profile = %q changed=%t", updated, changed)
	}
}

// TestResourceRenderValuesIncludeNamedRedis verifies Auth sessions participate in Redis discovery.
func TestResourceRenderValuesIncludeNamedRedis(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Auth: true, Cache: true}
	plan := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	values, err := resourceRenderValuesForPlanWithConsumers(plan, components, intent, nil)
	if err != nil {
		t.Fatalf("resourceRenderValuesForPlanWithConsumers returned error: %v", err)
	}
	if values.CacheSessionsDriver != "redis" || !values.RedisActive || !values.RedisSupported || !values.RedisLocal {
		t.Fatalf("render values = %#v", values)
	}
}

// TestResourceRenderValuesRejectInvalidConsumers prevents service-planning errors from degrading into different Redis flags.
func TestResourceRenderValuesRejectInvalidConsumers(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Cache: true}
	plan := defaultResourcePlanForTest(t, components)
	_, err := resourceRenderValuesForPlanWithConsumers(plan, components, project.LocalServiceIntent{}, []project.EffectiveResourceConsumer{
		{Resource: project.ResourceCache, Consumer: "cache", Driver: "missing"},
	})
	if err == nil || !strings.Contains(err.Error(), "unknown Cache driver") {
		t.Fatalf("resourceRenderValuesForPlanWithConsumers error = %v, want invalid-consumer failure", err)
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
		workspace: currentProjectRenderWorkspace(t),
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
