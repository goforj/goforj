package forj

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goforj/goforj/internal/apiindex"
	"github.com/goforj/goforj/internal/generate"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/internal/projectupgrade"
	"github.com/goforj/goforj/project"
	"gopkg.in/yaml.v3"
)

// TestFrameworkSnapshotEffectiveComponents keeps implicit scaffold dependencies and generated ownership aligned without rewriting the input recipe.
func TestFrameworkSnapshotEffectiveComponents(t *testing.T) {
	t.Setenv("TMPDIR", "/tmp")
	for _, test := range []struct {
		name       string
		components project.Components
		named      bool
	}{
		{name: "demo implicit mail", components: project.Components{CLI: true, DemoApp: true, DatabaseMySQL: true, DatabaseSQLite: true}},
		{name: "demo explicit mail", components: project.Components{CLI: true, DemoApp: true, Mail: true, DatabaseMySQL: true, DatabaseSQLite: true}},
		{name: "default auth", components: project.Components{CLI: true, Auth: true, DatabaseSQLite: true}},
		{name: "named auth", components: project.Components{CLI: true, DatabaseSQLite: true}, named: true},
		{name: "demo named auth", components: project.Components{CLI: true, DemoApp: true, DatabaseMySQL: true, DatabaseSQLite: true}, named: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := &project.Config{ProjectName: "Effective snapshot", GoModuleName: "example.com/effective", Render: project.RenderConfig{
				Components: test.components, StarterKit: project.StarterKitVue, StarterKitOptions: project.NewStarterKitOptions(false),
			}}
			app := project.DefaultApp()
			if test.named {
				config.Apps = map[string]project.AppConfig{"billing": {Components: project.Components{CLI: true, Auth: true, DatabaseSQLite: true}}}
				app = project.AppForName("billing")
			}
			before, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			digest, err := frameworkInputsDigest(&frameworkRenderIntent{Config: config})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := frameworkSnapshotForConfig(&frameworkRenderIntent{Config: config})
			if err != nil {
				t.Fatal(err)
			}
			after, err := yaml.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) || snapshot.InputsDigest != digest {
				t.Fatal("snapshot changed the declared recipe or its input digest")
			}
			ownership := map[string]string{}
			for _, artifact := range snapshot.Artifacts {
				ownership[artifact.Path] = artifact.Ownership
			}
			for _, name := range append(apiindex.OutputPaths(app), generate.MailManagerPath, generate.MailAccessorsPath) {
				if ownership[name] != "app-derived" {
					t.Errorf("effective output %s ownership = %q, want app-derived", name, ownership[name])
				}
			}
			if test.components.DemoApp {
				for _, name := range apiindex.OutputPaths(project.DefaultApp()) {
					if ownership[name] != "app-derived" {
						t.Errorf("default demo output %s ownership = %q, want app-derived", name, ownership[name])
					}
				}
			}
			frozen, err := frameworkSnapshotForConfig(&frameworkRenderIntent{Config: config})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(snapshot, frozen) {
				t.Fatal("rendering the same declared recipe changed the snapshot")
			}
		})
	}
}

// TestFrameworkSnapshotDiscoveredDemoApp preserves ordinary scaffold inheritance without changing the source recipe.
func TestFrameworkSnapshotDiscoveredDemoApp(t *testing.T) {
	t.Setenv("TMPDIR", "/tmp")
	root := t.TempDir()
	config := &project.Config{ProjectName: "Discovered demo", GoModuleName: "example.com/discovered", Render: project.RenderConfig{
		Components: project.Components{CLI: true, DemoApp: true, DatabaseMySQL: true, DatabaseSQLite: true}, StarterKit: project.StarterKitVue,
	}}
	if err := writeFrameworkProjection(root, &frameworkRenderIntent{Config: config}); err != nil {
		t.Fatal(err)
	}
	writeProjectRendererTestFile(t, filepath.Join(root, ".env"), "APP_ENV=local\nDB_DRIVER=mysql\nDB_SUPPORTED_DRIVERS=sqlite,mysql\n")
	writeProjectRendererTestFile(t, filepath.Join(root, "cmd/worker/main.go"), "package main\n")
	before, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		t.Fatal(err)
	}
	projection, err := frameworkUpgradeConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Render.Components != config.Render.Components || projection.Render.StarterKit != config.Render.StarterKit {
		t.Fatal("discovery changed the declared default recipe")
	}
	if _, explicit := projection.Apps["worker"]; explicit || !reflect.DeepEqual(projection.DiscoveredApps, []string{"worker"}) {
		t.Fatal("discovery converted inherited App participation into explicit settings")
	}
	snapshot, err := frameworkUpgradeSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(root, ".goforj.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("inspection changed the original configuration bytes")
	}
	artifacts := map[string]projectupgrade.Artifact{}
	for _, artifact := range snapshot.Artifacts {
		artifacts[artifact.Path] = artifact
	}
	renderer := NewProjectRenderer(logger.NewSilentLogger())
	renderer.inspection = &frameworkRenderInventory{artifacts: map[string]projectupgrade.Artifact{}}
	if err := renderer.Render(ComponentRenderInput{root: root, renderAll: true}); err != nil {
		t.Fatal(err)
	}
	compared := 0
	for name, ordinary := range renderer.inspection.artifacts {
		if ordinary.Ownership != "framework-template" && ordinary.Ownership != "framework-asset" {
			continue
		}
		source, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("ordinary artifact %s: %v", name, err)
		}
		artifact, found := artifacts[name]
		if !found || artifact.Ownership != ordinary.Ownership || artifact.Template != ordinary.Template || artifact.Mode != ordinary.Mode || artifact.Digest != projectupgrade.Digest(source) {
			t.Errorf("snapshot differs from ordinary scaffold artifact %s", name)
		}
		delete(artifacts, name)
		compared++
	}
	for name, artifact := range artifacts {
		if artifact.Ownership == "framework-template" || artifact.Ownership == "framework-asset" {
			t.Errorf("snapshot claimed artifact absent from ordinary scaffold: %s", name)
		}
	}
	if compared < 100 {
		t.Fatalf("insufficient managed artifact parity coverage: %d", compared)
	}
	for _, name := range append(apiindex.OutputPaths(project.AppForName("worker")), generate.MailManagerPath, generate.MailAccessorsPath) {
		if artifacts[name].Ownership != "app-derived" {
			t.Errorf("discovered output %s ownership = %q, want app-derived", name, artifacts[name].Ownership)
		}
	}
}
