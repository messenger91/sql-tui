package workspace

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestTableDropdownFetch(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")
	m.SetTables([]string{"users", "orders"})

	m, _ = m.Update(keyRunes("t"))
	if !m.tableMode {
		t.Fatal("t did not open the table dropdown")
	}
	if !strings.Contains(m.View(), "users") {
		t.Fatalf("tables not listed:\n%s", m.View())
	}

	m, cmd := m.Update(keyType(tea.KeyEnter))
	msgs := collect(cmd)
	var fetched *FetchTableMsg
	for _, msg := range msgs {
		if msg, ok := msg.(FetchTableMsg); ok {
			copied := msg
			fetched = &copied
		}
	}
	if fetched == nil || fetched.Table != "users" {
		t.Fatalf("fetch msg = %+v", fetched)
	}
	if m.tableMode {
		t.Fatal("dropdown stayed open")
	}
}

func TestTableDropdownEscAndEmpty(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")

	m.SetTables(nil)
	m, _ = m.Update(keyRunes("t"))
	if m.tableMode {
		t.Fatal("dropdown opened without tables")
	}

	m.SetTables([]string{"users"})
	m, _ = m.Update(keyRunes("t"))
	m, _ = m.Update(keyType(tea.KeyEsc))
	if m.tableMode {
		t.Fatal("esc did not close the dropdown")
	}
}
