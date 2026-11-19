package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/db"
)

func newTestWorkspace(t *testing.T) (Model, *Store, *db.Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	manager := db.NewManager()
	t.Cleanup(func() { _ = manager.CloseAll() })

	m := New(store, manager)
	m.SetSize(60, 20)
	m = dispatch(t, m, m.Init())
	return m, store, manager, path
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func keyType(k tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: k}
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		updated, _ := m.Update(keyRunes(string(r)))
		m = updated
	}
	return m
}

// collect runs a command (expanding batches) and returns its leaf messages.
// Commands returned by Update are deliberately not followed, so spinner ticks
// cannot loop forever.
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var messages []tea.Msg
		for _, nested := range batch {
			messages = append(messages, collect(nested)...)
		}
		return messages
	}
	return []tea.Msg{msg}
}

func dispatch(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range collect(cmd) {
		updated, _ := m.Update(msg)
		m = updated
	}
	return m
}

func addSQLiteConnection(t *testing.T, m Model, name string) Model {
	t.Helper()
	m, _ = m.Update(keyRunes("a"))
	if m.form == nil {
		t.Fatal("a did not open the add form")
	}
	m = typeText(t, m, name)
	m, _ = m.Update(keyType(tea.KeyTab))
	m, _ = m.Update(keyType(tea.KeyTab))
	m = typeText(t, m, ":memory:")
	m, cmd := m.Update(keyType(tea.KeyEnter))
	if m.form != nil {
		t.Fatal("form still open after enter")
	}
	if cmd == nil {
		t.Fatal("submit produced no command")
	}
	return dispatch(t, m, cmd)
}

func TestAddConnectionPersistsAndOpens(t *testing.T) {
	m, store, manager, path := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")

	if m.busy {
		t.Fatal("still busy after connect")
	}
	if m.modalErr != nil {
		t.Fatalf("modal error: %v", m.modalErr)
	}
	if len(m.connections) != 1 {
		t.Fatalf("connections = %+v", m.connections)
	}
	conn := m.connections[0]
	if conn.Name != "local" || conn.Driver != db.DriverSQLite || !conn.ReadOnly {
		t.Fatalf("connection = %+v", conn)
	}
	if _, ok := manager.Get(conn.ID); !ok {
		t.Fatal("connection handle is not open")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("connections file: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 1 || loaded[0].ID != conn.ID || loaded[0].Name != "local" {
		t.Fatalf("persisted = %+v", loaded)
	}
}

func TestActivateEmitsActivatedMsg(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")
	wantID := m.connections[0].ID

	m, cmd := m.Update(keyType(tea.KeyEnter))
	if !m.busy {
		t.Fatal("activation did not start")
	}
	var activated *ActivatedMsg
	for _, msg := range collect(cmd) {
		if msg, ok := msg.(ActivatedMsg); ok {
			copied := msg
			activated = &copied
		}
	}
	if activated == nil {
		t.Fatal("no ActivatedMsg emitted")
	}
	if activated.Connection.ID != wantID {
		t.Fatalf("activated %q, want %q", activated.Connection.ID, wantID)
	}
}

func TestDeleteConnection(t *testing.T) {
	m, store, manager, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")
	id := m.connections[0].ID

	m, _ = m.Update(keyRunes("d"))
	if m.confirm == nil {
		t.Fatal("d did not open the delete confirmation")
	}

	m, cmd := m.Update(keyRunes("y"))
	m = dispatch(t, m, cmd)

	if len(m.connections) != 0 {
		t.Fatalf("connections after delete = %+v", m.connections)
	}
	if _, ok := manager.Get(id); ok {
		t.Fatal("handle still open after delete")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded) != 0 {
		t.Fatalf("persisted = %+v", loaded)
	}
}

func TestDeleteConfirmCancel(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")

	m, _ = m.Update(keyRunes("d"))
	m, _ = m.Update(keyRunes("n"))
	if m.confirm != nil {
		t.Fatal("n did not cancel the delete")
	}
	if len(m.connections) != 1 {
		t.Fatalf("connections = %+v", m.connections)
	}
}

func TestLoadsPersistedConnections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	if err := store.Save([]db.Connection{{
		ID: "saved", Name: "Saved", Driver: db.DriverSQLite, DSN: ":memory:", ReadOnly: true,
	}}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	manager := db.NewManager()
	t.Cleanup(func() { _ = manager.CloseAll() })
	m := New(store, manager)
	m.SetSize(60, 20)
	m = dispatch(t, m, m.Init())

	if len(m.connections) != 1 || m.connections[0].Name != "Saved" {
		t.Fatalf("connections = %+v", m.connections)
	}
	if got := m.list.SelectedItem(); got == nil {
		t.Fatal("list has no selected item")
	}
}

func TestFilterTypingDoesNotTriggerAdd(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")

	m, _ = m.Update(keyRunes("/"))
	m = typeText(t, m, "a")
	if m.form != nil {
		t.Fatal("typing in the filter opened the add form")
	}
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("filter state = %v", m.list.FilterState())
	}
}

func TestConnectFailureShowsError(t *testing.T) {
	m, store, _, path := newTestWorkspace(t)
	m, _ = m.Update(keyRunes("a"))
	m = typeText(t, m, "broken")
	m, _ = m.Update(keyType(tea.KeyTab))
	m, _ = m.Update(keyType(tea.KeyTab))
	m = typeText(t, m, filepath.Join(t.TempDir(), "missing", "db.sqlite"))

	m, cmd := m.Update(keyType(tea.KeyEnter))
	m = dispatch(t, m, cmd)

	if m.modalErr == nil {
		t.Fatal("expected a connection error")
	}
	if len(m.connections) != 0 {
		t.Fatalf("connections = %+v", m.connections)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed connection must not be persisted, stat err = %v", err)
	}
	if loaded, err := store.Load(); err != nil || len(loaded) != 0 {
		t.Fatalf("Load = %v, %v", loaded, err)
	}
}

func TestActivationSucceededClearsBusy(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	m = addSQLiteConnection(t, m, "local")

	m, cmd := m.Update(keyType(tea.KeyEnter))
	if !m.busy {
		t.Fatal("activation did not start")
	}
	for _, msg := range collect(cmd) {
		if activated, ok := msg.(ActivatedMsg); ok {
			m.ActivationSucceeded(activated.Connection.ID)
		}
	}
	if m.busy {
		t.Fatal("activation success did not clear the busy state")
	}
	if strings.Contains(m.View(), "connecting") {
		t.Fatalf("view still shows connecting:\n%s", m.View())
	}
}

func TestEmptyStateView(t *testing.T) {
	m, _, _, _ := newTestWorkspace(t)
	view := m.View()
	if view == "" {
		t.Fatal("empty view")
	}
	if !strings.Contains(view, "no connections yet") {
		t.Fatalf("view = %q", view)
	}
}
