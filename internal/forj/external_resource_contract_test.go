package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

// TestDefaultResourceTemplatesOmitAdvancedPlaceholderWall keeps the brainless default compact.
func TestDefaultResourceTemplatesOmitAdvancedPlaceholderWall(t *testing.T) {
	components := project.Components{DatabaseMySQL: true, Docker: true, Jobs: true, Mail: true}
	plan := defaultResourcePlanForTest(t, components)
	environment, _ := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
	if strings.Contains(environment, "Advanced driver configuration") {
		t.Fatalf("normal environment gained Advanced placeholders:\n%s", environment)
	}
}

// TestAdvancedResourceTemplatesEmitSelectedSafePlaceholders verifies active and built-in opt-in drivers receive actionable dotenv hints.
func TestAdvancedResourceTemplatesEmitSelectedSafePlaceholders(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Jobs: true, Mail: true, Storage: true}
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
	environmentExample := string(RenderEnvironmentExample([]byte(environment)))
	if !strings.Contains(environment, "STORAGE_PUBLIC_DRIVER=s3") {
		t.Fatalf("rendered environment ignored the generated public-storage selection:\n%s", environment)
	}
	for _, want := range []string{
		"# Advanced driver configuration",
		"# CACHE_ADDRESSES=",
		"# QUEUE_REGION=us-east-1",
		"# QUEUE_ACCESS_KEY=",
		"# EVENTS_BROKERS=127.0.0.1:9092",
		"# STORAGE_BUCKET=",
		"# STORAGE_SECRET_ACCESS_KEY=",
		"# MAIL_RESEND_API_KEY=",
	} {
		if !strings.Contains(environment, want) {
			t.Errorf("rendered environment omitted %q:\n%s", want, environment)
		}
		if !strings.Contains(environmentExample, want) {
			t.Errorf("rendered environment example omitted %q:\n%s", want, environmentExample)
		}
	}
	for _, unwanted := range []string{"# STORAGE_TOKEN=", "# MAIL_POSTMARK_SERVER_TOKEN="} {
		if strings.Contains(environment, unwanted) {
			t.Errorf("rendered environment included unselected placeholder %q:\n%s", unwanted, environment)
		}
	}
	for _, secretKey := range []string{"QUEUE_ACCESS_KEY", "STORAGE_SECRET_ACCESS_KEY", "MAIL_RESEND_API_KEY"} {
		if strings.Contains(environment, "\n"+secretKey+"=") {
			t.Errorf("secret placeholder %s was emitted as an active assignment", secretKey)
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

// TestPortableRedisTemplatesRetainLocalTransitionDefaults preserves the normal inactive bridge and explicit unused-local intent.
func TestPortableRedisTemplatesRetainLocalTransitionDefaults(t *testing.T) {
	components := project.Components{DatabaseSQLite: true, Docker: true, Cache: true}
	plan := defaultResourcePlanForTest(t, components)
	environment, _ := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{})
	hostEnvironment := renderResourceHostEnvironment(t, components, plan, project.LocalServiceIntent{})
	if !strings.Contains(environment, "\nREDIS_HOST=redis\n") || !strings.Contains(hostEnvironment, "REDIS_HOST=localhost") {
		t.Fatalf("portable Redis defaults were omitted:\n.env:\n%s\n.env.host:\n%s", environment, hostEnvironment)
	}
	if strings.Contains(environment, "COMPOSE_PROFILES=redis") {
		t.Fatalf("support-only Redis unexpectedly started locally:\n%s", environment)
	}

	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	environment, _ = renderResourceTemplates(t, components, plan, intent)
	if !strings.Contains(environment, "COMPOSE_PROFILES=redis") {
		t.Fatalf("owner-retained unused local Redis omitted its exact profile:\n%s", environment)
	}
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
	renderer := NewProjectRenderer(logger.NewSilentLogger())
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
