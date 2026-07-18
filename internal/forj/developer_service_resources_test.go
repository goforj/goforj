package forj

import (
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/project"
)

// TestResourceBackedDeveloperServicesProjectOnlyExplicitLocalConnections keeps profile and App configuration decisions aligned.
func TestResourceBackedDeveloperServicesProjectOnlyExplicitLocalConnections(t *testing.T) {
	tests := []struct {
		name        string
		components  project.Components
		resource    project.ResourceKey
		driver      string
		service     project.ServiceKey
		profile     string
		environment []string
		host        []string
	}{
		{
			name: "Memcached", components: project.Components{Cache: true}, resource: project.ResourceCache,
			driver: "memcached", service: project.ServiceCacheMemcached, profile: "memcached",
			environment: []string{"CACHE_ADDRESSES=memcached:11211"}, host: []string{"CACHE_ADDRESSES=localhost:11211"},
		},
		{
			name: "DynamoDB", components: project.Components{Cache: true}, resource: project.ResourceCache,
			driver: "dynamodb", service: project.ServiceCacheDynamoDB, profile: "dynamodb",
			environment: []string{"CACHE_REGION=us-east-1", "CACHE_ENDPOINT=http://dynamodb:8000"}, host: []string{"CACHE_ENDPOINT=http://localhost:8000"},
		},
		{
			name: "Cache NATS", components: project.Components{Cache: true}, resource: project.ResourceCache,
			driver: "nats", service: project.ServiceCacheNATS, profile: "nats",
			environment: []string{"CACHE_URL=nats://goforj:goforj@nats:4222"}, host: []string{"CACHE_URL=nats://goforj:goforj@localhost:4222"},
		},
		{
			name: "Queue NATS", components: project.Components{Jobs: true}, resource: project.ResourceQueue,
			driver: "nats", service: project.ServiceQueueNATS, profile: "nats",
			environment: []string{"QUEUE_URL=nats://goforj:goforj@nats:4222"}, host: []string{"QUEUE_URL=nats://goforj:goforj@localhost:4222"},
		},
		{
			name: "ElasticMQ", components: project.Components{Jobs: true}, resource: project.ResourceQueue,
			driver: "sqs", service: project.ServiceQueueSQS, profile: "elasticmq",
			environment: []string{"QUEUE_REGION=elasticmq", "QUEUE_ENDPOINT=http://elasticmq:9324", "QUEUE_ACCESS_KEY=x", "QUEUE_SECRET_KEY=x"}, host: []string{"QUEUE_ENDPOINT=http://localhost:9324"},
		},
		{
			name: "RabbitMQ", components: project.Components{Jobs: true}, resource: project.ResourceQueue,
			driver: "rabbitmq", service: project.ServiceQueueRabbitMQ, profile: "rabbitmq",
			environment: []string{"QUEUE_URL=amqp://goforj:goforj@rabbitmq:5672/"}, host: []string{"QUEUE_URL=amqp://goforj:goforj@localhost:5672/"},
		},
		{
			name: "Events NATS JetStream", components: project.Components{Events: true}, resource: project.ResourceEvents,
			driver: "natsjetstream", service: project.ServiceEventsNATS, profile: "nats",
			environment: []string{"EVENTS_URL=nats://goforj:goforj@nats:4222"}, host: []string{"EVENTS_URL=nats://goforj:goforj@localhost:4222"},
		},
		{
			name: "Redpanda", components: project.Components{Events: true}, resource: project.ResourceEvents,
			driver: "kafka", service: project.ServiceEventsKafka, profile: "redpanda",
			environment: []string{"EVENTS_BROKERS=redpanda:9092"}, host: []string{"EVENTS_BROKERS=localhost:19092"},
		},
		{
			name: "Google PubSub", components: project.Components{Events: true}, resource: project.ResourceEvents,
			driver: "gcppubsub", service: project.ServiceEventsGCPPubSub, profile: "pubsub",
			environment: []string{"EVENTS_PROJECT_ID=goforj-local", "EVENTS_URI=gcppubsub:8085"}, host: []string{"EVENTS_URI=localhost:8085"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := test.components
			components.DatabaseSQLite = true
			components.Docker = true
			plan := defaultResourcePlanForTest(t, components)
			selection, ok := plan.Selection(test.resource)
			if !ok {
				t.Fatalf("resource plan omitted %s", test.resource)
			}
			selection.Active = test.driver
			if !stringSliceContainsFold(selection.Supported, test.driver) {
				selection.Supported = append(selection.Supported, test.driver)
			}
			var err error
			plan, err = plan.WithSelection(test.resource, selection).Normalized(components)
			if err != nil {
				t.Fatalf("normalize %s plan: %v", test.driver, err)
			}

			intent := project.LocalServiceIntent{}.WithMode(test.service, project.LocalServiceModeLocal)
			environment, _ := renderResourceTemplates(t, components, plan, intent)
			hostEnvironment := renderResourceHostEnvironment(t, components, plan, intent)
			profiles, set := envfile.Lookup(strings.Split(environment, "\n"), "COMPOSE_PROFILES")
			if !set || profiles != test.profile {
				t.Fatalf("COMPOSE_PROFILES = %q, set=%t; want %q\n%s", profiles, set, test.profile, environment)
			}
			for _, assignment := range test.environment {
				if !strings.Contains(environment, assignment+"\n") {
					t.Fatalf("local environment omitted %q:\n%s", assignment, environment)
				}
			}
			for _, assignment := range test.host {
				if !strings.Contains(hostEnvironment, assignment+"\n") {
					t.Fatalf("host environment omitted %q:\n%s", assignment, hostEnvironment)
				}
			}

			externalIntent := project.LocalServiceIntent{}.WithMode(test.service, project.LocalServiceModeExternal)
			externalEnvironment, _ := renderResourceTemplates(t, components, plan, externalIntent)
			externalProfiles, _ := envfile.Lookup(strings.Split(externalEnvironment, "\n"), "COMPOSE_PROFILES")
			if externalProfiles != "" {
				t.Fatalf("external provider activated profile %q:\n%s", externalProfiles, externalEnvironment)
			}
			for _, assignment := range test.environment {
				if strings.Contains(externalEnvironment, assignment+"\n") {
					t.Fatalf("external provider received local assignment %q:\n%s", assignment, externalEnvironment)
				}
			}
		})
	}
}
