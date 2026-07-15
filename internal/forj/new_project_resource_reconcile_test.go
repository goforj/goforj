package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestReconcileNewProjectTargetResourcesLeavesUntouchedTargetsAlone verifies empty and future destinations retain the proposal.
func TestReconcileNewProjectTargetResourcesLeavesUntouchedTargetsAlone(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true}
	proposed := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	targets := []string{t.TempDir(), filepath.Join(t.TempDir(), "not-created")}
	for _, target := range targets {
		result, err := reconcileNewProjectTargetResources(target, proposed, components, intent)
		if err != nil {
			t.Fatalf("reconcile target %q: %v", target, err)
		}
		if !reflect.DeepEqual(result.plan, proposed) {
			t.Fatalf("target %q changed plan\nwant: %#v\ngot:  %#v", target, proposed, result.plan)
		}
		if !reflect.DeepEqual(result.serviceIntent, intent) {
			t.Fatalf("target %q changed intent\nwant: %#v\ngot:  %#v", target, intent, result.serviceIntent)
		}
		if result.hasOverrides || result.overrideSummary != "" {
			t.Fatalf("target %q reported overrides: %#v", target, result)
		}
	}
}

// TestReconcileNewProjectTargetResourcesHidesInactiveRedisPlacement keeps ordinary owner files out of the simplified confirmation.
func TestReconcileNewProjectTargetResourcesHidesInactiveRedisPlacement(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true}
	proposed := defaultResourcePlanForTest(t, components)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, ".env"), []byte("APP_NAME=Existing\n"), 0o600); err != nil {
		t.Fatalf("write existing environment: %v", err)
	}

	result, err := reconcileNewProjectTargetResources(target, proposed, components, project.LocalServiceIntent{})
	if err != nil {
		t.Fatalf("reconcile existing target: %v", err)
	}
	if result.hasOverrides || result.overrideSummary != "" {
		t.Fatalf("inactive Redis placement leaked into confirmation: %#v", result)
	}
}

// TestReconcileNewProjectTargetResourcesPreviewsOwnerValues verifies concrete drivers and exact profile intent win without writes.
func TestReconcileNewProjectTargetResourcesPreviewsOwnerValues(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true, Auth: true, WebAPI: true, Cache: true}
	proposed := defaultResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal)
	target := t.TempDir()
	source := []byte(strings.Join([]string{
		"APP_NAME=Existing",
		"CACHE_DRIVER=redis",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"CACHE_SESSIONS_DRIVER=redis",
		"COMPOSE_PROFILES=metrics,redis",
		"",
	}, "\n"))
	path := filepath.Join(target, ".env")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("write existing environment: %v", err)
	}

	result, err := reconcileNewProjectTargetResources(target, proposed, components, intent)
	if err != nil {
		t.Fatalf("reconcile existing target: %v", err)
	}
	cache, _ := result.plan.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("effective cache active = %q, want redis", cache.Active)
	}
	named := result.plan.GeneratedNamedSelections(components)
	sessionDriver := ""
	for _, selection := range named {
		if selection.EnvironmentKey == "CACHE_SESSIONS_DRIVER" {
			sessionDriver = selection.Active
		}
	}
	if sessionDriver != "redis" {
		t.Fatalf("effective session cache = %q, want redis", sessionDriver)
	}
	mode, ok := result.serviceIntent.Mode(project.ServiceRedis)
	if !ok || mode != project.LocalServiceModeLocal {
		t.Fatalf("effective Redis intent = %q selected=%t, want local", mode, ok)
	}
	if !result.hasOverrides || !strings.Contains(result.overrideSummary, "Cache default: Redis") || !strings.Contains(result.overrideSummary, "Redis service: local") {
		t.Fatalf("override summary = %q", result.overrideSummary)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing environment: %v", err)
	}
	if !reflect.DeepEqual(after, source) {
		t.Fatalf("read-only reconciliation changed .env\nwant: %q\ngot:  %q", source, after)
	}
}

// TestReconcileNewProjectTargetResourcesKeepsExplicitPlanAboveSafeExample verifies a committed fallback cannot replace wizard choices.
func TestReconcileNewProjectTargetResourcesKeepsExplicitPlanAboveSafeExample(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true, Events: true}
	proposed := defaultResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	target := t.TempDir()
	source := []byte("EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\nCOMPOSE_PROFILES=metrics,redis-debug\n")
	path := filepath.Join(target, ".env.example")
	if err := os.WriteFile(path, source, 0o644); err != nil {
		t.Fatalf("write safe environment example: %v", err)
	}

	result, err := reconcileNewProjectTargetResources(target, proposed, components, intent)
	if err != nil {
		t.Fatalf("reconcile safe example: %v", err)
	}
	events, _ := result.plan.Selection(project.ResourceEvents)
	if events.Active != "inproc" {
		t.Fatalf("effective events active = %q, want explicit in-process selection", events.Active)
	}
	mode, _ := result.serviceIntent.Mode(project.ServiceRedis)
	if mode != project.LocalServiceModeLocal {
		t.Fatalf("safe fallback replaced explicit local Redis intent with %q", mode)
	}
	if result.hasOverrides || result.overrideSummary != "" {
		t.Fatalf("lower-precedence safe fallback reported owner overrides: %#v", result)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read safe environment example: %v", err)
	}
	if !reflect.DeepEqual(after, source) {
		t.Fatalf("read-only reconciliation changed .env.example\nwant: %q\ngot:  %q", source, after)
	}
}

// TestReconcileNewProjectTargetResourcesPrefersOwnerEnvironment verifies the safe example cannot replace concrete runtime ownership.
func TestReconcileNewProjectTargetResourcesPrefersOwnerEnvironment(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Events: true}
	proposed := defaultResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, ".env"), []byte("EVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\nCOMPOSE_PROFILES=redis\n"), 0o600); err != nil {
		t.Fatalf("write owner environment: %v", err)
	}
	if err := os.WriteFile(filepath.Join(target, ".env.example"), []byte("EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\n"), 0o644); err != nil {
		t.Fatalf("write safe environment example: %v", err)
	}

	result, err := reconcileNewProjectTargetResources(target, proposed, components, intent)
	if err != nil {
		t.Fatalf("reconcile owner environment: %v", err)
	}
	events, _ := result.plan.Selection(project.ResourceEvents)
	if events.Active != "inproc" {
		t.Fatalf("safe example replaced owner events driver with %q", events.Active)
	}
	if result.hasOverrides {
		t.Fatalf("matching owner environment should not report example overrides: %q", result.overrideSummary)
	}
}

// TestReconcileNewProjectTargetResourcesRejectsInvalidOwnerContract verifies mismatch errors cannot produce partial target edits.
func TestReconcileNewProjectTargetResourcesRejectsInvalidOwnerContract(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Jobs: true}
	proposed := defaultResourcePlanForTest(t, components)
	target := t.TempDir()
	source := []byte("QUEUE_DRIVER=redis\nQUEUE_SUPPORTED_DRIVERS=workerpool\n")
	path := filepath.Join(target, ".env")
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatalf("write invalid environment: %v", err)
	}

	_, err := reconcileNewProjectTargetResources(target, proposed, components, project.LocalServiceIntent{})
	if err == nil || !strings.Contains(err.Error(), "excludes active") {
		t.Fatalf("reconcile error = %v, want active/support mismatch", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read invalid environment: %v", readErr)
	}
	if !reflect.DeepEqual(after, source) {
		t.Fatalf("failed reconciliation changed .env\nwant: %q\ngot:  %q", source, after)
	}
}

// TestReconcileNewProjectTargetResourcesUsesLegacyQueueOnlyAsFallback protects explicit-plan precedence during migration.
func TestReconcileNewProjectTargetResourcesUsesLegacyQueueOnlyAsFallback(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Jobs: true}
	proposed := defaultResourcePlanForTest(t, components)
	target := t.TempDir()
	config := []byte("render:\n  queue_driver: nats\n")
	configPath := filepath.Join(target, ".goforj.yml")
	if err := os.WriteFile(configPath, config, 0o644); err != nil {
		t.Fatalf("write legacy project config: %v", err)
	}

	explicitResult, err := reconcileNewProjectTargetResources(target, proposed, components, project.LocalServiceIntent{})
	if err != nil {
		t.Fatalf("reconcile explicit queue plan: %v", err)
	}
	explicitQueue, _ := explicitResult.plan.Selection(project.ResourceQueue)
	if explicitQueue.Active != "workerpool" || explicitResult.hasOverrides {
		t.Fatalf("legacy queue replaced explicit plan: selection=%#v summary=%q", explicitQueue, explicitResult.overrideSummary)
	}

	incomplete := proposed.WithoutSelection(project.ResourceQueue)
	fallbackResult, err := reconcileNewProjectTargetResources(target, incomplete, components, project.LocalServiceIntent{})
	if err != nil {
		t.Fatalf("reconcile legacy queue fallback: %v", err)
	}
	fallbackQueue, _ := fallbackResult.plan.Selection(project.ResourceQueue)
	if fallbackQueue.Active != "nats" || !reflect.DeepEqual(fallbackQueue.Supported, []string{"nats"}) {
		t.Fatalf("legacy queue fallback = %#v", fallbackQueue)
	}
	if !fallbackResult.hasOverrides || !strings.Contains(fallbackResult.overrideSummary, "legacy config") {
		t.Fatalf("legacy fallback summary = %q", fallbackResult.overrideSummary)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatalf("read legacy project config: %v", readErr)
	}
	if !reflect.DeepEqual(after, config) {
		t.Fatalf("read-only reconciliation changed legacy config\nwant: %q\ngot:  %q", config, after)
	}
}

// TestReconcileNewProjectTargetResourcesDiscoversNamedAndAppServices verifies Path preview carries owner scopes into confirmation planning.
func TestReconcileNewProjectTargetResourcesDiscoversNamedAndAppServices(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	proposed := defaultResourcePlanForTest(t, components)
	target := t.TempDir()
	config := []byte("project_name: Existing\nmodule_name: example.com/existing\nrender:\n  components:\n    database_sqlite: true\n    docker: true\n    cache: true\napps:\n  billing:\n    components:\n      database_sqlite: true\n      cache: true\n")
	if err := os.WriteFile(filepath.Join(target, ".goforj.yml"), config, 0o644); err != nil {
		t.Fatalf("write existing project config: %v", err)
	}
	source := []byte(strings.Join([]string{
		"CACHE_REPORTS_DRIVER=redis",
		"CACHE_REPORTS_ADDR=redis:6379",
		"BILLING_CACHE_DRIVER=redis",
		"BILLING_CACHE_ADDR=redis://owner:top-secret@billing.redis.example:6379",
		"COMPOSE_PROFILES=redis",
		"",
	}, "\n"))
	if err := os.WriteFile(filepath.Join(target, ".env"), source, 0o600); err != nil {
		t.Fatalf("write existing environment: %v", err)
	}

	result, err := reconcileNewProjectTargetResources(target, proposed, components, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal))
	if err != nil {
		t.Fatalf("reconcile existing target: %v", err)
	}
	for _, want := range []string{"Named/App service use", "Redis local", "cache:reports", "billing:cache:reports", "Redis external", "billing:cache"} {
		if !strings.Contains(result.overrideSummary, want) {
			t.Errorf("override summary omitted %q: %q", want, result.overrideSummary)
		}
	}
	if !result.hasOverrides {
		t.Fatalf("service-impacting scoped resources were not reported: %q", result.overrideSummary)
	}
	for _, leaked := range []string{"redis:6379", "billing.redis.example", "owner", "top-secret"} {
		if strings.Contains(result.overrideSummary, leaked) {
			t.Errorf("override summary leaked %q: %q", leaked, result.overrideSummary)
		}
	}
	resolved, err := project.ResolveServicePlanWithConsumers(result.plan, components, result.serviceIntent, result.serviceConsumers)
	if err != nil {
		t.Fatalf("resolve effective target services: %v", err)
	}
	local := resolved.RequirementsInState(project.ServiceStateActiveLocal)
	external := resolved.RequirementsInState(project.ServiceStateExternalRequired)
	if len(local) != 1 || !reflect.DeepEqual(local[0].ActiveConsumers, []string{"cache:reports", "billing:cache:reports"}) {
		t.Fatalf("local named Redis requirement = %#v", local)
	}
	if len(external) != 1 || !reflect.DeepEqual(external[0].ActiveConsumers, []string{"billing:cache"}) {
		t.Fatalf("external named App Redis requirement = %#v", external)
	}
	if after, readErr := os.ReadFile(filepath.Join(target, ".env")); readErr != nil || !reflect.DeepEqual(after, source) {
		t.Fatalf("read-only target reconciliation changed owner environment: err=%v data=%q", readErr, after)
	}
}
