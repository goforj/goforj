package project

import (
	"reflect"
	"testing"
)

// TestResolveServicePlanRedisStates locks placement intent and Docker availability to the four Redis states.
func TestResolveServicePlanRedisStates(t *testing.T) {
	tests := []struct {
		name        string
		redisActive bool
		docker      bool
		intent      LocalServiceIntent
		wantState   ServiceState
		wantExists  bool
	}{
		{name: "active local", redisActive: true, docker: true, intent: LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal), wantState: ServiceStateActiveLocal, wantExists: true},
		{name: "active external explicitly", redisActive: true, docker: true, intent: LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeExternal), wantState: ServiceStateExternalRequired, wantExists: true},
		{name: "active external without intent", redisActive: true, docker: true, wantState: ServiceStateExternalRequired, wantExists: true},
		{name: "active without Docker", redisActive: true, intent: LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal), wantState: ServiceStateExternalRequired, wantExists: true},
		{name: "available local", docker: true, wantState: ServiceStateAvailableLocal, wantExists: true},
		{name: "local requested unused", docker: true, intent: LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal), wantState: ServiceStateLocalRequestedUnused, wantExists: true},
		{name: "support without Docker", wantExists: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := Components{DatabaseSQLite: true, Docker: test.docker, Jobs: true}
			resourcePlan := servicePlanTestDefaultPlan(t, components)
			if test.redisActive {
				resourcePlan = servicePlanTestRedisActivePlan(t, components)
			}
			servicePlan, err := ResolveServicePlan(resourcePlan, components, test.intent)
			if err != nil {
				t.Fatalf("ResolveServicePlan returned error: %v", err)
			}
			requirement, exists := servicePlan.Requirement(ServiceRedis)
			if exists != test.wantExists {
				t.Fatalf("Redis requirement exists = %v, want %v", exists, test.wantExists)
			}
			if exists && requirement.State != test.wantState {
				t.Fatalf("Redis state = %q, want %q", requirement.State, test.wantState)
			}
		})
	}
}

// TestResolveServicePlanS3States applies the shared local-provider policy to RustFS-backed S3 storage.
func TestResolveServicePlanS3States(t *testing.T) {
	tests := []struct {
		name       string
		s3Active   bool
		docker     bool
		intent     LocalServiceIntent
		wantState  ServiceState
		wantExists bool
	}{
		{name: "active local", s3Active: true, docker: true, intent: LocalServiceIntent{}.WithMode(ServiceStorageS3, LocalServiceModeLocal), wantState: ServiceStateActiveLocal, wantExists: true},
		{name: "active external explicitly", s3Active: true, docker: true, intent: LocalServiceIntent{}.WithMode(ServiceStorageS3, LocalServiceModeExternal), wantState: ServiceStateExternalRequired, wantExists: true},
		{name: "active external without intent", s3Active: true, docker: true, wantState: ServiceStateExternalRequired, wantExists: true},
		{name: "active without Docker", s3Active: true, intent: LocalServiceIntent{}.WithMode(ServiceStorageS3, LocalServiceModeLocal), wantState: ServiceStateExternalRequired, wantExists: true},
		{name: "available local", docker: true, wantState: ServiceStateAvailableLocal, wantExists: true},
		{name: "local requested unused", docker: true, intent: LocalServiceIntent{}.WithMode(ServiceStorageS3, LocalServiceModeLocal), wantState: ServiceStateLocalRequestedUnused, wantExists: true},
		{name: "support without Docker", wantExists: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := Components{DatabaseSQLite: true, Docker: test.docker, Storage: true}
			resourcePlan := servicePlanTestDefaultPlan(t, components)
			storage, _ := resourcePlan.Selection(ResourceStorage)
			storage.Supported = append(storage.Supported, "s3")
			if test.s3Active {
				storage.Active = "s3"
			}
			resourcePlan, err := resourcePlan.WithSelection(ResourceStorage, storage).Normalized(components)
			if err != nil {
				t.Fatalf("normalize S3 plan: %v", err)
			}
			servicePlan, err := ResolveServicePlan(resourcePlan, components, test.intent)
			if err != nil {
				t.Fatalf("ResolveServicePlan returned error: %v", err)
			}
			requirement, exists := servicePlan.Requirement(ServiceStorageS3)
			if exists != test.wantExists {
				t.Fatalf("S3 requirement exists = %v, want %v", exists, test.wantExists)
			}
			if exists && requirement.State != test.wantState {
				t.Fatalf("S3 state = %q, want %q", requirement.State, test.wantState)
			}
		})
	}
}

// TestResolveServicePlanDeduplicatesRedisConsumers verifies one service covers every normal shared Redis resource.
func TestResolveServicePlanDeduplicatesRedisConsumers(t *testing.T) {
	components := Components{Auth: true, Cache: true, DatabaseMySQL: true, Docker: true, Jobs: true, Events: true, Storage: true}
	resourcePlan := servicePlanTestRedisActivePlan(t, components)
	resourcePlan = resourcePlan.WithSelection(ResourceStorage, DriverSelection{Active: "redis", Supported: []string{"local", "redis"}})
	servicePlan, err := ResolveServicePlan(resourcePlan, components, LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal))
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}

	requirement, exists := servicePlan.Requirement(ServiceRedis)
	if !exists {
		t.Fatal("Redis requirement does not exist")
	}
	wantConsumers := []string{"cache", "queue", "events", "storage", "cache:sessions"}
	if !reflect.DeepEqual(requirement.ActiveConsumers, wantConsumers) {
		t.Fatalf("Redis consumers = %#v, want %#v", requirement.ActiveConsumers, wantConsumers)
	}
	if got := len(servicePlan.RequirementsInState(ServiceStateActiveLocal)); got != 2 {
		t.Fatalf("active local requirements = %d, want database and Redis", got)
	}
	if !servicePlan.HasActiveLocal() {
		t.Fatal("service plan should report active local services")
	}
}

// TestResolveServicePlanIgnoresStaleDisabledStorage verifies a transient selection cannot invent Storage infrastructure.
func TestResolveServicePlanIgnoresStaleDisabledStorage(t *testing.T) {
	components := Components{DatabaseSQLite: true}
	resourcePlan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	resourcePlan = resourcePlan.WithSelection(ResourceStorage, DriverSelection{Active: "s3", Supported: []string{"s3"}})
	servicePlan, err := ResolveServicePlan(resourcePlan, components, LocalServiceIntent{})
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}
	if requirement, exists := servicePlan.Requirement(ServiceStorageS3); exists {
		t.Fatalf("Storage-disabled plan created an S3 requirement: %#v", requirement)
	}
}

// TestResolveServicePlanIncludesGeneratedAuthSessions verifies named generated resources affect infrastructure discovery.
func TestResolveServicePlanIncludesGeneratedAuthSessions(t *testing.T) {
	components := Components{Auth: true, Cache: true, DatabaseSQLite: true, Docker: true, Events: true}
	resourcePlan := servicePlanTestRedisActivePlan(t, components)
	resourcePlan = resourcePlan.WithSelection(ResourceCache, DriverSelection{Active: "memory", Supported: []string{"memory", "redis"}})
	servicePlan, err := ResolveServicePlan(resourcePlan, components, LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal))
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}

	requirement, exists := servicePlan.Requirement(ServiceRedis)
	if !exists {
		t.Fatal("Redis requirement does not exist")
	}
	if requirement.State != ServiceStateActiveLocal {
		t.Fatalf("Redis state = %q, want %q", requirement.State, ServiceStateActiveLocal)
	}
	if !reflect.DeepEqual(requirement.ActiveConsumers, []string{"events", "cache:sessions"}) {
		t.Fatalf("Redis consumers = %#v, want events and generated sessions", requirement.ActiveConsumers)
	}
}

// TestResolveServicePlanDatabasePolicy keeps database placement independent from cache, queue, and event selections.
func TestResolveServicePlanDatabasePolicy(t *testing.T) {
	tests := []struct {
		name       string
		components Components
		wantKey    ServiceKey
		wantState  ServiceState
	}{
		{name: "MySQL local", components: Components{DatabaseMySQL: true, Docker: true}, wantKey: ServiceMySQL, wantState: ServiceStateActiveLocal},
		{name: "MySQL external", components: Components{DatabaseMySQL: true}, wantKey: ServiceMySQL, wantState: ServiceStateExternalRequired},
		{name: "Postgres local", components: Components{DatabasePostgres: true, Docker: true}, wantKey: ServicePostgres, wantState: ServiceStateActiveLocal},
		{name: "Postgres external", components: Components{DatabasePostgres: true}, wantKey: ServicePostgres, wantState: ServiceStateExternalRequired},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resourcePlan, err := DefaultResourcePlan(test.components)
			if err != nil {
				t.Fatalf("DefaultResourcePlan returned error: %v", err)
			}
			servicePlan, err := ResolveServicePlan(resourcePlan, test.components, LocalServiceIntent{})
			if err != nil {
				t.Fatalf("ResolveServicePlan returned error: %v", err)
			}
			requirement, exists := servicePlan.Requirement(test.wantKey)
			if !exists {
				t.Fatalf("%s requirement does not exist", test.wantKey)
			}
			if requirement.State != test.wantState {
				t.Fatalf("%s state = %q, want %q", test.wantKey, requirement.State, test.wantState)
			}
		})
	}
}

// TestResolveServicePlanSQLiteAndLocalDriversNeedNoService verifies in-process resources produce no runtime dependency.
func TestResolveServicePlanSQLiteAndLocalDriversNeedNoService(t *testing.T) {
	components := Components{DatabaseSQLite: true}
	resourcePlan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	servicePlan, err := ResolveServicePlan(resourcePlan, components, LocalServiceIntent{})
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}
	if len(servicePlan.Requirements) != 0 {
		t.Fatalf("service requirements = %#v, want none", servicePlan.Requirements)
	}
	if servicePlan.HasActiveLocal() {
		t.Fatal("service plan should not report active local services")
	}
}

// TestResolveServicePlanRejectsInvalidInput verifies transient planner inputs fail before generation consumes them.
func TestResolveServicePlanRejectsInvalidInput(t *testing.T) {
	components := Components{DatabaseSQLite: true}
	resourcePlan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	_, err = ResolveServicePlan(resourcePlan, components, LocalServiceIntent{Modes: map[ServiceKey]LocalServiceMode{ServiceRedis: "somewhere"}})
	if err == nil {
		t.Fatal("ResolveServicePlan should reject an unknown local-service mode")
	}
}

// TestResolveServicePlanRejectsUnknownIntentService prevents placement typos from becoming invisible transient state.
func TestResolveServicePlanRejectsUnknownIntentService(t *testing.T) {
	components := Components{DatabaseSQLite: true, Docker: true}
	resourcePlan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	_, err = ResolveServicePlan(resourcePlan, components, LocalServiceIntent{Modes: map[ServiceKey]LocalServiceMode{
		ServiceKey("redsi"): LocalServiceModeLocal,
	}})
	if err == nil {
		t.Fatal("ResolveServicePlan should reject an unknown service key")
	}
}

func TestServicePlanRequirementReturnsDefensiveCopy(t *testing.T) {
	components := Components{Cache: true, DatabaseSQLite: true, Docker: true}
	resourcePlan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	cache, _ := resourcePlan.Selection(ResourceCache)
	cache.Active = "redis"
	resourcePlan = resourcePlan.WithSelection(ResourceCache, cache)
	servicePlan, err := ResolveServicePlan(resourcePlan, components, LocalServiceIntent{})
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}
	requirement, exists := servicePlan.Requirement(ServiceRedis)
	if !exists {
		t.Fatal("Redis requirement does not exist")
	}
	requirement.ActiveConsumers[0] = "changed"
	original, _ := servicePlan.Requirement(ServiceRedis)
	if original.ActiveConsumers[0] == "changed" {
		t.Fatal("Requirement returned an aliased active-consumer slice")
	}
}

// TestResolveServicePlanReportsEveryExternalAdvancedDriver verifies the published inventory cannot disappear from confirmation.
func TestResolveServicePlanReportsEveryExternalAdvancedDriver(t *testing.T) {
	components := Components{Cache: true, DatabaseMySQL: true, Docker: true, Jobs: true, Mail: true}
	base, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	for _, definition := range ResourceCatalog() {
		if !definition.AppliesTo(components) {
			continue
		}
		for _, driver := range definition.Drivers {
			if driver.Service == "" || driver.LocallyProvisionable || driver.Service == ServiceMailSMTP {
				continue
			}
			t.Run(string(definition.Key)+"_"+driver.Name, func(t *testing.T) {
				selection, _ := base.Selection(definition.Key)
				selection.Active = driver.Name
				if !servicePlanTestContainsDriver(selection.Supported, driver.Name) {
					selection.Supported = append(selection.Supported, driver.Name)
				}
				plan := base.WithSelection(definition.Key, selection)
				intent := LocalServiceIntent{}.WithMode(driver.Service, LocalServiceModeLocal)
				resolved, resolveErr := ResolveServicePlan(plan, components, intent)
				if resolveErr != nil {
					t.Fatalf("ResolveServicePlan returned error: %v", resolveErr)
				}
				requirement, exists := resolved.Requirement(driver.Service)
				if !exists {
					t.Fatalf("service %q was omitted", driver.Service)
				}
				if requirement.State != ServiceStateExternalRequired {
					t.Fatalf("service %q state = %q, want external", driver.Service, requirement.State)
				}
				if requirement.Label != driver.ServiceLabel {
					t.Fatalf("service %q label = %q, want %q", driver.Service, requirement.Label, driver.ServiceLabel)
				}
			})
		}
	}
}

// TestResolveServicePlanKeepsResourceSpecificEndpointsSeparate prevents apparent provider reuse without an explicit connection mapping.
func TestResolveServicePlanKeepsResourceSpecificEndpointsSeparate(t *testing.T) {
	components := Components{Cache: true, DatabasePostgres: true, Docker: true, Jobs: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	cache, _ := plan.Selection(ResourceCache)
	cache.Active = "postgres"
	cache.Supported = append(cache.Supported, "postgres")
	queue, _ := plan.Selection(ResourceQueue)
	queue.Active = "postgres"
	queue.Supported = append(queue.Supported, "postgres")
	plan = plan.WithSelection(ResourceCache, cache).WithSelection(ResourceQueue, queue)

	resolved, err := ResolveServicePlan(plan, components, LocalServiceIntent{})
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}
	for _, check := range []struct {
		key   ServiceKey
		state ServiceState
	}{
		{key: ServicePostgres, state: ServiceStateActiveLocal},
		{key: ServiceCachePostgres, state: ServiceStateExternalRequired},
		{key: ServiceQueuePostgres, state: ServiceStateExternalRequired},
	} {
		requirement, exists := resolved.Requirement(check.key)
		if !exists || requirement.State != check.state {
			t.Errorf("service %q = %#v exists=%t, want state %q", check.key, requirement, exists, check.state)
		}
	}
}

// TestResolveServicePlanTreatsSMTPAsDevelopmentToolOnlyWithDocker preserves Mailpit's existing project-tool ownership.
func TestResolveServicePlanTreatsSMTPAsDevelopmentToolOnlyWithDocker(t *testing.T) {
	for _, docker := range []bool{true, false} {
		components := Components{DatabaseSQLite: true, Docker: docker, Mail: true}
		plan, err := DefaultResourcePlan(components)
		if err != nil {
			t.Fatalf("DefaultResourcePlan returned error: %v", err)
		}
		mail, _ := plan.Selection(ResourceMail)
		mail.Active = "smtp"
		plan = plan.WithSelection(ResourceMail, mail)
		resolved, err := ResolveServicePlan(plan, components, LocalServiceIntent{})
		if err != nil {
			t.Fatalf("ResolveServicePlan returned error: %v", err)
		}
		requirement, exists := resolved.Requirement(ServiceMailSMTP)
		if docker && exists {
			t.Fatalf("Docker SMTP should remain Mailpit tooling, got %#v", requirement)
		}
		if !docker && (!exists || requirement.State != ServiceStateExternalRequired) {
			t.Fatalf("non-Docker SMTP = %#v exists=%t, want external", requirement, exists)
		}
	}
}

// TestResolveServicePlanDoesNotDeduplicateResourceScopedNATS verifies endpoint affinity survives provider-name similarity.
func TestResolveServicePlanDoesNotDeduplicateResourceScopedNATS(t *testing.T) {
	components := Components{Cache: true, DatabaseSQLite: true, Jobs: true, Events: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	for _, key := range []ResourceKey{ResourceCache, ResourceQueue, ResourceEvents} {
		selection, _ := plan.Selection(key)
		selection.Active = "nats"
		selection.Supported = append(selection.Supported, "nats")
		plan = plan.WithSelection(key, selection)
	}
	resolved, err := ResolveServicePlan(plan, components, LocalServiceIntent{})
	if err != nil {
		t.Fatalf("ResolveServicePlan returned error: %v", err)
	}
	for _, key := range []ServiceKey{ServiceCacheNATS, ServiceQueueNATS, ServiceEventsNATS} {
		if requirement, exists := resolved.Requirement(key); !exists || requirement.State != ServiceStateExternalRequired {
			t.Errorf("NATS service %q = %#v exists=%t", key, requirement, exists)
		}
	}
}

// TestResolveServicePlanWithConsumersIncludesArbitraryNamedRedis verifies user-authored named resources participate in discovery.
func TestResolveServicePlanWithConsumersIncludesArbitraryNamedRedis(t *testing.T) {
	components := Components{Cache: true, DatabaseSQLite: true, Docker: true, Events: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	resolved, err := ResolveServicePlanWithConsumers(plan, components, LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal), []EffectiveResourceConsumer{
		{Resource: ResourceCache, Consumer: "cache:reports", Driver: "redis", LocalService: true},
	})
	if err != nil {
		t.Fatalf("ResolveServicePlanWithConsumers returned error: %v", err)
	}
	requirement, exists := resolved.Requirement(ServiceRedis)
	if !exists || requirement.State != ServiceStateActiveLocal {
		t.Fatalf("Redis requirement = %#v exists=%t, want active local", requirement, exists)
	}
	if !reflect.DeepEqual(requirement.ActiveConsumers, []string{"cache:reports"}) {
		t.Fatalf("Redis consumers = %#v, want arbitrary named cache", requirement.ActiveConsumers)
	}
}

// TestResolveServicePlanWithConsumersSeparatesExternalRedisEndpoints verifies affinity, rather than driver name, controls deduplication.
func TestResolveServicePlanWithConsumersSeparatesExternalRedisEndpoints(t *testing.T) {
	components := Components{Cache: true, DatabaseSQLite: true, Docker: true, Events: true}
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	resolved, err := ResolveServicePlanWithConsumers(plan, components, LocalServiceIntent{}.WithMode(ServiceRedis, LocalServiceModeLocal), []EffectiveResourceConsumer{
		{Resource: ResourceCache, Consumer: "cache", Driver: "redis", EndpointAffinity: "endpoint-cache"},
		{Resource: ResourceEvents, Consumer: "events", Driver: "redis", EndpointAffinity: "endpoint-events"},
	})
	if err != nil {
		t.Fatalf("ResolveServicePlanWithConsumers returned error: %v", err)
	}
	external := resolved.RequirementsInState(ServiceStateExternalRequired)
	if len(external) != 2 {
		t.Fatalf("external requirements = %#v, want two Redis endpoints", external)
	}
	if external[0].Key != ServiceRedis || external[1].Key != ServiceRedis || external[0].EndpointAffinity == external[1].EndpointAffinity {
		t.Fatalf("external Redis affinities were conflated: %#v", external)
	}
	if got := len(resolved.RequirementsFor(ServiceRedis)); got != 3 {
		t.Fatalf("all Redis requirements = %d, want two active endpoints plus the optional local build", got)
	}
}

// servicePlanTestDefaultPlan returns the production default or fails the current test.
func servicePlanTestDefaultPlan(t *testing.T, components Components) ResourcePlan {
	t.Helper()
	plan, err := DefaultResourcePlan(components)
	if err != nil {
		t.Fatalf("DefaultResourcePlan returned error: %v", err)
	}
	return plan
}

// servicePlanTestRedisActivePlan returns a valid plan with coordinated Redis consumers activated.
func servicePlanTestRedisActivePlan(t *testing.T, components Components) ResourcePlan {
	t.Helper()
	plan := servicePlanTestDefaultPlan(t, components)
	for _, key := range []ResourceKey{ResourceCache, ResourceQueue, ResourceEvents} {
		selection, ok := plan.Selection(key)
		if !ok {
			continue
		}
		selection.Active = "redis"
		plan = plan.WithSelection(key, selection)
	}
	if components.Auth && components.Cache {
		plan = plan.WithNamedSelection("CACHE_SESSIONS_DRIVER", "redis")
	}
	normalized, err := plan.Normalized(components)
	if err != nil {
		t.Fatalf("normalize Redis-active test plan: %v", err)
	}
	return normalized
}

// servicePlanTestContainsDriver reports whether a test selection already includes one driver.
func servicePlanTestContainsDriver(drivers []string, want string) bool {
	for _, driver := range drivers {
		if driver == want {
			return true
		}
	}
	return false
}
