package forj

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestDeveloperServiceMessagingContracts locks the rendered broker and emulator recipes to their verified runtime contracts.
func TestDeveloperServiceMessagingContracts(t *testing.T) {
	components := project.Components{Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	_, compose := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})

	type renderedHealthcheck struct {
		Test        []string `yaml:"test"`
		Interval    string   `yaml:"interval"`
		Timeout     string   `yaml:"timeout"`
		Retries     int      `yaml:"retries"`
		StartPeriod string   `yaml:"start_period"`
	}
	type renderedDependency struct {
		Condition string `yaml:"condition"`
	}
	type renderedService struct {
		Profiles     []string             `yaml:"profiles"`
		RenderedTest *bool                `yaml:"x-forj-test"`
		Restart      string               `yaml:"restart"`
		Image        string               `yaml:"image"`
		Hostname     string               `yaml:"hostname"`
		Entrypoint   string               `yaml:"entrypoint"`
		Command      yaml.Node            `yaml:"command"`
		Environment  yaml.Node            `yaml:"environment"`
		Ports        []string             `yaml:"ports"`
		Volumes      []string             `yaml:"volumes"`
		Healthcheck  *renderedHealthcheck `yaml:"healthcheck"`
		DependsOn    yaml.Node            `yaml:"depends_on"`
		Networks     []string             `yaml:"networks"`
	}
	var document struct {
		Volumes map[string]struct {
			Driver string `yaml:"driver"`
		} `yaml:"volumes"`
		Services map[string]renderedService `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("decode rendered messaging-service Compose: %v\n%s", err, compose)
	}

	mustService := func(name string) renderedService {
		t.Helper()
		service, ok := document.Services[name]
		if !ok {
			t.Fatalf("rendered Compose omitted %s:\n%s", name, compose)
		}
		return service
	}
	mustSequence := func(name, field string, node yaml.Node) []string {
		t.Helper()
		var values []string
		if err := node.Decode(&values); err != nil {
			t.Fatalf("decode %s %s: %v", name, field, err)
		}
		return values
	}
	assertSequence := func(name, field string, node yaml.Node, want []string) {
		t.Helper()
		if got := mustSequence(name, field, node); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s %s = %#v, want %#v", name, field, got, want)
		}
	}
	assertHealthcheck := func(name string, want *renderedHealthcheck) {
		t.Helper()
		if got := mustService(name).Healthcheck; !reflect.DeepEqual(got, want) {
			t.Fatalf("%s healthcheck = %#v, want %#v", name, got, want)
		}
	}
	mustDependencies := func(name string, node yaml.Node) map[string]renderedDependency {
		t.Helper()
		var dependencies map[string]renderedDependency
		if err := node.Decode(&dependencies); err != nil {
			t.Fatalf("decode %s depends_on: %v", name, err)
		}
		return dependencies
	}

	type serviceContract struct {
		name       string
		profile    string
		image      string
		excluded   bool
		persistent bool
	}
	contracts := []serviceContract{
		{name: "nats", profile: "nats", image: "nats:2.14.3-alpine", persistent: true},
		{name: "rabbitmq", profile: "rabbitmq", image: "rabbitmq:4.3.2-management", persistent: true},
		{name: "redpanda", profile: "redpanda", image: "docker.redpanda.com/redpandadata/redpanda:v26.1.13", excluded: true, persistent: true},
		{name: "redpanda-console", profile: "redpanda", image: "docker.redpanda.com/redpandadata/console:v3.8.0", excluded: true},
		{name: "elasticmq", profile: "elasticmq", image: "softwaremill/elasticmq-native:1.7.1", persistent: true},
		{name: "elasticmq-ui", profile: "elasticmq", image: "softwaremill/elasticmq-ui:1.7.1", excluded: true},
		{name: "gcppubsub", profile: "pubsub", image: "gcr.io/google.com/cloudsdktool/google-cloud-cli:573.0.0-emulators", excluded: true},
	}
	for _, contract := range contracts {
		service := mustService(contract.name)
		if !reflect.DeepEqual(service.Profiles, []string{contract.profile}) {
			t.Fatalf("%s profiles = %#v, want exact profile %q", contract.name, service.Profiles, contract.profile)
		}
		if service.Restart != "unless-stopped" {
			t.Fatalf("%s restart = %q, want unless-stopped", contract.name, service.Restart)
		}
		if service.Image != contract.image {
			t.Fatalf("%s image = %q, want %q", contract.name, service.Image, contract.image)
		}
		if !reflect.DeepEqual(service.Networks, []string{"backend"}) {
			t.Fatalf("%s networks = %#v, want backend", contract.name, service.Networks)
		}
		if contract.excluded {
			if service.RenderedTest == nil || *service.RenderedTest {
				t.Fatalf("%s x-forj-test = %#v, want false", contract.name, service.RenderedTest)
			}
		} else if service.RenderedTest != nil {
			t.Fatalf("%s x-forj-test = %#v, want the provider included in rendered tests", contract.name, service.RenderedTest)
		}
		volume, persistent := document.Volumes[contract.name]
		if persistent != contract.persistent {
			t.Fatalf("%s named-volume presence = %t, want %t", contract.name, persistent, contract.persistent)
		}
		if persistent && volume.Driver != "local" {
			t.Fatalf("%s named-volume driver = %q, want local", contract.name, volume.Driver)
		}
	}

	nats := mustService("nats")
	assertSequence("nats", "command", nats.Command, []string{
		"--jetstream",
		"--store_dir=/data",
		"--http_port=8222",
		"--user=${NATS_USERNAME:-goforj}",
		"--pass=${NATS_PASSWORD:-goforj}",
	})
	assertSequence("nats", "environment", nats.Environment, []string{"TZ=${TZ:-UTC}"})
	if want := []string{
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${NATS_CLIENT_PORT:-4222}:4222",
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${NATS_MONITORING_PORT:-8222}:8222",
	}; !reflect.DeepEqual(nats.Ports, want) {
		t.Fatalf("nats ports = %#v, want %#v", nats.Ports, want)
	}
	if !reflect.DeepEqual(nats.Volumes, []string{"nats:/data"}) {
		t.Fatalf("nats volumes = %#v, want durable JetStream data", nats.Volumes)
	}
	assertHealthcheck("nats", &renderedHealthcheck{
		Test:        []string{"CMD-SHELL", "wget -q -O /dev/null 'http://127.0.0.1:8222/healthz?js-enabled-only=true'"},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     10,
		StartPeriod: "5s",
	})

	rabbitMQ := mustService("rabbitmq")
	if rabbitMQ.Hostname != "rabbitmq" {
		t.Fatalf("rabbitmq hostname = %q, want stable persisted node identity", rabbitMQ.Hostname)
	}
	assertSequence("rabbitmq", "environment", rabbitMQ.Environment, []string{
		"TZ=${TZ:-UTC}",
		"RABBITMQ_DEFAULT_USER=${RABBITMQ_USERNAME:-goforj}",
		"RABBITMQ_DEFAULT_PASS=${RABBITMQ_PASSWORD:-goforj}",
	})
	if want := []string{
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${RABBITMQ_AMQP_PORT:-5672}:5672",
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${RABBITMQ_MANAGEMENT_PORT:-15672}:15672",
	}; !reflect.DeepEqual(rabbitMQ.Ports, want) {
		t.Fatalf("rabbitmq ports = %#v, want %#v", rabbitMQ.Ports, want)
	}
	if !reflect.DeepEqual(rabbitMQ.Volumes, []string{"rabbitmq:/var/lib/rabbitmq"}) {
		t.Fatalf("rabbitmq volumes = %#v, want durable broker data", rabbitMQ.Volumes)
	}
	assertHealthcheck("rabbitmq", &renderedHealthcheck{
		Test:        []string{"CMD", "rabbitmq-diagnostics", "-q", "ping"},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     10,
		StartPeriod: "30s",
	})

	redpanda := mustService("redpanda")
	assertSequence("redpanda", "command", redpanda.Command, []string{
		"redpanda",
		"start",
		"--kafka-addr internal://0.0.0.0:9092,external://0.0.0.0:19092",
		"--advertise-kafka-addr internal://redpanda:9092,external://${REDPANDA_ADVERTISED_HOST:-localhost}:${REDPANDA_KAFKA_PORT:-19092}",
		"--pandaproxy-addr internal://0.0.0.0:8082,external://0.0.0.0:18082",
		"--advertise-pandaproxy-addr internal://redpanda:8082,external://${REDPANDA_ADVERTISED_HOST:-localhost}:${REDPANDA_PANDAPROXY_PORT:-18082}",
		"--schema-registry-addr internal://0.0.0.0:8081,external://0.0.0.0:18081",
		"--rpc-addr redpanda:33145",
		"--advertise-rpc-addr redpanda:33145",
		"--mode dev-container",
		"--smp 1",
		"--default-log-level=info",
	})
	assertSequence("redpanda", "environment", redpanda.Environment, []string{"TZ=${TZ:-UTC}"})
	if want := []string{
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${REDPANDA_SCHEMA_REGISTRY_PORT:-18081}:18081",
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${REDPANDA_PANDAPROXY_PORT:-18082}:18082",
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${REDPANDA_KAFKA_PORT:-19092}:19092",
		"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${REDPANDA_ADMIN_PORT:-19644}:9644",
	}; !reflect.DeepEqual(redpanda.Ports, want) {
		t.Fatalf("redpanda ports = %#v, want %#v", redpanda.Ports, want)
	}
	if !reflect.DeepEqual(redpanda.Volumes, []string{"redpanda:/var/lib/redpanda/data"}) {
		t.Fatalf("redpanda volumes = %#v, want durable broker data", redpanda.Volumes)
	}
	assertHealthcheck("redpanda", &renderedHealthcheck{
		Test:        []string{"CMD", "rpk", "cluster", "info"},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     10,
		StartPeriod: "30s",
	})

	console := mustService("redpanda-console")
	if console.Entrypoint != "/bin/sh" {
		t.Fatalf("redpanda-console entrypoint = %q, want /bin/sh", console.Entrypoint)
	}
	if console.Command.Kind != yaml.ScalarNode || console.Command.Value != `-c 'echo "$$CONSOLE_CONFIG_FILE" > /tmp/config.yml; /app/console'` {
		t.Fatalf("redpanda-console command = kind %d value %q, want generated config bootstrap", console.Command.Kind, console.Command.Value)
	}
	var consoleEnvironment map[string]string
	if err := console.Environment.Decode(&consoleEnvironment); err != nil {
		t.Fatalf("decode redpanda-console environment: %v", err)
	}
	wantConsoleEnvironment := map[string]string{
		"TZ":              "${TZ:-UTC}",
		"CONFIG_FILEPATH": "/tmp/config.yml",
		"CONSOLE_CONFIG_FILE": "kafka:\n  brokers: [\"redpanda:9092\"]\nschemaRegistry:\n  enabled: true\n" +
			"  urls: [\"http://redpanda:8081\"]\nredpanda:\n  adminApi:\n    enabled: true\n" +
			"    urls: [\"http://redpanda:9644\"]\n",
	}
	if !reflect.DeepEqual(consoleEnvironment, wantConsoleEnvironment) {
		t.Fatalf("redpanda-console environment = %#v, want %#v", consoleEnvironment, wantConsoleEnvironment)
	}
	if got := mustDependencies("redpanda-console", console.DependsOn); !reflect.DeepEqual(got, map[string]renderedDependency{"redpanda": {Condition: "service_healthy"}}) {
		t.Fatalf("redpanda-console depends_on = %#v, want healthy broker", got)
	}
	if !reflect.DeepEqual(console.Ports, []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${REDPANDA_CONSOLE_PORT:-18080}:8080"}) {
		t.Fatalf("redpanda-console ports = %#v, want loopback console mapping", console.Ports)
	}

	elasticMQ := mustService("elasticmq")
	assertSequence("elasticmq", "environment", elasticMQ.Environment, []string{"TZ=${TZ:-UTC}"})
	if !reflect.DeepEqual(elasticMQ.Ports, []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${ELASTICMQ_PORT:-9324}:9324"}) {
		t.Fatalf("elasticmq ports = %#v, want API-only provider mapping", elasticMQ.Ports)
	}
	if want := []string{
		"elasticmq:/data",
		"./containers/elasticmq/elasticmq.conf:/opt/elasticmq.conf:ro",
	}; !reflect.DeepEqual(elasticMQ.Volumes, want) {
		t.Fatalf("elasticmq volumes = %#v, want %#v", elasticMQ.Volumes, want)
	}
	assertHealthcheck("elasticmq", &renderedHealthcheck{
		Test:        []string{"CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:9324/health"},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     10,
		StartPeriod: "5s",
	})

	elasticMQUI := mustService("elasticmq-ui")
	assertSequence("elasticmq-ui", "environment", elasticMQUI.Environment, []string{
		"TZ=${TZ:-UTC}",
		"SQS_ENDPOINT=http://elasticmq:9324",
		"AWS_REGION=elasticmq",
		"AWS_ACCESS_KEY_ID=x",
		"AWS_SECRET_ACCESS_KEY=x",
	})
	if got := mustDependencies("elasticmq-ui", elasticMQUI.DependsOn); !reflect.DeepEqual(got, map[string]renderedDependency{"elasticmq": {Condition: "service_healthy"}}) {
		t.Fatalf("elasticmq-ui depends_on = %#v, want healthy API", got)
	}
	if !reflect.DeepEqual(elasticMQUI.Ports, []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${ELASTICMQ_UI_PORT:-9325}:3000"}) {
		t.Fatalf("elasticmq-ui ports = %#v, want UI container port 3000", elasticMQUI.Ports)
	}

	pubSub := mustService("gcppubsub")
	assertSequence("gcppubsub", "command", pubSub.Command, []string{
		"gcloud",
		"beta",
		"emulators",
		"pubsub",
		"start",
		"--host-port=0.0.0.0:8085",
		"--project=${EVENTS_PROJECT_ID:-goforj-local}",
	})
	assertSequence("gcppubsub", "environment", pubSub.Environment, []string{"TZ=${TZ:-UTC}"})
	if !reflect.DeepEqual(pubSub.Ports, []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${PUBSUB_PORT:-8085}:8085"}) {
		t.Fatalf("pubsub ports = %#v, want loopback emulator mapping", pubSub.Ports)
	}
	assertHealthcheck("gcppubsub", &renderedHealthcheck{
		Test:        []string{"CMD", "bash", "-c", "echo > /dev/tcp/127.0.0.1/8085"},
		Interval:    "10s",
		Timeout:     "5s",
		Retries:     10,
		StartPeriod: "20s",
	})

	config, err := templatesFS.ReadFile("containers/elasticmq/elasticmq.conf.tmpl")
	if err != nil {
		t.Fatalf("read ElasticMQ config template: %v", err)
	}
	wantConfig := `# Code generated by GoForj. DO NOT EDIT.
include classpath("application.conf")

node-address {
  protocol = http
  host = "*"
  port = 9324
  context-path = ""
}

rest-sqs {
  enabled = true
  bind-port = 9324
  bind-hostname = "0.0.0.0"
  sqs-limits = strict
}

messages-storage {
  enabled = true
  uri = "jdbc:h2:/data/elasticmq"
}

aws {
  region = "elasticmq"
  accountId = "000000000000"
}
`
	// Git may check embedded templates out with CRLF on Windows, but this contract targets content rather than host line endings.
	gotConfig := strings.ReplaceAll(string(config), "\r\n", "\n")
	if gotConfig != wantConfig {
		t.Fatalf("ElasticMQ config template =\n%s\nwant:\n%s", gotConfig, wantConfig)
	}
}
