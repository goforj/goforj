package resourceenv

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestConsumersCrossFamilyAppOwnership prevents resource-like App names from creating default or sibling named connections.
func TestConsumersCrossFamilyAppOwnership(t *testing.T) {
	components := project.Components{CLI: true, Feature: true, Cache: true}
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "conventional", true: "configured"}[configured], func(t *testing.T) {
			config := &project.Config{Render: project.RenderConfig{Components: components}}
			appNames := []string{"feature", "feature-child", "cache"}
			if configured {
				config.Apps = map[string]project.AppConfig{}
				for _, name := range appNames {
					config.Apps[name] = project.AppConfig{Components: components}
				}
				appNames = nil
			}
			plan := defaultResourcePlanForTest(t, components)
			source := []byte("FEATURE_DRIVER=memory\nFEATURE_CHECKOUT_DRIVER=file\nFEATURE_FEATURE_DRIVER=file\nFEATURE_FEATURE_CHECKOUT_DRIVER=memory\nFEATURE_CACHE_DRIVER=redis\nFEATURE_CHILD_CACHE_DRIVER=redis\nFEATURE_CHILD_FEATURE_DRIVER=file\nCACHE_FEATURE_DRIVER=file\nCACHE_CACHE_DRIVER=redis\n")
			consumers, err := ResolveConsumers(source, plan, components, config, appNames...)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, consumer := range consumers {
				got[consumer.Consumer] = consumer.Driver
			}
			want := map[string]string{
				"feature": "memory", "feature:checkout": "file", "cache": "memory",
				"feature:feature": "file", "feature:feature:checkout": "memory", "feature:cache": "redis",
				"feature-child:feature": "file", "feature-child:feature:checkout": "file", "feature-child:cache": "redis",
				"cache:feature": "file", "cache:feature:checkout": "file", "cache:cache": "redis",
				"feature:cache:cache":       "redis",
				"feature-child:cache:cache": "redis",
			}
			if len(got) != len(want) {
				t.Fatalf("consumers = %#v, want %#v", got, want)
			}
			for name, driver := range want {
				if got[name] != driver {
					t.Errorf("%s driver = %q, want %q", name, got[name], driver)
				}
			}
		})
	}
}

// TestConsumersFeatureNamedCacheWithoutApp retains legitimate resource names when no App owns their keys.
func TestConsumersFeatureNamedCacheWithoutApp(t *testing.T) {
	components := project.Components{Feature: true}
	consumers, err := ResolveConsumers([]byte("FEATURE_CACHE_DRIVER=file\n"), defaultResourcePlanForTest(t, components), components, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, consumer := range consumers {
		if consumer.Consumer == "feature:cache" && consumer.Driver == "file" {
			return
		}
	}
	t.Fatalf("missing default App named Feature cache: %#v", consumers)
}

// TestConsumersFeatureRawDriverFallback matches the generated manager's empty inheritance and whitespace defaulting.
func TestConsumersFeatureRawDriverFallback(t *testing.T) {
	components := project.Components{Feature: true}
	for _, suffix := range []string{"", "_CHECKOUT"} {
		for _, test := range []struct{ name, overlay, want string }{
			{"empty", "", "file"}, {"whitespace", " \t ", "memory"}, {"explicit", " MeMoRy ", "memory"},
		} {
			t.Run(suffix+test.name, func(t *testing.T) {
				key := "FEATURE" + suffix + "_DRIVER"
				source := []byte(key + "=file\nBILLING_" + key + "=\"" + test.overlay + "\"\n")
				consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, nil, "billing")
				if err != nil {
					t.Fatal(err)
				}
				name := "billing:feature"
				if suffix != "" {
					name += ":checkout"
				}
				for _, consumer := range consumers {
					if consumer.Consumer == name {
						if consumer.Driver != test.want {
							t.Fatalf("driver = %q, want %q", consumer.Driver, test.want)
						}
						return
					}
				}
				t.Fatalf("missing %s: %#v", name, consumers)
			})
		}
	}
}

// TestConsumersDisabledAppStillOwnsResourceKeys prevents stale App settings from becoming default named resources.
func TestConsumersDisabledAppStillOwnsResourceKeys(t *testing.T) {
	components := project.Components{CLI: true, Feature: true, Cache: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps:   map[string]project.AppConfig{"feature": {Components: project.Components{CLI: true}}},
	}
	consumers, err := ResolveConsumers([]byte("FEATURE_CACHE_DRIVER=redis\nFEATURE_FEATURE_DRIVER=file\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 2 {
		t.Fatalf("disabled App keys invented consumers: %#v", consumers)
	}
	for _, consumer := range consumers {
		if consumer.Driver != "memory" {
			t.Fatalf("disabled App key changed root driver: %#v", consumer)
		}
	}
}

// TestConsumersLegacyCacheNamedFeature keeps legacy Cache discovery aligned with its generated runtime.
func TestConsumersLegacyCacheNamedFeature(t *testing.T) {
	components := project.Components{CLI: true, Cache: true, Docker: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps:   map[string]project.AppConfig{"cache": {Components: project.Components{CLI: true, Cache: true}}},
	}
	plan := defaultResourcePlanForTest(t, components)
	source := []byte("CACHE_FEATURE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\n")
	consumers, err := ResolveConsumers(source, plan, components, config)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, consumer := range consumers {
		if consumer.Consumer == "cache:feature" && consumer.Resource == project.ResourceCache && consumer.Driver == "redis" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing legacy named Cache: %#v", consumers)
	}
	servicePlan, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal), consumers)
	if err != nil {
		t.Fatal(err)
	}
	if !servicePlan.HasActiveLocal() {
		t.Fatalf("legacy named Cache did not activate Redis: %#v", servicePlan)
	}
}

// TestConsumersEnabledFeatureAppExcludesLegacyCollision keeps an App Feature driver from becoming a root Cache store.
func TestConsumersEnabledFeatureAppExcludesLegacyCollision(t *testing.T) {
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, Cache: true}},
		Apps:   map[string]project.AppConfig{"cache": {Components: project.Components{CLI: true, Feature: true}}},
	}
	components := project.ProjectComponents(config)
	consumers, err := ResolveConsumers([]byte("CACHE_DRIVER=memory\nCACHE_FEATURE_DRIVER=file\nCACHE_FEATURE_PATH=flags.json\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 2 {
		t.Fatalf("App Feature settings created legacy consumers: %#v", consumers)
	}
	for _, consumer := range consumers {
		if consumer.Resource == project.ResourceCache && (consumer.Consumer != "cache" || consumer.Driver != "memory") {
			t.Fatalf("App Feature changed Cache: %#v", consumer)
		}
	}
}

// TestConsumersIgnoreFeatureOverlaysWithoutKnownApps keeps stale keys out of Feature discovery while preserving legacy App inference.
func TestConsumersIgnoreFeatureOverlaysWithoutKnownApps(t *testing.T) {
	components := project.Components{CLI: true, Feature: true, Cache: true}
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "default-only", true: "configured"}[configured], func(t *testing.T) {
			var config *project.Config
			if configured {
				config = &project.Config{Render: project.RenderConfig{Components: components}}
			}
			source := []byte("FEATURE_DRIVER=memory\nFEATURE_CACHE_DRIVER=file\nMISSING_FEATURE_STALE_DRIVER=hosted\nMISSING_CACHE_DRIVER=redis\n")
			consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config)
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, consumer := range consumers {
				got[consumer.Consumer] = consumer.Driver
			}
			want := map[string]string{"feature": "memory", "feature:cache": "file", "cache": "memory", "missing:cache": "redis"}
			if len(got) != len(want) {
				t.Fatalf("consumers = %#v, want %#v", got, want)
			}
			for name, driver := range want {
				if got[name] != driver {
					t.Errorf("%s driver = %q, want %q", name, got[name], driver)
				}
			}
		})
	}
}

// TestConsumersKnownFeatureInventory validates conventional and Dev Apps even without an explicit component override.
func TestConsumersKnownFeatureInventory(t *testing.T) {
	components := project.Components{CLI: true, Feature: true}
	for _, inventory := range []string{"conventional", "default-only-conventional", "dev-apps", "dev-run"} {
		t.Run(inventory, func(t *testing.T) {
			config := &project.Config{Render: project.RenderConfig{Components: components}}
			var names []string
			switch inventory {
			case "conventional":
				names = []string{"billing"}
			case "default-only-conventional":
				config = nil
				names = []string{"billing"}
			case "dev-apps":
				config.Dev.Apps = map[string]project.DevApp{"billing": {}}
			case "dev-run":
				config.Dev.Run = map[string]string{"billing": "go run ./app/billing"}
			}
			plan := defaultResourcePlanForTest(t, components)
			consumers, err := ResolveConsumers([]byte("BILLING_FEATURE_CHECKOUT_DRIVER=file\n"), plan, components, config, names...)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, consumer := range consumers {
				if consumer.Consumer == "billing:feature:checkout" && consumer.Driver == "file" {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing known App Feature: %#v", consumers)
			}
			_, err = ResolveConsumers([]byte("BILLING_FEATURE_CHECKOUT_DRIVER=hosted\n"), plan, components, config, names...)
			if err == nil || err.Error() != `effective consumer billing:feature:checkout selects unknown Feature Flags driver "hosted"` {
				t.Fatalf("known App driver error = %v, want checkout driver validation", err)
			}
		})
	}
}

// TestConsumersKnownDisabledDevAppOverridesDefault prevents Dev inventory from restoring a disabled Feature component.
func TestConsumersKnownDisabledDevAppOverridesDefault(t *testing.T) {
	components := project.Components{CLI: true, Feature: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps:   map[string]project.AppConfig{"feature-child": {Components: project.Components{CLI: true}}},
	}
	config.Dev.Apps = map[string]project.DevApp{"feature": {}, "feature-child": {}}
	consumers, err := ResolveConsumers([]byte("FEATURE_FEATURE_DRIVER=file\nFEATURE_CHILD_FEATURE_DRIVER=hosted\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(consumers) != 2 {
		t.Fatalf("disabled longest App prefix invented consumers: %#v", consumers)
	}
	for _, consumer := range consumers {
		if consumer.Consumer == "feature:feature" && consumer.Driver == "file" {
			return
		}
	}
	t.Fatalf("missing enabled parent App Feature: %#v", consumers)
}

// TestKnownAppNamesSharesTrustedInventory keeps consumer and generator ownership deterministic across all App sources.
func TestKnownAppNamesSharesTrustedInventory(t *testing.T) {
	config := &project.Config{Apps: map[string]project.AppConfig{
		" Billing ": {}, "": {}, project.DefaultAppName: {},
	}}
	config.Dev.Apps = map[string]project.DevApp{"BILLING": {}, " Feature-Child ": {}}
	config.Dev.Run = map[string]string{" Reporting ": "go run .", "  ": "", project.DefaultAppName: ""}
	tests := []struct {
		name     string
		config   *project.Config
		supplied []string
		want     []string
	}{
		{name: "empty", want: []string{}},
		{name: "supplied-only", supplied: []string{" Billing ", "BILLING", "  ", project.DefaultAppName}, want: []string{"billing"}},
		{name: "config-only", config: config, want: []string{"billing", "feature-child", "reporting"}},
		{name: "combined", config: config, supplied: []string{"Zebra", " REPORTING ", "analytics"}, want: []string{"analytics", "billing", "feature-child", "reporting", "zebra"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := KnownAppNames(test.config, test.supplied...); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("known App names = %#v, want %#v", got, test.want)
			}
		})
	}
}

// TestConsumersFeatureOnlyEvidenceDoesNotInventLegacyApps prevents stale overlays from adding inherited service consumers.
func TestConsumersFeatureOnlyEvidenceDoesNotInventLegacyApps(t *testing.T) {
	components := project.Components{CLI: true, Feature: true, Cache: true}
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "default-only", true: "configured"}[configured], func(t *testing.T) {
			var config *project.Config
			if configured {
				config = &project.Config{Render: project.RenderConfig{Components: components}}
			}
			source := []byte("FEATURE_DRIVER=memory\nCACHE_DRIVER=redis\nCACHE_ADDR=cache.example:6379\nMISSING_FEATURE_STALE_DRIVER=hosted\n")
			plan := defaultResourcePlanForTest(t, components)
			consumers, err := ResolveConsumers(source, plan, components, config)
			if err != nil {
				t.Fatal(err)
			}
			if len(consumers) != 2 {
				t.Fatalf("Feature-only keys invented legacy consumers: %#v", consumers)
			}
			servicePlan, err := project.ResolveServicePlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
			if err != nil {
				t.Fatal(err)
			}
			for _, requirement := range servicePlan.Requirements {
				if len(requirement.ActiveConsumers) != 0 && !reflect.DeepEqual(requirement.ActiveConsumers, []string{"cache"}) {
					t.Fatalf("Feature-only keys polluted service consumers: %#v", requirement)
				}
			}
		})
	}
}

// TestResourceAppPrefixesIgnoreFeatureEvidence protects nested flags and known resource-word App slugs from phantom scopes.
func TestResourceAppPrefixesIgnoreFeatureEvidence(t *testing.T) {
	prefixes := resourceAppPrefixes(map[string]string{
		"MISSING_FEATURE_DRIVER":                     "hosted",
		"FOO_CACHE_FEATURE_FLAGS_DB_DRIVER":          "file",
		"FOO_CACHE_CHILD_FEATURE_FLAGS_QUEUE_DRIVER": "file",
		"BILLING_CACHE_DRIVER":                       "redis",
		"REPORTING_DB_HOST":                          "db.example",
	}, []string{"foo-cache", "foo-cache-child"}, project.Components{Feature: true}, map[string]project.Components{
		"FOO_CACHE": {Feature: true}, "FOO_CACHE_CHILD": {Feature: true},
	})
	want := []resourceAppPrefix{
		{name: "billing", prefix: "BILLING"},
		{name: "foo-cache", prefix: "FOO_CACHE"},
		{name: "foo-cache-child", prefix: "FOO_CACHE_CHILD"},
		{name: "reporting", prefix: "REPORTING"},
	}
	if !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("App prefixes = %#v, want %#v", prefixes, want)
	}
}

// TestResourceAppPrefixesPreserveFeatureWordLegacyApps keeps legacy resource evidence independent of Feature inference.
func TestResourceAppPrefixesPreserveFeatureWordLegacyApps(t *testing.T) {
	prefixes := resourceAppPrefixes(map[string]string{
		"MISSING_FEATURE_CACHE_DRIVER": "redis",
	}, nil, project.Components{}, nil)
	want := []resourceAppPrefix{{name: "missing-feature", prefix: "MISSING_FEATURE"}}
	if !reflect.DeepEqual(prefixes, want) {
		t.Fatalf("legacy App prefixes = %#v, want %#v", prefixes, want)
	}
}

// TestConsumersDisabledFeatureAppPreservesLegacyInference follows legacy managers when a known App does not enable Feature.
func TestConsumersDisabledFeatureAppPreservesLegacyInference(t *testing.T) {
	components := project.Components{CLI: true, Feature: true, Cache: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps:   map[string]project.AppConfig{"foo-cache": {Components: project.Components{CLI: true}}},
	}
	consumers, err := ResolveConsumers([]byte("FOO_CACHE_FEATURE_DRIVER=redis\nMISSING_FEATURE_CACHE_DRIVER=redis\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"foo:cache:feature": "redis", "missing-feature:cache": "redis"}
	for _, consumer := range consumers {
		if driver, ok := want[consumer.Consumer]; ok && consumer.Driver == driver {
			delete(want, consumer.Consumer)
		}
	}
	if len(want) != 0 {
		t.Fatalf("legacy consumers missing: %#v; got %#v", want, consumers)
	}
}

// TestConsumersKnownFeatureFlagsDoNotInventLegacyApps keeps legacy resource words inside enabled Feature App scopes.
func TestConsumersKnownFeatureFlagsDoNotInventLegacyApps(t *testing.T) {
	components := project.Components{CLI: true, Feature: true, Cache: true}
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps: map[string]project.AppConfig{
			"billing":   {Components: project.Components{CLI: true, Feature: true}},
			"foo-cache": {Components: project.Components{CLI: true, Feature: true}},
		},
	}
	consumers, err := ResolveConsumers([]byte("BILLING_FEATURE_CACHE_DRIVER=file\nFOO_CACHE_FEATURE_CACHE_DRIVER=file\n"), defaultResourcePlanForTest(t, components), components, config)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"feature": "memory", "cache": "memory",
		"billing:feature": "memory", "billing:feature:cache": "file",
		"foo-cache:feature": "memory", "foo-cache:feature:cache": "file",
	}
	got := map[string]string{}
	for _, consumer := range consumers {
		got[consumer.Consumer] = consumer.Driver
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Feature flags invented legacy consumers: %#v, want %#v", got, want)
	}
}

// TestConsumersReserveFeatureNamespaceWhenEnabled preserves legacy inference until any App opts into shared Feature defaults.
func TestConsumersReserveFeatureNamespaceWhenEnabled(t *testing.T) {
	for _, test := range []struct {
		name           string
		defaultFeature bool
		namedFeature   bool
		configuredApps bool
		wantLegacy     bool
	}{
		{name: "disabled-default", wantLegacy: true},
		{name: "named-feature-only", namedFeature: true},
		{name: "enabled-default", defaultFeature: true},
		{name: "configured-apps", defaultFeature: true, configuredApps: true, wantLegacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &project.Config{
				Render: project.RenderConfig{Components: project.Components{CLI: true, Cache: true, Feature: test.defaultFeature}},
				Apps:   map[string]project.AppConfig{},
			}
			if test.namedFeature {
				config.Apps["billing"] = project.AppConfig{Components: project.Components{CLI: true, Feature: true}}
			}
			if test.configuredApps {
				for _, name := range []string{"feature", "feature-child"} {
					config.Apps[name] = project.AppConfig{Components: project.Components{CLI: true, Cache: true}}
				}
			}
			components := project.ProjectComponents(config)
			driver := "redis"
			if (test.defaultFeature || test.namedFeature) && !test.configuredApps {
				// Every participating App inherits unowned root Feature settings, including named evaluators.
				driver = "file"
			}
			source := []byte("CACHE_DRIVER=memory\nFEATURE_CACHE_DRIVER=" + driver + "\nFEATURE_CACHE_ADDR=cache.example:6379\nFEATURE_CHILD_CACHE_DRIVER=" + driver + "\nFEATURE_CHILD_CACHE_ADDR=child.example:6379\n")
			consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			for _, consumer := range consumers {
				if consumer.Consumer == "feature:cache" && consumer.Resource == project.ResourceCache && consumer.Driver == driver {
					found["feature"] = true
				}
				if consumer.Consumer == "feature-child:cache" && consumer.Resource == project.ResourceCache && consumer.Driver == driver {
					found["feature-child"] = true
				}
				if !test.wantLegacy && consumer.Resource == project.ResourceCache && consumer.Consumer != "cache" {
					t.Fatalf("default Feature settings invented a legacy App: %#v", consumers)
				}
			}
			for _, name := range []string{"feature", "feature-child"} {
				if found[name] != test.wantLegacy {
					t.Fatalf("%s legacy App present = %t, want %t; consumers = %#v", name, found[name], test.wantLegacy, consumers)
				}
			}
		})
	}
}
