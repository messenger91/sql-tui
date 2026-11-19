package workspace

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestFormValidation(t *testing.T) {
	form := newForm(formAdd)
	if _, ok := form.validate(); ok {
		t.Fatal("empty form validated")
	}
	if form.err == "" {
		t.Fatal("expected a validation message")
	}

	form.name.SetValue("local")
	if _, ok := form.validate(); ok {
		t.Fatal("form without a DSN validated")
	}

	form.dsn.SetValue(":memory:")
	conn, ok := form.validate()
	if !ok {
		t.Fatalf("valid form rejected: %s", form.err)
	}
	if conn.Driver != db.DriverSQLite || conn.DSN != ":memory:" || !conn.ReadOnly {
		t.Fatalf("connection = %+v", conn)
	}
}

func TestFormDriverCycleAndHints(t *testing.T) {
	form := newForm(formAdd)
	if form.driver != db.DriverSQLite {
		t.Fatalf("default driver = %s", form.driver)
	}
	form.cycleDriver(1)
	if form.driver != db.DriverMySQL {
		t.Fatalf("driver = %s", form.driver)
	}
	form.cycleDriver(1)
	if form.driver != db.DriverPostgres {
		t.Fatalf("driver = %s", form.driver)
	}
	form.cycleDriver(1)
	if form.driver != db.DriverSQLite {
		t.Fatalf("driver did not wrap: %s", form.driver)
	}
	form.cycleDriver(-1)
	if form.driver != db.DriverPostgres {
		t.Fatalf("reverse cycle = %s", form.driver)
	}
}

func TestFormTabMovesFocus(t *testing.T) {
	form := newForm(formAdd)
	if form.focus != 0 {
		t.Fatalf("initial focus = %d", form.focus)
	}
	form.update(tea.KeyMsg{Type: tea.KeyTab})
	if form.focus != 1 {
		t.Fatalf("focus after tab = %d", form.focus)
	}
	form.update(tea.KeyMsg{Type: tea.KeyTab})
	if form.focus != 2 {
		t.Fatalf("focus after second tab = %d", form.focus)
	}
	form.update(tea.KeyMsg{Type: tea.KeyTab})
	if form.focus != 3 {
		t.Fatalf("focus after third tab = %d", form.focus)
	}
	form.update(tea.KeyMsg{Type: tea.KeyTab})
	if form.focus != 0 {
		t.Fatalf("focus did not wrap: %d", form.focus)
	}
	form.update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if form.focus != 3 {
		t.Fatalf("shift+tab focus = %d", form.focus)
	}
}

func TestFormAccessToggle(t *testing.T) {
	form := newForm(formAdd)
	if !form.readOnly {
		t.Fatal("new connections must default to read-only")
	}
	form.focus = 3
	form.update(tea.KeyMsg{Type: tea.KeyRight})
	if form.readOnly {
		t.Fatal("access toggle did not switch to writable")
	}
	form.update(tea.KeyMsg{Type: tea.KeyLeft})
	if !form.readOnly {
		t.Fatal("access toggle did not switch back to read-only")
	}
}

func TestFormEditPreservesIdentity(t *testing.T) {
	conn := db.Connection{ID: "keep", Name: "old", Driver: db.DriverMySQL, DSN: "u:p@tcp(h)/db", ReadOnly: false}
	form := newForm(formEdit)
	form.setConnection(conn)

	if form.id != "keep" || form.readOnly {
		t.Fatalf("form identity = %q readonly=%v", form.id, form.readOnly)
	}
	updated, ok := form.validate()
	if !ok {
		t.Fatalf("valid edit rejected: %s", form.err)
	}
	if updated.ID != "keep" || updated.ReadOnly {
		t.Fatalf("connection = %+v", updated)
	}
	if updated.Driver != db.DriverMySQL {
		t.Fatalf("driver = %s", updated.Driver)
	}
}
