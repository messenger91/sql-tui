package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/session"
	"github.com/messenger91/sql-tui/internal/workspace"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	dir := t.TempDir()
	return NewWithConfig(Config{
		ConnectionsPath: filepath.Join(dir, "connections.json"),
		FavoritesPath:   filepath.Join(dir, "favorites.json"),
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

func TestLongModalStaysWithinScreen(t *testing.T) {
	m := newSized(t, 80, 24)
	m.openModal("Long error", strings.Repeat("statement failed because of a very long reason\n", 60))
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != 24 {
		t.Fatalf("modal screen has %d lines", len(lines))
	}
	for i, line := range lines {
		if width := ansi.StringWidth(line); width > 80 {
			t.Fatalf("line %d width %d > 80", i, width)
		}
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
	if view := ansi.Strip(m.workspace.View()); strings.Contains(view, "connecting") {
		t.Fatalf("workspace still shows the connecting spinner:\n%s", view)
	}
}

func TestSchemaResultStoredPerConnection(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	updated, _ := m.Update(workspace.ActivatedMsg{Connection: connection})
	m = updated.(Model)

	if !m.schemaLoad {
		t.Fatal("schema load flag not set on activation")
	}
	if !strings.Contains(ansi.Strip(m.View()), "schema: loading…") {
		t.Fatalf("status missing loading state:\n%s", m.View())
	}

	updated, _ = m.Update(schemaLoadedMsg{ConnectionID: "c1", Tables: []db.TableMeta{{Name: "users"}}})
	m = updated.(Model)
	if m.schemaLoad {
		t.Fatal("schema load flag not cleared")
	}
	entry, ok := m.schema.Get("c1")
	if !ok || len(entry.Tables) != 1 || entry.Tables[0].Name != "users" {
		t.Fatalf("entry = %+v", entry)
	}

	updated, _ = m.Update(schemaLoadedMsg{ConnectionID: "other", Tables: []db.TableMeta{{Name: "other"}}})
	m = updated.(Model)
	if entry, ok := m.schema.Get("other"); !ok || len(entry.Tables) != 1 {
		t.Fatal("schema for a non-active connection was not cached")
	}
	if entry, _ := m.schema.Get("c1"); len(entry.Tables) != 1 || entry.Tables[0].Name != "users" {
		t.Fatalf("active schema changed: %+v", entry)
	}
}

func TestSchemaPartialFailureStatus(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	updated, _ := m.Update(workspace.ActivatedMsg{Connection: connection})
	m = updated.(Model)

	updated, _ = m.Update(schemaLoadedMsg{
		ConnectionID: "c1",
		Tables:       []db.TableMeta{{Name: "users"}},
		Err:          errors.New("permission denied"),
	})
	m = updated.(Model)
	if !strings.Contains(ansi.Strip(m.View()), "schema: 1 object (partial)") {
		t.Fatalf("status missing partial warning:\n%s", m.View())
	}

	updated, _ = m.Update(schemaLoadedMsg{ConnectionID: "c1", Err: errors.New("permission denied")})
	m = updated.(Model)
	if !strings.Contains(ansi.Strip(m.View()), "schema: unavailable") {
		t.Fatalf("status missing unavailable state:\n%s", m.View())
	}
}

func TestDeactivationClearsActiveConnection(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	updated, _ := m.Update(workspace.ActivatedMsg{Connection: connection})
	m = updated.(Model)
	updated, _ = m.Update(schemaLoadedMsg{ConnectionID: "c1", Tables: []db.TableMeta{{Name: "users"}}})
	m = updated.(Model)

	updated, _ = m.Update(workspace.DeactivatedMsg{})
	m = updated.(Model)
	if m.active != nil || m.schemaLoad {
		t.Fatalf("active=%+v loading=%v", m.active, m.schemaLoad)
	}
	if _, ok := m.schema.Get("c1"); !ok {
		t.Fatal("schema cache should survive deactivation")
	}
	if strings.Contains(ansi.Strip(m.View()), "conn:") {
		t.Fatalf("status still shows a connection:\n%s", m.View())
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

// runQuerySync starts the async runner with a fake sender, waits for all
// events, and feeds them into the model in order.
func runQuerySync(t *testing.T, m Model, sql string) Model {
	t.Helper()
	var mu sync.Mutex
	var messages []tea.Msg
	m.SetSender(func(msg tea.Msg) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	})

	updated, cmd := m.Update(query.ExecuteRequestedMsg{SQL: sql})
	m = updated.(Model)
	if cmd != nil {
		_ = collectCmds(cmd)
	}
	m.runners.wait()

	mu.Lock()
	pending := append([]tea.Msg(nil), messages...)
	mu.Unlock()
	for _, msg := range pending {
		updated, _ = m.Update(msg)
		m = updated.(Model)
	}
	return m
}

func TestQueryExecutionReturnsRows(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	m = runQuerySync(t, m, "SELECT 1 AS n")

	if s := m.activeSession(); s.State != session.StateDone {
		t.Fatalf("session state = %v", s.State)
	}
	if m.focus != FocusResult {
		t.Fatalf("focus = %v, want results", m.focus)
	}
	res, ok := m.activeSession().Results.Result()
	if !ok || len(res.Rows) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if len(res.Columns) != 1 || res.Columns[0] != "n" {
		t.Fatalf("columns = %v", res.Columns)
	}
	if got := res.Rows[0][0]; got != int64(1) {
		t.Fatalf("value = %#v", got)
	}
}

func TestQueryExecutionRunsAllStatements(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	sql := "CREATE TABLE t (x INTEGER); INSERT INTO t VALUES (7); SELECT x FROM t"

	m = runQuerySync(t, m, sql)
	if s := m.activeSession(); s.State != session.StateDone {
		t.Fatalf("session state = %v", s.State)
	}
	res, ok := m.activeSession().Results.Result()
	if !ok || len(res.Rows) != 1 {
		t.Fatalf("result = %+v", res)
	}
	if got := res.Rows[0][0]; got != int64(7) {
		t.Fatalf("value = %#v", got)
	}
}

func TestQueryExecutionFailureKeepsFocus(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))

	m = runQuerySync(t, m, "SELECT * FROM missing")
	if s := m.activeSession(); s.State != session.StateError {
		t.Fatalf("session state = %v", s.State)
	}
	if m.focus != FocusWorkspace {
		t.Fatalf("focus changed on failure: %v", m.focus)
	}
	if !strings.Contains(m.activeSession().Editor.View(), "error:") {
		t.Fatalf("query pane missing error:\n%s", m.activeSession().Editor.View())
	}
}

func TestQueryWithoutConnectionIsRejected(t *testing.T) {
	m := newSized(t, 80, 24)
	updated, cmd := m.Update(query.ExecuteRequestedMsg{SQL: "SELECT 1"})
	m = updated.(Model)
	if cmd != nil {
		t.Fatal("executed without a connection")
	}
	if !strings.Contains(m.activeSession().Editor.View(), "no connection selected") {
		t.Fatalf("query pane = %q", m.activeSession().Editor.View())
	}
}

func TestResultTableRendersInPane(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))

	m = runQuerySync(t, m, "SELECT 42 AS answer")

	view := ansi.Strip(m.View())
	for _, want := range []string{"answer", "42", "table"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	for i, line := range strings.Split(view, "\n") {
		if width := ansi.StringWidth(line); width > 80 {
			t.Fatalf("line %d width %d > 80: %q", i, width, line)
		}
	}
}

func TestViewToggleCyclesFormats(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	m = executeSQL(t, m, "SELECT 42 AS answer")

	steps := []result.Format{result.FormatCSV, result.FormatJSON, result.FormatTable}
	for _, want := range steps {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
		m = updated.(Model)
		if m.activeSession().Results.Format() != want {
			t.Fatalf("format = %q, want %q", m.activeSession().Results.Format(), want)
		}
	}
}

func TestViewToggleIgnoredOutsideResults(t *testing.T) {
	m := newSized(t, 80, 24) // workspace focus
	before := m.activeSession().Results.Format()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(Model)
	if m.activeSession().Results.Format() != before {
		t.Fatalf("format changed to %q outside results", m.activeSession().Results.Format())
	}
}

func TestViewKeyIsTypedInEditor(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = updated.(Model)
	if got := m.activeSession().Editor.SQL(); got != "v" {
		t.Fatalf("editor = %q, want the v keystroke", got)
	}
}
