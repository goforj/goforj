package forj

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/goforj/goforj/project"
	"github.com/goforj/goforj/version"
	"gopkg.in/yaml.v3"
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

func selectAtlasModeRow(t *testing.T, m *model, mode atlasMode) {
	t.Helper()
	for idx, item := range m.atlasModeList.Items() {
		atlasModeItem := item.(AtlasModeItem)
		if atlasModeItem.Mode != mode {
			continue
		}
		m.atlasModeList.Select(idx)
		return
	}
	t.Fatalf("atlas mode %q not found", mode)
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
	if m.stage != StageHelpFormat {
		t.Fatalf("expected to be on help format stage")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageStarterKit {
		t.Fatalf("expected to be on starter kit stage after help format")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected to be on extras stage after starter kit")
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSupport {
		t.Fatalf("expected to be on atlas support stage after extras, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected to be on project path stage after atlas support, got %v", m.stage)
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

// TestValidatePathInputRejectsNonEmptyDirectoryByDefault keeps project creation conservative unless explicitly overridden.
func TestValidatePathInputRejectsNonEmptyDirectoryByDefault(t *testing.T) {
	temp := t.TempDir()
	if err := os.WriteFile(filepath.Join(temp, "README.md"), []byte("# existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed target directory: %v", err)
	}

	m := initialModel()
	m.pathInput.SetValue(temp)

	err := m.validatePathInput()
	if err == nil {
		t.Fatalf("expected non-empty directory validation error")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("expected non-empty error, got %v", err)
	}
}

// TestValidatePathInputAllowsNonEmptyDirectoryWithFlag protects the explicit opt-in escape hatch.
func TestValidatePathInputAllowsNonEmptyDirectoryWithFlag(t *testing.T) {
	temp := t.TempDir()
	if err := os.WriteFile(filepath.Join(temp, "README.md"), []byte("# existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed target directory: %v", err)
	}

	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: true})
	m.pathInput.SetValue(temp)

	if err := m.validatePathInput(); err != nil {
		t.Fatalf("expected non-empty directory to validate with allow-non-empty, got %v", err)
	}
}

// TestPathStatusAllowsNonEmptyDirectoryWithFlag ensures the wizard preview matches validation behavior.
func TestPathStatusAllowsNonEmptyDirectoryWithFlag(t *testing.T) {
	temp := t.TempDir()
	if err := os.WriteFile(filepath.Join(temp, "README.md"), []byte("# existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed target directory: %v", err)
	}

	m := initialModelWithOptions(newProjectModelOptions{allowNonEmpty: true})
	m.pathInput.SetValue(temp)

	status, ok := m.pathStatus()
	if !ok {
		t.Fatalf("expected non-empty path status to be valid with allow-non-empty, got %q", status)
	}
	if !strings.Contains(status, "not empty") || !strings.Contains(status, "preserved") {
		t.Fatalf("expected status to explain existing files are preserved, got %q", status)
	}
}

// TestEnsureNewProjectConfigCanBeWrittenRejectsExistingConfig prevents accidental reinitialization.
func TestEnsureNewProjectConfigCanBeWrittenRejectsExistingConfig(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, ".goforj.yml")
	if err := os.WriteFile(configPath, []byte("project_name: existing\n"), 0644); err != nil {
		t.Fatalf("failed to seed config file: %v", err)
	}

	err := ensureNewProjectConfigCanBeWritten(configPath)
	if err == nil {
		t.Fatalf("expected existing .goforj.yml to be rejected")
	}
	if !strings.Contains(err.Error(), "already contains .goforj.yml") {
		t.Fatalf("expected existing config error, got %v", err)
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

	if m.stage != StageHelpFormat {
		t.Fatalf("expected help format stage when web ui is enabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageStarterKit {
		t.Fatalf("expected starter kit stage after help format when web ui is enabled, got %v", m.stage)
	}
}

func TestStarterKitStageSkippedWhenWebUIDisabled(t *testing.T) {
	m := initialModel()
	m.stage = StageSelectComponents
	m.config.Render.StarterKit = project.StarterKitVue
	setComponentSelectedByKey(t, &m, project.ComponentWebUI, false)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageHelpFormat {
		t.Fatalf("expected help format stage when web ui is disabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after help format when web ui is disabled, got %v", m.stage)
	}
	if m.config.Render.StarterKit != project.StarterKitNone {
		t.Fatalf("expected starter kit to be cleared when web ui is disabled, got %q", m.config.Render.StarterKit)
	}
}

// TestHelpFormatPreviewOrderMatchesOptionOrder prevents the side-by-side panes from drifting away from the option list.
func TestHelpFormatPreviewOrderMatchesOptionOrder(t *testing.T) {
	m := initialModel()
	m.stage = StageHelpFormat
	m.termWidth = 160
	m.helpFormatList.Select(1)

	view := ansi.Strip(m.renderHelpFormatPanel())

	frameworkIndex := strings.Index(view, "Framework Preview")
	guidedIndex := strings.Index(view, "Guided Preview")
	externalIndex := strings.Index(view, "External CLI Preview")
	if frameworkIndex < 0 || guidedIndex < 0 || externalIndex < 0 {
		t.Fatalf("expected all preview panels to be rendered, got %q", view)
	}
	if !(frameworkIndex < guidedIndex && guidedIndex < externalIndex) {
		t.Fatalf("expected preview panels to match option order, got %q", view)
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

func TestAtlasRecommendedMovesToProjectPath(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeRecommended)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageProjectPath {
		t.Fatalf("expected recommended atlas to continue to project path, got %v", m.stage)
	}
	if !m.atlasInstallEnabled() {
		t.Fatalf("expected recommended atlas install to be enabled")
	}
	surfaces := m.selectedAtlasSurfaces()
	if !surfaces.guidelines || !surfaces.skills || !surfaces.mcp {
		t.Fatalf("expected recommended atlas surfaces, got %#v", surfaces)
	}
	if len(m.selectedAtlasAgents()) == 0 {
		t.Fatalf("expected recommended atlas agents")
	}
}

func TestAtlasMinimalInstallsGuidelinesOnly(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeMinimal)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	surfaces := m.selectedAtlasSurfaces()
	if !surfaces.guidelines || surfaces.skills || surfaces.mcp {
		t.Fatalf("expected minimal atlas guidelines only, got %#v", surfaces)
	}
}

func TestAtlasSkipDisablesInstall(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeSkip)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)

	if m.stage != StageProjectPath {
		t.Fatalf("expected skip to continue to project path, got %v", m.stage)
	}
	if m.atlasInstallEnabled() {
		t.Fatalf("expected skip to disable atlas install")
	}
}

func TestAtlasCustomRequiresAgentAndSurface(t *testing.T) {
	m := initialModel()
	m.stage = StageAtlasSupport
	selectAtlasModeRow(t, &m, atlasModeCustom)

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasAgents {
		t.Fatalf("expected custom to open agent selection, got %v", m.stage)
	}

	for idx, listItem := range m.atlasAgentList.Items() {
		item := listItem.(AtlasAgentItem)
		item.Selected = false
		m.atlasAgentList.SetItem(idx, item)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasAgents || !strings.Contains(m.errorMsg, "Select at least one agent") {
		t.Fatalf("expected custom agent validation, stage=%v error=%q", m.stage, m.errorMsg)
	}

	m.toggleAtlasAgentSelection()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSurfaces {
		t.Fatalf("expected custom to open surface selection, got %v", m.stage)
	}

	for idx, listItem := range m.atlasSurfaceList.Items() {
		item := listItem.(AtlasSurfaceItem)
		item.Selected = false
		m.atlasSurfaceList.SetItem(idx, item)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSurfaces || !strings.Contains(m.errorMsg, "Select at least one install option") {
		t.Fatalf("expected custom surface validation, stage=%v error=%q", m.stage, m.errorMsg)
	}

	m.toggleAtlasSurfaceSelection()
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected custom selections to continue to project path, got %v", m.stage)
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
	if m.stage != StageHelpFormat {
		t.Fatalf("expected help format stage, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageStarterKit {
		t.Fatalf("expected starter kit stage after help format, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageExtras {
		t.Fatalf("expected extras stage after starter kit, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageAtlasSupport {
		t.Fatalf("expected atlas support stage when jobs enabled, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageProjectPath {
		t.Fatalf("expected project path stage after atlas support, got %v", m.stage)
	}

	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model)
	if m.stage != StageConfirm {
		t.Fatalf("expected confirmation stage after project path selection, got %v", m.stage)
	}
	if m.queueDriver != "redis" {
		t.Fatalf("expected default queue driver to be redis, got %q", m.queueDriver)
	}
}

func TestFinalizeConfigDefaultsQueueDriverForJobs(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.Jobs = true
	m.queueDriver = "  "

	m.finalizeConfig()

	if m.queueDriver != "redis" {
		t.Fatalf("expected queue driver default redis, got %q", m.queueDriver)
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal finalized config: %v", err)
	}
	if strings.Contains(string(encoded), "queue_driver:") {
		t.Fatalf("wizard-only queue driver was persisted:\n%s", encoded)
	}
	if m.config.Render.GoForjVersion != version.Semver() {
		t.Fatalf("expected goforj version %q, got %q", version.Semver(), m.config.Render.GoForjVersion)
	}
}

// TestFinalizeConfigUsesNativeDefaultAppLifecycle verifies generated runtime Apps expose their native lifecycle configuration.
func TestFinalizeConfigUsesNativeDefaultAppLifecycle(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebAPI = true

	m.finalizeConfig()

	for _, task := range m.config.Dev.Pre {
		if task.Name == "Initial build" && task.Cmd == "forj build -o ./bin/app" {
			t.Fatalf("expected initial build to be owned by forj dev, got pre task %#v", task)
		}
	}

	for _, watch := range m.config.Dev.Watches {
		if watch.Name == "Build App" || watch.Name == "Run App" || watch.Name == "Wire" {
			t.Fatalf("expected app lifecycle to avoid framework raw watchers, got %#v", watch)
		}
	}
	app, ok := m.config.Dev.Apps[project.DefaultAppName]
	if !ok {
		t.Fatalf("expected native default app runtime, got %#v", app)
	}
	wantBuild := conventionalDevAppBuildCommand(&m.config, project.DefaultApp())
	if app.Build == nil || !reflect.DeepEqual(*app.Build, wantBuild) {
		t.Fatalf("generated build = %#v, want %#v", app.Build, wantBuild)
	}
	wantRun := conventionalDevAppRuntimeCommand(project.DefaultApp())
	if app.Run == nil || !reflect.DeepEqual(*app.Run, wantRun) {
		t.Fatalf("generated runtime = %#v, want %#v", app.Run, wantRun)
	}
	compiled, err := compileDevWatchers(&m.config)
	if err != nil {
		t.Fatalf("compile generated dev lifecycle: %v", err)
	}
	if len(compiled) != 2 || len(compiled[0].Watch.Excludes) != len(wantBuild.Ignore) || compiled[1].FullProcessOverride {
		t.Fatalf("generated explicit lifecycle changed native defaults: %#v", compiled)
	}
	if m.config.Dev.Run != nil {
		t.Fatalf("expected generated config not to use the legacy dev.run allowlist, got %#v", m.config.Dev.Run)
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal generated project config: %v", err)
	}
	var decoded project.Config
	if err := yaml.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal generated project config: %v", err)
	}
	decodedApp := decoded.Dev.Apps[project.DefaultAppName]
	if decodedApp.Build == nil || !reflect.DeepEqual(*decodedApp.Build, wantBuild) ||
		decodedApp.Run == nil || !reflect.DeepEqual(*decodedApp.Run, wantRun) {
		t.Fatalf("generated lifecycle changed across YAML round trip: %#v", decodedApp)
	}
	decodedCompiled, err := compileDevWatchers(&decoded)
	if err != nil {
		t.Fatalf("compile round-tripped dev lifecycle: %v", err)
	}
	if len(decodedCompiled) != 2 || decodedCompiled[1].FullProcessOverride {
		t.Fatalf("round-tripped conventional runtime became a process override: %#v", decodedCompiled)
	}
	for _, want := range []string{
		"apps:",
		"exec: forj build -o ./bin/app",
		"watch: [.go, .env, .env.*]",
		"ignore: [forj, _data, wire_gen.go, .git, .hg, .svn, .idea, .vscode, .settings, node_modules]",
		"root: .",
		"postpone: true",
		"exec: ./bin/app",
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("expected generated dev.apps YAML to contain %q, got:\n%s", want, encoded)
		}
	}
	if strings.Contains(string(encoded), "run: run") {
		t.Fatalf("expected generated runtime to use its complete command, got:\n%s", encoded)
	}
	for _, legacy := range []string{"watches:", "Build App", "Run App"} {
		if strings.Contains(string(encoded), legacy) {
			t.Fatalf("expected generated YAML to omit legacy %q config, got:\n%s", legacy, encoded)
		}
	}
}

// TestFinalizeConfigInstallsVueStarterDependencies verifies Vue generation exposes its full frontend lifecycle.
func TestFinalizeConfigInstallsVueStarterDependencies(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebUI = true
	m.config.Render.StarterKit = project.StarterKitVue

	m.finalizeConfig()

	foundInstall := false
	for _, task := range m.config.Dev.Pre {
		if task.Name == "Install Frontend Dependencies" && task.Cmd == "cd cmd/app/frontend && npm install" {
			foundInstall = true
			break
		}
	}
	if !foundInstall {
		t.Fatalf("expected vue starter dev pre-task to install frontend dependencies, got %#v", m.config.Dev.Pre)
	}
	app := m.config.Dev.Apps[project.DefaultAppName]
	wantSPA := conventionalDevSPAConfig("./cmd/app/frontend")
	if spa, ok := app.SPAs[generatedFrontendSPAName]; !ok || !reflect.DeepEqual(spa, wantSPA) {
		t.Fatalf("generated Vue SPA = %#v, want %#v", spa, wantSPA)
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal generated Vue config: %v", err)
	}
	for _, want := range []string{
		"path: ./cmd/app/frontend",
		"build: npm run build -s -- --logLevel silent",
		"watch: [.ts, .tsx, .js, .jsx, .vue, .css, .html, package.json, package-lock.json]",
		"ignore: [_data, node_modules, dist]",
	} {
		if !strings.Contains(string(encoded), want) {
			t.Fatalf("expected generated Vue config to contain %q, got:\n%s", want, encoded)
		}
	}
}

func TestFinalizeConfigDoesNotAddGrafanaSeedTask(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.Docker = true
	m.config.Render.Components.Grafana = true

	m.finalizeConfig()

	for _, task := range m.config.Dev.Pre {
		if task.Name == "Seed Grafana Dashboards" {
			t.Fatalf("expected grafana seed to run through docker-compose up, got separate task %#v", task)
		}
	}
}

// TestFinalizeConfigTemplStarterUsesOwnedFrontendLifecycle verifies generated SPAs replace raw frontend watcher wiring.
func TestFinalizeConfigTemplStarterUsesOwnedFrontendLifecycle(t *testing.T) {
	m := initialModel()
	m.config.Render.Components.WebUI = true
	m.config.Render.Components.WebAPI = true
	m.config.Render.StarterKit = project.StarterKitTemplHTMX

	m.finalizeConfig()

	for _, watch := range m.config.Dev.Watches {
		if watch.Name == "Build App" || watch.Name == "Run App" || watch.Name == "NPM" {
			t.Fatalf("expected templ lifecycle to avoid generated raw watchers, got %#v", watch)
		}
	}
	app := m.config.Dev.Apps[project.DefaultAppName]
	spa, ok := app.SPAs[generatedFrontendSPAName]
	wantSPA := conventionalDevSPAConfig("./cmd/app/frontend")
	wantBuild := conventionalDevAppBuildCommand(&m.config, project.DefaultApp())
	wantRun := conventionalDevAppRuntimeCommand(project.DefaultApp())
	if !ok || !reflect.DeepEqual(spa, wantSPA) || app.Build == nil || !reflect.DeepEqual(*app.Build, wantBuild) ||
		app.Run == nil || !reflect.DeepEqual(*app.Run, wantRun) {
		t.Fatalf("expected templ frontend ownership in the default app lifecycle, got %#v", app)
	}
	if !reflect.DeepEqual(app.Build.Watch, []string{".go", ".env", ".env.*", ".templ"}) ||
		app.Build.Ignore[len(app.Build.Ignore)-1] != `re:.*_templ\.go$` {
		t.Fatalf("expected templ-specific build matchers, got %#v", app.Build)
	}
}

// TestFinalizeConfigOmitsCLIOnlyAppFromDev verifies omission remains the unmanaged state.
func TestFinalizeConfigOmitsCLIOnlyAppFromDev(t *testing.T) {
	m := initialModel()
	m.config.Render.Components = project.Components{CLI: true}

	m.finalizeConfig()

	if len(m.config.Dev.Apps) != 0 {
		t.Fatalf("expected CLI-only generated app to remain unmanaged by dev, got %#v", m.config.Dev.Apps)
	}
	if !m.config.Dev.UsesStructuredApps() {
		t.Fatal("expected CLI-only generation to retain an explicit native-none Apps allowlist")
	}
	encoded, err := yaml.Marshal(m.config)
	if err != nil {
		t.Fatalf("marshal CLI-only generated config: %v", err)
	}
	if !strings.Contains(string(encoded), "apps: {}") {
		t.Fatalf("expected CLI-only generated YAML to retain apps: {}, got:\n%s", encoded)
	}
}
