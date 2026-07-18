package forj

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestDeveloperServiceDevtoolContracts locks the runtime and safety boundaries of the standalone development tools.
func TestDeveloperServiceDevtoolContracts(t *testing.T) {
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
	type renderedService struct {
		Profiles        []string             `yaml:"profiles"`
		RenderedTest    *bool                `yaml:"x-forj-test"`
		Restart         string               `yaml:"restart"`
		Image           string               `yaml:"image"`
		User            string               `yaml:"user"`
		SecurityOptions []string             `yaml:"security_opt"`
		MemoryLimit     string               `yaml:"mem_limit"`
		StopGracePeriod string               `yaml:"stop_grace_period"`
		Command         []string             `yaml:"command"`
		Environment     []string             `yaml:"environment"`
		ExtraHosts      []string             `yaml:"extra_hosts"`
		Ports           []string             `yaml:"ports"`
		Volumes         []string             `yaml:"volumes"`
		Healthcheck     *renderedHealthcheck `yaml:"healthcheck"`
	}
	var document struct {
		Volumes map[string]struct {
			Driver string `yaml:"driver"`
		} `yaml:"volumes"`
		Services map[string]yaml.Node `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("decode rendered developer-service Compose: %v\n%s", err, compose)
	}

	type expectedService struct {
		image           string
		user            string
		memoryLimit     string
		stopGracePeriod string
		command         []string
		environment     []string
		extraHosts      []string
		ports           []string
		volumes         []string
		healthcheck     *renderedHealthcheck
		persistent      bool
	}
	tests := []struct {
		name string
		want expectedService
	}{
		{
			name: "jaeger",
			want: expectedService{
				image: "cr.jaegertracing.io/jaegertracing/jaeger:2.19.0",
				environment: []string{
					"TZ=${TZ:-UTC}",
					"JAEGER_LISTEN_HOST=0.0.0.0",
				},
				ports: []string{
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${JAEGER_UI_PORT:-16686}:16686",
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${JAEGER_OTLP_GRPC_PORT:-4317}:4317",
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${JAEGER_OTLP_HTTP_PORT:-4318}:4318",
				},
				healthcheck: &renderedHealthcheck{
					Test:        []string{"CMD-SHELL", "wget -q -O /dev/null http://127.0.0.1:13133/status"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "10s",
				},
			},
		},
		{
			name: "temporal",
			want: expectedService{
				image: "temporalio/temporal:1.8.0",
				command: []string{
					"server",
					"start-dev",
					"--ip",
					"0.0.0.0",
					"--ui-disable-news-fetch",
				},
				environment: []string{"TZ=${TZ:-UTC}"},
				ports: []string{
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${TEMPORAL_GRPC_PORT:-7233}:7233",
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${TEMPORAL_UI_PORT:-8233}:8233",
				},
				healthcheck: &renderedHealthcheck{
					Test: []string{
						"CMD",
						"/usr/local/bin/temporal",
						"operator",
						"cluster",
						"health",
						"--address",
						"127.0.0.1:7233",
					},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     12,
					StartPeriod: "20s",
				},
			},
		},
		{
			name: "keycloak",
			want: expectedService{
				image:           "quay.io/keycloak/keycloak:26.7.0",
				memoryLimit:     "${KEYCLOAK_MEMORY_LIMIT:-1g}",
				stopGracePeriod: "30s",
				command:         []string{"start-dev"},
				environment: []string{
					"TZ=${TZ:-UTC}",
					"KC_HEALTH_ENABLED=true",
					"KC_BOOTSTRAP_ADMIN_USERNAME=${KEYCLOAK_BOOTSTRAP_ADMIN_USERNAME:-admin}",
					"KC_BOOTSTRAP_ADMIN_PASSWORD=${KEYCLOAK_BOOTSTRAP_ADMIN_PASSWORD:-G0Forj-Local-Keycloak!}",
				},
				ports: []string{
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${KEYCLOAK_HTTP_PORT:-8180}:8080",
				},
				volumes: []string{"keycloak:/opt/keycloak/data"},
				healthcheck: &renderedHealthcheck{
					Test: []string{
						"CMD-SHELL",
						`{ printf 'HEAD /health/ready HTTP/1.0\r\n\r\n' >&0; grep 'HTTP/1.0 200'; } 0<>/dev/tcp/localhost/9000`,
					},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     15,
					StartPeriod: "60s",
				},
				persistent: true,
			},
		},
		{
			name: "mockserver",
			want: expectedService{
				image:       "mockserver/mockserver:7.4.0",
				memoryLimit: "${MOCKSERVER_MEMORY_LIMIT:-1g}",
				environment: []string{
					"TZ=${TZ:-UTC}",
					"MOCKSERVER_LOG_LEVEL=${MOCKSERVER_LOG_LEVEL:-INFO}",
				},
				ports: []string{
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${MOCKSERVER_PORT:-1080}:1080",
				},
			},
		},
		{
			name: "toxiproxy",
			want: expectedService{
				image: "ghcr.io/shopify/toxiproxy:2.12.0",
				user:  "65532:65532",
				environment: []string{
					"TZ=${TZ:-UTC}",
					"LOG_LEVEL=${TOXIPROXY_LOG_LEVEL:-info}",
				},
				extraHosts: []string{"host.docker.internal:host-gateway"},
				ports: []string{
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${TOXIPROXY_API_PORT:-8474}:8474",
					"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${TOXIPROXY_PROXY_PORT:-8666}:8666",
				},
				healthcheck: &renderedHealthcheck{
					Test:        []string{"CMD", "/toxiproxy-cli", "list"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "5s",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node, ok := document.Services[test.name]
			if !ok {
				t.Fatalf("rendered Compose omitted %s:\n%s", test.name, compose)
			}
			var service renderedService
			if err := node.Decode(&service); err != nil {
				t.Fatalf("decode rendered %s service: %v", test.name, err)
			}
			if !reflect.DeepEqual(service.Profiles, []string{test.name}) {
				t.Fatalf("%s profiles = %#v, want exact profile %q", test.name, service.Profiles, test.name)
			}
			if service.RenderedTest == nil || *service.RenderedTest {
				t.Fatalf("%s x-forj-test = %#v, want false", test.name, service.RenderedTest)
			}
			if service.Restart != "unless-stopped" {
				t.Fatalf("%s restart = %q, want unless-stopped", test.name, service.Restart)
			}
			if service.Image != test.want.image {
				t.Fatalf("%s image = %q, want %q", test.name, service.Image, test.want.image)
			}
			if !reflect.DeepEqual(service.SecurityOptions, []string{"no-new-privileges:true"}) {
				t.Fatalf("%s security_opt = %#v, want no-new-privileges", test.name, service.SecurityOptions)
			}
			if service.User != test.want.user {
				t.Fatalf("%s user = %q, want %q", test.name, service.User, test.want.user)
			}
			if service.MemoryLimit != test.want.memoryLimit {
				t.Fatalf("%s mem_limit = %q, want %q", test.name, service.MemoryLimit, test.want.memoryLimit)
			}
			if service.StopGracePeriod != test.want.stopGracePeriod {
				t.Fatalf("%s stop_grace_period = %q, want %q", test.name, service.StopGracePeriod, test.want.stopGracePeriod)
			}
			if !reflect.DeepEqual(service.Command, test.want.command) {
				t.Fatalf("%s command = %#v, want %#v", test.name, service.Command, test.want.command)
			}
			if !reflect.DeepEqual(service.Environment, test.want.environment) {
				t.Fatalf("%s environment = %#v, want %#v", test.name, service.Environment, test.want.environment)
			}
			if !reflect.DeepEqual(service.ExtraHosts, test.want.extraHosts) {
				t.Fatalf("%s extra_hosts = %#v, want %#v", test.name, service.ExtraHosts, test.want.extraHosts)
			}
			if !reflect.DeepEqual(service.Ports, test.want.ports) {
				t.Fatalf("%s ports = %#v, want loopback mappings %#v", test.name, service.Ports, test.want.ports)
			}
			if !reflect.DeepEqual(service.Volumes, test.want.volumes) {
				t.Fatalf("%s volumes = %#v, want %#v", test.name, service.Volumes, test.want.volumes)
			}
			if !reflect.DeepEqual(service.Healthcheck, test.want.healthcheck) {
				t.Fatalf("%s healthcheck = %#v, want %#v", test.name, service.Healthcheck, test.want.healthcheck)
			}

			volume, persistent := document.Volumes[test.name]
			if persistent != test.want.persistent {
				t.Fatalf("%s named-volume presence = %t, want %t", test.name, persistent, test.want.persistent)
			}
			if persistent && volume.Driver != "local" {
				t.Fatalf("%s named-volume driver = %q, want local", test.name, volume.Driver)
			}
		})
	}
}
