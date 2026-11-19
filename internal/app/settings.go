package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/settings"
)

type settingsLoadedMsg struct {
	Settings settings.Settings
	Err      error
}

func (m Model) loadSettings() tea.Cmd {
	store := m.settingsPath()
	return func() tea.Msg {
		loaded, err := settings.Load(store)
		return settingsLoadedMsg{Settings: loaded, Err: err}
	}
}

func (m Model) settingsPath() string { return m.config.SettingsPath }

// effectiveMaxRows is the configured in-memory row cap for queries.
func (m Model) effectiveMaxRows() int {
	if m.settings.MaxRows > 0 {
		return m.settings.MaxRows
	}
	return db.DefaultMaxRows
}
