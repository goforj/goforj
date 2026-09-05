package forj

import (
	"reflect"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/devservices"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

type developerServiceDataHealthcheck struct {
	Test        []string `yaml:"test"`
	Interval    string   `yaml:"interval"`
	Timeout     string   `yaml:"timeout"`
	Retries     int      `yaml:"retries"`
	StartPeriod string   `yaml:"start_period"`
}

type developerServiceDataBuild struct {
	Context    string `yaml:"context"`
	Dockerfile string `yaml:"dockerfile"`
}

type developerServiceDataService struct {
	Profiles         []string                         `yaml:"profiles"`
	RenderedTest     *bool                            `yaml:"x-forj-test"`
	Restart          string                           `yaml:"restart"`
	StopGracePeriod  string                           `yaml:"stop_grace_period"`
	Image            string                           `yaml:"image"`
	User             string                           `yaml:"user"`
	WorkingDirectory string                           `yaml:"working_dir"`
	SecurityOptions  []string                         `yaml:"security_opt"`
	Privileged       bool                             `yaml:"privileged"`
	Capabilities     []string                         `yaml:"cap_add"`
	NetworkMode      string                           `yaml:"network_mode"`
	Build            *developerServiceDataBuild       `yaml:"build"`
	DependsOn        []string                         `yaml:"depends_on"`
	ExtraHosts       []string                         `yaml:"extra_hosts"`
	EntryPoint       []string                         `yaml:"entrypoint"`
	Command          []string                         `yaml:"command"`
	Environment      []string                         `yaml:"environment"`
	Ports            []string                         `yaml:"ports"`
	Volumes          []string                         `yaml:"volumes"`
	Healthcheck      *developerServiceDataHealthcheck `yaml:"healthcheck"`
	ULimits          map[string]struct {
		Soft int `yaml:"soft"`
		Hard int `yaml:"hard"`
	} `yaml:"ulimits"`
}

type developerServiceDataVolume struct {
	Driver string `yaml:"driver"`
}

type developerServiceDataDocument struct {
	Volumes  map[string]developerServiceDataVolume  `yaml:"volumes"`
	Services map[string]developerServiceDataService `yaml:"services"`
}

// TestDeveloperServiceDataContracts locks the verified image, exposure, persistence, and readiness contracts for data-oriented tools.
func TestDeveloperServiceDataContracts(t *testing.T) {
	document, compose := renderDeveloperServiceDataCompose(t, project.Components{Docker: true})

	type expectedService struct {
		image            string
		profiles         []string
		ports            []string
		volumes          []string
		namedVolumes     []string
		environment      []string
		command          []string
		healthcheck      *developerServiceDataHealthcheck
		healthContains   string
		user             string
		workingDirectory string
		renderedTestOff  bool
	}
	tests := []struct {
		name string
		want expectedService
	}{
		{
			name: "dynamodb",
			want: expectedService{
				image:            "amazon/dynamodb-local:3.3.0",
				profiles:         []string{"dynamodb"},
				ports:            []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${DYNAMODB_PORT:-8000}:8000"},
				volumes:          []string{"dynamodb:/home/dynamodblocal/data"},
				namedVolumes:     []string{"dynamodb"},
				environment:      []string{"TZ=${TZ:-UTC}"},
				command:          []string{"-jar", "DynamoDBLocal.jar", "-sharedDb", "-dbPath", "./data", "-disableTelemetry"},
				user:             "root",
				workingDirectory: "/home/dynamodblocal",
				renderedTestOff:  true,
				healthcheck: &developerServiceDataHealthcheck{
					Test:        []string{"CMD-SHELL", "bash -ec 'exec 3<>/dev/tcp/127.0.0.1/8000'"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "10s",
				},
			},
		},
		{
			name: "memcached",
			want: expectedService{
				image:       "memcached:1.6.44-alpine",
				profiles:    []string{"memcached"},
				ports:       []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${MEMCACHED_PORT:-11211}:11211"},
				environment: []string{"TZ=${TZ:-UTC}"},
				command:     []string{"memcached", "--memory-limit=${MEMCACHED_MEMORY_MB:-64}"},
				healthcheck: &developerServiceDataHealthcheck{
					Test:        []string{"CMD-SHELL", "printf 'version\\r\\n' | nc -w 1 127.0.0.1 11211 | grep -q '^VERSION '"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "5s",
				},
			},
		},
		{
			name: "sftpgo",
			want: expectedService{
				image:        "drakkan/sftpgo:v2.7.5",
				profiles:     []string{"sftpgo"},
				ports:        []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${SFTPGO_SFTP_PORT:-2022}:2022", "${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${SFTPGO_HTTP_PORT:-8081}:8080"},
				volumes:      []string{"sftpgo-data:/var/lib/sftpgo", "sftpgo-home:/srv/sftpgo"},
				namedVolumes: []string{"sftpgo-data", "sftpgo-home"},
				environment: []string{
					"TZ=${TZ:-UTC}",
					"SFTPGO_DATA_PROVIDER__CREATE_DEFAULT_ADMIN=true",
					"SFTPGO_DEFAULT_ADMIN_USERNAME=${SFTPGO_ADMIN_USERNAME:-goforj}",
					"SFTPGO_DEFAULT_ADMIN_PASSWORD=${SFTPGO_ADMIN_PASSWORD:-goforj-local}",
				},
				renderedTestOff: true,
				healthcheck: &developerServiceDataHealthcheck{
					Test:        []string{"CMD", "sftpgo", "ping"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "10s",
				},
			},
		},
		{
			name: "adminer",
			want: expectedService{
				image:       "adminer:5.4.2",
				profiles:    []string{"adminer"},
				ports:       []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${ADMINER_PORT:-8080}:8080"},
				environment: []string{"TZ=${TZ:-UTC}"},
				healthcheck: &developerServiceDataHealthcheck{
					Test:        []string{"CMD", "php", "-r", "exit(@fsockopen('127.0.0.1',8080)?0:1);"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "5s",
				},
			},
		},
		{
			name: "qdrant",
			want: expectedService{
				image:           "qdrant/qdrant:v1.18.3",
				profiles:        []string{"qdrant"},
				ports:           []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${QDRANT_HTTP_PORT:-6333}:6333", "${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${QDRANT_GRPC_PORT:-6334}:6334"},
				volumes:         []string{"qdrant:/qdrant/storage"},
				namedVolumes:    []string{"qdrant"},
				environment:     []string{"TZ=${TZ:-UTC}"},
				renderedTestOff: true,
				healthContains:  "/readyz",
			},
		},
		{
			name: "clickhouse",
			want: expectedService{
				image:        "clickhouse/clickhouse-server:26.3.17.4",
				profiles:     []string{"clickhouse"},
				ports:        []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${CLICKHOUSE_HTTP_PORT:-8123}:8123", "${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${CLICKHOUSE_NATIVE_PORT:-19000}:9000"},
				volumes:      []string{"clickhouse:/var/lib/clickhouse"},
				namedVolumes: []string{"clickhouse"},
				environment: []string{
					"TZ=${TZ:-UTC}",
					"CLICKHOUSE_DB=${CLICKHOUSE_DATABASE:-app}",
					"CLICKHOUSE_USER=${CLICKHOUSE_USERNAME:-goforj}",
					"CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-goforj-local}",
					"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1",
				},
				renderedTestOff: true,
				healthcheck: &developerServiceDataHealthcheck{
					Test:        []string{"CMD-SHELL", "clickhouse-client --user \"$${CLICKHOUSE_USER}\" --password \"$${CLICKHOUSE_PASSWORD}\" --query 'SELECT 1'"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "20s",
				},
			},
		},
		{
			name: "meilisearch",
			want: expectedService{
				image:           "getmeili/meilisearch:v1.49.0",
				profiles:        []string{"meilisearch"},
				ports:           []string{"${DEV_SERVICE_IP_ADDRESS:-127.0.0.1}:${MEILISEARCH_PORT:-7700}:7700"},
				volumes:         []string{"meilisearch:/meili_data"},
				namedVolumes:    []string{"meilisearch"},
				environment:     []string{"TZ=${TZ:-UTC}", "MEILI_ENV=development", "MEILI_NO_ANALYTICS=true"},
				renderedTestOff: true,
				healthcheck: &developerServiceDataHealthcheck{
					Test:        []string{"CMD", "curl", "-fsS", "http://127.0.0.1:7700/health"},
					Interval:    "10s",
					Timeout:     "5s",
					Retries:     10,
					StartPeriod: "20s",
				},
			},
		},
		{
			name: "mailpit",
			want: expectedService{
				image:       "axllent/mailpit:v1.30.4",
				profiles:    []string{"mailpit"},
				ports:       []string{"${IP_ADDRESS:-127.0.0.1}:${MAILPIT_SMTP_PORT:-1025}:1025", "${IP_ADDRESS:-127.0.0.1}:${MAILPIT_HTTP_PORT:-8025}:8025"},
				environment: []string{"TZ=${TZ:-UTC}"},
			},
		},
		{
			name: "victoriametrics",
			want: expectedService{
				image:        "victoriametrics/victoria-metrics:v1.120.0",
				profiles:     []string{"victoriametrics", "grafana"},
				ports:        []string{"${IP_ADDRESS:-127.0.0.1}:${OBSERVABILITY_VM_PORT:-8428}:8428"},
				volumes:      []string{"victoriametrics:/victoria-metrics-data"},
				namedVolumes: []string{"victoriametrics"},
				environment:  []string{"TZ=${TZ:-UTC}"},
				command:      []string{"--storageDataPath=/victoria-metrics-data", "--httpListenAddr=:8428", "--retentionPeriod=30d"},
			},
		},
		{
			name: "grafana",
			want: expectedService{
				image:        "grafana/grafana:12.0.2",
				profiles:     []string{"grafana"},
				ports:        []string{"${IP_ADDRESS:-127.0.0.1}:${GRAFANA_PORT:-13001}:3000"},
				volumes:      []string{"grafana:/var/lib/grafana"},
				namedVolumes: []string{"grafana"},
				environment: []string{
					"TZ=${TZ:-UTC}",
					"GF_SECURITY_ADMIN_USER=${GRAFANA_ADMIN_USER:-admin}",
					"GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD:-admin}",
					"GF_USERS_ALLOW_SIGN_UP=false",
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service, ok := document.Services[test.name]
			if !ok {
				t.Fatalf("rendered Compose omitted %s:\n%s", test.name, compose)
			}
			if service.Image != test.want.image {
				t.Fatalf("%s image = %q, want %q", test.name, service.Image, test.want.image)
			}
			if !reflect.DeepEqual(service.Profiles, test.want.profiles) {
				t.Fatalf("%s profiles = %#v, want %#v", test.name, service.Profiles, test.want.profiles)
			}
			if service.Restart != "unless-stopped" {
				t.Fatalf("%s restart = %q, want unless-stopped", test.name, service.Restart)
			}
			if !reflect.DeepEqual(service.SecurityOptions, []string{"no-new-privileges:true"}) {
				t.Fatalf("%s security_opt = %#v, want no-new-privileges", test.name, service.SecurityOptions)
			}
			if service.Privileged || len(service.Capabilities) != 0 || service.NetworkMode != "" {
				t.Fatalf("%s unexpectedly expands container privileges: privileged=%t cap_add=%#v network_mode=%q", test.name, service.Privileged, service.Capabilities, service.NetworkMode)
			}
			if !reflect.DeepEqual(service.Ports, test.want.ports) {
				t.Fatalf("%s ports = %#v, want %#v", test.name, service.Ports, test.want.ports)
			}
			if !reflect.DeepEqual(service.Volumes, test.want.volumes) {
				t.Fatalf("%s volumes = %#v, want %#v", test.name, service.Volumes, test.want.volumes)
			}
			if !reflect.DeepEqual(service.Environment, test.want.environment) {
				t.Fatalf("%s environment = %#v, want %#v", test.name, service.Environment, test.want.environment)
			}
			if !reflect.DeepEqual(service.Command, test.want.command) {
				t.Fatalf("%s command = %#v, want %#v", test.name, service.Command, test.want.command)
			}
			if service.User != test.want.user || service.WorkingDirectory != test.want.workingDirectory {
				t.Fatalf("%s runtime identity = user %q working_dir %q, want user %q working_dir %q", test.name, service.User, service.WorkingDirectory, test.want.user, test.want.workingDirectory)
			}
			if test.want.renderedTestOff {
				if service.RenderedTest == nil || *service.RenderedTest {
					t.Fatalf("%s x-forj-test = %#v, want false", test.name, service.RenderedTest)
				}
			} else if service.RenderedTest != nil {
				t.Fatalf("%s x-forj-test = %#v, want inherited test support", test.name, service.RenderedTest)
			}
			if test.want.healthcheck != nil && !reflect.DeepEqual(service.Healthcheck, test.want.healthcheck) {
				t.Fatalf("%s healthcheck = %#v, want %#v", test.name, service.Healthcheck, test.want.healthcheck)
			}
			if test.want.healthContains != "" {
				if service.Healthcheck == nil || len(service.Healthcheck.Test) != 2 || !strings.Contains(service.Healthcheck.Test[1], test.want.healthContains) {
					t.Fatalf("%s healthcheck = %#v, want probe containing %q", test.name, service.Healthcheck, test.want.healthContains)
				}
			}
			for _, volumeName := range test.want.namedVolumes {
				volume, ok := document.Volumes[volumeName]
				if !ok || volume.Driver != "local" {
					t.Fatalf("%s named volume %q = %#v, present=%t; want local driver", test.name, volumeName, volume, ok)
				}
			}
			if len(test.want.namedVolumes) == 0 {
				if volume, ok := document.Volumes[test.name]; ok {
					t.Fatalf("%s unexpectedly declares persistent volume %#v", test.name, volume)
				}
			}
		})
	}

	clickhouse := document.Services["clickhouse"]
	if got := clickhouse.ULimits["nofile"]; got.Soft != 262144 || got.Hard != 262144 {
		t.Fatalf("ClickHouse nofile ulimit = %#v, want 262144/262144", got)
	}
}

// TestSFTPGoDataServiceRemainsStandalone verifies the profile bootstraps only its administrator and never claims or creates an SFTP consumer account.
func TestSFTPGoDataServiceRemainsStandalone(t *testing.T) {
	definition, ok := devservices.DefinitionByKey(devservices.KeySFTPGo)
	if !ok {
		t.Fatal("developer-service catalog omitted SFTPGo")
	}
	if len(definition.Providers) != 0 {
		t.Fatalf("SFTPGo providers = %#v, want standalone service", definition.Providers)
	}

	document, _ := renderDeveloperServiceDataCompose(t, project.Components{Docker: true})
	service := document.Services["sftpgo"]
	for _, variable := range service.Environment {
		if strings.Contains(variable, "LOAD_DATA") || strings.Contains(variable, "DEFAULT_USER") || strings.HasPrefix(variable, "STORAGE_") {
			t.Fatalf("SFTPGo unexpectedly bootstraps a protocol consumer through %q", variable)
		}
	}
}

// TestDeveloperServiceDataConditionalExtensions keeps component-owned build contexts out of base profile renders.
func TestDeveloperServiceDataConditionalExtensions(t *testing.T) {
	base, baseCompose := renderDeveloperServiceDataCompose(t, project.Components{Docker: true})
	if _, ok := base.Services["vmagent"]; ok {
		t.Fatalf("Docker-only Compose unexpectedly includes vmagent:\n%s", baseCompose)
	}
	if _, ok := base.Services["grafana-seed"]; ok {
		t.Fatalf("Docker-only Compose unexpectedly includes grafana-seed:\n%s", baseCompose)
	}
	baseGrafana := base.Services["grafana"]
	if baseGrafana.Image != "grafana/grafana:12.0.2" || baseGrafana.Build != nil {
		t.Fatalf("base Grafana runtime = image %q build %#v, want pinned upstream image", baseGrafana.Image, baseGrafana.Build)
	}

	observability, observabilityCompose := renderDeveloperServiceDataCompose(t, project.Components{Docker: true, Observability: true})
	vmagent, ok := observability.Services["vmagent"]
	if !ok {
		t.Fatalf("observability Compose omitted vmagent:\n%s", observabilityCompose)
	}
	if !reflect.DeepEqual(vmagent.Profiles, []string{"victoriametrics", "grafana"}) ||
		vmagent.Build == nil || vmagent.Build.Context != "./containers/observability/vmagent" || vmagent.Build.Dockerfile != "" ||
		!reflect.DeepEqual(vmagent.DependsOn, []string{"victoriametrics"}) ||
		!reflect.DeepEqual(vmagent.ExtraHosts, []string{"host.docker.internal:host-gateway"}) ||
		!reflect.DeepEqual(vmagent.Command, []string{"-promscrape.config=/etc/vmagent/prometheus.yml", "-remoteWrite.url=http://victoriametrics:8428/api/v1/write"}) ||
		!reflect.DeepEqual(vmagent.SecurityOptions, []string{"no-new-privileges:true"}) {
		t.Fatalf("vmagent extension contract = %#v", vmagent)
	}
	if _, ok := observability.Services["grafana-seed"]; ok {
		t.Fatalf("observability-only Compose unexpectedly includes grafana-seed:\n%s", observabilityCompose)
	}
	observabilityGrafana := observability.Services["grafana"]
	if observabilityGrafana.Image != "grafana/grafana:12.0.2" || observabilityGrafana.Build != nil {
		t.Fatalf("observability-only Grafana runtime = image %q build %#v, want pinned base image", observabilityGrafana.Image, observabilityGrafana.Build)
	}

	grafana, grafanaCompose := renderDeveloperServiceDataCompose(t, project.Components{Docker: true, Observability: true, Grafana: true})
	grafanaService := grafana.Services["grafana"]
	if grafanaService.Image != "" || grafanaService.Build == nil || grafanaService.Build.Context != "./containers/observability/grafana" || grafanaService.Build.Dockerfile != "" {
		t.Fatalf("component Grafana runtime = image %q build %#v, want generated build context", grafanaService.Image, grafanaService.Build)
	}
	if !reflect.DeepEqual(grafanaService.Profiles, []string{"grafana"}) ||
		!reflect.DeepEqual(grafanaService.DependsOn, []string{"victoriametrics"}) ||
		!reflect.DeepEqual(grafanaService.SecurityOptions, []string{"no-new-privileges:true"}) ||
		!reflect.DeepEqual(grafanaService.Ports, []string{"${IP_ADDRESS:-127.0.0.1}:${GRAFANA_PORT:-13001}:3000"}) ||
		!reflect.DeepEqual(grafanaService.Volumes, []string{"grafana:/var/lib/grafana"}) {
		t.Fatalf("component Grafana service contract = %#v", grafanaService)
	}
	seed, ok := grafana.Services["grafana-seed"]
	if !ok {
		t.Fatalf("Grafana component Compose omitted seed extension:\n%s", grafanaCompose)
	}
	if !reflect.DeepEqual(seed.Profiles, []string{"grafana"}) || seed.Restart != "no" || seed.StopGracePeriod != "1s" ||
		seed.Build == nil || seed.Build.Context != "./containers/observability/grafana" || seed.Build.Dockerfile != "Dockerfile.seed" ||
		!reflect.DeepEqual(seed.DependsOn, []string{"grafana"}) || !reflect.DeepEqual(seed.EntryPoint, []string{"sh"}) ||
		!reflect.DeepEqual(seed.Command, []string{"/seed-dashboards.sh"}) ||
		!reflect.DeepEqual(seed.Environment, []string{
			"TZ=${TZ:-UTC}",
			"GRAFANA_URL=http://grafana:3000",
			"GRAFANA_ADMIN_USER=${GRAFANA_ADMIN_USER:-admin}",
			"GRAFANA_ADMIN_PASSWORD=${GRAFANA_ADMIN_PASSWORD:-admin}",
		}) ||
		!reflect.DeepEqual(seed.SecurityOptions, []string{"no-new-privileges:true"}) {
		t.Fatalf("Grafana seed extension contract = %#v", seed)
	}
}

// renderDeveloperServiceDataCompose renders and decodes the shared Compose surface for data-service contract tests.
func renderDeveloperServiceDataCompose(t *testing.T, components project.Components) (developerServiceDataDocument, string) {
	t.Helper()
	plan := defaultResourcePlanForTest(t, components)
	_, compose := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
	var rendered struct {
		Volumes  map[string]developerServiceDataVolume `yaml:"volumes"`
		Services map[string]yaml.Node                  `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &rendered); err != nil {
		t.Fatalf("decode rendered developer-service Compose: %v\n%s", err, compose)
	}
	document := developerServiceDataDocument{
		Volumes:  rendered.Volumes,
		Services: make(map[string]developerServiceDataService),
	}
	for _, serviceName := range []string{
		"dynamodb", "memcached", "sftpgo", "adminer", "qdrant", "clickhouse", "meilisearch",
		"mailpit", "victoriametrics", "vmagent", "grafana", "grafana-seed",
	} {
		node, ok := rendered.Services[serviceName]
		if !ok {
			continue
		}
		var service developerServiceDataService
		if err := node.Decode(&service); err != nil {
			t.Fatalf("decode rendered developer service %s: %v\n%s", serviceName, err, compose)
		}
		document.Services[serviceName] = service
	}
	return document, compose
}
