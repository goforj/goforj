package makeapp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/goforj/goforj/internal/logger"
	"github.com/goforj/goforj/project"
)

func TestCmdRunsWizardByDefaultInInteractiveTerminal(t *testing.T) {
	restore := stubInteractiveTerminal(t, true)
	defer restore()

	cmd := &Cmd{}
	if !cmd.shouldRunWizard() {
		t.Fatal("expected make:app to open the wizard by default in an interactive terminal")
	}
}

func TestCmdSkipsWizardWhenSelectionIsExplicit(t *testing.T) {
	restore := stubInteractiveTerminal(t, true)
	defer restore()

	for name, cmd := range map[string]*Cmd{
		"components":        {Components: "web-api"},
		"without":           {Without: "web-ui"},
		"starter-kit":       {StarterKit: "vue"},
		"component-library": {ComponentLibrary: "off"},
		"help-format":       {HelpFormat: string(project.HelpFormatExternalCLI)},
	} {
		t.Run(name, func(t *testing.T) {
			if cmd.shouldRunWizard() {
				t.Fatalf("expected explicit %s selection to skip wizard", name)
			}
		})
	}
}

// TestAppSelectionSupportsComponentLibraryOptOut verifies scriptable app creation persists a nested starter-kit option.
func TestAppSelectionSupportsComponentLibraryOptOut(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "photos", Components: "web-ui", StarterKit: "react", ComponentLibrary: "off"}
	selection, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, WebUI: true}},
	})
	if err != nil {
		t.Fatalf("appSelection() error = %v", err)
	}
	if selection.StarterKit != project.StarterKitReact {
		t.Fatalf("StarterKit = %q, want react", selection.StarterKit)
	}
	if selection.StarterKitOptions.ComponentLibraryEnabled() {
		t.Fatal("component library = true, want false")
	}
}

// TestAppSelectionRejectsInvalidComponentLibrary verifies the new command validation branch has direct coverage.
func TestAppSelectionRejectsInvalidComponentLibrary(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "photos", Components: "web-ui", StarterKit: "react", ComponentLibrary: "maybe"}
	_, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true, WebUI: true}},
	})
	if err == nil || !strings.Contains(err.Error(), "use on or off") {
		t.Fatalf("appSelection() error = %v, want on/off validation", err)
	}
}

func TestAppSelectionAllowsExternalHelpFormatWithOtherComponents(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "ship", Components: "web-api,jobs", HelpFormat: string(project.HelpFormatExternalCLI)}
	selection, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, WebAPI: true, Jobs: true},
		},
	})
	if err != nil {
		t.Fatalf("appSelection() error = %v", err)
	}
	wantComponents := project.Components{CLI: true, WebAPI: true, Jobs: true}
	if selection.Components != wantComponents {
		t.Fatalf("Components = %+v, want %+v", selection.Components, wantComponents)
	}
	if selection.StarterKit != project.StarterKitNone {
		t.Fatalf("StarterKit = %q, want none", selection.StarterKit)
	}
	if selection.HelpFormat != project.HelpFormatExternalCLI {
		t.Fatalf("HelpFormat = %q, want %q", selection.HelpFormat, project.HelpFormatExternalCLI)
	}
}

func TestAppSelectionAllowsGuidedHelpFormatWithOtherComponents(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "tasks", Components: "scheduler,database_sqlite", HelpFormat: string(project.HelpFormatGuided)}
	selection, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, Scheduler: true, DatabaseSQLite: true},
		},
	})
	if err != nil {
		t.Fatalf("appSelection() error = %v", err)
	}
	wantComponents := project.Components{CLI: true, Scheduler: true, DatabaseSQLite: true}
	if selection.Components != wantComponents {
		t.Fatalf("Components = %+v, want %+v", selection.Components, wantComponents)
	}
	if selection.StarterKit != project.StarterKitNone {
		t.Fatalf("StarterKit = %q, want none", selection.StarterKit)
	}
	if selection.HelpFormat != project.HelpFormatGuided {
		t.Fatalf("HelpFormat = %q, want %q", selection.HelpFormat, project.HelpFormatGuided)
	}
}

// TestAppSelectionAllowsExplicitCLIOnlyApp keeps non-interactive app creation viable for user-facing tools.
func TestAppSelectionAllowsExplicitCLIOnlyApp(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "ship", Components: "cli", HelpFormat: string(project.HelpFormatExternalCLI)}
	selection, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:              true,
				WebAPI:           true,
				WebUI:            true,
				Auth:             true,
				OAuth:            true,
				DatabaseMySQL:    true,
				Scheduler:        true,
				Jobs:             true,
				DatabasePostgres: true,
				DatabaseSQLite:   true,
			},
		},
	})
	if err != nil {
		t.Fatalf("appSelection() error = %v", err)
	}
	wantComponents := project.Components{CLI: true}
	if selection.Components != wantComponents {
		t.Fatalf("Components = %+v, want %+v", selection.Components, wantComponents)
	}
	if selection.StarterKit != project.StarterKitNone {
		t.Fatalf("StarterKit = %q, want none", selection.StarterKit)
	}
	if selection.HelpFormat != project.HelpFormatExternalCLI {
		t.Fatalf("HelpFormat = %q, want %q", selection.HelpFormat, project.HelpFormatExternalCLI)
	}
}

func TestAppSelectionLeavesDevRunDisabledByDefault(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "ship", Components: "cli"}
	selection, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
	})
	if err != nil {
		t.Fatalf("appSelection() error = %v", err)
	}
	if selection.DevRunCommand != "" {
		t.Fatalf("expected default make:app dev run to be disabled, got %q", selection.DevRunCommand)
	}
}

func TestAppSelectionSupportsDevRunCommand(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{Name: "ship", Components: "cli", DevRun: "sync --once"}
	selection, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{Components: project.Components{CLI: true}},
	})
	if err != nil {
		t.Fatalf("appSelection() error = %v", err)
	}
	if selection.DevRunCommand != "sync --once" {
		t.Fatalf("expected custom dev run command, got %q", selection.DevRunCommand)
	}
}

func TestCmdSkipsWizardByDefaultOutsideInteractiveTerminal(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{}
	if cmd.shouldRunWizard() {
		t.Fatal("expected non-interactive make:app to use default app selection")
	}
}

func TestCmdRunTreatsMissingNameAsNormalExit(t *testing.T) {
	renderer := &recordingRenderer{}
	cmd := NewCmd(logger.NewSilentLogger(), renderer)

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected missing name to return nil, got %v", err)
	}
	if renderer.called {
		t.Fatalf("expected missing name not to render")
	}
}

func TestCmdRunTreatsWizardCancellationAsNormalExit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(".goforj.yml", []byte(`project_name: test
module_name: test
render:
  components:
    cli: true
    web_api: true
    web_ui: true
  starter_kit: vue
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	restoreTerminal := stubInteractiveTerminal(t, true)
	defer restoreTerminal()
	restoreWizard := stubAppWizardRunner(t, func(string, *project.Config) (RenderOptions, error) {
		return RenderOptions{}, errAppCreationCancelled
	})
	defer restoreWizard()
	renderer := &recordingRenderer{}
	cmd := NewCmd(logger.NewSilentLogger(), renderer)
	cmd.Name = "reporting"

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected cancellation to return nil, got %v", err)
	}
	if renderer.called {
		t.Fatalf("expected cancellation not to render")
	}
}

func TestCmdRunTreatsExistingAppAsNormalExit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join("cmd", "billing"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
	}
	if err := os.WriteFile(filepath.Join("cmd", "billing", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write app file: %v", err)
	}
	renderer := &recordingRenderer{}
	cmd := NewCmd(logger.NewSilentLogger(), renderer)
	cmd.Name = "billing"

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected existing app to return nil, got %v", err)
	}
	if renderer.called {
		t.Fatalf("expected existing app not to render")
	}
}

func TestCmdRunRemovesApp(t *testing.T) {
	renderer := &recordingRenderer{
		removeResult: RemoveResult{
			Removed: []string{filepath.Join("app", "billing")},
			Updated: []string{filepath.Join("internal", "runtime", "apps.go")},
		},
	}
	cmd := NewCmd(logger.NewSilentLogger(), renderer)
	cmd.Name = "billing"
	cmd.Remove = true

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected remove to return nil, got %v", err)
	}
	if renderer.called {
		t.Fatalf("expected remove not to render a new app")
	}
	if !renderer.removeCalled {
		t.Fatalf("expected remove to call renderer")
	}
	if renderer.removeApp.Name != "billing" {
		t.Fatalf("remove app = %q, want billing", renderer.removeApp.Name)
	}
}

func TestCmdRunTreatsMissingRemoveAppAsNormalExit(t *testing.T) {
	renderer := &recordingRenderer{}
	cmd := NewCmd(logger.NewSilentLogger(), renderer)
	cmd.Name = "billing"
	cmd.Remove = true

	if err := cmd.Run(); err != nil {
		t.Fatalf("expected missing remove app to return nil, got %v", err)
	}
	if !renderer.removeCalled {
		t.Fatalf("expected remove to call renderer")
	}
}

func TestAppSelectionReturnsCancellationSentinel(t *testing.T) {
	restoreTerminal := stubInteractiveTerminal(t, true)
	defer restoreTerminal()
	restoreWizard := stubAppWizardRunner(t, func(string, *project.Config) (RenderOptions, error) {
		return RenderOptions{}, errAppCreationCancelled
	})
	defer restoreWizard()

	cmd := &Cmd{Name: "reporting"}
	_, err := cmd.appSelection(&project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
				WebUI:  true,
			},
		},
	})
	if !errors.Is(err, errAppCreationCancelled) {
		t.Fatalf("expected cancellation sentinel, got %v", err)
	}
}

func TestWizardShowsAllDatabaseDrivers(t *testing.T) {
	model := initialAppWizardModel("reporting", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI:        true,
				DatabaseMySQL: true,
			},
		},
	})

	seen := map[project.ComponentKey]bool{}
	for _, raw := range model.componentList.Items() {
		item := raw.(componentItem)
		seen[item.Key] = true
	}
	for _, key := range []project.ComponentKey{
		project.ComponentDatabaseMySQL,
		project.ComponentDatabasePostgres,
		project.ComponentDatabaseSQLite,
	} {
		if !seen[key] {
			t.Fatalf("expected app wizard to include %s", key)
		}
	}
}

// TestWizardShowsCLIComponentSelected makes the always-on app CLI visible to users.
func TestWizardShowsCLIComponentSelected(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:    true,
				WebAPI: true,
			},
		},
	})

	if !model.componentSelected(project.ComponentCLI) {
		t.Fatalf("expected CLI component to be visible and selected")
	}
	if !strings.Contains(model.renderComponentList(), "CLI") {
		t.Fatalf("expected component list to render CLI row, got %q", model.renderComponentList())
	}
}

// TestWizardShowsPrimitiveComponentsSelectedFromProjectDefaults keeps make:app on the same flat component model as forj new.
func TestWizardShowsPrimitiveComponentsSelectedFromProjectDefaults(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:     true,
				Cache:   true,
				Events:  true,
				Storage: true,
				Jobs:    true,
			},
		},
	})
	want := map[project.ComponentKey]string{
		project.ComponentCache:   "Cache",
		project.ComponentEvents:  "Events",
		project.ComponentStorage: "File Storage",
		project.ComponentJobs:    "Background Jobs",
	}
	for _, raw := range model.componentList.Items() {
		item := raw.(componentItem)
		label, ok := want[item.Key]
		if !ok {
			continue
		}
		if item.Name != label || !item.Selected {
			t.Fatalf("primitive row %q = %#v, want selected %q", item.Key, item, label)
		}
		delete(want, item.Key)
	}
	if len(want) != 0 {
		t.Fatalf("make:app wizard is missing primitive rows: %#v", want)
	}
	model.applyComponentSelection()
	if !model.components.Cache || !model.components.Events || !model.components.Storage || !model.components.Jobs {
		t.Fatalf("App primitive defaults were lost: %#v", model.components)
	}
}

// TestWizardUnselectingWebAPIRemovesDependents lets users reach CLI-only without hidden dependency re-selection.
func TestWizardUnselectingWebAPIRemovesDependents(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				WebAPI:        true,
				WebUI:         true,
				Auth:          true,
				OAuth:         true,
				DatabaseMySQL: true,
			},
		},
	})
	selectAppWizardComponent(t, &model, project.ComponentWebAPI)

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	model = next.(appWizardModel)

	if !model.componentSelected(project.ComponentCLI) {
		t.Fatalf("expected CLI to remain selected")
	}
	for _, key := range []project.ComponentKey{
		project.ComponentWebAPI,
		project.ComponentWebUI,
		project.ComponentAuth,
		project.ComponentOAuth,
	} {
		if model.componentSelected(key) {
			t.Fatalf("expected %s to be unselected after Web API was removed", key)
		}
	}
}

// TestWizardCanProduceCLIOnlyApp verifies the selected component contract after all optional rows are disabled.
func TestWizardCanProduceCLIOnlyApp(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				WebAPI:        true,
				WebUI:         true,
				Auth:          true,
				OAuth:         true,
				DatabaseMySQL: true,
				Scheduler:     true,
				Jobs:          true,
			},
		},
	})
	for idx, raw := range model.componentList.Items() {
		item := raw.(componentItem)
		if item.Key != project.ComponentCLI {
			item.Selected = false
			model.componentList.SetItem(idx, item)
		}
	}

	model.applyComponentSelection()

	if !model.components.CLI {
		t.Fatalf("expected CLI to remain enabled, got %+v", model.components)
	}
	if model.components.WebAPI || model.components.WebUI || model.components.Auth || model.components.OAuth || model.components.HasDatabase() || model.components.Scheduler || model.components.Jobs {
		t.Fatalf("expected CLI-only app components, got %+v", model.components)
	}
}

// TestWizardDevRunOmitsCLIOnlyAppByDefault verifies tooling Apps do not enter the dev graph without an explicit choice.
func TestWizardDevRunOmitsCLIOnlyAppByDefault(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true},
		},
	})

	if got := model.devRunCommand(); got != "" {
		t.Fatalf("expected wizard to omit CLI-only app from dev, got %q", got)
	}
}

// TestWizardDevRunDefaultsRuntimeAppToConventionalRun verifies runtime capability selects concise native participation.
func TestWizardDevRunDefaultsRuntimeAppToConventionalRun(t *testing.T) {
	model := initialAppWizardModel("portal", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, WebAPI: true},
		},
	})

	if got := model.devRunCommand(); got != "run" {
		t.Fatalf("expected runtime app to use conventional dev participation, got %q", got)
	}
}

func TestWizardCanDisableDevRun(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{CLI: true, WebAPI: true},
		},
	})
	model.stage = appWizardDevRun

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	model = next.(appWizardModel)

	if got := model.devRunCommand(); got != "" {
		t.Fatalf("expected disabled wizard dev run to be empty, got %q", got)
	}
}

// TestWizardBulkComponentShortcutsExposeCLIOnlyAndFullAppPaths keeps make:app parity with the project wizard.
func TestWizardBulkComponentShortcutsExposeCLIOnlyAndFullAppPaths(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:           true,
				WebAPI:        true,
				WebUI:         true,
				Auth:          true,
				OAuth:         true,
				DatabaseMySQL: true,
				Scheduler:     true,
				Jobs:          true,
			},
		},
	})

	next, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	model = next.(appWizardModel)

	if !model.componentSelected(project.ComponentCLI) {
		t.Fatalf("expected clear shortcut to keep CLI selected")
	}
	for _, key := range []project.ComponentKey{
		project.ComponentWebAPI,
		project.ComponentWebUI,
		project.ComponentAuth,
		project.ComponentOAuth,
		project.ComponentDatabaseMySQL,
		project.ComponentScheduler,
		project.ComponentJobs,
	} {
		if model.componentSelected(key) {
			t.Fatalf("expected clear shortcut to unselect %s", key)
		}
	}

	next, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	model = next.(appWizardModel)

	for _, key := range []project.ComponentKey{
		project.ComponentCLI,
		project.ComponentWebAPI,
		project.ComponentWebUI,
		project.ComponentAuth,
		project.ComponentOAuth,
		project.ComponentDatabaseMySQL,
		project.ComponentScheduler,
		project.ComponentJobs,
	} {
		if !model.componentSelected(key) {
			t.Fatalf("expected select-all shortcut to select %s", key)
		}
	}
	if model.componentSelected(project.ComponentDatabasePostgres) || model.componentSelected(project.ComponentDatabaseSQLite) {
		t.Fatalf("expected select-all shortcut to preserve one database choice")
	}
}

// TestWizardComponentFooterIncludesBulkShortcuts documents the visible make:app component controls.
func TestWizardComponentFooterIncludesBulkShortcuts(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI:    true,
				WebAPI: true,
			},
		},
	})

	view := model.View()
	for _, expected := range []string{"A select all", "C select none"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected wizard footer to contain %q, got %q", expected, view)
		}
	}
}

// TestWizardHelpFormatPreviewHighlightsGuidedAsSecondOption ties the recommended external formatter to the active preview.
func TestWizardHelpFormatPreviewHighlightsGuidedAsSecondOption(t *testing.T) {
	model := initialAppWizardModel("ship", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI: true,
			},
		},
	})
	model.stage = appWizardHelpFormat
	model.termWidth = 160
	model.helpFormatList.Select(1)

	view := ansi.Strip(model.renderHelpFormatStage())

	if !strings.Contains(view, "Guided Preview") {
		t.Fatalf("expected guided preview panel, got %q", view)
	}
	if !strings.Contains(view, wizardAccentStyle.Render(" Guided Preview ")) {
		t.Fatalf("expected selected preview title to use active style, got %q", view)
	}

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

func TestWizardShowsAuthComponents(t *testing.T) {
	model := initialAppWizardModel("reporting", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				CLI: true,
			},
		},
	})

	seen := map[project.ComponentKey]bool{}
	for _, raw := range model.componentList.Items() {
		item := raw.(componentItem)
		seen[item.Key] = true
	}
	for _, key := range []project.ComponentKey{project.ComponentAuth, project.ComponentOAuth} {
		if !seen[key] {
			t.Fatalf("expected app wizard to include auth component %s", key)
		}
	}
	model.setComponentSelected(project.ComponentAuth, true)
	model.normalizeComponentSelections()
	if !model.componentSelected(project.ComponentCache) {
		t.Fatalf("expected the visible Cache row to be selected with Auth")
	}
	model.applyComponentSelection()
	if !model.components.Auth {
		t.Fatalf("expected selected Auth to remain enabled: %#v", model.components)
	}
	if !model.components.Cache {
		t.Fatalf("expected selected Auth to include its Cache dependency: %#v", model.components)
	}
}

func TestWizardMarksDefaultStarterKitSelected(t *testing.T) {
	model := initialAppWizardModel("reporting", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
				WebUI:  true,
			},
			StarterKit: project.StarterKitVue,
		},
	})

	for _, raw := range model.starterKitList.Items() {
		item := raw.(starterKitItem)
		if item.Key == project.StarterKitVue && !item.Selected {
			t.Fatalf("expected vue starter kit to be selected")
		}
		if item.Key != project.StarterKitVue && item.Selected {
			t.Fatalf("expected only vue starter kit to be selected, got %s", item.Key)
		}
	}
	if !strings.Contains(model.renderStarterKitList(), "● Vue") {
		t.Fatalf("expected starter kit list to render selected marker, got %q", model.renderStarterKitList())
	}
}

func TestWizardUpdatesStarterKitSelectionFromCursor(t *testing.T) {
	model := initialAppWizardModel("reporting", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
				WebUI:  true,
			},
			StarterKit: project.StarterKitVue,
		},
	})

	model.starterKitList.Select(0)
	model.syncStarterKitSelectionFromCursor()

	if model.starterKit != project.StarterKitNone {
		t.Fatalf("expected starter kit to follow cursor selection, got %s", model.starterKit)
	}
	for _, raw := range model.starterKitList.Items() {
		item := raw.(starterKitItem)
		if item.Key == project.StarterKitNone && !item.Selected {
			t.Fatalf("expected none starter kit to be selected")
		}
		if item.Key == project.StarterKitVue && item.Selected {
			t.Fatalf("expected vue starter kit to be deselected")
		}
	}
}

func TestWizardViewKeepsLeadingSpacingWithoutTrailingBlankLine(t *testing.T) {
	model := initialAppWizardModel("reporting", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI: true,
				WebUI:  true,
			},
		},
	})

	view := model.View()
	if !strings.HasPrefix(view, "\n") {
		t.Fatalf("expected wizard view to start with one spacing newline, got %q", view[:min(len(view), 12)])
	}
	if strings.HasSuffix(view, "\n") {
		t.Fatalf("expected wizard view not to leave a trailing blank line")
	}
}

func TestWizardPanelHeaderMatchesPanelWidth(t *testing.T) {
	panel := wizardPanel("Components", "x", 90, true)
	lines := strings.Split(panel, "\n")
	if len(lines) < 3 {
		t.Fatalf("expected rendered panel lines, got %q", panel)
	}
	for idx, line := range lines[:3] {
		if got := lipgloss.Width(line); got != 90 {
			t.Fatalf("line %d width = %d, want 90: %q", idx, got, line)
		}
	}
}

// stubInteractiveTerminal centralizes stub interactive terminal behavior so callers follow the same contract.
func stubInteractiveTerminal(t *testing.T, interactive bool) func() {
	t.Helper()
	original := isInteractiveTerminal
	isInteractiveTerminal = func() bool { return interactive }
	return func() {
		isInteractiveTerminal = original
	}
}

// stubAppWizardRunner centralizes stub app wizard runner behavior so callers follow the same contract.
func stubAppWizardRunner(t *testing.T, runner func(string, *project.Config) (RenderOptions, error)) func() {
	t.Helper()
	original := appWizardRunner
	appWizardRunner = runner
	return func() {
		appWizardRunner = original
	}
}

// selectAppWizardComponent moves the wizard cursor to a component row by key.
func selectAppWizardComponent(t *testing.T, model *appWizardModel, key project.ComponentKey) {
	t.Helper()
	for idx, raw := range model.componentList.Items() {
		item := raw.(componentItem)
		if item.Key == key {
			model.componentList.Select(idx)
			return
		}
	}
	t.Fatalf("component %s not found", key)
}

type recordingRenderer struct {
	called       bool
	options      RenderOptions
	removeCalled bool
	removeApp    project.App
	removeResult RemoveResult
	removeErr    error
}

// RenderAppOnly records render attempts so cancellation tests can assert no files were written.
func (r *recordingRenderer) RenderAppOnly(_ project.App, options RenderOptions) error {
	r.called = true
	r.options = options
	return nil
}

// RemoveApp records removal attempts so command tests can verify the remove branch.
func (r *recordingRenderer) RemoveApp(target project.App) (RemoveResult, error) {
	r.removeCalled = true
	r.removeApp = target
	return r.removeResult, r.removeErr
}
