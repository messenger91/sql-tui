package app

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/result"
)

// handleMouse implements click-to-focus, tab-bar clicks, and the "+ new tab"
// affordance. Wheel events are handled natively by bubbles/list, table, and
// viewport once the program enables mouse support.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return m, nil
	}
	if m.anyOverlayOpen() {
		return m, nil
	}

	lay := m.layout()
	rightStart := lay.workspaceW
	if msg.X >= rightStart {
		// Results header: clicking a column title sorts by it.
		if msg.Y == lay.queryH+2 {
			if s := m.activeSession(); s != nil && s.Results.Format() == result.FormatTable {
				if name, ok := s.Results.ColumnAt(msg.X - rightStart - 2); ok {
					m.focus = FocusResult
					m.syncFocus()
					s.Results.SelectColumn(name)
					m.sortResultsByColumn(s, name)
					return m, nil
				}
			}
		}
		// Query/results column. Rows: 0 border, 1 title, 2 tab bar, 3+ editor.
		if msg.Y == 2 && m.activeSession() != nil {
			if index, onPlus, ok := m.tabAt(msg.X - rightStart - 2); ok {
				order := m.sessions.Sessions()
				switch {
				case onPlus:
					m.newTab()
				case index >= 0 && index < len(order):
					m.sessions.SetActive(order[index].ID)
					m.afterTabSwitch()
				}
				return m, nil
			}
		}
		if msg.Y >= 1 && msg.Y < 1+lay.queryH {
			m.focus = FocusQuery
		} else {
			m.focus = FocusResult
		}
		m.syncFocus()
		return m, nil
	}
	if !m.wsHidden {
		m.focus = FocusWorkspace
		m.syncFocus()
	}
	return m, nil
}

// anyOverlayOpen reports whether a modal owns the screen; clicks behind it are
// ignored so users cannot accidentally change focus.
func (m Model) anyOverlayOpen() bool {
	if m.export != nil || m.cellEdit != nil || m.rowMenu != nil || m.preview != nil ||
		m.browser != nil || m.confirmClose != "" || m.modal != nil {
		return true
	}
	if _, _, ok := m.favoritesDialog(); ok {
		return true
	}
	_, _, ok := m.workspace.Modal()
	return ok
}

// tabAt maps a click position (relative to the tab bar content) to a tab
// index. onPlus marks the "+" affordance.
func (m Model) tabAt(contentX int) (int, bool, bool) {
	if contentX < 0 {
		return 0, false, false
	}
	position := 0
	for index, label := range m.tabLabels() {
		width := ansi.StringWidth(label)
		if contentX >= position && contentX < position+width {
			if label == "+" {
				return index, true, true
			}
			return index, false, true
		}
		position += width + 3 // " │ "
	}
	return 0, false, false
}
