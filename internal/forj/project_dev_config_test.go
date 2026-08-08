package forj

import (
	"reflect"
	"testing"

	"github.com/goforj/goforj/project"
)

// TestMigrateGeneratedDevSPABuildCommandsRestoresFrameworkDiagnostics upgrades only the generated silent command.
func TestMigrateGeneratedDevSPABuildCommandsRestoresFrameworkDiagnostics(t *testing.T) {
	config := &project.Config{Dev: project.DevConfig{Apps: map[string]project.DevApp{
		"app": {
			SPAs: map[string]project.DevSPA{
				"frontend": {Build: legacyFrontendSPABuildCommand},
				"custom":   {Build: "make frontend"},
			},
		},
	}}}

	if !migrateGeneratedDevSPABuildCommands(config) {
		t.Fatal("generated SPA build command was not migrated")
	}
	if got := config.Dev.Apps["app"].SPAs["frontend"].Build; got != generatedFrontendSPABuildCommand {
		t.Fatalf("frontend build = %q, want %q", got, generatedFrontendSPABuildCommand)
	}
	if got := config.Dev.Apps["app"].SPAs["custom"].Build; got != "make frontend" {
		t.Fatalf("custom build = %q, want owner-authored command", got)
	}
	if migrateGeneratedDevSPABuildCommands(config) {
		t.Fatal("current SPA build command migrated twice")
	}
}

// TestMigrateGeneratedDevFrontendInstallTasksUpdatesExactLegacyTasks verifies default and named generated tasks migrate once.
func TestMigrateGeneratedDevFrontendInstallTasksUpdatesExactLegacyTasks(t *testing.T) {
	defaultApp := project.DefaultApp()
	portalApp := project.DefaultNamedApp("portal")
	custom := project.DevTask{Name: "Custom Frontend Setup", Cmd: "npm ci"}
	config := &project.Config{
		Apps: map[string]project.AppConfig{"portal": {}},
		Dev: project.DevConfig{Pre: []project.DevTask{
			legacyGeneratedDevFrontendInstallTask(defaultApp),
			custom,
			generatedDevFrontendInstallTask(portalApp),
			legacyGeneratedDevFrontendInstallTask(portalApp),
		}},
	}

	if !migrateGeneratedDevFrontendInstallTasks(config) {
		t.Fatal("expected exact legacy frontend install tasks to migrate")
	}
	want := []project.DevTask{
		generatedDevFrontendInstallTask(defaultApp),
		custom,
		generatedDevFrontendInstallTask(portalApp),
	}
	if !reflect.DeepEqual(config.Dev.Pre, want) {
		t.Fatalf("migrated tasks = %#v, want %#v", config.Dev.Pre, want)
	}
	if migrateGeneratedDevFrontendInstallTasks(config) {
		t.Fatal("expected migration to be idempotent")
	}
}

func TestMigrateGeneratedDevFrontendInstallTasksPreservesCustomizedTasks(t *testing.T) {
	custom := []project.DevTask{
		{Name: "Install Frontend Dependencies", Cmd: "cd cmd/app/frontend && npm install --ignore-scripts"},
		{Name: "Install portal Frontend Dependencies", Cmd: "cd cmd/portal/frontend && npm ci"},
	}
	config := &project.Config{
		Apps: map[string]project.AppConfig{"portal": {}},
		Dev:  project.DevConfig{Pre: append([]project.DevTask(nil), custom...)},
	}

	if migrateGeneratedDevFrontendInstallTasks(config) {
		t.Fatal("customized frontend install tasks were classified as generated")
	}
	if !reflect.DeepEqual(config.Dev.Pre, custom) {
		t.Fatalf("customized tasks changed from %#v to %#v", custom, config.Dev.Pre)
	}
}

// TestSetAppDevRunMigratesNamedFrontendInstallTask verifies named-App reconciliation cannot duplicate the legacy task.
func TestSetAppDevRunMigratesNamedFrontendInstallTask(t *testing.T) {
	portalApp := project.DefaultNamedApp("portal")
	custom := project.DevTask{Name: "Custom Frontend Setup", Cmd: "npm ci"}
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{WebUI: true}},
		Apps: map[string]project.AppConfig{
			"portal": {Components: project.Components{WebUI: true}, StarterKit: project.StarterKitVue},
		},
		Dev: project.DevConfig{Pre: []project.DevTask{
			legacyGeneratedDevFrontendInstallTask(portalApp),
			custom,
		}},
	}
	renderer := projectRendererForTest(t, config)

	renderer.setAppDevRun("portal", "run")

	want := []project.DevTask{generatedDevFrontendInstallTask(portalApp), custom}
	if !reflect.DeepEqual(config.Dev.Pre, want) {
		t.Fatalf("named App tasks = %#v, want %#v", config.Dev.Pre, want)
	}
}

// TestSetAppDevRunDoesNotDuplicateCustomizedFrontendInstallTask keeps named-App command edits authoritative.
func TestSetAppDevRunDoesNotDuplicateCustomizedFrontendInstallTask(t *testing.T) {
	portalApp := project.DefaultNamedApp("portal")
	custom := legacyGeneratedDevFrontendInstallTask(portalApp)
	custom.Cmd = "cd cmd/portal/frontend && npm ci"
	config := &project.Config{
		Render: project.RenderConfig{Components: project.Components{WebUI: true}},
		Apps: map[string]project.AppConfig{
			"portal": {Components: project.Components{WebUI: true}, StarterKit: project.StarterKitVue},
		},
		Dev: project.DevConfig{Pre: []project.DevTask{custom}},
	}
	renderer := projectRendererForTest(t, config)

	renderer.setAppDevRun("portal", "run")

	if !reflect.DeepEqual(config.Dev.Pre, []project.DevTask{custom}) {
		t.Fatalf("customized named App task was duplicated or overwritten: %#v", config.Dev.Pre)
	}
}
