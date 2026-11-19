package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newFavoritesModel(t *testing.T) (Model, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "favorites.json")
	m := NewWithConfig(Config{
		ConnectionsPath: filepath.Join(dir, "connections.json"),
		FavoritesPath:   path,
	})
	model, _ := appUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return model, path
}

func appUpdate(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(msg)
	model, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update returned %T", updated)
	}
	return model, cmd
}

func typeRunes(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func saveFavoriteFromQuery(t *testing.T, m Model, name, sql string) Model {
	t.Helper()
	m.focus = FocusQuery
	m.activeSession().Editor.SetSQL(sql)

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, _, ok := m.favorites.Modal(); !ok {
		t.Fatal("ctrl+s did not open the save dialog")
	}
	m = typeRunes(t, m, name)

	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("save produced no command")
	}
	m, _ = appUpdate(t, m, cmd())
	return m
}

func TestSaveFavoriteFromQuery(t *testing.T) {
	m, path := newFavoritesModel(t)
	m = saveFavoriteFromQuery(t, m, "one", "SELECT 1 AS v")

	if _, _, ok := m.favorites.Modal(); ok {
		t.Fatal("save dialog still open")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read favorites: %v", err)
	}
	if !strings.Contains(string(data), `"name": "one"`) || !strings.Contains(string(data), `"sql": "SELECT 1 AS v"`) {
		t.Fatalf("favorites file = %s", data)
	}
}

func TestSaveFavoriteWithEmptyEditor(t *testing.T) {
	m, _ := newFavoritesModel(t)
	m.focus = FocusQuery

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, _, ok := m.favorites.Modal(); ok {
		t.Fatal("save dialog opened for empty SQL")
	}
	if !strings.Contains(m.notice, "nothing to save") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestBrowseAndLoadFavorite(t *testing.T) {
	m, _ := newFavoritesModel(t)
	m = saveFavoriteFromQuery(t, m, "one", "SELECT 1")
	m.activeSession().Editor.SetSQL("SELECT 0")
	m.focus = FocusQuery

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if _, _, ok := m.favorites.Modal(); !ok {
		t.Fatal("ctrl+f did not open favorites")
	}

	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("load produced no command")
	}
	m, _ = appUpdate(t, m, cmd())

	if got := m.activeSession().Editor.SQL(); got != "SELECT 1" {
		t.Fatalf("editor = %q", got)
	}
	if m.focus != FocusQuery {
		t.Fatalf("focus = %v", m.focus)
	}
	if !strings.Contains(m.notice, "loaded favorite") || !strings.Contains(m.notice, "one") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestFavoriteKeysOutsideQueryAreIgnored(t *testing.T) {
	m, _ := newFavoritesModel(t)
	m.focus = FocusWorkspace

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, _, ok := m.favorites.Modal(); ok {
		t.Fatal("ctrl+s opened outside the query pane")
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if _, _, ok := m.favorites.Modal(); ok {
		t.Fatal("ctrl+f opened outside the query pane")
	}
}
