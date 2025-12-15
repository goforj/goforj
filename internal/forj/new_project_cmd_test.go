package forj

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if m.stage != StageConfirm {
		t.Fatalf("expected confirmation stage after selecting components")
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
