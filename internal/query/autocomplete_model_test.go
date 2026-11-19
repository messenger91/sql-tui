package query

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func autocompleteModel(t *testing.T) Model {
	t.Helper()
	m := New()
	m.SetSize(80, 20)
	m.SetSchema(testIndex())
	return m
}

func TestPopupAppearsAndAccepts(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM us")
	m.editor.CursorEnd()

	m = m.computeSuggestions()
	if !m.PopupVisible() {
		t.Fatal("popup did not open")
	}
	if m.popup.items[0].Name != "users" {
		t.Fatalf("items = %+v", m.popup.items)
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if m.PopupVisible() {
		t.Fatal("popup still open after accept")
	}
	if got := m.editor.Value(); got != "SELECT * FROM users" {
		t.Fatalf("editor = %q", got)
	}
}

func TestPopupAcceptsAliasPrefix(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT u. FROM users u")
	m.editor.SetCursor(9) // after "u."

	m = m.computeSuggestions()
	if !m.PopupVisible() {
		t.Fatal("popup did not open")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	value := m.editor.Value()
	if !strings.HasPrefix(value, "SELECT u.") {
		t.Fatalf("editor = %q", value)
	}
	if !strings.Contains(value, "u.email") && !strings.Contains(value, "u.id") {
		t.Fatalf("editor = %q", value)
	}
}

func TestPopupNavigation(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM u")
	m.editor.CursorEnd()
	m = m.computeSuggestions()
	if !m.PopupVisible() {
		t.Fatal("popup did not open")
	}
	if len(m.popup.items) < 2 {
		t.Fatalf("items = %+v", m.popup.items)
	}
	second := m.popup.items[1].Name

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated
	if m.popup.cursor != 1 {
		t.Fatalf("cursor = %d", m.popup.cursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if got, want := m.editor.Value(), "SELECT * FROM "+second; got != want {
		t.Fatalf("editor = %q, want %q", got, want)
	}
}

func TestPopupEscapeDismisses(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM us")
	m.editor.CursorEnd()
	m = m.computeSuggestions()
	if !m.PopupVisible() {
		t.Fatal("popup did not open")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated
	if m.PopupVisible() {
		t.Fatal("popup still open after esc")
	}
	if got := m.editor.Value(); got != "SELECT * FROM us" {
		t.Fatalf("editor changed: %q", got)
	}
}

func TestPopupDebounce(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM u")
	m.editor.CursorEnd()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated
	if m.PopupVisible() {
		t.Fatal("popup appeared before debounce elapsed")
	}
	if cmd == nil {
		t.Fatal("no debounce command scheduled")
	}

	var tick suggestTickMsg
	found := false
	for _, msg := range collect(cmd) {
		if msg, ok := msg.(suggestTickMsg); ok {
			tick = msg
			found = true
		}
	}
	if !found {
		t.Fatal("no suggestTickMsg")
	}
	m, _ = m.Update(tick)
	if !m.PopupVisible() {
		t.Fatal("popup did not open after debounce")
	}
}

func TestPopupIgnoresStaleDebounce(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM u")
	m.editor.CursorEnd()

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = updated
	var tick suggestTickMsg
	for _, msg := range collect(cmd) {
		if msg, ok := msg.(suggestTickMsg); ok {
			tick = msg
		}
	}
	m.editor.InsertString("x")
	m, _ = m.Update(tick)
	if m.PopupVisible() {
		t.Fatal("stale debounce opened the popup")
	}
}

func TestPopupManualTrigger(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM us")
	m.editor.CursorEnd()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlAt}) // ctrl+space
	m = updated
	if !m.PopupVisible() {
		t.Fatal("ctrl+space did not open the popup")
	}
}

func TestPopupShrinksEditor(t *testing.T) {
	m := autocompleteModel(t)
	full := m.editor.Height()
	m.editor.SetValue("SELECT * FROM u")
	m.editor.CursorEnd()
	m = m.computeSuggestions()
	if !m.PopupVisible() {
		t.Fatal("popup did not open")
	}
	if m.editor.Height() >= full {
		t.Fatalf("editor height %d, want less than %d", m.editor.Height(), full)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated
	if m.editor.Height() != full {
		t.Fatalf("editor height not restored: %d", m.editor.Height())
	}
}

func TestPopupRendersItems(t *testing.T) {
	m := autocompleteModel(t)
	m.editor.SetValue("SELECT * FROM us")
	m.editor.CursorEnd()
	m = m.computeSuggestions()
	view := m.View()
	if !strings.Contains(view, "users") || !strings.Contains(view, "tab/enter accept") {
		t.Fatalf("view:\n%s", view)
	}
}
