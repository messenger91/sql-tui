package app

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/settings"
)

func settingsModel(t *testing.T, maxRows int) Model {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := settings.Save(path, settings.Settings{MaxRows: maxRows}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m := NewWithConfig(Config{
		ConnectionsPath: filepath.Join(dir, "connections.json"),
		FavoritesPath:   filepath.Join(dir, "favorites.json"),
		HistoryPath:     filepath.Join(dir, "history.json"),
		SettingsPath:    path,
	})
	m, _ = appUpdate(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, msg := range collectCmds(m.loadSettings()) {
		m, _ = appUpdate(t, m, msg)
	}
	return m
}

func TestSettingsCapLimitsRunnerRows(t *testing.T) {
	m := settingsModel(t, 100)
	m = openActive(t, m)
	m = runQuerySync(t, m, `CREATE TABLE nums (id INTEGER)`)
	values := make([]string, 0, 120)
	for i := 1; i <= 120; i++ {
		values = append(values, itoaTest(i))
	}
	_ = runQuerySync(t, m, `INSERT INTO nums (id) VALUES (`+strings.Join(values, "), (")+`)`)
	_ = runMsgSync(t, m, query.ExecuteRequestedMsg{SQL: "SELECT id FROM nums"})

	res, ok := m.activeSession().Results.Result()
	if !ok {
		t.Fatal("no result")
	}
	if res.RowCount != 100 || len(res.Rows) != 100 {
		t.Fatalf("rows = %d, want cap 100", res.RowCount)
	}
	if res.Fetched != 120 {
		t.Fatalf("fetched = %d, want 120 (drained)", res.Fetched)
	}
	if !res.Truncated {
		t.Fatal("expected truncation marker")
	}
}

func itoaTest(i int) string {
	return strconv.Itoa(i)
}

func TestSettingsLoadErrorKeepsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := settings.Load(path)
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if loaded.MaxRows != 10000 {
		t.Fatalf("fallback max rows = %d", loaded.MaxRows)
	}
}
