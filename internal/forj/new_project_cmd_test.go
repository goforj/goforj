package forj

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

func TestModelHandlesCtrlC(t *testing.T) {
	m := initialModel()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command, got nil")
	}

	cancelledModel, ok := updated.(model)
	if !ok {
		t.Fatalf("expected model type %T, got %T", m, updated)
	}

	if !cancelledModel.cancelled {
		t.Fatalf("expected cancelled flag to be set")
	}

	msg := cmd()
	if msg == nil {
		t.Fatalf("expected QuitMsg from quit command, got nil")
	}
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg from quit command, got %T", msg)
	}
}

func TestModelBackNavigation(t *testing.T) {
	m := initialModel()
	m.projectInput.SetValue("MyApp")

	projectToModule, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	moduleStage, ok := projectToModule.(model)
	if !ok {
		t.Fatalf("expected model type after project stage advance")
	}
	if moduleStage.stage != StageModuleName {
		t.Fatalf("expected module stage, got %v", moduleStage.stage)
	}

	moduleStage.moduleInput.SetValue("github.com/example/myapp")

	backToProject, _ := moduleStage.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	projectStage := backToProject.(model)
	if projectStage.stage != StageProjectName {
		t.Fatalf("expected project stage after back navigation, got %v", projectStage.stage)
	}
	if projectStage.projectInput.Value() != "MyApp" {
		t.Fatalf("project name should be preserved on back navigation")
	}
}

func TestConfirmationFlow(t *testing.T) {
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	temp := t.TempDir()
	_ = os.Chdir(temp)

	m := initialModel()
	m.projectInput.SetValue("MyApp")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	m.moduleInput.SetValue("github.com/example/myapp")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageSelectComponents {
		t.Fatalf("expected to be on component selection stage")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected to be on extras stage")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected to be on project path stage after extras")
	}

	// accept current temp dir
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageConfirm {
		t.Fatalf("expected confirmation stage after path step")
	}

	if !m.config.Components.CLI {
		t.Fatalf("expected CLI component to remain selected in config")
	}

	confirmedModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	finalModel := confirmedModel.(model)
	if finalModel.stage != StageDone {
		t.Fatalf("expected final stage after confirmation")
	}
	if cmd == nil {
		t.Fatalf("expected quit command on confirmation")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg on confirmation")
	}
}

func TestDemoAppEnablesCoreComponents(t *testing.T) {
	m := initialModel()
	m.config.Components.CLI = true
	m.config.Components.DatabaseMySQL = true
	m.extrasIndex = 1

	m.applyExtrasSelection()

	if !m.config.Components.DemoApp {
		t.Fatalf("expected demo app to be enabled")
	}
	if !m.config.Components.WebAPI || !m.config.Components.WebUI || !m.config.Components.Scheduler || !m.config.Components.Jobs {
		t.Fatalf("expected core demo components to be enabled")
	}
	if !m.config.Components.DatabaseMySQL {
		t.Fatalf("expected mysql to be enabled for demo app")
	}
	if m.config.Components.DatabaseSQLite || m.config.Components.DatabasePostgres {
		t.Fatalf("expected other database selections to be cleared")
	}
}

func TestQueueDriverStageAppearsWhenJobsEnabled(t *testing.T) {
	m := initialModel()
	m.projectInput.SetValue("MyApp")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	m.moduleInput.SetValue("github.com/example/myapp")
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	// Select Jobs in component list.
	for idx, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Name == "Jobs" {
			component.Selected = true
			m.componentList.SetItem(idx, component)
			break
		}
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected extras stage, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageRuntime {
		t.Fatalf("expected runtime stage when jobs enabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected project path stage after runtime selection, got %v", m.stage)
	}
	if m.config.Render.QueueDriver != "redis" {
		t.Fatalf("expected default queue driver to be redis, got %q", m.config.Render.QueueDriver)
	}
}

func TestFinalizeConfigDefaultsQueueDriverForJobs(t *testing.T) {
	m := initialModel()
	m.config.Components.Jobs = true
	m.config.Render.QueueDriver = "  "

	m.finalizeConfig()

	if m.config.Render.QueueDriver != "redis" {
		t.Fatalf("expected queue driver default redis, got %q", m.config.Render.QueueDriver)
	}
	if m.config.Render.GoForjVersion != version.Semver() {
		t.Fatalf("expected goforj version %q, got %q", version.Semver(), m.config.Render.GoForjVersion)
	}
}

func TestFinalizeConfigUsesSingleBuildWatcher(t *testing.T) {
	m := initialModel()
	m.config.Components.WebAPI = true

	m.finalizeConfig()

	var buildWatch *string
	for _, watch := range m.config.Dev.Watches {
		switch watch.Name {
		case "Build App":
			value := watch.Watch
			buildWatch = &value
			if watch.Exec != "forj build -o ./bin/app" {
				t.Fatalf("expected build watcher to execute forj build, got %q", watch.Exec)
			}
		case "Wire":
			t.Fatalf("expected no standalone wire watcher, got %#v", watch)
		}
	}

	if buildWatch == nil {
		t.Fatalf("expected Build App watcher to be configured")
	}
	if !strings.Contains(*buildWatch, "-xfile wire/wire_gen\\.go$") {
		t.Fatalf("expected Build App watcher to exclude wire_gen.go, got %q", *buildWatch)
	}

	var runWatch *project.DevWatch
	for i := range m.config.Dev.Watches {
		if m.config.Dev.Watches[i].Name == "Run App" {
			runWatch = &m.config.Dev.Watches[i]
			break
		}
	}
	if runWatch == nil {
		t.Fatalf("expected Run App watcher to be configured")
	}
	if runWatch.Exec != "./bin/app run" {
		t.Fatalf("expected Run App watcher to execute ./bin/app run, got %q", runWatch.Exec)
	}
}
