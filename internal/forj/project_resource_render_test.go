package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/internal/resourceenv"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestResourceTemplatesRenderDefaultAndRedisPlans verifies environment and Compose consume the same concrete plan.
func TestResourceTemplatesRenderDefaultAndRedisPlans(t *testing.T) {
	components := project.Components{
		CLI:           true,
		Docker:        true,
		Mail:          true,
		Auth:          true,
		Cache:         true,
		DatabaseMySQL: true,
		Jobs:          true,
		Events:        true,
	}
	tests := []struct {
		name        string
		redisActive bool
		wantEnv     []string
		forbidEnv   []string
		redisLocal  bool
	}{
		{
			name: "default",
			wantEnv: []string{
				"CACHE_DRIVER=memory",
				"CACHE_SUPPORTED_DRIVERS=memory,redis",
				"QUEUE_DRIVER=workerpool",
				"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
				"EVENTS_DRIVER=inproc",
				"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
				"CACHE_SESSIONS_DRIVER=memory",
				"MAIL_SUPPORTED_DRIVERS=log,smtp",
			},
			forbidEnv: []string{"COMPOSE_PROFILES=redis", "REDIS_PASSWORD=", "REDIS_DB=", "CACHE_LIGHTHOUSE_DRIVER="},
		},
		{
			name:        "Redis active",
			redisActive: true,
			redisLocal:  true,
			wantEnv: []string{
				"# Redis\n",
				"CACHE_DRIVER=redis",
				"CACHE_SUPPORTED_DRIVERS=memory,redis",
				"QUEUE_DRIVER=redis",
				"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
				"EVENTS_DRIVER=redis",
				"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
				"CACHE_SESSIONS_DRIVER=redis",
				"COMPOSE_PROFILES=redis",
				"REDIS_PASSWORD=\n",
				"REDIS_DB=0\n",
			},
			forbidEnv: []string{"CACHE_LIGHTHOUSE_DRIVER="},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := defaultResourcePlanForTest(t, components)
			if test.redisActive {
				plan = redisResourcePlanForTest(t, components)
			}
			intent := project.LocalServiceIntent{}
			if test.redisLocal {
				intent = intent.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
			}
			environment, compose := renderResourceTemplates(t, components, plan, intent)
			for _, want := range test.wantEnv {
				if !strings.Contains(environment, want) {
					t.Fatalf("environment omitted %q:\n%s", want, environment)
				}
			}
			for _, forbidden := range test.forbidEnv {
				if strings.Contains(environment, forbidden) {
					t.Fatalf("environment unexpectedly contains %q:\n%s", forbidden, environment)
				}
			}
			if !strings.Contains(compose, "  redis:\n    profiles: [redis]") {
				t.Fatalf("portable Compose bridge omitted profiled Redis:\n%s", compose)
			}
		})
	}
}

// TestDefaultEnvironmentIsCompactAndIntentional keeps a new project's primary configuration focused on active choices.
func TestDefaultEnvironmentIsCompactAndIntentional(t *testing.T) {
	const environmentReference = "# Environment reference: https://goforj.dev/reference/env-vars"

	components := project.DefaultSelectedComponents()
	plan := defaultResourcePlanForTest(t, components)
	environment, _ := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})

	if !strings.HasPrefix(environment, environmentReference+"\n\n# App\n") {
		t.Fatalf("default environment omitted its configuration reference:\n%s", environment)
	}
	if strings.Contains(environment, "\n\n\n") {
		t.Fatalf("default environment contains consecutive blank lines:\n%s", environment)
	}
	if !strings.HasSuffix(environment, "\n") {
		t.Fatal("default environment is missing its terminal newline")
	}
	lines := strings.Split(environment, "\n")
	sections := make([]string, 0)
	keys := make([]string, 0)
	for lineIndex, line := range lines {
		if len(line) > 100 {
			t.Fatalf("default environment line %d exceeds 100 characters: %s", lineIndex+1, line)
		}
		if strings.HasPrefix(line, "# ") && line != environmentReference && !strings.HasPrefix(line, "# Available drivers:") {
			sections = append(sections, line)
		}
		if key, _, ok := envfile.ParseAssignment(line); ok {
			keys = append(keys, key)
		}
	}

	wantSections := []string{
		"# App",
		"# Logging",
		"# Lighthouse",
		"# API",
		"# Metrics",
		"# Auth",
		"# Mail",
		"# Mailpit",
		"# Observability",
		"# Database",
		"# Cache",
		"# File Storage",
		"# Events",
		"# Queue",
		"# Runtime",
		"# Docker",
	}
	if strings.Join(sections, "\n") != strings.Join(wantSections, "\n") {
		t.Fatalf("default environment sections:\nwant: %q\ngot:  %q\n%s", wantSections, sections, environment)
	}

	previous := -1
	for _, section := range wantSections {
		index := strings.Index(environment, section+"\n")
		if index <= previous {
			t.Fatalf("default environment section %q is out of order:\n%s", section, environment)
		}
		if index > 0 && environment[index-2:index] != "\n\n" {
			t.Fatalf("default environment section %q is not separated by one blank line:\n%s", section, environment)
		}
		previous = index
	}

	wantKeys := []string{
		"APP_NAME", "APP_KEY", "APP_DIAG_TOKEN", "APP_ENV", "TZ", "APP_DEBUG", "APP_URL",
		"APP_LOG_FORMAT", "APP_LOG_TIME",
		"LIGHTHOUSE_ENABLED", "LIGHTHOUSE_URL", "LIGHTHOUSE_SECRET",
		"API_JWT_SECRET_KEY", "API_SWAGGER_ENABLED", "API_HTTP_HOST", "API_HTTP_PORT",
		"METRICS_PORT", "METRICS_SCHEDULER_PORT", "METRICS_JOBS_PORT",
		"AUTH_BOOTSTRAP_USERNAME", "AUTH_BOOTSTRAP_EMAIL", "AUTH_BOOTSTRAP_PASSWORD",
		"MAIL_DRIVER", "MAIL_SUPPORTED_DRIVERS", "MAIL_SMTP_HOST", "MAIL_SMTP_PORT",
		"MAIL_FROM_ADDRESS", "MAIL_FROM_NAME", "MAILPIT_SMTP_PORT", "MAILPIT_HTTP_PORT",
		"OBSERVABILITY_VM_PORT", "GRAFANA_ADMIN_USER", "GRAFANA_ADMIN_PASSWORD", "GRAFANA_PORT",
		"DB_DRIVER", "DB_SUPPORTED_DRIVERS", "DB_HOST", "DB_PORT", "DB_DATABASE",
		"DB_USERNAME", "DB_PASSWORD", "DB_ROOT_PASSWORD",
		"CACHE_DRIVER", "CACHE_SUPPORTED_DRIVERS", "CACHE_SESSIONS_DRIVER",
		"STORAGE_DRIVER", "STORAGE_SUPPORTED_DRIVERS", "STORAGE_ROOT",
		"STORAGE_PUBLIC_DRIVER", "STORAGE_PUBLIC_ROOT",
		"EVENTS_DRIVER", "EVENTS_SUPPORTED_DRIVERS",
		"QUEUE_DRIVER", "QUEUE_SUPPORTED_DRIVERS", "QUEUE_WORKERS", "QUEUE_SHUTDOWN_TIMEOUT",
		"SCHEDULER_SUBPROCESS_SHUTDOWN_TIMEOUT",
		"IP_ADDRESS",
	}
	if strings.Join(keys, "\n") != strings.Join(wantKeys, "\n") {
		t.Fatalf("default environment assignments:\nwant: %q\ngot:  %q\n%s", wantKeys, keys, environment)
	}
	for _, want := range []string{
		"# Available drivers: log,smtp,resend,postmark,mailgun,sendgrid,ses\nMAIL_DRIVER=smtp\nMAIL_SUPPORTED_DRIVERS=log,smtp",
		"# Available drivers: sqlite,mysql,postgres\nDB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=mysql",
		"# Available drivers: memory,file,null,redis,memcached,dynamodb,sqlite,postgres,mysql,nats\nCACHE_DRIVER=memory\nCACHE_SUPPORTED_DRIVERS=memory,redis",
		"# Available drivers: local,memory,redis,ftp,sftp,s3,gcs,dropbox,rclone\nSTORAGE_DRIVER=local\nSTORAGE_SUPPORTED_DRIVERS=local",
		"# Available drivers: inproc,null,redis,nats,natsjetstream,kafka,gcppubsub,sns\nEVENTS_DRIVER=inproc\nEVENTS_SUPPORTED_DRIVERS=inproc,redis",
		"# Available drivers: null,sync,workerpool,redis,nats,sqs,rabbitmq,sqlite,postgres,mysql\nQUEUE_DRIVER=workerpool\nQUEUE_SUPPORTED_DRIVERS=workerpool,redis",
	} {
		if !strings.Contains(environment, want+"\n") {
			t.Fatalf("default environment omitted available-driver choices %q:\n%s", want, environment)
		}
	}
	if !strings.Contains(environment, "STORAGE_PUBLIC_DRIVER=local\n") {
		t.Fatalf("default environment omitted the public storage driver:\n%s", environment)
	}
	for _, want := range []string{"QUEUE_WORKERS=30", "QUEUE_SHUTDOWN_TIMEOUT=10s"} {
		if !strings.Contains(environment, want+"\n") {
			t.Fatalf("default environment omitted queue operating default %q:\n%s", want, environment)
		}
	}
	if !strings.Contains(environment, "IP_ADDRESS=0.0.0.0\n") {
		t.Fatalf("default environment omitted the Docker bind-address default:\n%s", environment)
	}
	if !strings.Contains(environment, "APP_ENV=local\nTZ=UTC\nAPP_DEBUG=0\n") {
		t.Fatalf("default environment omitted the UTC application default:\n%s", environment)
	}
}

// TestResourceTemplatesDefaultComposeServicesToUTC verifies every generated service inherits the project timezone contract.
func TestResourceTemplatesDefaultComposeServicesToUTC(t *testing.T) {
	components := project.DefaultSelectedComponents()
	plan := defaultResourcePlanForTest(t, components)
	environment, compose := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
	if strings.Count(environment, "TZ=UTC\n") != 1 {
		t.Fatalf("default environment must define TZ=UTC exactly once:\n%s", environment)
	}

	var document struct {
		Services map[string]struct {
			Environment []string `yaml:"environment"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(compose), &document); err != nil {
		t.Fatalf("decode rendered Compose: %v\n%s", err, compose)
	}
	for _, serviceName := range []string{"victoriametrics", "vmagent", "grafana", "grafana-seed", "mailpit", "mysql", "redis"} {
		service, ok := document.Services[serviceName]
		if !ok {
			t.Fatalf("rendered Compose omitted expected service %q:\n%s", serviceName, compose)
		}
		found := false
		for _, value := range service.Environment {
			if value == "TZ=${TZ:-UTC}" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("rendered Compose service %q omitted the UTC timezone default: %#v", serviceName, service.Environment)
		}
	}

	postgresComponents := project.Components{Docker: true, DatabasePostgres: true}
	postgresPlan := defaultResourcePlanForTest(t, postgresComponents)
	_, postgresCompose := renderResourceTemplates(t, postgresComponents, postgresPlan, project.LocalServiceIntent{})
	document.Services = nil
	if err := yaml.Unmarshal([]byte(postgresCompose), &document); err != nil {
		t.Fatalf("decode rendered Postgres Compose: %v\n%s", err, postgresCompose)
	}
	postgres, ok := document.Services["postgres"]
	if !ok || len(postgres.Environment) == 0 || postgres.Environment[len(postgres.Environment)-1] != "TZ=${TZ:-UTC}" {
		t.Fatalf("rendered Postgres service omitted the UTC timezone default: %#v", postgres.Environment)
	}
}

// TestMailpitEnvironmentSectionRequiresMailAndDocker verifies development ports follow the service's actual render gate.
func TestMailpitEnvironmentSectionRequiresMailAndDocker(t *testing.T) {
	tests := []struct {
		name       string
		components project.Components
		want       bool
	}{
		{name: "Mail and Docker", components: project.Components{Mail: true, Docker: true}, want: true},
		{name: "Mail only", components: project.Components{Mail: true}},
		{name: "Docker only", components: project.Components{Docker: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := defaultResourcePlanForTest(t, test.components)
			environment, _ := renderResourceTemplates(t, test.components, plan, project.LocalServiceIntent{})
			got := strings.Contains(environment, "\n# Mailpit\n")
			if got != test.want {
				t.Fatalf("Mailpit section presence = %t, want %t:\n%s", got, test.want, environment)
			}
		})
	}
}

// TestEventsEnvironmentUsesProjectEnvelopeAndAppParticipation verifies initial renders omit Events only when every App disables it.
func TestEventsEnvironmentUsesProjectEnvelopeAndAppParticipation(t *testing.T) {
	tests := []struct {
		name       string
		config     *project.Config
		wantEvents bool
	}{
		{
			name: "all Apps disabled",
			config: &project.Config{
				ProjectName:  "No Events",
				GoModuleName: "example.test/no-events",
				Render:       project.RenderConfig{Components: project.Components{CLI: true}},
			},
		},
		{
			name: "named App enabled",
			config: &project.Config{
				ProjectName:  "Named Events",
				GoModuleName: "example.test/named-events",
				Render:       project.RenderConfig{Components: project.Components{CLI: true}},
				Apps: map[string]project.AppConfig{
					"events-worker": {Components: project.Components{CLI: true, Events: true}},
				},
			},
			wantEvents: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			components := project.ProjectComponents(test.config)
			plan := defaultResourcePlanForTest(t, components)
			renderer := unitProjectRenderer(t)
			renderer.config = test.config
			renderer.resources.plan = plan
			renderer.stats = &renderStats{}
			path := filepath.Join(t.TempDir(), ".env")
			if err := renderer.renderTemplateFile(path, ".env.tmpl", test.config); err != nil {
				t.Fatalf("render environment: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read environment: %v", err)
			}
			text := string(data)
			if strings.Contains(text, "EVENTS_DRIVER=") != test.wantEvents {
				t.Fatalf("root Events environment presence = %t, want %t:\n%s", strings.Contains(text, "EVENTS_DRIVER="), test.wantEvents, text)
			}
			if test.wantEvents && (!strings.Contains(text, "EVENTS_SUPPORTED_DRIVERS=inproc,redis") || !strings.Contains(text, "EVENTS_WORKER_EVENTS_DRIVER=inproc")) {
				t.Fatalf("named-App Events environment omitted project support or App activation:\n%s", text)
			}
		})
	}
}

// TestResourceTemplatesKeepDatabaseIndependent verifies other resource defaults do not select the database.
func TestResourceTemplatesKeepDatabaseIndependent(t *testing.T) {
	components := project.Components{Docker: true, DatabasePostgres: true, Cache: true}
	plan := defaultResourcePlanForTest(t, components)
	environment, compose := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
	for _, want := range []string{"DB_DRIVER=postgres", "DB_SUPPORTED_DRIVERS=postgres", "CACHE_DRIVER=memory"} {
		if !strings.Contains(environment, want) {
			t.Fatalf("environment omitted %q:\n%s", want, environment)
		}
	}
	if !strings.Contains(compose, "  postgres:") || strings.Contains(compose, "  mysql:") {
		t.Fatalf("Compose did not follow the active database plan:\n%s", compose)
	}
	if strings.Contains(environment, "QUEUE_DRIVER=") {
		t.Fatalf("Jobs-disabled environment contains queue settings:\n%s", environment)
	}
}

// TestResourceTemplatesRenderDatabasePlanMatrix covers every normal database with local and Redis-active defaults.
func TestResourceTemplatesRenderDatabasePlanMatrix(t *testing.T) {
	databases := []struct {
		name       string
		components project.Components
		driver     string
		service    string
	}{
		{name: "mysql", components: project.Components{DatabaseMySQL: true, Cache: true}, driver: "mysql", service: "mysql"},
		{name: "postgres", components: project.Components{DatabasePostgres: true, Cache: true}, driver: "postgres", service: "postgres"},
		{name: "sqlite", components: project.Components{DatabaseSQLite: true, Cache: true}, driver: "sqlite"},
	}
	for _, redisActive := range []bool{false, true} {
		for _, database := range databases {
			name := "default_" + database.name
			if redisActive {
				name = "redis_" + database.name
			}
			t.Run(name, func(t *testing.T) {
				components := database.components
				components.Docker = true
				plan := defaultResourcePlanForTest(t, components)
				if redisActive {
					plan = redisResourcePlanForTest(t, components)
				}
				intent := project.LocalServiceIntent{}
				if redisActive {
					intent = intent.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
				}
				environment, compose := renderResourceTemplates(t, components, plan, intent)
				if !strings.Contains(environment, "DB_DRIVER="+database.driver) || !strings.Contains(environment, "DB_SUPPORTED_DRIVERS="+database.driver) {
					t.Fatalf("database contract mismatch:\n%s", environment)
				}
				for _, candidate := range []string{"mysql", "postgres"} {
					contains := strings.Contains(compose, "\n  "+candidate+":\n")
					if contains != (candidate == database.service) {
						t.Fatalf("Compose service %s presence=%t, selected=%q:\n%s", candidate, contains, database.service, compose)
					}
				}
				wantCache := "CACHE_DRIVER=memory"
				wantProfile := false
				if redisActive {
					wantCache = "CACHE_DRIVER=redis"
					wantProfile = true
				}
				if !strings.Contains(environment, wantCache) || strings.Contains(environment, "COMPOSE_PROFILES=redis") != wantProfile {
					t.Fatalf("shape projection mismatch:\n%s", environment)
				}
			})
		}
	}
}

// TestResourceTemplatesOmitDisabledCapabilities keeps concise resource choices from introducing unrelated services.
func TestResourceTemplatesOmitDisabledCapabilities(t *testing.T) {
	components := project.Components{DatabaseSQLite: true}
	plan := redisResourcePlanForTest(t, components)
	environment, compose := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal))
	for _, forbidden := range []string{
		"QUEUE_DRIVER=", "MAIL_DRIVER=", "MAILPIT_", "SCHEDULER_", "IP_ADDRESS=",
		"# API\n", "# Lighthouse\n", "# Mailpit\n", "# Runtime\n", "# Docker\n",
	} {
		if strings.Contains(environment, forbidden) {
			t.Fatalf("disabled capability emitted %q:\n%s", forbidden, environment)
		}
	}
	for _, forbidden := range []string{"\n  redis:\n", "\n  mysql:\n", "\n  postgres:\n", "\n  mailpit:\n"} {
		if strings.Contains(compose, forbidden) {
			t.Fatalf("Docker-disabled project emitted service %q:\n%s", forbidden, compose)
		}
	}
}

// TestResourceTemplatesRetainLocalDatabaseForDockerProject keeps the first-slice database policy intentionally brainless.
func TestResourceTemplatesRetainLocalDatabaseForDockerProject(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	consumers, err := resourceenv.ResolveConsumers([]byte("DB_DRIVER=mysql\nDB_HOST=db.example.internal\nDB_PORT=3306\n"), plan, components, nil)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	values, err := resourceRenderValuesForPlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
	if err != nil {
		t.Fatalf("resourceRenderValuesForPlanWithConsumers returned error: %v", err)
	}
	if !values.DatabaseMySQL || values.DatabaseExternal {
		t.Fatalf("database render policy = mysql:%t external:%t, want Docker-managed MySQL", values.DatabaseMySQL, values.DatabaseExternal)
	}

	_, compose := renderResourceTemplatesWithConsumers(t, components, plan, project.LocalServiceIntent{}, consumers)
	if !strings.Contains(compose, "\n  mysql:\n") {
		t.Fatalf("Docker-enabled MySQL project omitted its local development service:\n%s", compose)
	}
}

// TestResourceTemplatesOmitUnownedAlternateAppDatabase keeps external App engines out of the root Compose contract.
func TestResourceTemplatesOmitUnownedAlternateAppDatabase(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true}
	plan := defaultResourcePlanForTest(t, components)
	database, _ := plan.Selection(project.ResourceDatabase)
	database.Supported = append(database.Supported, "mysql")
	plan = plan.WithSelection(project.ResourceDatabase, database)
	config := &project.Config{
		Render: project.RenderConfig{Components: components},
		Apps: map[string]project.AppConfig{
			"billing": {Components: components},
		},
	}
	source := []byte("DB_DRIVER=sqlite\nBILLING_DB_DRIVER=mysql\nBILLING_DB_HOST=mysql.billing.example\n")
	consumers, err := resourceenv.ResolveConsumers(source, plan, components, config)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}

	_, compose := renderResourceTemplatesWithConsumers(t, components, plan, project.LocalServiceIntent{}, consumers)
	if strings.Contains(compose, "\n  mysql:\n") {
		t.Fatalf("SQLite root emitted an unowned MySQL Compose service:\n%s", compose)
	}
}

// TestProjectRendererConsumesExplicitResourcePlan verifies the wizard handoff reaches environment, Compose, and generated source together.
func TestProjectRendererConsumesExplicitResourcePlan(t *testing.T) {
	previousDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDirectory) })
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatalf("change to render directory: %v", err)
	}

	components := project.Components{CLI: true, Docker: true, DatabaseSQLite: true, Jobs: true, Events: true, Cache: true}
	config := project.Config{
		ProjectName:  "Shared Resource App",
		GoModuleName: "example.com/shared-resource-app",
		Render: project.RenderConfig{
			Components: components,
		},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal project config: %v", err)
	}
	if err := os.WriteFile(".goforj.yml", encoded, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	plan := redisResourcePlanForTest(t, components)
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	renderer := unitProjectRenderer(t)
	if err := renderer.Render(ComponentRenderInput{renderAll: true, resourcePlan: plan, localServiceIntent: intent}); err != nil {
		t.Fatalf("render explicit resource plan: %v", err)
	}

	environment, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read rendered environment: %v", err)
	}
	for _, want := range []string{
		"CACHE_DRIVER=redis",
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"QUEUE_DRIVER=redis",
		"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
		"EVENTS_DRIVER=redis",
		"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
		"COMPOSE_PROFILES=redis",
	} {
		if !strings.Contains(string(environment), want) {
			t.Fatalf("rendered environment omitted %q:\n%s", want, environment)
		}
	}
	compose, err := os.ReadFile("docker-compose.yml")
	if err != nil {
		t.Fatalf("read rendered Compose: %v", err)
	}
	if strings.Count(string(compose), "\n  redis:\n") != 2 || !strings.Contains(string(compose), "profiles: [redis]") {
		t.Fatalf("rendered Compose did not contain one Redis volume and service:\n%s", compose)
	}
	cacheManager, err := os.ReadFile(filepath.Join("internal", "caches", "manager_gen.go"))
	if err != nil {
		t.Fatalf("read generated cache manager: %v", err)
	}
	if !strings.Contains(string(cacheManager), "driver/rediscache") {
		t.Fatalf("generated cache manager omitted built-in Redis bridge:\n%s", cacheManager)
	}
	projectConfig, err := os.ReadFile(".goforj.yml")
	if err != nil {
		t.Fatalf("read rewritten project config: %v", err)
	}
	for _, forbidden := range []string{"resource_shape:", "resource_plan:", "queue_driver:"} {
		if strings.Contains(string(projectConfig), forbidden) {
			t.Fatalf("transient resource state %q leaked into project YAML:\n%s", forbidden, projectConfig)
		}
	}
	if err := os.Remove(".env"); err != nil {
		t.Fatalf("remove owner environment for clean-checkout rerender: %v", err)
	}
	cleanCheckoutRenderer := unitProjectRenderer(t)
	if err := cleanCheckoutRenderer.Render(ComponentRenderInput{renderAll: true}); err != nil {
		t.Fatalf("rerender clean checkout from environment example: %v", err)
	}
	cleanEnvironment, err := os.ReadFile(".env")
	if err != nil {
		t.Fatalf("read recreated environment: %v", err)
	}
	for _, want := range []string{
		"CACHE_SUPPORTED_DRIVERS=memory,redis",
		"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
		"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
		"COMPOSE_PROFILES=redis",
	} {
		if !strings.Contains(string(cleanEnvironment), want) {
			t.Fatalf("clean-checkout rerender narrowed %q:\n%s", want, cleanEnvironment)
		}
	}
}

// renderResourceTemplates renders only the environment and Compose surfaces into a temporary test directory.
func renderResourceTemplates(t *testing.T, components project.Components, plan project.ResourcePlan, intent project.LocalServiceIntent) (string, string) {
	return renderResourceTemplatesWithConsumers(t, components, plan, intent, nil)
}

// renderResourceTemplatesWithConsumers renders resource surfaces with the same endpoint records used by full-project rerenders.
func renderResourceTemplatesWithConsumers(t *testing.T, components project.Components, plan project.ResourcePlan, intent project.LocalServiceIntent, consumers []project.EffectiveResourceConsumer) (string, string) {
	t.Helper()
	root := t.TempDir()
	config := &project.Config{
		ProjectName:  "Resource Plan",
		GoModuleName: "example.com/resource-plan",
		Render:       project.RenderConfig{Components: components},
	}
	renderer := unitProjectRenderer(t)
	renderer.config = config
	renderer.resources = resourceRenderState{
		plan:             plan,
		serviceIntent:    intent,
		serviceConsumers: cloneEffectiveResourceConsumers(consumers),
	}
	renderer.stats = &renderStats{}
	environmentPath := filepath.Join(root, ".env")
	if err := renderer.renderTemplateFile(environmentPath, ".env.tmpl", config); err != nil {
		t.Fatalf("render environment: %v", err)
	}
	composePath := filepath.Join(root, "docker-compose.yml")
	if err := renderer.renderTemplateFile(composePath, "docker-compose.yml.tmpl", config); err != nil {
		t.Fatalf("render Compose: %v", err)
	}
	environment, err := os.ReadFile(environmentPath)
	if err != nil {
		t.Fatalf("read environment: %v", err)
	}
	compose, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("read Compose: %v", err)
	}
	return string(environment), string(compose)
}
