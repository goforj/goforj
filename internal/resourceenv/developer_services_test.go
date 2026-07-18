package resourceenv

import (
	"testing"

	"github.com/goforj/goforj/project"
)

// TestDeveloperServiceEndpointsRequireExactGeneratedIdentities protects external endpoints from profile capture.
func TestDeveloperServiceEndpointsRequireExactGeneratedIdentities(t *testing.T) {
	tests := []struct {
		name       string
		components project.Components
		resource   project.ResourceKey
		driver     string
		service    project.ServiceKey
		local      string
		neighbor   string
	}{
		{
			name:       "Memcached",
			components: project.Components{Cache: true},
			resource:   project.ResourceCache,
			driver:     "memcached",
			service:    project.ServiceCacheMemcached,
			local:      "CACHE_DRIVER=memcached\nCACHE_ADDRESSES=memcached:11211\n",
			neighbor:   "CACHE_DRIVER=memcached\nCACHE_ADDRESSES=memcached-tools:11211\n",
		},
		{
			name:       "DynamoDB",
			components: project.Components{Cache: true},
			resource:   project.ResourceCache,
			driver:     "dynamodb",
			service:    project.ServiceCacheDynamoDB,
			local:      "CACHE_DRIVER=dynamodb\nCACHE_ENDPOINT=http://dynamodb:8000\n",
			neighbor:   "CACHE_DRIVER=dynamodb\nCACHE_ENDPOINT=http://dynamodb-tools:8000\n",
		},
		{
			name:       "Cache NATS",
			components: project.Components{Cache: true},
			resource:   project.ResourceCache,
			driver:     "nats",
			service:    project.ServiceCacheNATS,
			local:      "CACHE_DRIVER=nats\nCACHE_URL=nats://another:secret@nats:4222\n",
			neighbor:   "CACHE_DRIVER=nats\nCACHE_URL=nats://another:secret@nats-tools:4222\n",
		},
		{
			name:       "Queue NATS",
			components: project.Components{Jobs: true},
			resource:   project.ResourceQueue,
			driver:     "nats",
			service:    project.ServiceQueueNATS,
			local:      "QUEUE_DRIVER=nats\nQUEUE_URL=nats://goforj:goforj@nats:4222\n",
			neighbor:   "QUEUE_DRIVER=nats\nQUEUE_URL=nats://goforj:goforj@nats:4223\n",
		},
		{
			name:       "ElasticMQ",
			components: project.Components{Jobs: true},
			resource:   project.ResourceQueue,
			driver:     "sqs",
			service:    project.ServiceQueueSQS,
			local:      "QUEUE_DRIVER=sqs\nQUEUE_ENDPOINT=http://elasticmq:9324\n",
			neighbor:   "QUEUE_DRIVER=sqs\nQUEUE_ENDPOINT=http://elasticmq-tools:9324\n",
		},
		{
			name:       "RabbitMQ",
			components: project.Components{Jobs: true},
			resource:   project.ResourceQueue,
			driver:     "rabbitmq",
			service:    project.ServiceQueueRabbitMQ,
			local:      "QUEUE_DRIVER=rabbitmq\nQUEUE_URL=amqp://another:secret@rabbitmq:5672/\n",
			neighbor:   "QUEUE_DRIVER=rabbitmq\nQUEUE_URL=amqp://another:secret@rabbitmq-tools:5672/\n",
		},
		{
			name:       "Events NATS JetStream",
			components: project.Components{Events: true},
			resource:   project.ResourceEvents,
			driver:     "natsjetstream",
			service:    project.ServiceEventsNATS,
			local:      "EVENTS_DRIVER=natsjetstream\nEVENTS_URL=nats://goforj:goforj@nats:4222\n",
			neighbor:   "EVENTS_DRIVER=natsjetstream\nEVENTS_URL=tls://nats:4222\n",
		},
		{
			name:       "Redpanda",
			components: project.Components{Events: true},
			resource:   project.ResourceEvents,
			driver:     "kafka",
			service:    project.ServiceEventsKafka,
			local:      "EVENTS_DRIVER=kafka\nEVENTS_BROKERS=redpanda:9092\n",
			neighbor:   "EVENTS_DRIVER=kafka\nEVENTS_BROKERS=redpanda:9092,kafka.example:9092\n",
		},
		{
			name:       "Google PubSub",
			components: project.Components{Events: true},
			resource:   project.ResourceEvents,
			driver:     "gcppubsub",
			service:    project.ServiceEventsGCPPubSub,
			local:      "EVENTS_DRIVER=gcppubsub\nEVENTS_URI=gcppubsub:8085\n",
			neighbor:   "EVENTS_DRIVER=gcppubsub\nEVENTS_URI=http://gcppubsub:8085\n",
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
			selection.Supported = append(selection.Supported, test.driver)
			var err error
			plan, err = plan.WithSelection(test.resource, selection).Normalized(components)
			if err != nil {
				t.Fatalf("normalize %s plan: %v", test.driver, err)
			}

			for _, endpoint := range []struct {
				name      string
				source    string
				wantState project.ServiceState
			}{
				{name: "local", source: test.local, wantState: project.ServiceStateActiveLocal},
				{name: "neighbor", source: test.neighbor, wantState: project.ServiceStateExternalRequired},
				{name: "missing", source: test.driver + "_unused=1\n", wantState: project.ServiceStateExternalRequired},
			} {
				t.Run(endpoint.name, func(t *testing.T) {
					consumers, err := ResolveConsumers([]byte(endpoint.source), plan, components, nil)
					if err != nil {
						t.Fatalf("resolve consumers: %v", err)
					}
					intent := project.LocalServiceIntent{}.WithMode(test.service, project.LocalServiceModeLocal)
					resolved, err := project.ResolveServicePlanWithConsumers(plan, components, intent, consumers)
					if err != nil {
						t.Fatalf("resolve service plan: %v", err)
					}
					requirements := resolved.RequirementsFor(test.service)
					matching := []project.ServiceRequirement{}
					for _, requirement := range requirements {
						if requirement.State == endpoint.wantState {
							matching = append(matching, requirement)
						}
					}
					if len(matching) != 1 {
						t.Fatalf("%s requirements = %#v, want one %s requirement", test.service, requirements, endpoint.wantState)
					}
					if endpoint.wantState == project.ServiceStateExternalRequired && matching[0].EndpointAffinity == "" {
						t.Fatalf("external %s endpoint lost its affinity: %#v", test.service, matching[0])
					}
				})
			}
		})
	}
}
