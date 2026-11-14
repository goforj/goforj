//go:build integration

package forj

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goforj/goforj/internal/testkit"
	"github.com/goforj/goforj/project"
)

// TestRenderedFeatureResourceComposition verifies the optional resource in disabled and complete generated Projects.
func TestRenderedFeatureResourceComposition(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		root := t.TempDir()
		testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: project.Config{ProjectName: "Without Features", GoModuleName: "example.com/withoutfeatures", Render: project.RenderConfig{Components: project.Components{CLI: true, WebAPI: true}}}})
		for _, path := range []string{"go.mod", ".env", ".env.example", "app/wire/app.go", "app/wire/inject_managers.go"} {
			body, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range []string{"github.com/goforj/feature", "FEATURE_", "internal/features"} {
				if strings.Contains(string(body), marker) {
					t.Fatalf("disabled feature leaked %s into %s", marker, path)
				}
			}
		}
		if _, err := os.Stat(filepath.Join(root, "internal/features")); !os.IsNotExist(err) {
			t.Fatal("disabled feature generated a package")
		}
	})
	t.Run("complete composition", func(t *testing.T) {
		root := t.TempDir()
		if err := writeConventionalAppMarker(root, "billing"); err != nil {
			t.Fatal(err)
		}
		components := project.DefaultSelectedComponents()
		components.DemoApp = true
		components.Feature = true
		config := project.Config{ProjectName: "Feature Composition", GoModuleName: "example.com/featurecomposition", Render: project.RenderConfig{Components: components}, Apps: map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, WebAPI: true, Feature: true}}}}
		testkit.RenderProjectWithForj(t, root, testkit.RenderProjectRequest{Config: config})
		for _, path := range []string{".env", ".env.example"} {
			body, err := os.ReadFile(filepath.Join(root, path))
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"FEATURE_REFRESH_INTERVAL=", "FEATURE_CHECKOUT_REFRESH_INTERVAL="} {
				if strings.Contains(string(body), key) {
					t.Fatalf("fallback tuning leaked into %s", path)
				}
			}
		}
		setRenderedEnvValue(t, root, "FEATURE_CHECKOUT_DRIVER", "memory")
		setRenderedEnvValue(t, root, "FEATURE_CHECKOUT_DOCUMENT", `'{"schema":1,"flags":{"checkout-v2":{"default":true}}}'`)
		setRenderedEnvValue(t, root, "BILLING_FEATURE_BILLING_ONLY_DRIVER", "memory")
		runFeatureRenderCommand(t, root, "generate", "--feature")
		before := featureGeneratedSnapshot(t, root)
		runFeatureRenderCommand(t, root, "generate", "--feature")
		after := featureGeneratedSnapshot(t, root)
		for path, body := range before {
			if after[path] != body {
				t.Fatalf("regeneration drift in %s", path)
			}
		}
		for _, app := range []string{"app", "billing"} {
			args := []string{"build"}
			if app != "app" {
				args = append([]string{app}, args...)
			}
			runFeatureRenderCommand(t, root, args...)
		}
		command := exec.Command("go", "test", "./...", "-run", "^$")
		command.Dir = root
		command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("compile complete feature composition: %v\n%s", err, output)
		}
		command = exec.Command("go", "list", "-m", "-json", "github.com/goforj/feature")
		command.Dir = root
		command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
		output, err := command.CombinedOutput()
		if err != nil || !strings.Contains(string(output), `"Version": "v0.1.0"`) || strings.Contains(string(output), `"Replace"`) {
			t.Fatalf("feature module resolution: %v\n%s", err, output)
		}
	})
}

// runFeatureRenderCommand executes the real CLI without allowing generated files into the framework checkout.
func runFeatureRenderCommand(t *testing.T, root string, args ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, testkit.EnsureIntegrationForjBinary(t), args...)
	command.Dir = root
	command.Env = testkit.IntegrationGoProcessEnv(t, map[string]string{"GOWORK": "off"})
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("forj %v: %v\n%s", args, err, output)
	}
}

// featureGeneratedSnapshot retains authoritative manager and accessor bytes for a second generation pass.
func featureGeneratedSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	for _, path := range []string{"internal/features/manager_gen.go", "internal/features/accessors_gen.go"} {
		body, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatal(err)
		}
		result[path] = string(body)
	}
	return result
}
