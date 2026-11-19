package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/history"
)

type historyLoadedMsg struct {
	Queries []string
	Err     error
}

type historySavedMsg struct{}

func (m Model) loadHistory() tea.Cmd {
	store := m.historyStore
	return func() tea.Msg {
		if store == nil {
			return historyLoadedMsg{}
		}
		queries, err := store.Load()
		return historyLoadedMsg{Queries: queries, Err: err}
	}
}

// recordHistory appends SQL to the shared history and persists it.
func (m *Model) recordHistory(sql string) tea.Cmd {
	m.history = history.Append(m.history, sql)
	queries := append([]string(nil), m.history...)
	store := m.historyStore
	return func() tea.Msg {
		if store != nil {
			_ = store.Save(queries)
		}
		return historySavedMsg{}
	}
}
