package devservices

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestCatalogPublishesStableProfileProviders locks profile identity separately from infrastructure identity.
func TestCatalogPublishesStableProfileProviders(t *testing.T) {
	want := []struct {
		key        Key
		profile    string
		providers  []project.ServiceKey
		defaultFor []project.ComponentKey
	}{
		{key: KeyRedis, profile: "redis", providers: []project.ServiceKey{project.ServiceRedis}},
		{key: KeyRustFS, profile: "rustfs", providers: []project.ServiceKey{project.ServiceStorageS3}},
		{key: KeyOpenSearch, profile: "opensearch"},
		{key: KeyNATS, profile: "nats", providers: []project.ServiceKey{project.ServiceCacheNATS, project.ServiceQueueNATS, project.ServiceEventsNATS}},
		{key: KeyRabbitMQ, profile: "rabbitmq", providers: []project.ServiceKey{project.ServiceQueueRabbitMQ}},
		{key: KeyRedpanda, profile: "redpanda", providers: []project.ServiceKey{project.ServiceEventsKafka}},
		{key: KeyDynamoDB, profile: "dynamodb", providers: []project.ServiceKey{project.ServiceCacheDynamoDB}},
		{key: KeyElasticMQ, profile: "elasticmq", providers: []project.ServiceKey{project.ServiceQueueSQS}},
		{key: KeyPubSub, profile: "pubsub", providers: []project.ServiceKey{project.ServiceEventsGCPPubSub}},
		{key: KeyMemcached, profile: "memcached", providers: []project.ServiceKey{project.ServiceCacheMemcached}},
		{key: KeySFTPGo, profile: "sftpgo"},
		{key: KeyAdminer, profile: "adminer"},
		{key: KeyJaeger, profile: "jaeger"},
		{key: KeyQdrant, profile: "qdrant"},
		{key: KeyTemporal, profile: "temporal"},
		{key: KeyKeycloak, profile: "keycloak"},
		{key: KeyMockServer, profile: "mockserver"},
		{key: KeyToxiproxy, profile: "toxiproxy"},
		{key: KeyClickHouse, profile: "clickhouse"},
		{key: KeyMeilisearch, profile: "meilisearch"},
		{key: KeyMailpit, profile: "mailpit", providers: []project.ServiceKey{project.ServiceMailSMTP}, defaultFor: []project.ComponentKey{project.ComponentMail}},
		{key: KeyVictoriaMetrics, profile: "victoriametrics", defaultFor: []project.ComponentKey{project.ComponentObservability}},
		{key: KeyGrafana, profile: "grafana", defaultFor: []project.ComponentKey{project.ComponentGrafana}},
	}
	got := Catalog()
	if len(got) != len(want) {
		t.Fatalf("Catalog() length = %d, want %d: %#v", len(got), len(want), got)
	}
	for index, expected := range want {
		definition := got[index]
		if definition.Key != expected.key || definition.Profile != expected.profile ||
			!reflect.DeepEqual(definition.Providers, expected.providers) || !reflect.DeepEqual(definition.DefaultFor, expected.defaultFor) {
			t.Fatalf("Catalog()[%d] = %#v, want key=%q profile=%q providers=%#v defaultFor=%#v", index, definition, expected.key, expected.profile, expected.providers, expected.defaultFor)
		}
		wantTemplate := "containers/developer-services/" + string(expected.key) + ".yml.tmpl"
		if definition.Label == "" || definition.Template != wantTemplate || definition.Order != (index+1)*10 {
			t.Fatalf("Catalog()[%d] metadata = %#v, want template %q and order %d", index, definition, wantTemplate, (index+1)*10)
		}
	}
	if got[1].Profile == string(got[1].Providers[0]) {
		t.Fatal("RustFS profile identity collapsed into its S3 infrastructure identity")
	}
}

// TestCatalogReturnsDefensiveCopies prevents callers from mutating the process-wide service inventory.
func TestCatalogReturnsDefensiveCopies(t *testing.T) {
	definitions := Catalog()
	definitions[0].Profile = "changed"
	definitions[0].Providers[0] = project.ServiceStorageS3
	definitions[len(definitions)-1].DefaultFor[0] = project.ComponentMail

	second := Catalog()
	if second[0].Profile != "redis" || !reflect.DeepEqual(second[0].Providers, []project.ServiceKey{project.ServiceRedis}) ||
		!reflect.DeepEqual(second[len(second)-1].DefaultFor, []project.ComponentKey{project.ComponentGrafana}) {
		t.Fatalf("Catalog() retained caller mutation: %#v", second)
	}
}

// TestDefinitionLookupsRequireExactIdentity prevents neighboring profile names from enabling built-ins.
func TestDefinitionLookupsRequireExactIdentity(t *testing.T) {
	if definition, ok := DefinitionByKey(KeyRustFS); !ok || definition.Profile != "rustfs" {
		t.Fatalf("DefinitionByKey(KeyRustFS) = %#v, %t", definition, ok)
	}
	if definition, ok := DefinitionByProfile(" rustfs "); !ok || definition.Key != KeyRustFS {
		t.Fatalf("DefinitionByProfile(rustfs) = %#v, %t", definition, ok)
	}
	if definition, ok := DefinitionByProfile("rustfs-debug"); ok {
		t.Fatalf("DefinitionByProfile(rustfs-debug) = %#v, want no match", definition)
	}
}

// TestEnabledReturnsExactProfilesInCatalogOrder keeps owner token order from changing generated presentation.
func TestEnabledReturnsExactProfilesInCatalogOrder(t *testing.T) {
	got := Enabled("opensearch,rustfs-debug,nats,redis,rustfs,redis")
	want := []Key{KeyRedis, KeyRustFS, KeyOpenSearch, KeyNATS}
	keys := make([]Key, 0, len(got))
	for _, definition := range got {
		keys = append(keys, definition.Key)
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("Enabled() keys = %#v, want %#v", keys, want)
	}
}
