package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// commandMenuState is the x context menu for bulk commands (§16).
type commandMenuState struct {
	menuCursor int
}

type commandItem struct {
	label string
	hint  string
	run   func(*Model) tea.Cmd
}

func (m Model) commandItems() []commandItem {
	return []commandItem{
		{label: "Add connection", hint: "a", run: func(m *Model) tea.Cmd {
			m.workspace.OpenAdd()
			return nil
		}},
		{label: "Edit connection", hint: "selected", run: func(m *Model) tea.Cmd {
			m.workspace.OpenEdit()
			return nil
		}},
		{label: "Delete connection", hint: "selected", run: func(m *Model) tea.Cmd {
			m.workspace.OpenDelete()
			return nil
		}},
		{label: "Connect selected", hint: "enter", run: func(m *Model) tea.Cmd {
			return m.workspace.ActivateSelected()
		}},
		{label: "New tab", hint: "ctrl+t", run: func(m *Model) tea.Cmd {
			m.newTab()
			return nil
		}},
		{label: "List favorites", hint: "ctrl+f", run: func(m *Model) tea.Cmd {
			m.favorites.OpenBrowse()
			return nil
		}},
		{label: "Save query as favorite", hint: "ctrl+s", run: func(m *Model) tea.Cmd {
			_, cmd := m.openSaveFavorite()
			return cmd
		}},
		{label: "Schema browser", hint: "ctrl+b", run: func(m *Model) tea.Cmd {
			_, cmd := m.openSchemaBrowser()
			return cmd
		}},
		{label: "Export results", hint: "ctrl+e", run: func(m *Model) tea.Cmd {
			_, cmd := m.openExport()
			return cmd
		}},
		{label: "View DBML", hint: "D", run: func(m *Model) tea.Cmd {
			_, cmd := m.openDBML()
			return cmd
		}},
		{label: "Refresh schema", hint: "", run: func(m *Model) tea.Cmd {
			if m.active == nil {
				return nil
			}
			return m.refreshSchema(*m.active)
		}},
		{label: "Quit", hint: "ctrl+c", run: func(m *Model) tea.Cmd {
			return tea.Quit
		}},
	}
}

func (m Model) openCommandMenu() (tea.Model, tea.Cmd) {
	m.menu = &commandMenuState{}
	return m, nil
}

func (m Model) handleCommandMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.commandItems()
	switch msg.String() {
	case "esc":
		m.menu = nil
		return m, nil
	case "up", "k", "ctrl+p":
		m.menu.menuCursor = max(m.menu.menuCursor-1, 0)
	case "down", "j", "ctrl+n":
		m.menu.menuCursor = min(m.menu.menuCursor+1, len(items)-1)
	case "enter":
		cursor := m.menu.menuCursor
		m.menu = nil
		if cursor >= 0 && cursor < len(items) {
			cmd := items[cursor].run(&m)
			return m, cmd
		}
	}
	return m, nil
}

func (m Model) commandMenuView() string {
	selected := lipgloss.NewStyle().Bold(true).Reverse(true)
	dim := lipgloss.NewStyle().Faint(true)
	lines := make([]string, 0, len(m.commandItems()))
	items := m.commandItems()
	for i, item := range items {
		label := item.label
		if item.hint != "" {
			label += dim.Render("  " + item.hint)
		}
		if i == m.menu.menuCursor {
			label = selected.Render("› " + item.label)
		} else {
			label = "  " + label
		}
		lines = append(lines, label)
	}
	lines = append(lines, lipgloss.NewStyle().Faint(true).Render(
		"↑/↓ move · enter run · esc close",
	))
	return strings.Join(lines, "\n")
}
