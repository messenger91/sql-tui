package app

import (
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/history"
)

func TestPersistentHistoryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	cfg := Config{
		ConnectionsPath: filepath.Join(dir, "connections.json"),
		FavoritesPath:   filepath.Join(dir, "favorites.json"),
		HistoryPath:     path,
	}

	m := NewWithConfig(cfg)
	m, _ = appUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	m = openActive(t, m)
	_ = runQuerySync(t, m, "SELECT 42 AS answer")

	queries, err := history.NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(queries) != 1 || !strings.Contains(queries[0], "SELECT 42") {
		t.Fatalf("history = %#v", queries)
	}

	// A fresh model loads history and seeds the editor recall buffer.
	reloaded := NewWithConfig(cfg)
	reloaded, _ = appUpdate(t, reloaded, tea.WindowSizeMsg{Width: 80, Height: 24})
	reloaded, _ = appUpdate(t, reloaded, historyLoadedMsg{Queries: queries})
	reloaded.focus = FocusQuery
	reloaded, _ = appUpdate(t, reloaded, tea.KeyMsg{Type: tea.KeyCtrlUp})
	if got := reloaded.activeSession().Editor.SQL(); !strings.Contains(got, "SELECT 42") {
		t.Fatalf("editor history recall = %q", got)
	}
}

func TestNewTabsInheritLoadedHistory(t *testing.T) {
	m := newSized(t, 80, 24)
	m, _ = appUpdate(t, m, historyLoadedMsg{Queries: []string{"SELECT 1"}})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	m.focus = FocusQuery
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlUp})
	if got := m.activeSession().Editor.SQL(); got != "SELECT 1" {
		t.Fatalf("new tab history = %q", got)
	}
}
