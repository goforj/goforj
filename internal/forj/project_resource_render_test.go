package forj

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestResourceTemplatesRenderNormalShapes verifies environment and Compose consume the same typed plan.
func TestResourceTemplatesRenderNormalShapes(t *testing.T) {
	components := project.Components{
		CLI:           true,
		Docker:        true,
		Mail:          true,
		Auth:          true,
		DatabaseMySQL: true,
		Jobs:          true,
	}
	tests := []struct {
		name       string
		shape      project.StartingResourceShape
		wantEnv    []string
		forbidEnv  []string
		redisLocal bool
	}{
		{
			name:  "standalone",
			shape: project.ResourceShapeStandalone,
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
			forbidEnv: []string{"COMPOSE_PROFILES=redis"},
		},
		{
			name:       "shared",
			shape:      project.ResourceShapeSharedRedis,
			redisLocal: true,
			wantEnv: []string{
				"CACHE_DRIVER=redis",
				"CACHE_SUPPORTED_DRIVERS=memory,redis",
				"QUEUE_DRIVER=redis",
				"QUEUE_SUPPORTED_DRIVERS=workerpool,redis",
				"EVENTS_DRIVER=redis",
				"EVENTS_SUPPORTED_DRIVERS=inproc,redis",
				"CACHE_SESSIONS_DRIVER=redis",
				"COMPOSE_PROFILES=redis",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan, err := project.ResolveResourcePlan(test.shape, components)
			if err != nil {
				t.Fatalf("ResolveResourcePlan returned error: %v", err)
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

// TestResourceTemplatesKeepDatabaseIndependent verifies resource shape changes do not select the database.
func TestResourceTemplatesKeepDatabaseIndependent(t *testing.T) {
	components := project.Components{Docker: true, DatabasePostgres: true}
	plan, err := project.ResolveResourcePlan(project.ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("ResolveResourcePlan returned error: %v", err)
	}
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

// TestResourceTemplatesRenderDatabaseShapeMatrix covers every normal database and starting-shape combination.
func TestResourceTemplatesRenderDatabaseShapeMatrix(t *testing.T) {
	databases := []struct {
		name       string
		components project.Components
		driver     string
		service    string
	}{
		{name: "mysql", components: project.Components{DatabaseMySQL: true}, driver: "mysql", service: "mysql"},
		{name: "postgres", components: project.Components{DatabasePostgres: true}, driver: "postgres", service: "postgres"},
		{name: "sqlite", components: project.Components{DatabaseSQLite: true}, driver: "sqlite"},
	}
	for _, shape := range []project.StartingResourceShape{project.ResourceShapeStandalone, project.ResourceShapeSharedRedis} {
		for _, database := range databases {
			t.Run(string(shape)+"_"+database.name, func(t *testing.T) {
				components := database.components
				components.Docker = true
				plan, err := project.ResolveResourcePlan(shape, components)
				if err != nil {
					t.Fatalf("resolve plan: %v", err)
				}
				intent := project.LocalServiceIntent{}
				if shape == project.ResourceShapeSharedRedis {
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
				if shape == project.ResourceShapeSharedRedis {
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
	plan, err := project.ResolveResourcePlan(project.ResourceShapeSharedRedis, components)
	if err != nil {
		t.Fatalf("resolve plan: %v", err)
	}
	environment, compose := renderResourceTemplates(t, components, plan, project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeExternal))
	for _, forbidden := range []string{"QUEUE_DRIVER=", "MAIL_DRIVER=", "MAILPIT_"} {
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
	plan, err := project.ResolveResourcePlan(project.ResourceShapeStandalone, components)
	if err != nil {
		t.Fatalf("resolve plan: %v", err)
	}
	consumers, err := effectiveResourceConsumersFromEnvironment([]byte("DB_DRIVER=mysql\nDB_HOST=db.example.internal\nDB_PORT=3306\n"), plan, components, nil)
	if err != nil {
		t.Fatalf("discover effective consumers: %v", err)
	}
	values := resourceRenderValuesForPlanWithConsumers(plan, components, project.LocalServiceIntent{}, consumers)
	if !values.DatabaseMySQL || values.DatabaseExternal {
		t.Fatalf("database render policy = mysql:%t external:%t, want Docker-managed MySQL", values.DatabaseMySQL, values.DatabaseExternal)
	}

	_, compose := renderResourceTemplatesWithConsumers(t, components, plan, project.LocalServiceIntent{}, consumers)
	if !strings.Contains(compose, "\n  mysql:\n") {
		t.Fatalf("Docker-enabled MySQL project omitted its local development service:\n%s", compose)
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

	components := project.Components{CLI: true, Docker: true, DatabaseSQLite: true, Jobs: true}
	config := project.Config{
		ProjectName:  "Shared Resource App",
		GoModuleName: "example.com/shared-resource-app",
		Render:       project.RenderConfig{Components: components},
	}
	encoded, err := yaml.Marshal(config)
	if err != nil {
		t.Fatalf("marshal project config: %v", err)
	}
	if err := os.WriteFile(".goforj.yml", encoded, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	plan, err := project.ResolveResourcePlan(project.ResourceShapeSharedRedis, components)
	if err != nil {
		t.Fatalf("resolve resource plan: %v", err)
	}
	intent := project.LocalServiceIntent{}.WithMode(project.ServiceRedis, project.LocalServiceModeLocal)
	renderer := NewProjectRenderer(logger.NewSilentLogger())
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
	cleanCheckoutRenderer := NewProjectRenderer(logger.NewSilentLogger())
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
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.config = config
	renderer.resourcePlan = plan
	renderer.localServiceIntent = intent
	renderer.serviceConsumers = cloneEffectiveResourceConsumers(consumers)
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
