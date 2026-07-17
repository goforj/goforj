package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/envfile"
	"github.com/goforj/goforj/project"
)

// TestSupportOnlyRedisOmitsTopologyAssignments keeps compiled alternatives from cluttering runtime configuration.
func TestSupportOnlyRedisOmitsTopologyAssignments(t *testing.T) {
	tests := []struct {
		name          string
		components    project.Components
		wantCachePair bool
	}{
		{
			name:       "unrelated storage",
			components: project.Components{CLI: true, Docker: true, Storage: true},
		},
		{
			name:          "portable cache",
			components:    project.Components{CLI: true, Cache: true},
			wantCachePair: true,
		},
		{
			name:          "compose cache",
			components:    project.Components{CLI: true, Docker: true, Cache: true},
			wantCachePair: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := defaultResourcePlanForTest(t, test.components)
			environment, _ := renderResourceTemplates(t, test.components, plan, project.LocalServiceIntent{})
			hostEnvironment := renderResourceHostEnvironment(t, test.components, plan, project.LocalServiceIntent{})
			for path, source := range map[string]string{".env": environment, ".env.host": hostEnvironment} {
				if strings.Contains(source, "REDIS_") || strings.Contains(source, "COMPOSE_PROFILES=redis") {
					t.Fatalf("support-only Redis projected topology into %s:\n%s", path, source)
				}
			}
			if test.wantCachePair {
				lines := strings.Split(environment, "\n")
				active, activeSet := envfile.Lookup(lines, "CACHE_DRIVER")
				supported, supportedSet := envfile.Lookup(lines, "CACHE_SUPPORTED_DRIVERS")
				if !activeSet || active != "memory" || !supportedSet || supported != "memory,redis" {
					t.Fatalf("support-only Redis lost the Cache driver contract:\n%s", environment)
				}
			}
		})
	}
}

// TestAdvancedResourceSelectionsDoNotProjectPlaceholderComments keeps driver manifests without a commented configuration wall.
func TestAdvancedResourceSelectionsDoNotProjectPlaceholderComments(t *testing.T) {
	components := project.Components{Cache: true, DatabaseSQLite: true, Docker: true, Events: true, Jobs: true, Mail: true, Storage: true}
	plan := defaultResourcePlanForTest(t, components)
	plan = resourceContractTestSelection(plan, project.ResourceCache, "memcached", "memcached")
	plan = resourceContractTestSelection(plan, project.ResourceQueue, "workerpool", "sqs")
	plan = resourceContractTestSelection(plan, project.ResourceEvents, "kafka", "kafka")
	plan = resourceContractTestSelection(plan, project.ResourceStorage, "s3", "s3")
	plan = plan.WithNamedSelection("STORAGE_PUBLIC_DRIVER", "s3")
	plan = resourceContractTestSelection(plan, project.ResourceMail, "resend", "resend")
	if err := plan.Validate(components); err != nil {
		t.Fatalf("custom plan is invalid: %v", err)
	}

	environment, _ := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
	environmentExample := string(envfile.RedactExample([]byte(environment)))
	wantSelections := map[string]string{
		"CACHE_DRIVER":              "memcached",
		"CACHE_SUPPORTED_DRIVERS":   "memory,redis,memcached",
		"QUEUE_DRIVER":              "workerpool",
		"QUEUE_SUPPORTED_DRIVERS":   "workerpool,redis,sqs",
		"EVENTS_DRIVER":             "kafka",
		"EVENTS_SUPPORTED_DRIVERS":  "inproc,redis,kafka",
		"STORAGE_DRIVER":            "s3",
		"STORAGE_SUPPORTED_DRIVERS": "local,s3",
		"STORAGE_PUBLIC_DRIVER":     "s3",
		"MAIL_DRIVER":               "resend",
		"MAIL_SUPPORTED_DRIVERS":    "log,smtp,resend",
	}
	for path, source := range map[string]string{".env": environment, ".env.example": environmentExample} {
		lines := strings.Split(source, "\n")
		for key, want := range wantSelections {
			got, set := envfile.Lookup(lines, key)
			if !set || got != want {
				t.Errorf("%s %s = %q, %t; want %q:\n%s", path, key, got, set, want, source)
			}
		}
	}
	for _, unwanted := range []string{
		"# Advanced driver configuration",
		"# CACHE_ADDRESSES=",
		"# QUEUE_REGION=us-east-1",
		"# QUEUE_ACCESS_KEY=",
		"# EVENTS_BROKERS=127.0.0.1:9092",
		"# STORAGE_BUCKET=",
		"# STORAGE_SECRET_ACCESS_KEY=",
		"# MAIL_RESEND_API_KEY=",
	} {
		if strings.Contains(environment, unwanted) {
			t.Errorf("rendered environment projected placeholder %q:\n%s", unwanted, environment)
		}
		if strings.Contains(environmentExample, unwanted) {
			t.Errorf("rendered environment example projected placeholder %q:\n%s", unwanted, environmentExample)
		}
	}
}

// TestDriverEnvironmentPlaceholdersDeduplicateSharedScopeKeys verifies SQL alternatives do not produce conflicting assignments.
func TestDriverEnvironmentPlaceholdersDeduplicateSharedScopeKeys(t *testing.T) {
	components := project.Components{DatabaseSQLite: true}
	plan := defaultResourcePlanForTest(t, components)
	cache, _ := plan.Selection(project.ResourceCache)
	cache.Supported = append(cache.Supported, "mysql", "postgres")
	plan = plan.WithSelection(project.ResourceCache, cache)
	values, err := resourceRenderValuesForPlanWithConsumers(plan, components, project.LocalServiceIntent{}, nil)
	if err != nil {
		t.Fatalf("resourceRenderValuesForPlanWithConsumers returned error: %v", err)
	}
	placeholders := values.DriverEnvironmentPlaceholders()
	if count := strings.Count(placeholders, "# CACHE_DSN="); count != 1 {
		t.Fatalf("CACHE_DSN placeholder count = %d, want one:\n%s", count, placeholders)
	}
	if !strings.Contains(placeholders, "# Cache · Postgres") || strings.Contains(placeholders, "# Cache · MySQL") {
		t.Fatalf("catalog-first supported driver did not own CACHE_DSN:\n%s", placeholders)
	}
}

// TestDriverEnvironmentPlaceholdersPreferActiveSharedKeyExample keeps transition support from advertising the wrong active endpoint.
func TestDriverEnvironmentPlaceholdersPreferActiveSharedKeyExample(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Jobs: true}
	plan := defaultResourcePlanForTest(t, components)
	queue, _ := plan.Selection(project.ResourceQueue)
	queue.Active = "rabbitmq"
	queue.Supported = append(queue.Supported, "nats", "rabbitmq")
	plan = plan.WithSelection(project.ResourceQueue, queue)
	values, err := resourceRenderValuesForPlanWithConsumers(plan, components, project.LocalServiceIntent{}, nil)
	if err != nil {
		t.Fatalf("resourceRenderValuesForPlanWithConsumers returned error: %v", err)
	}
	placeholders := values.DriverEnvironmentPlaceholders()
	if count := strings.Count(placeholders, "# QUEUE_URL="); count != 1 {
		t.Fatalf("QUEUE_URL placeholder count = %d, want one:\n%s", count, placeholders)
	}
	if !strings.Contains(placeholders, "# QUEUE_URL=amqp://guest:guest@127.0.0.1:5672/") {
		t.Fatalf("active RabbitMQ example was omitted:\n%s", placeholders)
	}
	if strings.Contains(placeholders, "# QUEUE_URL=nats://") || strings.Contains(placeholders, "# Queue · NATS") {
		t.Fatalf("supported NATS displaced the active RabbitMQ example:\n%s", placeholders)
	}
}

// TestExternalResourceTemplatesDoNotInventLocalHosts keeps external placement actionable instead of pointing at absent Compose services.
func TestExternalResourceTemplatesDoNotInventLocalHosts(t *testing.T) {
	t.Run("Redis", func(t *testing.T) {
		components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true}
		plan := redisResourcePlanForTest(t, components)
		intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal)
		environment, _ := renderResourceTemplates(t, components, plan, intent)
		hostEnvironment := renderResourceHostEnvironment(t, components, plan, intent)
		if !strings.Contains(environment, "\nREDIS_HOST=\n") || strings.Contains(environment, "\nREDIS_HOST=redis\n") {
			t.Fatalf("external Redis received a local hostname:\n%s", environment)
		}
		if strings.Contains(hostEnvironment, "REDIS_HOST=localhost") {
			t.Fatalf("host overrides redirected external Redis to localhost:\n%s", hostEnvironment)
		}
	})

	for _, database := range []string{"mysql", "postgres"} {
		t.Run(database, func(t *testing.T) {
			components := project.Components{}
			if database == "mysql" {
				components.DatabaseMySQL = true
			} else {
				components.DatabasePostgres = true
			}
			plan := defaultResourcePlanForTest(t, components)
			environment, _ := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
			hostEnvironment := renderResourceHostEnvironment(t, components, plan, project.LocalServiceIntent{})
			if !strings.Contains(environment, "\nDB_HOST=\n") || strings.Contains(environment, "\nDB_HOST="+database+"\n") {
				t.Fatalf("external %s received a local hostname:\n%s", database, environment)
			}
			if strings.Contains(hostEnvironment, "DB_HOST=localhost") {
				t.Fatalf("host overrides redirected external %s to localhost:\n%s", database, hostEnvironment)
			}
		})
	}
}

// TestRedisTopologyRendersOnlyForActiveOrLocalIntent keeps profile and endpoint state tied to actual placement.
func TestRedisTopologyRendersOnlyForActiveOrLocalIntent(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	plan := defaultResourcePlanForTest(t, components)

	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	t.Run("local service requested without an active consumer", func(t *testing.T) {
		environment, _ := renderResourceTemplates(t, components, plan, intent)
		hostEnvironment := renderResourceHostEnvironment(t, components, plan, intent)
		if !strings.Contains(environment, "COMPOSE_PROFILES=redis") {
			t.Fatalf("local Redis intent omitted its exact Compose profile:\n%s", environment)
		}
		if strings.Contains(environment, "REDIS_HOST=") || strings.Contains(environment, "REDIS_PORT=") {
			t.Fatalf("inactive Redis consumer projected root endpoint assignments:\n%s", environment)
		}
		if !strings.Contains(hostEnvironment, "REDIS_HOST=localhost") || !strings.Contains(hostEnvironment, "REDIS_PORT=6379") {
			t.Fatalf("requested local Redis omitted host topology:\n%s", hostEnvironment)
		}
	})

	t.Run("active local consumer", func(t *testing.T) {
		activePlan := redisResourcePlanForTest(t, components)
		environment, _ := renderResourceTemplates(t, components, activePlan, intent)
		hostEnvironment := renderResourceHostEnvironment(t, components, activePlan, intent)
		for _, want := range []string{
			"COMPOSE_PROFILES=redis", "REDIS_HOST=redis", "REDIS_PORT=6379",
			"REDIS_PASSWORD=\n", "REDIS_DB=0\n",
		} {
			if !strings.Contains(environment, want) {
				t.Fatalf("active local Redis omitted %q:\n%s", want, environment)
			}
		}
		if !strings.Contains(hostEnvironment, "REDIS_HOST=localhost") || !strings.Contains(hostEnvironment, "REDIS_PORT=6379") {
			t.Fatalf("active local Redis omitted host topology:\n%s", hostEnvironment)
		}
	})
}

// resourceContractTestSelection replaces one active driver while retaining every existing built-in choice.
func resourceContractTestSelection(plan project.ResourcePlan, key project.ResourceKey, active string, supported string) project.ResourcePlan {
	selection, _ := plan.Selection(key)
	selection.Active = active
	if !stringSliceContainsFold(selection.Supported, supported) {
		selection.Supported = append(selection.Supported, supported)
	}
	return plan.WithSelection(key, selection)
}

// renderResourceHostEnvironment renders the host-only network overrides for one transient resource contract.
func renderResourceHostEnvironment(t *testing.T, components project.Components, plan project.ResourcePlan, intent project.LocalServiceIntent) string {
	t.Helper()
	root := t.TempDir()
	config := &project.Config{
		ProjectName:  "Resource Plan",
		GoModuleName: "example.com/resource-plan",
		Render:       project.RenderConfig{Components: components},
	}
	renderer := unitProjectRenderer(t)
	renderer.config = config
	renderer.resources = resourceRenderState{plan: plan, serviceIntent: intent}
	renderer.stats = &renderStats{}
	path := filepath.Join(root, ".env.host")
	if err := renderer.renderTemplateFile(path, ".env.host.tmpl", config); err != nil {
		t.Fatalf("render host environment: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read host environment: %v", err)
	}
	return string(contents)
}
