package resourceenv

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestConsumersSearchDriverFallback mirrors Search's App-root and named driver precedence without changing legacy resources.
func TestConsumersSearchDriverFallback(t *testing.T) {
	components := project.Components{Search: true}
	for _, test := range []struct{ name, extra, wantRoot, wantNamed string }{
		{"App inherits root", "", "meilisearch", "meilisearch"},
		{"empty App root", "BILLING_SEARCH_DRIVER=\n", "bleve", "bleve"},
		{"whitespace App root", "BILLING_SEARCH_DRIVER='   '\n", "bleve", "bleve"},
		{"global named overrides App root", "BILLING_SEARCH_DRIVER=meilisearch\nSEARCH_CATALOG_DRIVER=bleve\n", "meilisearch", "bleve"},
		{"empty global named inherits global root", "BILLING_SEARCH_DRIVER=bleve\nSEARCH_CATALOG_DRIVER=\n", "bleve", "meilisearch"},
		{"empty App named inherits App root", "BILLING_SEARCH_DRIVER=meilisearch\nSEARCH_CATALOG_DRIVER=bleve\nBILLING_SEARCH_CATALOG_DRIVER=\n", "meilisearch", "meilisearch"},
		{"whitespace App named inherits App root", "BILLING_SEARCH_DRIVER=meilisearch\nSEARCH_CATALOG_DRIVER=bleve\nBILLING_SEARCH_CATALOG_DRIVER='   '\n", "meilisearch", "meilisearch"},
		{"explicit App named", "BILLING_SEARCH_DRIVER=meilisearch\nBILLING_SEARCH_CATALOG_DRIVER=bleve\n", "meilisearch", "bleve"},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("SEARCH_DRIVER=meilisearch\nBILLING_SEARCH_CATALOG_PATH=catalog\n" + test.extra)
			consumers, err := ResolveConsumers(source, defaultResourcePlanForTest(t, components), components, nil, "billing")
			if err != nil {
				t.Fatal(err)
			}
			got := map[string]string{}
			for _, consumer := range consumers {
				got[consumer.Consumer] = consumer.Driver
			}
			if got["billing:search"] != test.wantRoot || got["billing:search:catalog"] != test.wantNamed {
				t.Fatalf("consumers=%#v root=%s named=%s", got, test.wantRoot, test.wantNamed)
			}
		})
	}
}

// TestConsumersSearchCrossResourceOwnership excludes other App families from default Search indexes.
func TestConsumersSearchCrossResourceOwnership(t *testing.T) {
	components := project.Components{CLI: true, Search: true, Notification: true, Feature: true, Cache: true}
	consumers, err := ResolveConsumers([]byte("SEARCH_SEARCH_DRIVER=bleve\nSEARCH_CACHE_DRIVER=redis\nSEARCH_NOTIFICATION_DRIVER=log\nSEARCH_FEATURE_DRIVER=file\nSEARCH_SEARCH_NOTIFICATION_DRIVER=log\n"), defaultResourcePlanForTest(t, components), components, nil, "search", "search-search")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, consumer := range consumers {
		got[consumer.Consumer] = consumer.Driver
		if consumer.Resource == project.ResourceSearch && consumer.Consumer != "search" && consumer.Consumer != "search:search" && consumer.Consumer != "search-search:search" {
			t.Fatalf("other resource became a Search index: %#v", consumer)
		}
	}
	for _, name := range []string{"search:cache", "search:notification", "search:feature", "search-search:notification"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("missing App consumer %s: %#v", name, got)
		}
	}
	for _, name := range []string{"search:cache:notification", "search:notification:driver", "search:search:notification"} {
		if _, ok := got[name]; ok {
			t.Fatalf("cross-family Search index %s: %#v", name, got)
		}
	}
	for _, name := range []string{"search:notification:search", "search:cache:search", "search:feature:search"} {
		if _, ok := got[name]; ok {
			t.Fatalf("cross-family index %s: %#v", name, got)
		}
	}
}

// TestConsumersSearchValueFallback retains root and named configuration through empty App settings.
func TestConsumersSearchValueFallback(t *testing.T) {
	definition, ok := project.ResourceDefinitionByKey(project.ResourceSearch)
	if !ok {
		t.Fatal("missing Search definition")
	}
	for _, test := range []struct{ name, extra, want string }{
		{"base root", "", "base-root"},
		{"App root", "BILLING_SEARCH_URL=app-root\n", "app-root"},
		{"empty App root", "BILLING_SEARCH_URL=\n", "base-root"},
		{"base named overrides App root", "BILLING_SEARCH_URL=app-root\nSEARCH_CATALOG_URL=base-named\n", "base-named"},
		{"empty App named inherits base named", "BILLING_SEARCH_URL=app-root\nSEARCH_CATALOG_URL=base-named\nBILLING_SEARCH_CATALOG_URL=\n", "base-named"},
		{"App named", "SEARCH_CATALOG_URL=base-named\nBILLING_SEARCH_CATALOG_URL=app-named\n", "app-named"},
	} {
		t.Run(test.name, func(t *testing.T) {
			components := project.Components{Search: true}
			resolver, err := newResourceConsumerResolver([]byte("SEARCH_URL=base-root\n"+test.extra), defaultResourcePlanForTest(t, components), components)
			if err != nil {
				t.Fatal(err)
			}
			resolver.appPrefixes = []string{"BILLING"}
			scope := resourceConsumerScope{app: resourceAppPrefix{prefix: "BILLING"}}
			connection := resourceConnection{definition: definition, name: "catalog"}
			if value := resolver.scopedValue(scope, connection, "URL"); value != test.want {
				t.Fatalf("URL=%q want=%q", value, test.want)
			}
		})
	}
	// A resource-word App owns its root key, so another App's index with the same name inherits its own root.
	resolver, err := newResourceConsumerResolver([]byte("SEARCH_URL=base-root\nSEARCH_SEARCH_URL=other-app-root\n"), defaultResourcePlanForTest(t, project.Components{Search: true}), project.Components{Search: true})
	if err != nil {
		t.Fatal(err)
	}
	resolver.appPrefixes = []string{"SEARCH", "BILLING"}
	if value := resolver.scopedValue(resourceConsumerScope{app: resourceAppPrefix{prefix: "BILLING"}}, resourceConnection{definition: definition, name: "search"}, "URL"); value != "base-root" {
		t.Fatalf("other App root became named override: %q", value)
	}
}
