package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/workspace"
)

func TestSchemaTablesReachWorkspace(t *testing.T) {
	m := activateEditable(t, false)
	for _, msg := range collectCmds(m.workspace.Init()) {
		m, _ = appUpdate(t, m, msg)
	}

	m.focus = FocusWorkspace
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !strings.Contains(m.workspace.View(), "users") {
		t.Fatalf("table dropdown missing users:\n%s", m.workspace.View())
	}
}

func TestFetchTableRunsLimitedSelect(t *testing.T) {
	m := editableWithRows(t, false)
	s := m.activeSession()
	_ = runMsgSync(t, m, workspace.FetchTableMsg{Table: "users"})

	res, ok := s.Results.Result()
	if !ok || len(res.Rows) != 2 {
		t.Fatalf("result = %+v", res)
	}
	if s.Editor.SQL() != "" {
		t.Fatalf("editor changed: %q", s.Editor.SQL())
	}
	if !strings.Contains(s.Title, `SELECT * FROM "users"`) {
		t.Fatalf("title = %q", s.Title)
	}
	if res.Fetched != 2 || res.Fetched != res.RowCount {
		t.Fatalf("fetched = %d rows = %d", res.Fetched, res.RowCount)
	}
}

func TestFetchTableQuoting(t *testing.T) {
	m := editableWithRows(t, false)
	sql := m.fetchTableSQL(`we"ird`)
	if sql != `SELECT * FROM "we""ird" LIMIT 100` {
		t.Fatalf("sql = %q", sql)
	}
}

func TestAppTableToggleViaWorkspace(t *testing.T) {
	m := activateEditable(t, false)
	m.focus = FocusWorkspace
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	if !m.workspace.TablesVisible() {
		t.Fatal("t did not toggle the table dropdown through the app")
	}
}

func TestConnectionFormWritableThroughWorkspace(t *testing.T) {
	m := newSized(t, 100, 24)
	m.focus = FocusWorkspace
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if !m.workspace.CapturesInput() {
		t.Fatal("form did not open")
	}
	for _, r := range "local" {
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	for _, r := range ":memory:" {
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectCmds(cmd) {
		m, _ = appUpdate(t, m, msg)
	}
	if m.workspace.CapturesInput() {
		t.Fatal("form still open after submit")
	}
	if len(m.workspace.WorkspaceConnections()) != 1 {
		t.Fatalf("connections = %d", len(m.workspace.WorkspaceConnections()))
	}
	if conn := m.workspace.WorkspaceConnections()[0]; conn.ReadOnly || conn.Name != "local" {
		t.Fatalf("connection = %+v", conn)
	}
}

func TestTableInfoDocOpens(t *testing.T) {
	m := activateEditable(t, false)
	for _, msg := range collectCmds(m.workspace.Init()) {
		m, _ = appUpdate(t, m, msg)
	}
	m.focus = FocusWorkspace
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	msg, ok := cmd().(workspace.TableInfoMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	m = runMsgSync(t, m, msg)
	if m.doc == nil {
		t.Fatalf("table info doc did not open; notice=%q", m.notice)
	}
	view := ansi.Strip(m.doc.viewport.View())
	for _, want := range []string{"Columns", "id", "email"} {
		if !strings.Contains(view, want) {
			t.Fatalf("table info missing %q:\n%s", want, view)
		}
	}
}

func TestDBMLDocOpens(t *testing.T) {
	m := activateEditable(t, false)
	m.focus = FocusWorkspace
	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'D'}})
	for _, msg := range collectCmds(cmd) {
		m, _ = appUpdate(t, m, msg)
	}
	if m.doc == nil {
		t.Fatal("DBML doc did not open")
	}
	view := ansi.Strip(m.doc.viewport.View())
	if !strings.Contains(view, "Table users {") || !strings.Contains(view, "id INTEGER [pk, not null]") {
		t.Fatalf("DBML missing content:\n%s", view)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.doc != nil {
		t.Fatal("esc did not close the doc viewer")
	}
}

func TestTableListHidesDescription(t *testing.T) {
	m := activateEditable(t, false)
	for _, msg := range collectCmds(m.workspace.Init()) {
		m, _ = appUpdate(t, m, msg)
	}
	m.focus = FocusWorkspace
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	view := ansi.Strip(m.workspace.View())
	if strings.Contains(view, "SELECT *") || strings.Contains(view, "LIMIT 100") {
		t.Fatalf("fetch description leaked into the table list:\n%s", view)
	}
	if !strings.Contains(view, "users") {
		t.Fatalf("table list missing users:\n%s", view)
	}
}
