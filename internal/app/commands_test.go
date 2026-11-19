package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestCommandMenuOpensAndRunsNewTab(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusResult

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.menu == nil {
		t.Fatal("x did not open the command menu")
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Command menu", "Add connection", "List favorites", "Quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("menu missing %q:\n%s", want, view)
		}
	}

	before := m.sessions.Len()
	for i := 0; i < 4; i++ { // navigate to "New tab"
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.menu != nil {
		t.Fatal("menu stayed open after enter")
	}
	if m.sessions.Len() != before+1 {
		t.Fatalf("sessions = %d, want %d", m.sessions.Len(), before+1)
	}
}

func TestCommandMenuAddConnection(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusResult

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // first item: Add connection
	if !m.workspace.CapturesInput() {
		t.Fatal("add connection form did not open")
	}
	if !strings.Contains(ansi.Strip(m.View()), "Add connection") {
		t.Fatal("add form not rendered")
	}
}

func TestCommandMenuQuits(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusResult
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	for i := 0; i < 10; i++ { // last item: Quit
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	_, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("quit produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit item did not emit tea.QuitMsg")
	}
}

func TestXTypesInEditor(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if got := m.activeSession().Editor.SQL(); got != "x" {
		t.Fatalf("editor = %q", got)
	}
	if m.menu != nil {
		t.Fatal("x opened the menu while the editor was focused")
	}
}
