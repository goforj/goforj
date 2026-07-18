package resourceenv

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestReconcilePreservesOwners verifies full resource ownership and portable initialization.
func TestReconcilePreservesOwners(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Mail: true, Docker: true, Jobs: true, Auth: true, Events: true, Cache: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\nCOMPOSE_PROFILES=metrics\n")
	reconciliation, err := Reconcile(source, seed, components, true)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if !reconciliation.Changed {
		t.Fatal("Reconcile() did not initialize missing resource keys")
	}
	cache, _ := reconciliation.EffectivePlan.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("owner cache driver = %q, want redis", cache.Active)
	}
	text := string(reconciliation.Source)
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

// TestReconcileRemovesObsoleteDiagnosticCaches verifies upgrades stop regenerating retired framework stores.
func TestReconcileRemovesObsoleteDiagnosticCaches(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Jobs: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_INSPECTS_DRIVER=memory\nCACHE_LIGHTHOUSE_DRIVER=redis\n# CACHE_LIGHTHOUSE_DRIVER=memory\n")
	reconciliation, err := Reconcile(source, seed, components, true)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if !reconciliation.Changed {
		t.Fatal("Reconcile() did not request a write for obsolete diagnostic caches")
	}
	if strings.Contains(string(reconciliation.Source), "CACHE_INSPECTS_DRIVER") || strings.Contains(string(reconciliation.Source), "CACHE_LIGHTHOUSE_DRIVER") {
		t.Fatalf("reconciled environment retained an obsolete diagnostic cache:\n%s", reconciliation.Source)
	}
	for _, named := range reconciliation.EffectivePlan.GeneratedNamedSelections(components) {
		if named.EnvironmentKey == "CACHE_INSPECTS_DRIVER" || named.EnvironmentKey == "CACHE_LIGHTHOUSE_DRIVER" {
			t.Fatalf("effective resource plan retained an obsolete diagnostic cache: %#v", reconciliation.EffectivePlan)
		}
	}
}

// TestReconcileIgnoresDisabledResourceResidue verifies owner values cannot reactivate omitted resources.
func TestReconcileIgnoresDisabledResourceResidue(t *testing.T) {
	tests := []struct {
		name     string
		resource project.ResourceKey
		source   string
		want     []string
	}{
		{
			name:     "events",
			resource: project.ResourceEvents,
			source:   "EVENTS_DRIVER=redis\nEVENTS_SUPPORTED_DRIVERS=redis\n",
			want:     []string{"EVENTS_DRIVER=redis\n"},
		},
		{
			name:     "storage",
			resource: project.ResourceStorage,
			source:   "STORAGE_DRIVER=s3\nSTORAGE_SUPPORTED_DRIVERS=s3\nSTORAGE_PUBLIC_DRIVER=s3\n",
			want:     []string{"STORAGE_DRIVER=s3\n", "STORAGE_PUBLIC_DRIVER=s3\n"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := project.Components{DatabaseSQLite: true}
			reconciliation, err := Reconcile([]byte(test.source), defaultResourcePlanForTest(t, components), components, true)
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			if _, exists := reconciliation.EffectivePlan.Selection(test.resource); exists {
				t.Fatalf("disabled resource was resurrected from owner environment: %#v", reconciliation.EffectivePlan)
			}
			for _, want := range test.want {
				if !strings.Contains(string(reconciliation.Source), want) {
					t.Fatalf("reconciliation deleted owner residue %q:\n%s", want, reconciliation.Source)
				}
			}
		})
	}
}

// TestReconcileAcceptsDotenvSyntax keeps preview, rerender, and generation on one parsing contract.
func TestReconcileAcceptsDotenvSyntax(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Cache: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("export CACHE_DRIVER=\"redis\"\nCACHE_SUPPORTED_DRIVERS='memory,redis' # portable pair\nEVENTS_DRIVER=\"inproc\"\nEVENTS_SUPPORTED_DRIVERS=\"inproc,redis\"\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\nDB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite\nCACHE_PREFIX=\"owner#prefix\"\n")
	reconciliation, err := Reconcile(source, seed, components, true)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cache, _ := reconciliation.EffectivePlan.Selection(project.ResourceCache)
	if cache.Active != "redis" || strings.Join(cache.Supported, ",") != "memory,redis" {
		t.Fatalf("dotenv values were not normalized: %#v", cache)
	}
	if !strings.Contains(string(reconciliation.Source), "CACHE_PREFIX=\"owner#prefix\"") {
		t.Fatalf("line-aware reconciliation damaged a quoted hash:\n%s", reconciliation.Source)
	}
}

// TestReconcileResolvesDotenvReferences prevents owner indirection from being mistaken for a missing value.
func TestReconcileResolvesDotenvReferences(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Cache: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_BACKEND=redis\nCACHE_DRIVER=${CACHE_BACKEND}\nCACHE_SUPPORTED_DRIVERS=memory,redis\nEVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local\nDB_DRIVER=sqlite\nDB_SUPPORTED_DRIVERS=sqlite\n")

	reconciliation, err := Reconcile(source, seed, components, true)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	cache, _ := reconciliation.EffectivePlan.Selection(project.ResourceCache)
	if cache.Active != "redis" {
		t.Fatalf("interpolated cache driver = %q, want redis", cache.Active)
	}
	if !strings.Contains(string(reconciliation.Source), "CACHE_DRIVER=${CACHE_BACKEND}") {
		t.Fatalf("reconciliation replaced owner interpolation:\n%s", reconciliation.Source)
	}
}

// TestReconcilePreservesDatabaseAliases keeps owner syntax while planning with canonical driver identities.
func TestReconcilePreservesDatabaseAliases(t *testing.T) {
	components := project.Components{DatabasePostgres: true}
	seed := defaultResourcePlanForTest(t, components)
	source := []byte("DB_DRIVER=postgresql\nDB_SUPPORTED_DRIVERS=postgres\n")
	reconciliation, err := Reconcile(source, seed, components, true)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	database, _ := reconciliation.EffectivePlan.Selection(project.ResourceDatabase)
	if database.Active != "postgres" || strings.Join(database.Supported, ",") != "postgres" {
		t.Fatalf("canonical database selection = %#v, want Postgres", database)
	}
	if !strings.Contains(string(reconciliation.Source), "DB_DRIVER=postgresql") {
		t.Fatalf("reconciliation replaced owner alias:\n%s", reconciliation.Source)
	}
}

// TestReconcileRejectsOwnerMismatch verifies validation completes before any rewrite is returned for publication.
func TestReconcileRejectsOwnerMismatch(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Cache: true}
	seed := defaultResourcePlanForTest(t, components)
	reconciliation, err := Reconcile([]byte("CACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory\n"), seed, components, true)
	if err == nil || !strings.Contains(err.Error(), "excludes active") {
		t.Fatalf("Reconcile() error = %v, want active-subset failure", err)
	}
	if reconciliation.Source != nil || reconciliation.Changed {
		t.Fatalf("invalid contract returned a rewrite: changed=%t data=%q", reconciliation.Changed, reconciliation.Source)
	}
}

// TestRemoveGeneratedAssignmentsIncludesAppLocalCache verifies component removal cleans every generated Cache driver scope.
func TestRemoveGeneratedAssignmentsIncludesAppLocalCache(t *testing.T) {
	source := []byte("CACHE_DRIVER=memory\nBILLING_CACHE_DRIVER=redis\nBILLING_CACHE_ADDR=redis:6379\nCACHE_LIGHTHOUSE_DRIVER=memory\nOWNER_VALUE=keep\n")
	apps := []project.App{{Name: "billing"}}
	updated, changed := RemoveGeneratedAssignments(source, project.Components{CLI: true}, apps)
	if !changed {
		t.Fatal("RemoveGeneratedAssignments() did not report generated Cache removals")
	}
	want := "BILLING_CACHE_ADDR=redis:6379\nOWNER_VALUE=keep\n"
	if string(updated) != want {
		t.Fatalf("RemoveGeneratedAssignments() = %q, want %q", updated, want)
	}
}

// TestResolveServiceIntentUsesExactCatalogProfiles protects provider placement from neighboring tokens.
func TestResolveServiceIntentUsesExactCatalogProfiles(t *testing.T) {
	fallback := project.LocalServiceIntent{}.
		WithMode(project.ServiceRedis, project.LocalServiceModeLocal).
		WithMode(project.ServiceStorageS3, project.LocalServiceModeLocal)
	intent := ResolveServiceIntent([]byte("COMPOSE_PROFILES=metrics,redis-debug,rustfs-debug\n"), fallback)
	for _, provider := range []project.ServiceKey{project.ServiceRedis, project.ServiceStorageS3} {
		mode, ok := intent.Mode(provider)
		if !ok || mode != project.LocalServiceModeExternal {
			t.Fatalf("%s mode = %q selected=%t, want external", provider, mode, ok)
		}
	}

	intent = ResolveServiceIntent([]byte("COMPOSE_PROFILES=opensearch,rustfs,redis\n"), fallback)
	for _, provider := range []project.ServiceKey{project.ServiceRedis, project.ServiceStorageS3} {
		mode, ok := intent.Mode(provider)
		if !ok || mode != project.LocalServiceModeLocal {
			t.Fatalf("%s mode = %q selected=%t, want local", provider, mode, ok)
		}
	}
	if _, ok := intent.Mode(project.ServiceKey("opensearch")); ok {
		t.Fatal("standalone OpenSearch profile invented a resource-service placement")
	}

	intent = ResolveServiceIntent([]byte("APP_NAME=Existing\n"), project.LocalServiceIntent{})
	if mode, ok := intent.Mode(project.ServiceRedis); ok {
		t.Fatalf("ordinary environment invented Redis placement %q", mode)
	}
	if mode, ok := intent.Mode(project.ServiceStorageS3); ok {
		t.Fatalf("ordinary environment invented S3 placement %q", mode)
	}
}

// defaultResourcePlanForTest builds the same concrete driver plan used by a new project.
func defaultResourcePlanForTest(t *testing.T, components project.Components) project.ResourcePlan {
	t.Helper()
	plan, err := project.DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan() error = %v", err)
	}
	return plan
}

// TestRemoveGeneratedAssignmentsPreservesUnchangedSource verifies cleanup does not manufacture a replacement buffer.
func TestRemoveGeneratedAssignmentsPreservesUnchangedSource(t *testing.T) {
	source := []byte("OWNER_VALUE=keep")
	updated, changed := RemoveGeneratedAssignments(source, project.Components{Cache: true}, nil)
	if changed || !reflect.DeepEqual(updated, source) {
		t.Fatalf("RemoveGeneratedAssignments() = %q, %t, want unchanged source", updated, changed)
	}
}
