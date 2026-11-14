package resourceenv

import (
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
