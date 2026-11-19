package favorites

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

func newTestFavorites(t *testing.T) (Model, *Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "favorites.json")
	store := NewStore(path)
	m := New(store)
	m.SetSize(80, 24)
	m = dispatchFavorites(t, m, m.Init())
	return m, store, path
}

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		updated, _ := m.Update(keyRunes(string(r)))
		m = updated
	}
	return m
}

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

func dispatchFavorites(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for _, msg := range collect(cmd) {
		updated, _ := m.Update(msg)
		m = updated
	}
	return m
}

func saveFavorite(t *testing.T, m Model, name, sql string) Model {
	t.Helper()
	m.OpenSave(sql)
	m = typeText(t, m, name)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if cmd == nil {
		t.Fatal("save produced no command")
	}
	return dispatchFavorites(t, m, cmd)
}

func TestSaveAndLoad(t *testing.T) {
	m, store, _ := newTestFavorites(t)
	m = saveFavorite(t, m, "one", "SELECT 1")

	if len(m.favorites) != 1 || m.favorites[0].Name != "one" {
		t.Fatalf("favorites = %+v", m.favorites)
	}
	if m.CapturesInput() {
		t.Fatal("save dialog still captures input")
	}
	persisted, err := store.Load()
	if err != nil || len(persisted) != 1 || persisted[0].SQL != "SELECT 1" {
		t.Fatalf("persisted = %+v, %v", persisted, err)
	}

	m.OpenBrowse()
	if !m.CapturesInput() {
		t.Fatal("browse overlay does not capture input")
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if cmd == nil {
		t.Fatal("load produced no command")
	}
	var loaded *LoadMsg
	for _, msg := range collect(cmd) {
		if msg, ok := msg.(LoadMsg); ok {
			copied := msg
			loaded = &copied
		}
	}
	if loaded == nil || loaded.SQL != "SELECT 1" || loaded.Name != "one" {
		t.Fatalf("loaded = %+v", loaded)
	}
	if m.visible {
		t.Fatal("overlay still visible after loading")
	}
}

func TestSaveRequiresName(t *testing.T) {
	m, _, _ := newTestFavorites(t)
	m.OpenSave("SELECT 1")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if cmd != nil {
		t.Fatal("blank name saved")
	}
	if m.save == nil || m.save.err == "" {
		t.Fatal("expected a validation error")
	}
}

func TestOverwritePrompt(t *testing.T) {
	m, store, _ := newTestFavorites(t)
	m = saveFavorite(t, m, "one", "SELECT 1")

	m.OpenSave("SELECT 2")
	m = typeText(t, m, "ONE")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if cmd != nil || m.overwrite == nil {
		t.Fatal("expected overwrite prompt")
	}

	// n keeps the form open without changing anything.
	updated, _ = m.Update(keyRunes("n"))
	m = updated
	if m.overwrite != nil || m.save == nil {
		t.Fatal("cancel did not return to the save form")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated
	if m.overwrite == nil {
		t.Fatal("prompt missing on retry")
	}
	updated, cmd = m.Update(keyRunes("y"))
	m = dispatchFavorites(t, updated, cmd)

	if len(m.favorites) != 1 {
		t.Fatalf("favorites = %+v", m.favorites)
	}
	if m.favorites[0].SQL != "SELECT 2" {
		t.Fatalf("sql = %q", m.favorites[0].SQL)
	}
	persisted, err := store.Load()
	if err != nil || len(persisted) != 1 || persisted[0].SQL != "SELECT 2" {
		t.Fatalf("persisted = %+v, %v", persisted, err)
	}
}

func TestDeleteFavorite(t *testing.T) {
	m, store, _ := newTestFavorites(t)
	m = saveFavorite(t, m, "one", "SELECT 1")
	m.OpenBrowse()

	updated, _ := m.Update(keyRunes("d"))
	m = updated
	if m.confirm == nil {
		t.Fatal("d did not open confirmation")
	}
	updated, cmd := m.Update(keyRunes("y"))
	m = dispatchFavorites(t, updated, cmd)

	if len(m.favorites) != 0 {
		t.Fatalf("favorites = %+v", m.favorites)
	}
	persisted, err := store.Load()
	if err != nil || len(persisted) != 0 {
		t.Fatalf("persisted = %+v, %v", persisted, err)
	}
}

func TestEmptyBrowseBody(t *testing.T) {
	m, _, _ := newTestFavorites(t)
	m.OpenBrowse()
	title, body, ok := m.Modal()
	if !ok || title != "Favorites" {
		t.Fatalf("modal = %q, %v", title, ok)
	}
	if !strings.Contains(body, "no favorites yet") {
		t.Fatalf("body = %q", body)
	}
}

func TestFilterTypingDoesNotDelete(t *testing.T) {
	m, _, _ := newTestFavorites(t)
	m = saveFavorite(t, m, "one", "SELECT 1")
	m.OpenBrowse()

	m = typeText(t, m, "/d")
	if m.confirm != nil {
		t.Fatal("typing in the filter triggered delete")
	}
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("filter state = %v", m.list.FilterState())
	}
}
