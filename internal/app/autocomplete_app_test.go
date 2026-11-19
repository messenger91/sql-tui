package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func collectCmds(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var messages []tea.Msg
		for _, nested := range batch {
			messages = append(messages, collectCmds(nested)...)
		}
		return messages
	}
	return []tea.Msg{msg}
}

func TestAutocompleteTabAcceptDoesNotCyclePanes(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))
	m.query.SetSQL("SELECT * FROM us")

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlAt}) // ctrl+space
	if !m.query.PopupVisible() {
		t.Fatal("popup did not open")
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusQuery {
		t.Fatalf("tab cycled panes while the popup was open: %v", m.focus)
	}
	if got := m.query.SQL(); got != "SELECT * FROM users" {
		t.Fatalf("editor = %q", got)
	}
}

func TestAutocompleteCapturesQuestionMark(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))
	m.query.SetSQL("SELECT * FROM us")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlAt})
	if !m.query.PopupVisible() {
		t.Fatal("popup did not open")
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.help.ShowAll {
		t.Fatal("'?' toggled help while the popup was open")
	}
	if !strings.Contains(m.query.SQL(), "?") {
		t.Fatalf("editor = %q", m.query.SQL())
	}
}

func TestAutocompleteDebounceThroughApp(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))
	m.query.SetSQL("SELECT * FROM u")

	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.query.PopupVisible() {
		t.Fatal("popup appeared before debounce")
	}
	if cmd == nil {
		t.Fatal("no debounce command")
	}
	for _, msg := range collectCmds(cmd) {
		m, _ = appUpdate(t, m, msg)
	}
	if !m.query.PopupVisible() {
		t.Fatal("popup did not open after debounce")
	}
}
