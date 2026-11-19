package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/workspace"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	return NewWithConfig(Config{
		ConnectionsPath: filepath.Join(t.TempDir(), "connections.json"),
	})
}

func newSized(t *testing.T, width, height int) Model {
	t.Helper()
	updated, _ := newTestModel(t).Update(tea.WindowSizeMsg{Width: width, Height: height})
	return updated.(Model)
}

func press(m Model, msg tea.KeyMsg) (Model, tea.Cmd) {
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestFocusCycling(t *testing.T) {
	m := newSized(t, 80, 24)
	if m.focus != FocusWorkspace {
		t.Fatalf("initial focus = %v", m.focus)
	}

	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusQuery {
		t.Fatalf("after tab focus = %v", m.focus)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusResult {
		t.Fatalf("after second tab focus = %v", m.focus)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusWorkspace {
		t.Fatalf("focus must wrap to workspace, got %v", m.focus)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != FocusResult {
		t.Fatalf("shift+tab focus = %v", m.focus)
	}
}

func TestQuitKey(t *testing.T) {
	m := newSized(t, 80, 24)
	_, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c returned no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c did not produce tea.QuitMsg")
	}
}

func TestWindowSizeUpdatesHelpWidth(t *testing.T) {
	m := newSized(t, 120, 40)
	if m.width != 120 || m.height != 40 {
		t.Fatalf("size = %dx%d", m.width, m.height)
	}
	if m.help.Width != 120 {
		t.Fatalf("help width = %d", m.help.Width)
	}
}

func TestHelpKeyTogglesFullHelp(t *testing.T) {
	m := newSized(t, 80, 24)
	helpKey := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}

	m, _ = press(m, helpKey)
	if !m.help.ShowAll {
		t.Fatal("? did not expand help")
	}
	m, _ = press(m, helpKey)
	if m.help.ShowAll {
		t.Fatal("? did not collapse help")
	}
}

func TestModalBlocksPaneKeysButNotQuit(t *testing.T) {
	m := newSized(t, 80, 24)
	m.openModal("Favorites", "nothing here")

	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusWorkspace {
		t.Fatalf("tab changed focus while a modal was open: %v", m.focus)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.modal != nil {
		t.Fatal("esc did not close the modal")
	}

	m.openModal("Favorites", "nothing here")
	_, cmd := press(m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("ctrl+c must quit even with a modal open")
	}
}

func TestSessionEventIsRoutedWithoutPanic(t *testing.T) {
	m := newSized(t, 80, 24)
	updated, cmd := m.Update(sessionEventMsg{SessionID: "s1", Event: 1})
	if cmd != nil {
		t.Fatal("session events are not expected to emit commands yet")
	}
	if updated.(Model).focus != FocusWorkspace {
		t.Fatal("routing a session event changed focus")
	}
}

func TestViewFillsTerminal(t *testing.T) {
	sizes := []struct{ width, height int }{
		{80, 24},
		{120, 40},
		{61, 19},
	}
	for _, size := range sizes {
		m := newSized(t, size.width, size.height)
		view := ansi.Strip(m.View())
		lines := strings.Split(view, "\n")
		if len(lines) != size.height {
			t.Fatalf("%dx%d: got %d lines", size.width, size.height, len(lines))
		}
		for i, line := range lines {
			if got := lipgloss.Width(line); got != size.width {
				t.Fatalf("%dx%d: line %d width = %d, want %d: %q", size.width, size.height, i, got, size.width, line)
			}
		}
	}
}

func TestViewContainsPaneTitles(t *testing.T) {
	view := ansi.Strip(newSized(t, 80, 24).View())
	for _, want := range []string{"Connections", "Query", "Results", "focus: workspace"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestModalIsRenderedOverPanes(t *testing.T) {
	m := newSized(t, 80, 24)
	m.openModal("Export", "CSV or JSON")
	view := ansi.Strip(m.View())

	lines := strings.Split(view, "\n")
	if len(lines) != 24 {
		t.Fatalf("modal view has %d lines", len(lines))
	}
	if !strings.Contains(view, "Export") || !strings.Contains(view, "CSV or JSON") {
		t.Fatalf("modal content missing:\n%s", view)
	}
	// The modal body is left-aligned inside a centred box, so it must sit in
	// the middle of the screen rather than against either edge. Exact overlay
	// maths is covered by the overlay tests.
	for _, line := range lines {
		if idx := strings.Index(line, "CSV or JSON"); idx >= 0 {
			if idx == 0 || idx >= 40 {
				t.Fatalf("modal not centred: index %d in %q", idx, line)
			}
		}
	}
}

func TestTinyTerminalDoesNotPanic(t *testing.T) {
	m := newSized(t, 20, 6)
	if view := m.View(); view == "" {
		t.Fatal("empty view at tiny size")
	}
}

func TestActivationFocusesQueryAndLoadsSchema(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{
		ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:", ReadOnly: true,
	}

	updated, cmd := m.Update(workspace.ActivatedMsg{Connection: connection})
	m = updated.(Model)

	if m.focus != FocusQuery {
		t.Fatalf("focus = %v, want query", m.focus)
	}
	if m.active == nil || m.active.ID != "c1" {
		t.Fatalf("active = %+v", m.active)
	}
	if cmd == nil {
		t.Fatal("activation did not schedule a schema load")
	}
}

func TestSchemaResultOnlyAppliesToActiveConnection(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	updated, _ := m.Update(workspace.ActivatedMsg{Connection: connection})
	m = updated.(Model)

	updated, _ = m.Update(schemaLoadedMsg{ConnectionID: "c1", Tables: []db.TableMeta{{Name: "users"}}})
	m = updated.(Model)
	if len(m.schema) != 1 || m.schema[0].Name != "users" {
		t.Fatalf("schema = %+v", m.schema)
	}

	updated, _ = m.Update(schemaLoadedMsg{ConnectionID: "stale", Tables: []db.TableMeta{{Name: "other"}}})
	m = updated.(Model)
	if len(m.schema) != 1 || m.schema[0].Name != "users" {
		t.Fatalf("stale schema overwrote the active connection: %+v", m.schema)
	}
}

func TestDeactivationClearsActiveConnection(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	updated, _ := m.Update(workspace.ActivatedMsg{Connection: connection})
	m = updated.(Model)

	updated, _ = m.Update(workspace.DeactivatedMsg{})
	m = updated.(Model)
	if m.active != nil || m.schema != nil {
		t.Fatalf("active=%+v schema=%+v", m.active, m.schema)
	}
}

func openActive(t *testing.T, m Model) Model {
	t.Helper()
	connection := db.Connection{
		ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:", ReadOnly: true,
	}
	if _, err := m.manager.Open(context.Background(), connection); err != nil {
		t.Fatalf("open: %v", err)
	}
	m.active = &connection
	return m
}

func TestQueryExecutionReturnsRows(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))

	updated, cmd := m.Update(query.ExecuteRequestedMsg{SQL: "SELECT 1 AS n"})
	m = updated.(Model)
	if !m.query.Running() {
		t.Fatal("query pane not marked running")
	}
	if cmd == nil {
		t.Fatal("no execution command")
	}

	msg, ok := cmd().(queryFinishedMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	updated, _ = m.Update(msg)
	m = updated.(Model)

	if m.lastQueryErr != nil {
		t.Fatalf("query error: %v", m.lastQueryErr)
	}
	if m.focus != FocusResult {
		t.Fatalf("focus = %v, want results", m.focus)
	}
	if m.lastResult == nil || len(m.lastResult.Rows) != 1 {
		t.Fatalf("result = %+v", m.lastResult)
	}
	if len(m.lastResult.Columns) != 1 || m.lastResult.Columns[0] != "n" {
		t.Fatalf("columns = %v", m.lastResult.Columns)
	}
	if got := m.lastResult.Rows[0][0]; got != int64(1) {
		t.Fatalf("value = %#v", got)
	}
}

func TestQueryExecutionRunsAllStatements(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	sql := "CREATE TABLE t (x INTEGER); INSERT INTO t VALUES (7); SELECT x FROM t"

	updated, cmd := m.Update(query.ExecuteRequestedMsg{SQL: sql})
	m = updated.(Model)
	msg := cmd().(queryFinishedMsg)
	if msg.Statements != 3 {
		t.Fatalf("statements = %d", msg.Statements)
	}

	updated, _ = m.Update(msg)
	m = updated.(Model)
	if m.lastQueryErr != nil {
		t.Fatalf("query error: %v", m.lastQueryErr)
	}
	if m.lastResult == nil || len(m.lastResult.Rows) != 1 {
		t.Fatalf("result = %+v", m.lastResult)
	}
	if got := m.lastResult.Rows[0][0]; got != int64(7) {
		t.Fatalf("value = %#v", got)
	}
}

func TestQueryExecutionFailureKeepsFocus(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))

	updated, cmd := m.Update(query.ExecuteRequestedMsg{SQL: "SELECT * FROM missing"})
	m = updated.(Model)
	failed, ok := cmd().(queryFailedMsg)
	if !ok {
		t.Fatal("expected queryFailedMsg")
	}

	updated, _ = m.Update(failed)
	m = updated.(Model)
	if m.lastQueryErr == nil {
		t.Fatal("error not recorded")
	}
	if m.focus != FocusWorkspace {
		t.Fatalf("focus changed on failure: %v", m.focus)
	}
	if !strings.Contains(m.query.View(), "error:") {
		t.Fatalf("query pane missing error:\n%s", m.query.View())
	}
}

func TestQueryWithoutConnectionIsRejected(t *testing.T) {
	m := newSized(t, 80, 24)
	updated, cmd := m.Update(query.ExecuteRequestedMsg{SQL: "SELECT 1"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("executed without a connection")
	}
	if !strings.Contains(m.query.View(), "no connection selected") {
		t.Fatalf("query pane = %q", m.query.View())
	}
}
