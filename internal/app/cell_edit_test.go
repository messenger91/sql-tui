package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/session"
)

func editableWithRows(t *testing.T, readOnly bool) Model {
	t.Helper()
	m := activateEditable(t, readOnly)
	handle, _ := m.manager.Get("c1")
	if _, err := handle.Exec(`INSERT INTO users (id, email) VALUES (1, 'a'), (2, 'b')`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	return runQuerySync(t, m, "SELECT id, email FROM users")
}

func TestEditCellStoresPendingValue(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()

	row, _, original, ok := s.Results.SelectedCell()
	if !ok {
		t.Fatal("no selected cell")
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.cellEdit == nil {
		t.Fatal("e did not open the cell editor")
	}
	m.cellEdit.input.SetValue("42")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.cellEdit != nil {
		t.Fatalf("editor stays open: %s", m.cellEdit.err)
	}

	edit, ok := s.Pending.Cell(row, "id")
	if !ok || edit.Value != int64(42) || edit.Original != original {
		t.Fatalf("edit = %+v", edit)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "1 pending") || !strings.Contains(view, "* ") {
		t.Fatalf("pending not rendered:\n%s", view)
	}
}

func TestEditCellValidationKeepsEditorOpen(t *testing.T) {
	m := editableWithRows(t, false)
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.cellEdit.input.SetValue("not-a-number")

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.cellEdit == nil || m.cellEdit.err == "" {
		t.Fatal("invalid value did not keep the editor open with an error")
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.cellEdit != nil {
		t.Fatal("esc did not close the editor")
	}
	if !m.activeSession().Pending.Empty() {
		t.Fatal("invalid value was stored")
	}
}

func TestEditCellNullToggle(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight}) // email column
	row, _, _, _ := s.Results.SelectedCell()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlN})
	if m.cellEdit == nil || !m.cellEdit.isNull {
		t.Fatalf("ctrl+n did not toggle NULL: %+v", m.cellEdit)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	edit, ok := s.Pending.Cell(row, "email")
	if !ok || !edit.IsNull || edit.Value != nil {
		t.Fatalf("edit = %+v", edit)
	}
}

func TestRevertKeys(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.cellEdit.input.SetValue("42")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if s.Pending.Count() != 1 {
		t.Fatal("edit not stored")
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlZ})
	if !s.Pending.Empty() {
		t.Fatalf("ctrl+z did not revert the row: %+v", s.Pending)
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.cellEdit.input.SetValue("7")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ctrl+shift+z")})
	if !s.Pending.Empty() {
		t.Fatalf("ctrl+shift+z did not revert all: %+v", s.Pending)
	}
}

func TestRowMenuOperations(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()

	row, _, _, _ := s.Results.SelectedCell()

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if m.rowMenu == nil {
		t.Fatal("m did not open the row menu")
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if s.Pending.RowOp(row) != session.RowDelete {
		t.Fatalf("row op = %q", s.Pending.RowOp(row))
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	if s.Pending.RowOp(row) != session.RowDuplicate {
		t.Fatalf("row op = %q", s.Pending.RowOp(row))
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.rowMenu == nil || m.rowMenu.err == "" {
		t.Fatal("nullable check missing for id column")
	}
	if _, ok := s.Pending.Cell(row, "id"); ok {
		t.Fatal("non-nullable column was set to NULL")
	}
}

func TestReadOnlyResultRejectsEditing(t *testing.T) {
	m := editableWithRows(t, true)
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	if m.cellEdit != nil {
		t.Fatal("cell editor opened on a read-only result")
	}
	if !strings.Contains(m.notice, "read-only") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestPendingSurvivesViewAndTabSwitches(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.cellEdit.input.SetValue("42")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	// table -> CSV -> JSON -> table
	for i := 0; i < 3; i++ {
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	}
	if !strings.Contains(ansi.Strip(m.View()), "* ") {
		t.Fatalf("pending marker lost after view toggle:\n%s", m.View())
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if m.activeSession().ID != s.ID {
		t.Fatal("did not return to the edited tab")
	}
	if s.Pending.Count() != 1 {
		t.Fatalf("pending lost across tab switch: %+v", s.Pending)
	}
}

func TestNewQueryClearsPending(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.cellEdit.input.SetValue("42")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})

	m = runQuerySync(t, m, "SELECT id, email FROM users")
	if !s.Pending.Empty() {
		t.Fatalf("pending survived a new query: %+v", s.Pending)
	}
	if strings.Contains(ansi.Strip(m.View()), "pending") {
		t.Fatalf("status still shows pending:\n%s", m.View())
	}
}
