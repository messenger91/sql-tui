package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/workspace"
)

func activateWithSchema(t *testing.T, m Model) Model {
	t.Helper()
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	m, _ = appUpdate(t, m, workspace.ActivatedMsg{Connection: connection})
	m, _ = appUpdate(t, m, schemaLoadedMsg{ConnectionID: "c1", Tables: browserTables()})
	return m
}

func TestSchemaBrowserRequiresConnection(t *testing.T) {
	m := newSized(t, 80, 24)
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.browser != nil {
		t.Fatal("browser opened without a connection")
	}
	if !strings.Contains(m.notice, "select a connection") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestSchemaBrowserRequiresLoadedSchema(t *testing.T) {
	m := newSized(t, 80, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:"}
	m, _ = appUpdate(t, m, workspace.ActivatedMsg{Connection: connection}) // schemaLoad=true

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.browser != nil {
		t.Fatal("browser opened while the schema was loading")
	}
	if !strings.Contains(m.notice, "schema still loading") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestSchemaBrowserInsertIntoEditor(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	if m.browser == nil {
		t.Fatal("browser did not open")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Schema · main") {
		t.Fatalf("modal title missing:\n%s", view)
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRight}) // expand users
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyDown})  // move to id
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // insert

	if m.browser != nil {
		t.Fatal("browser did not close after insertion")
	}
	if got := m.query.SQL(); got != "id" {
		t.Fatalf("editor = %q", got)
	}
	if m.focus != FocusQuery {
		t.Fatalf("focus = %v", m.focus)
	}
}

func TestSchemaBrowserEscapeCloses(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.browser != nil {
		t.Fatal("esc did not close the browser")
	}
}

func TestSchemaBrowserRefreshTriggersLoad(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})

	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	if cmd == nil {
		t.Fatal("ctrl+r did not start a schema refresh")
	}
	if !m.schemaLoad {
		t.Fatal("schemaLoad not set by refresh")
	}
	if m.browser == nil {
		t.Fatal("browser closed during refresh")
	}
}

func TestSchemaBrowserUpdatesFromLoadResult(t *testing.T) {
	m := activateWithSchema(t, newSized(t, 80, 24))
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlB})

	m, _ = appUpdate(t, m, schemaLoadedMsg{
		ConnectionID: "c1",
		Tables:       []db.TableMeta{{Schema: "main", Name: "fresh_table", Kind: db.TableKindTable}},
	})
	if view := ansi.Strip(m.browser.View()); !strings.Contains(view, "fresh_table") {
		t.Fatalf("browser not refreshed:\n%s", view)
	}
}
