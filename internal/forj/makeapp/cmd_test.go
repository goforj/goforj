package makeapp

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
		"components":  {Components: "web-api"},
		"without":     {Without: "web-ui"},
		"starter-kit": {StarterKit: "vue"},
	} {
		t.Run(name, func(t *testing.T) {
			if cmd.shouldRunWizard() {
				t.Fatalf("expected explicit %s selection to skip wizard", name)
			}
		})
	}
}

func TestCmdSkipsWizardByDefaultOutsideInteractiveTerminal(t *testing.T) {
	restore := stubInteractiveTerminal(t, false)
	defer restore()

	cmd := &Cmd{}
	if cmd.shouldRunWizard() {
		t.Fatal("expected non-interactive make:app to use default target selection")
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
	restoreWizard := stubAppWizardRunner(t, func(string, *project.Config) (project.Components, project.StarterKit, bool, error) {
		return project.Components{}, project.StarterKitNone, true, nil
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

func TestCmdRunTreatsExistingTargetAsNormalExit(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.MkdirAll(filepath.Join("cmd", "billing"), 0o755); err != nil {
		t.Fatalf("mkdir app: %v", err)
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

func TestCmdRunRemovesTarget(t *testing.T) {
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

func TestCmdRunTreatsMissingRemoveTargetAsNormalExit(t *testing.T) {
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
	restoreWizard := stubAppWizardRunner(t, func(string, *project.Config) (project.Components, project.StarterKit, bool, error) {
		return project.Components{}, project.StarterKitNone, true, nil
	})
	defer restoreWizard()

	cmd := &Cmd{Name: "reporting"}
	_, _, err := cmd.appSelection(&project.Config{
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

func TestWizardShowsAuthComponents(t *testing.T) {
	model := initialAppWizardModel("reporting", &project.Config{
		Render: project.RenderConfig{
			Components: project.Components{
				WebAPI:        true,
				Auth:          true,
				OAuth:         true,
				DatabaseMySQL: true,
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

func stubInteractiveTerminal(t *testing.T, interactive bool) func() {
	t.Helper()
	original := isInteractiveTerminal
	isInteractiveTerminal = func() bool { return interactive }
	return func() {
		isInteractiveTerminal = original
	}
}

func stubAppWizardRunner(t *testing.T, runner func(string, *project.Config) (project.Components, project.StarterKit, bool, error)) func() {
	t.Helper()
	original := appWizardRunner
	appWizardRunner = runner
	return func() {
		appWizardRunner = original
	}
}

type recordingRenderer struct {
	called       bool
	removeCalled bool
	removeApp    project.App
	removeResult RemoveResult
	removeErr    error
}

// RenderAppOnly records render attempts so cancellation tests can assert no files were written.
func (r *recordingRenderer) RenderAppOnly(project.App, RenderOptions) error {
	r.called = true
	return nil
}

// RemoveApp records removal attempts so command tests can verify the remove branch.
func (r *recordingRenderer) RemoveApp(target project.App) (RemoveResult, error) {
	r.removeCalled = true
	r.removeApp = target
	return r.removeResult, r.removeErr
}
