package forj

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
)

func setComponentSelectedByKey(t *testing.T, m *model, key project.ComponentKey, selected bool) {
	t.Helper()
	for idx, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Key != key {
			continue
		}
		component.Selected = selected
		m.componentList.SetItem(idx, component)
		return
	}
	t.Fatalf("component %q not found", key)
}

func selectComponentRowByKey(t *testing.T, m *model, key project.ComponentKey) {
	t.Helper()
	for idx, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Key != key {
			continue
		}
		m.componentList.Select(idx)
		return
	}
	t.Fatalf("component %q not found", key)
}

func componentSelectedByKey(t *testing.T, m model, key project.ComponentKey) bool {
	t.Helper()
	for _, item := range m.componentList.Items() {
		component := item.(ListItem)
		if component.Key == key {
			return component.Selected
		}
	}
	t.Fatalf("component %q not found", key)
	return false
}

func selectStarterKitRow(t *testing.T, m *model, key project.StarterKit) {
	t.Helper()
	for idx, item := range m.starterKitList.Items() {
		starterKit := item.(StarterKitItem)
		if starterKit.Key != key {
			continue
		}
		m.starterKitList.Select(idx)
		return
	}
	t.Fatalf("starter kit %q not found", key)
}

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
	if m.stage != StageStarterKit {
		t.Fatalf("expected to be on starter kit stage")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected to be on extras stage after starter kit")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected to be on project path stage after extras when jobs are selected by default")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageConfirm {
		t.Fatalf("expected confirmation stage after project path")
	}

	if !m.config.Render.Components.CLI {
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
	m.config.Render.Components.CLI = true
	m.config.Render.Components.DatabaseMySQL = true
	m.config.Render.StarterKit = project.StarterKitVue
	m.extrasIndex = 1

	m.applyExtrasSelection()

	if !m.config.Render.Components.DemoApp {
		t.Fatalf("expected demo app to be enabled")
	}
	if !m.config.Render.Components.Auth || !m.config.Render.Components.WebAPI || !m.config.Render.Components.WebUI || !m.config.Render.Components.Scheduler || !m.config.Render.Components.Jobs {
		t.Fatalf("expected core demo components to be enabled")
	}
	if !m.config.Render.Components.DatabaseMySQL {
		t.Fatalf("expected mysql to be enabled for demo app")
	}
	if m.config.Render.Components.DatabaseSQLite || m.config.Render.Components.DatabasePostgres {
		t.Fatalf("expected other database selections to be cleared")
	}
	if m.config.Render.StarterKit != project.StarterKitNone {
		t.Fatalf("expected demo app to clear starter kit, got %q", m.config.Render.StarterKit)
	}
}

func TestOAuthSelectionAlsoEnablesAuth(t *testing.T) {
	m := initialModel()

	setComponentSelectedByKey(t, &m, project.ComponentOAuth, true)

	m.applyComponentSelection()

	if !m.config.Render.Components.OAuth {
		t.Fatalf("expected oauth component to be enabled")
	}
	if !m.config.Render.Components.Auth {
		t.Fatalf("expected oauth selection to force auth on")
	}
	if !m.config.Render.Components.Mail {
		t.Fatalf("expected auth selection to force mail on")
	}
}

func TestMailSelectionEnablesMailComponent(t *testing.T) {
	m := initialModel()

	setComponentSelectedByKey(t, &m, project.ComponentMail, true)

	m.applyComponentSelection()

	if !m.config.Render.Components.Mail {
		t.Fatalf("expected mail component to be enabled")
	}
}

func TestAuthSelectionAlsoEnablesMail(t *testing.T) {
	m := initialModel()

	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)

	m.applyComponentSelection()

	if !m.config.Render.Components.Auth {
		t.Fatalf("expected auth component to be enabled")
	}
	if !m.config.Render.Components.Mail {
		t.Fatalf("expected auth selection to force mail on")
	}
}

func TestAuthToggleAutoSelectsMailInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentMail, false)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, false)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, false)
	selectComponentRowByKey(t, &m, project.ComponentAuth)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to be selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMail) {
		t.Fatalf("expected mail to be auto-selected when auth is selected")
	}
}

func TestOAuthToggleAutoSelectsAuthAndMailInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentMail, false)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, false)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, false)
	selectComponentRowByKey(t, &m, project.ComponentOAuth)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentOAuth) {
		t.Fatalf("expected oauth to be selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to be auto-selected when oauth is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMail) {
		t.Fatalf("expected mail to be auto-selected when oauth is selected")
	}
}

func TestGrafanaToggleAutoSelectsObservabilityChainInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentGrafana, false)
	setComponentSelectedByKey(t, &m, project.ComponentObservability, false)
	setComponentSelectedByKey(t, &m, project.ComponentMetrics, false)
	setComponentSelectedByKey(t, &m, project.ComponentWebAPI, false)
	setComponentSelectedByKey(t, &m, project.ComponentDocker, false)
	selectComponentRowByKey(t, &m, project.ComponentGrafana)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentGrafana) {
		t.Fatalf("expected grafana to be selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentObservability) {
		t.Fatalf("expected observability to be auto-selected when grafana is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMetrics) {
		t.Fatalf("expected metrics to be auto-selected when grafana is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentWebAPI) {
		t.Fatalf("expected web api to be auto-selected when grafana is selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentDocker) {
		t.Fatalf("expected docker to be auto-selected when grafana is selected")
	}
}

func TestAuthToggleAlsoClearsOAuthInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentOAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentMail, true)
	selectComponentRowByKey(t, &m, project.ComponentAuth)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if componentSelectedByKey(t, m, project.ComponentOAuth) {
		t.Fatalf("expected oauth to be deselected when auth is deselected")
	}
	if componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to be deselected")
	}
}

func TestMailToggleDoesNotClearAuthOrOAuthInWizard(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents

	setComponentSelectedByKey(t, &m, project.ComponentMail, true)
	setComponentSelectedByKey(t, &m, project.ComponentAuth, true)
	setComponentSelectedByKey(t, &m, project.ComponentOAuth, true)
	selectComponentRowByKey(t, &m, project.ComponentMail)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	m = next.(model)

	if !componentSelectedByKey(t, m, project.ComponentAuth) {
		t.Fatalf("expected auth to remain selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentOAuth) {
		t.Fatalf("expected oauth to remain selected")
	}
	if !componentSelectedByKey(t, m, project.ComponentMail) {
		t.Fatalf("expected mail to remain selected because auth depends on it")
	}
	if !strings.Contains(m.errorMsg, "Mail remains enabled because Auth requires it.") {
		t.Fatalf("expected explanatory error message, got %q", m.errorMsg)
	}
}

func TestStarterKitStageAppearsWhenWebUIEnabled(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	setComponentSelectedByKey(t, &m, project.ComponentWebUI, true)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageStarterKit {
		t.Fatalf("expected starter kit stage when web ui is enabled, got %v", m.stage)
	}
}

func TestStarterKitStageSkippedWhenWebUIDisabled(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	m.config.Render.StarterKit = project.StarterKitVue
	setComponentSelectedByKey(t, &m, project.ComponentWebUI, false)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageExtras {
		t.Fatalf("expected extras stage when web ui is disabled, got %v", m.stage)
	}
	if m.config.Render.StarterKit != project.StarterKitNone {
		t.Fatalf("expected starter kit to be cleared when web ui is disabled, got %q", m.config.Render.StarterKit)
	}
}

func TestVueStarterKitSelectionPersists(t *testing.T) {
	m := initialModel()
	m.stage = StageStarterKit
	m.config.Render.Components.WebUI = true
	selectStarterKitRow(t, &m, project.StarterKitVue)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after starter kit selection, got %v", m.stage)
	}
	if m.config.Render.StarterKit != project.StarterKitVue {
		t.Fatalf("expected vue starter kit, got %q", m.config.Render.StarterKit)
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
	setComponentSelectedByKey(t, &m, project.ComponentJobs, true)

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageStarterKit {
		t.Fatalf("expected starter kit stage, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after starter kit, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected project path stage when jobs enabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageConfirm {
		t.Fatalf("expected confirmation stage after project path selection, got %v", m.stage)
	}
	if m.config.Render.QueueDriver != "redis" {
		t.Fatalf("expected default queue driver to be redis, got %q", m.config.Render.QueueDriver)
	}
}

func TestFinalizeConfigDefaultsQueueDriverForJobs(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.Jobs = true
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
	m.config.Render.Components.WebAPI = true

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
	if !strings.Contains(*buildWatch, "-xfile app/wire/wire_gen\\.go$") {
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

func TestFinalizeConfigInstallsVueStarterDependencies(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebUI = true
	m.config.Render.StarterKit = project.StarterKitVue

	m.finalizeConfig()

	for _, task := range m.config.Dev.Pre {
		if task.Name == "Install Frontend Dependencies" && task.Cmd == "cd cmd/app/frontend && npm install" {
			return
		}
	}
	t.Fatalf("expected vue starter dev pre-task to install frontend dependencies, got %#v", m.config.Dev.Pre)
}
