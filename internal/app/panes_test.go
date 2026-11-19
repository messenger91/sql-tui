package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestPaneHotkeys(t *testing.T) {
	m := newSized(t, 80, 24)

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if m.focus != FocusResult {
		t.Fatalf("R focus = %v", m.focus)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	if m.focus != FocusQuery {
		t.Fatalf("Q focus = %v", m.focus)
	}
	// The editor starts in insert mode, so Esc is needed before E navigates.
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if m.focus != FocusWorkspace {
		t.Fatalf("E focus = %v", m.focus)
	}
}

func TestPaneHotkeysTypeInInsertMode(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	m.syncFocus()

	for _, r := range "EQR" {
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.focus != FocusQuery {
		t.Fatalf("focus changed while typing: %v", m.focus)
	}
	if got := m.activeSession().Editor.SQL(); !strings.HasSuffix(got, "EQR") {
		t.Fatalf("editor = %q", got)
	}
}

func TestPaneHotkeysInVimNormalMode(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	m.syncFocus()

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.activeSession().Editor.NormalMode() {
		t.Fatal("esc did not enter normal mode")
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	if m.focus != FocusResult {
		t.Fatalf("R in normal mode focus = %v", m.focus)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'E'}})
	if m.focus != FocusWorkspace {
		t.Fatalf("E in normal mode focus = %v", m.focus)
	}
}

func TestVimModeInPaneTitle(t *testing.T) {
	m := newSized(t, 80, 24)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Query · INSERT") {
		t.Fatalf("insert mode missing from title:\n%s", view)
	}
	m.focus = FocusQuery
	m.syncFocus()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Query · NORMAL") {
		t.Fatalf("normal mode missing from title:\n%s", view)
	}
	if !strings.Contains(ansi.Strip(m.View()), "focus: query") {
		t.Fatalf("focus line changed:\n%s", m.View())
	}
}

func TestHelpOpensInVimNormalMode(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	m.syncFocus()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if !m.helpOpen {
		t.Fatal("? did not open help in normal mode")
	}
}
