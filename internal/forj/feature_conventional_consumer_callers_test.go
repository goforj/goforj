package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestProjectDescribeConventionalResourceNamedApps includes filesystem App ownership in service discovery.
func TestProjectDescribeConventionalResourceNamedApps(t *testing.T) {
	root, config, _ := conventionalFeatureConsumerFixture(t)
	requirements, err := projectDescribeServiceRequirements(root, config)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 4 {
		t.Fatalf("service requirements = %#v, want available Redis and three external endpoints", requirements)
	}
	var owners []string
	for _, requirement := range requirements {
		if requirement.ServiceKey == "redis" && requirement.Owner == "available" && len(requirement.Consumers) == 0 {
			continue
		}
		if requirement.ServiceKey != "redis" || requirement.Kind != "cache" || requirement.Driver != "redis" || requirement.Owner != "external" || len(requirement.Consumers) != 1 {
			t.Fatalf("unexpected service requirement: %#v", requirement)
		}
		owners = append(owners, requirement.Consumers[0])
	}
	sort.Strings(owners)
	if !reflect.DeepEqual(owners, []string{"app", "feature", "feature-child"}) {
		t.Fatalf("Redis App consumers = %#v", owners)
	}
}

// TestPrepareNewProjectConventionalResourceNamedApps discovers existing target Apps independently of the current directory.
func TestPrepareNewProjectConventionalResourceNamedApps(t *testing.T) {
	root, config, source := conventionalFeatureConsumerFixture(t)
	components := config.Render.Components
	plan := defaultResourcePlanForTest(t, components)
	result, err := prepareNewProjectTargetResources(root, plan, components, project.LocalServiceIntent{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.servicePlan.Requirements) != 4 {
		t.Fatalf("service requirements = %#v, want available Redis and three external endpoints", result.servicePlan.Requirements)
	}
	var consumers []string
	for _, requirement := range result.servicePlan.Requirements {
		if requirement.Key == project.ServiceRedis && requirement.State == project.ServiceStateAvailableLocal && len(requirement.ActiveConsumers) == 0 {
			continue
		}
		if requirement.Key != project.ServiceRedis || requirement.State != project.ServiceStateExternalRequired || len(requirement.ActiveConsumers) != 1 {
			t.Fatalf("unexpected service requirement: %#v", requirement)
		}
		consumers = append(consumers, requirement.ActiveConsumers[0])
	}
	sort.Strings(consumers)
	if !reflect.DeepEqual(consumers, []string{"cache", "feature-child:cache", "feature:cache"}) {
		t.Fatalf("Redis resource consumers = %#v", consumers)
	}
	if after, err := os.ReadFile(filepath.Join(root, ".env")); err != nil || string(after) != source {
		t.Fatalf("target environment changed: %q, %v", after, err)
	}
}

// conventionalFeatureConsumerFixture separates each App's Redis endpoint so ownership errors cannot collapse into one requirement.
func conventionalFeatureConsumerFixture(t *testing.T) (string, *project.Config, string) {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"feature", "feature-child"} {
		directory := filepath.Join(root, "cmd", name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "main.go"), []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	config := &project.Config{ProjectName: "Existing", GoModuleName: "example.com/existing", Render: project.RenderConfig{Components: project.Components{CLI: true, Feature: true, Cache: true, Docker: true}}}
	if err := writeProjectConfig(filepath.Join(root, ".goforj.yml"), config); err != nil {
		t.Fatal(err)
	}
	source := "FEATURE_DRIVER=memory\nCACHE_DRIVER=redis\nCACHE_SUPPORTED_DRIVERS=memory,redis\nCACHE_ADDR=root.example:6379\nFEATURE_FEATURE_DRIVER=file\nFEATURE_CACHE_DRIVER=redis\nFEATURE_CACHE_ADDR=feature.example:6379\nFEATURE_CHILD_FEATURE_DRIVER=memory\nFEATURE_CHILD_CACHE_DRIVER=redis\nFEATURE_CHILD_CACHE_ADDR=child.example:6379\n"
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return root, config, source
}
