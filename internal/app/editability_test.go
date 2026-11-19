package app

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/workspace"
)

func editabilityTables() []db.TableMeta {
	return []db.TableMeta{{
		Schema: "main", Name: "users", Kind: db.TableKindTable,
		Columns: []db.ColumnMeta{
			{Name: "id", Type: "INTEGER", IsPrimaryKey: true, Ordinal: 0},
			{Name: "email", Type: "TEXT", Nullable: true, Ordinal: 1},
		},
	}}
}

func activateEditable(t *testing.T, readOnly bool) Model {
	t.Helper()
	m := newSized(t, 120, 24)
	connection := db.Connection{ID: "c1", Name: "local", Driver: db.DriverSQLite, DSN: ":memory:", ReadOnly: readOnly}
	if _, err := m.manager.Open(context.Background(), connection); err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = m.manager.CloseAll() })
	m, _ = appUpdate(t, m, workspace.ActivatedMsg{Connection: connection})
	m.schema.Store("c1", editabilityTables(), nil)
	m, _ = appUpdate(t, m, schemaLoadedMsg{ConnectionID: "c1", Tables: editabilityTables()})

	handle, _ := m.manager.Get("c1")
	if _, err := handle.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return m
}

func TestEditableResultStatus(t *testing.T) {
	m := activateEditable(t, false)
	m = runQuerySync(t, m, "SELECT id, email FROM users")

	e := m.activeSession().Editability
	if !e.Editable || e.Table != "users" || strings.Join(e.PrimaryKey, ",") != "id" {
		t.Fatalf("editability = %+v", e)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "editable: PK id") {
		t.Fatalf("status missing editability:\n%s", view)
	}
}

func TestReadOnlyConnectionIsNotEditable(t *testing.T) {
	m := activateEditable(t, true)
	m = runQuerySync(t, m, "SELECT id, email FROM users")

	e := m.activeSession().Editability
	if e.Editable || e.Reason != "connection read-only" {
		t.Fatalf("editability = %+v", e)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "read-only (connection read-only)") {
		t.Fatalf("status missing read-only reason:\n%s", view)
	}
}

func TestComplexQueryIsNotEditable(t *testing.T) {
	m := activateEditable(t, false)
	m = runQuerySync(t, m, "SELECT id FROM users WHERE id IN (1, 2)")

	if e := m.activeSession().Editability; e.Editable || e.Reason != "complex query" {
		t.Fatalf("editability = %+v", e)
	}
}

func TestEditabilityResetsOnNewQuery(t *testing.T) {
	m := activateEditable(t, false)
	m = runQuerySync(t, m, "SELECT id, email FROM users")
	if !m.activeSession().Editability.Editable {
		t.Fatal("expected editable result")
	}

	m = runQuerySync(t, m, "SELECT id FROM users WHERE id IN (1, 2)")
	if e := m.activeSession().Editability; e.Editable || e.Reason != "complex query" {
		t.Fatalf("editability = %+v", e)
	}
}
