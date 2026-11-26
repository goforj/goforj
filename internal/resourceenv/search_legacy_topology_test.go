package resourceenv

import (
	"github.com/goforj/goforj/project"
	"strings"
	"testing"
)

// TestConsumersDisabledSearchPreservesLegacyAppMarkers keeps inactive Search words inside legacy App names and endpoints.
func TestConsumersDisabledSearchPreservesLegacyAppMarkers(t *testing.T) {
	for _, family := range []struct {
		resource                                      project.ResourceKey
		components                                    project.Components
		prefix, rootDriver, driver, endpoint, address string
		service                                       project.ServiceKey
	}{
		{project.ResourceDatabase, project.Components{CLI: true, DatabaseSQLite: true}, "DB", "sqlite", "mysql", "HOST", "legacy.example:3306", project.ServiceMySQL},
		{project.ResourceCache, project.Components{CLI: true, Cache: true}, "CACHE", "memory", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceQueue, project.Components{CLI: true, Jobs: true}, "QUEUE", "workerpool", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceEvents, project.Components{CLI: true, Events: true}, "EVENTS", "inproc", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceStorage, project.Components{CLI: true, Storage: true}, "STORAGE", "local", "redis", "ADDR", "legacy.example:6379", project.ServiceRedis},
		{project.ResourceMail, project.Components{CLI: true, Mail: true}, "MAIL", "log", "smtp", "SMTP_HOST", "legacy.example:1025", project.ServiceMailSMTP},
	} {
		for _, app := range []string{"search", "search-child", "billing-search", "missing-search", "missing-search-child"} {
			for _, configured := range []bool{false, true} {
				t.Run(string(family.resource)+"/"+app+"/"+map[bool]string{false: "configless", true: "config"}[configured], func(t *testing.T) {
					var config *project.Config
					if configured {
						config = &project.Config{Render: project.RenderConfig{Components: family.components}}
					}
					prefix := project.AppEnvironmentPrefix(app) + "_" + family.prefix
					endpoint := family.address
					if family.endpoint != "ADDR" {
						endpoint = "legacy.example"
					}
					source := []byte(family.prefix + "_DRIVER=" + family.rootDriver + "\n" + prefix + "_DRIVER=" + family.driver + "\n" + prefix + "_" + family.endpoint + "=" + endpoint + "\n")
					consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, family.components), family.components, config)
					if err != nil {
						t.Fatal(err)
					}
					found := false
					for _, consumer := range consumers {
						if strings.HasPrefix(consumer.Consumer, "billing:") || strings.HasPrefix(consumer.Consumer, "missing:") {
							t.Fatalf("inactive Search marker shortened the legacy App: %#v", consumers)
						}
						if consumer.Consumer == app+":"+string(family.resource) {
							found = true
							wantAffinity := opaqueEndpointAffinity(family.service, []string{"addr=" + family.address})
							if consumer.Driver != family.driver || consumer.EndpointAffinity != wantAffinity || consumer.LocalService {
								t.Errorf("legacy consumer = %#v, want driver %s and external endpoint %s", consumer, family.driver, family.address)
							}
						}
					}
					if !found {
						t.Fatalf("missing legacy App %s: %#v", app, consumers)
					}
				})
			}
		}
	}
}

// TestConsumersDisabledSearchIgnoresNativeMarkers prevents inactive SDK settings from inventing legacy consumers.
func TestConsumersDisabledSearchIgnoresNativeMarkers(t *testing.T) {
	components := project.Components{CLI: true, Cache: true}
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "configless", true: "config"}[configured], func(t *testing.T) {
			var config *project.Config
			if configured {
				config = &project.Config{Render: project.RenderConfig{Components: components}}
			}
			consumers, err := ResolveConsumers([]byte("CACHE_DRIVER=memory\nMISSING_SEARCH_DRIVER=bleve\n"), defaultResourcePlanForTest(t, components), components, config)
			if err != nil {
				t.Fatal(err)
			}
			if len(consumers) != 1 || consumers[0].Consumer != "cache" || consumers[0].Driver != "memory" {
				t.Fatalf("inactive Search setting invented consumers: %#v", consumers)
			}
		})
	}
}

// TestConsumersKnownSearchInfixKeepsDisabledAppScope prevents resource markers inside known Apps from inventing shorter active Apps.
func TestConsumersKnownSearchInfixKeepsDisabledAppScope(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, app := range []string{"billing-search", "billing-search-child"} {
			t.Run(app+"/"+map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
				components := project.Components{CLI: true, Cache: true, Search: enabled}
				config := &project.Config{Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{app: {Components: project.Components{CLI: true}}}}
				source := []byte(project.AppEnvironmentPrefix(app) + "_CACHE_DRIVER=redis\n")
				consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config)
				if err != nil {
					t.Fatal(err)
				}
				for _, consumer := range consumers {
					if strings.HasPrefix(consumer.Consumer, "billing:") || strings.HasPrefix(consumer.Consumer, app+":") {
						t.Fatalf("disabled known App invented consumers: %#v", consumers)
					}
				}
			})
		}
	}
}

// TestConsumersReserveSearchNamespaceWhenEnabled preserves legacy App inference until Search defaults become active.
func TestConsumersReserveSearchNamespaceWhenEnabled(t *testing.T) {
	for _, test := range []struct {
		name          string
		defaultSearch bool
		namedSearch   bool
		inventory     string
		wantLegacy    bool
	}{
		{name: "disabled-without-config", wantLegacy: true},
		{name: "disabled-with-config", inventory: "empty", wantLegacy: true},
		{name: "enabled-default", defaultSearch: true},
		{name: "enabled-named-app", namedSearch: true},
		{name: "inherited-default", defaultSearch: true, inventory: "inherited"},
		{name: "configured-resource-names", defaultSearch: true, inventory: "configured", wantLegacy: true},
		{name: "conventional-resource-names", defaultSearch: true, inventory: "conventional", wantLegacy: true},
		{name: "dev-resource-names", defaultSearch: true, inventory: "dev", wantLegacy: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			components := project.Components{CLI: true, Cache: true, Search: test.defaultSearch}
			var config *project.Config
			var knownApps []string
			if test.inventory != "" || test.namedSearch {
				config = &project.Config{Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{}}
			}
			if test.namedSearch {
				config.Apps["billing"] = project.AppConfig{Components: project.Components{CLI: true, Search: true}}
			}
			switch test.inventory {
			case "inherited":
				knownApps = []string{"billing"}
			case "configured":
				for _, name := range []string{"search", "search-child"} {
					config.Apps[name] = project.AppConfig{Components: project.Components{CLI: true, Cache: true}}
				}
			case "conventional":
				knownApps = []string{"search", "search-child"}
			case "dev":
				config.Dev.Apps = map[string]project.DevApp{"search": {}, "search-child": {}}
			}
			if config != nil {
				components = project.ProjectComponents(config)
			}
			driver := "redis"
			if !test.wantLegacy {
				// Active shared Search settings must select a Search driver.
				driver = "meilisearch"
			}
			source := []byte("CACHE_DRIVER=memory\nSEARCH_CACHE_DRIVER=" + driver + "\nSEARCH_CACHE_URL=https://example.com/cache\nSEARCH_CHILD_CACHE_DRIVER=" + driver + "\nSEARCH_CHILD_CACHE_URL=https://example.com/child\nUNKNOWN_SEARCH_DRIVER=log\n")
			consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, config, knownApps...)
			if err != nil {
				t.Fatal(err)
			}
			found := map[string]bool{}
			shared := map[string]bool{}
			inherited := false
			for _, consumer := range consumers {
				if consumer.Resource == project.ResourceCache {
					for _, name := range []string{"search", "search-child"} {
						if consumer.Consumer == name+":cache" && consumer.Driver == "redis" {
							found[name] = true
						}
					}
					if !test.wantLegacy && consumer.Consumer != "cache" && consumer.Consumer != "billing:cache" && consumer.Consumer != "unknown:cache" {
						t.Fatalf("Search defaults invented a legacy App: %#v", consumers)
					}
				}
				if consumer.Resource == project.ResourceSearch && consumer.Driver == "meilisearch" {
					shared[consumer.Consumer] = true
				}
				if consumer.Consumer == "billing:search:cache" && consumer.Driver == "meilisearch" {
					inherited = true
				}
			}
			for _, name := range []string{"search", "search-child"} {
				if found[name] != test.wantLegacy {
					t.Errorf("%s legacy App present = %t, want %t; consumers = %#v", name, found[name], test.wantLegacy, consumers)
				}
			}
			if test.defaultSearch && !test.wantLegacy && (!shared["search:cache"] || !shared["search:child_cache"]) {
				t.Fatalf("active root Search namespace lost shared senders: %#v", consumers)
			}
			if (test.inventory == "inherited" || test.namedSearch) && !inherited {
				t.Fatalf("known App did not inherit shared Search settings: %#v", consumers)
			}
		})
	}
}
