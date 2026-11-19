package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/session"
)

func sortableAppModel(t *testing.T) Model {
	t.Helper()
	m := activateEditable(t, false)
	handle, ok := m.manager.Get("c1")
	if !ok {
		t.Fatal("connection not open")
	}
	if _, err := handle.Exec(`INSERT INTO users (id, email) VALUES (3, 'b'), (1, 'c'), (2, 'a')`); err != nil {
		t.Fatalf("insert rows: %v", err)
	}
	m = runQuerySync(t, m, "SELECT id, email FROM users")
	return m
}

func resultIDs(m Model) []any {
	res, ok := m.activeSession().Results.Result()
	if !ok {
		return nil
	}
	ids := make([]any, 0, len(res.Rows))
	for _, row := range res.Rows {
		ids = append(ids, row[0])
	}
	return ids
}

func TestSortKeySortsSelectedColumn(t *testing.T) {
	m := sortableAppModel(t)
	s := m.activeSession()

	// The streaming view leaves the cursor on the last row (id=3, email b).
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})
	row, column, _, ok := s.Results.SelectedCell()
	if !ok || row != 2 || column != "email" {
		t.Fatalf("selected = %d %q %v", row, column, ok)
	}
	s.Pending.SetCell(row, "email", session.CellEdit{Value: "edited", Original: "b"})
	m.syncPending(s)

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})

	ids := resultIDs(m)
	want := []any{int64(2), int64(3), int64(1)}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}
	if !m.activeSession().Results.Sorted() {
		t.Fatal("sort state not active")
	}

	// Pending edits follow their row: id=3 moved from index 2 to index 1.
	if _, ok := s.Pending.Cell(1, "email"); !ok {
		t.Fatalf("pending edit lost after sort: %+v", s.Pending.Cells)
	}
	selRow, selColumn, selValue, _ := s.Results.SelectedCell()
	if selRow != 1 || selColumn != "email" || selValue != "b" {
		t.Fatalf("cursor = %d %q %v, want row 1 email=b", selRow, selColumn, selValue)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "email ▲") || !strings.Contains(view, "1 pending") {
		t.Fatalf("sorted view missing marker/pending:\n%s", view)
	}

	// Pressing s again flips the direction.
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	ids = resultIDs(m)
	want = []any{int64(1), int64(3), int64(2)}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("descending ids = %v, want %v", ids, want)
		}
	}
	if !strings.Contains(ansi.Strip(m.View()), "email ▼") {
		t.Fatalf("descending marker missing:\n%s", m.View())
	}
	if _, ok := s.Pending.Cell(1, "email"); !ok {
		t.Fatalf("pending edit lost after direction flip: %+v", s.Pending.Cells)
	}
}

func TestSortKeyIgnoredInEditor(t *testing.T) {
	m := sortableAppModel(t)
	m.focus = FocusQuery
	m.syncFocus()
	before := resultIDs(m)

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.activeSession().Results.Sorted() {
		t.Fatal("s sorted results while the editor was focused")
	}
	after := resultIDs(m)
	for i := range before {
		if before[i] != after[i] {
			t.Fatalf("rows changed while typing: %v -> %v", before, after)
		}
	}
	if !strings.HasSuffix(m.activeSession().Editor.SQL(), "s") {
		t.Fatalf("editor did not receive s: %q", m.activeSession().Editor.SQL())
	}
}

func TestMouseHeaderClickSorts(t *testing.T) {
	m := sortableAppModel(t)
	lay := m.layout()

	// First content cell of the results header: pane border + padding.
	x := lay.workspaceW + 2 + 1
	y := lay.queryH + 2
	m, _ = appUpdate(t, m, mouseClick(x, y))

	if m.focus != FocusResult {
		t.Fatalf("header click focus = %v", m.focus)
	}
	if !m.activeSession().Results.Sorted() {
		t.Fatal("header click did not sort")
	}
	if ids := resultIDs(m); ids[0] != int64(1) {
		t.Fatalf("ascending ids = %v", ids)
	}
	if !strings.Contains(ansi.Strip(m.View()), "id ▲") {
		t.Fatalf("ascending marker missing:\n%s", m.View())
	}

	// Clicking the same header flips to descending.
	m, _ = appUpdate(t, m, mouseClick(x, y))
	if ids := resultIDs(m); ids[0] != int64(3) {
		t.Fatalf("descending ids = %v", ids)
	}
	if !strings.Contains(ansi.Strip(m.View()), "id ▼") {
		t.Fatalf("descending marker missing:\n%s", m.View())
	}
}

func TestMouseHeaderClickOutsideColumnsOnlyFocuses(t *testing.T) {
	m := sortableAppModel(t)
	lay := m.layout()
	m.focus = FocusQuery
	m.syncFocus()

	// Far to the right, past the last header cell.
	m, _ = appUpdate(t, m, mouseClick(lay.workspaceW+lay.rightW-1, lay.queryH+2))
	if m.focus != FocusResult {
		t.Fatalf("focus = %v", m.focus)
	}
	if m.activeSession().Results.Sorted() {
		t.Fatal("click past the columns sorted the result")
	}
}
