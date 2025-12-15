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
