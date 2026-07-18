// Package devservices defines the optional local tools published through Docker Compose profiles.
package devservices

import (
	"sort"
	"strings"

	"github.com/goforj/goforj/project"
)

// Key identifies one built-in developer service independently from the infrastructure it provides.
type Key string

const (
	// KeyRedis identifies the Redis developer service.
	KeyRedis Key = "redis"
	// KeyRustFS identifies the RustFS S3-compatible developer service.
	KeyRustFS Key = "rustfs"
	// KeyOpenSearch identifies the standalone OpenSearch developer service.
	KeyOpenSearch Key = "opensearch"
	// KeyNATS identifies the NATS and JetStream developer service.
	KeyNATS Key = "nats"
	// KeyRabbitMQ identifies the RabbitMQ developer service.
	KeyRabbitMQ Key = "rabbitmq"
	// KeyRedpanda identifies the Redpanda Kafka-compatible developer service.
	KeyRedpanda Key = "redpanda"
	// KeyDynamoDB identifies the DynamoDB Local developer service.
	KeyDynamoDB Key = "dynamodb"
	// KeyElasticMQ identifies the ElasticMQ SQS-compatible developer service.
	KeyElasticMQ Key = "elasticmq"
	// KeyPubSub identifies the Google Cloud Pub/Sub emulator developer service.
	KeyPubSub Key = "pubsub"
	// KeyMemcached identifies the Memcached developer service.
	KeyMemcached Key = "memcached"
	// KeySFTPGo identifies the SFTPGo file-transfer developer service.
	KeySFTPGo Key = "sftpgo"
	// KeyAdminer identifies the Adminer database browser.
	KeyAdminer Key = "adminer"
	// KeyJaeger identifies the Jaeger tracing developer service.
	KeyJaeger Key = "jaeger"
	// KeyQdrant identifies the Qdrant vector database developer service.
	KeyQdrant Key = "qdrant"
	// KeyTemporal identifies the Temporal development server.
	KeyTemporal Key = "temporal"
	// KeyKeycloak identifies the Keycloak identity developer service.
	KeyKeycloak Key = "keycloak"
	// KeyMockServer identifies the MockServer API testing developer service.
	KeyMockServer Key = "mockserver"
	// KeyToxiproxy identifies the Toxiproxy network-failure developer service.
	KeyToxiproxy Key = "toxiproxy"
	// KeyClickHouse identifies the ClickHouse analytics developer service.
	KeyClickHouse Key = "clickhouse"
	// KeyMeilisearch identifies the Meilisearch application-search developer service.
	KeyMeilisearch Key = "meilisearch"
	// KeyMailpit identifies the Mailpit development inbox.
	KeyMailpit Key = "mailpit"
	// KeyVictoriaMetrics identifies the VictoriaMetrics developer service.
	KeyVictoriaMetrics Key = "victoriametrics"
	// KeyGrafana identifies the Grafana dashboard developer service.
	KeyGrafana Key = "grafana"
)

// Definition describes one optional Compose profile and the infrastructure capabilities it provides.
type Definition struct {
	Key        Key
	Label      string
	Profile    string
	Providers  []project.ServiceKey
	DefaultFor []project.ComponentKey
	Template   string
	Order      int
}

var catalog = []Definition{
	{
		Key:       KeyRedis,
		Label:     "Redis",
		Profile:   "redis",
		Providers: []project.ServiceKey{project.ServiceRedis},
		Template:  "containers/developer-services/redis.yml.tmpl",
		Order:     10,
	},
	{
		Key:       KeyRustFS,
		Label:     "RustFS",
		Profile:   "rustfs",
		Providers: []project.ServiceKey{project.ServiceStorageS3},
		Template:  "containers/developer-services/rustfs.yml.tmpl",
		Order:     20,
	},
	{
		Key:      KeyOpenSearch,
		Label:    "OpenSearch",
		Profile:  "opensearch",
		Template: "containers/developer-services/opensearch.yml.tmpl",
		Order:    30,
	},
	{
		Key:       KeyNATS,
		Label:     "NATS",
		Profile:   "nats",
		Providers: []project.ServiceKey{project.ServiceCacheNATS, project.ServiceQueueNATS, project.ServiceEventsNATS},
		Template:  "containers/developer-services/nats.yml.tmpl",
		Order:     40,
	},
	{
		Key:       KeyRabbitMQ,
		Label:     "RabbitMQ",
		Profile:   "rabbitmq",
		Providers: []project.ServiceKey{project.ServiceQueueRabbitMQ},
		Template:  "containers/developer-services/rabbitmq.yml.tmpl",
		Order:     50,
	},
	{
		Key:       KeyRedpanda,
		Label:     "Redpanda",
		Profile:   "redpanda",
		Providers: []project.ServiceKey{project.ServiceEventsKafka},
		Template:  "containers/developer-services/redpanda.yml.tmpl",
		Order:     60,
	},
	{
		Key:       KeyDynamoDB,
		Label:     "DynamoDB Local",
		Profile:   "dynamodb",
		Providers: []project.ServiceKey{project.ServiceCacheDynamoDB},
		Template:  "containers/developer-services/dynamodb.yml.tmpl",
		Order:     70,
	},
	{
		Key:       KeyElasticMQ,
		Label:     "ElasticMQ",
		Profile:   "elasticmq",
		Providers: []project.ServiceKey{project.ServiceQueueSQS},
		Template:  "containers/developer-services/elasticmq.yml.tmpl",
		Order:     80,
	},
	{
		Key:       KeyPubSub,
		Label:     "Google Pub/Sub emulator",
		Profile:   "pubsub",
		Providers: []project.ServiceKey{project.ServiceEventsGCPPubSub},
		Template:  "containers/developer-services/pubsub.yml.tmpl",
		Order:     90,
	},
	{
		Key:       KeyMemcached,
		Label:     "Memcached",
		Profile:   "memcached",
		Providers: []project.ServiceKey{project.ServiceCacheMemcached},
		Template:  "containers/developer-services/memcached.yml.tmpl",
		Order:     100,
	},
	{
		Key:      KeySFTPGo,
		Label:    "SFTPGo",
		Profile:  "sftpgo",
		Template: "containers/developer-services/sftpgo.yml.tmpl",
		Order:    110,
	},
	{Key: KeyAdminer, Label: "Adminer", Profile: "adminer", Template: "containers/developer-services/adminer.yml.tmpl", Order: 120},
	{Key: KeyJaeger, Label: "Jaeger", Profile: "jaeger", Template: "containers/developer-services/jaeger.yml.tmpl", Order: 130},
	{Key: KeyQdrant, Label: "Qdrant", Profile: "qdrant", Template: "containers/developer-services/qdrant.yml.tmpl", Order: 140},
	{Key: KeyTemporal, Label: "Temporal", Profile: "temporal", Template: "containers/developer-services/temporal.yml.tmpl", Order: 150},
	{Key: KeyKeycloak, Label: "Keycloak", Profile: "keycloak", Template: "containers/developer-services/keycloak.yml.tmpl", Order: 160},
	{Key: KeyMockServer, Label: "MockServer", Profile: "mockserver", Template: "containers/developer-services/mockserver.yml.tmpl", Order: 170},
	{Key: KeyToxiproxy, Label: "Toxiproxy", Profile: "toxiproxy", Template: "containers/developer-services/toxiproxy.yml.tmpl", Order: 180},
	{Key: KeyClickHouse, Label: "ClickHouse", Profile: "clickhouse", Template: "containers/developer-services/clickhouse.yml.tmpl", Order: 190},
	{Key: KeyMeilisearch, Label: "Meilisearch", Profile: "meilisearch", Template: "containers/developer-services/meilisearch.yml.tmpl", Order: 200},
	{
		Key:        KeyMailpit,
		Label:      "Mailpit",
		Profile:    "mailpit",
		Providers:  []project.ServiceKey{project.ServiceMailSMTP},
		DefaultFor: []project.ComponentKey{project.ComponentMail},
		Template:   "containers/developer-services/mailpit.yml.tmpl",
		Order:      210,
	},
	{
		Key:        KeyVictoriaMetrics,
		Label:      "VictoriaMetrics",
		Profile:    "victoriametrics",
		DefaultFor: []project.ComponentKey{project.ComponentObservability},
		Template:   "containers/developer-services/victoriametrics.yml.tmpl",
		Order:      220,
	},
	{
		Key:        KeyGrafana,
		Label:      "Grafana",
		Profile:    "grafana",
		DefaultFor: []project.ComponentKey{project.ComponentGrafana},
		Template:   "containers/developer-services/grafana.yml.tmpl",
		Order:      230,
	},
}

// Catalog returns a defensive copy in stable presentation and reconciliation order.
func Catalog() []Definition {
	definitions := make([]Definition, len(catalog))
	for index, definition := range catalog {
		definition.Providers = append([]project.ServiceKey(nil), definition.Providers...)
		definition.DefaultFor = append([]project.ComponentKey(nil), definition.DefaultFor...)
		definitions[index] = definition
	}
	sort.SliceStable(definitions, func(left, right int) bool {
		return definitions[left].Order < definitions[right].Order
	})
	return definitions
}

// DefinitionByKey returns one developer-service definition by its exact stable key.
func DefinitionByKey(key Key) (Definition, bool) {
	for _, definition := range Catalog() {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

// DefinitionByProfile returns one developer-service definition by its exact Compose profile token.
func DefinitionByProfile(profile string) (Definition, bool) {
	profile = strings.TrimSpace(profile)
	for _, definition := range Catalog() {
		if definition.Profile == profile {
			return definition, true
		}
	}
	return Definition{}, false
}

// Enabled returns known definitions selected by exact profile tokens in catalog order.
func Enabled(value string) []Definition {
	tokens := make(map[string]struct{})
	for _, token := range strings.Split(value, ",") {
		token = strings.TrimSpace(token)
		if token != "" {
			tokens[token] = struct{}{}
		}
	}

	enabled := make([]Definition, 0, len(tokens))
	for _, definition := range Catalog() {
		if _, ok := tokens[definition.Profile]; ok {
			enabled = append(enabled, definition)
		}
	}
	return enabled
}
