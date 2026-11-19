package app

import (
	"github.com/messenger91/sql-tui/internal/session"
)

// sortResultsByColumn sorts the results table and keeps pending edits attached
// to their rows by remapping them through the sort permutation.
func (m *Model) sortResultsByColumn(s *session.Session, name string) {
	if s == nil {
		return
	}
	perm, ok := s.Results.SortColumn(name)
	if !ok {
		return
	}
	s.Pending.Remap(perm)
	m.syncPending(s)
}

// editorNormalMode reports whether the query editor is in Vim normal mode.
func (m Model) editorNormalMode() bool {
	s := m.activeSession()
	return s != nil && s.Editor.NormalMode()
}

// paneHotkeysActive reports whether E/Q/R should switch panes. They stay
// ordinary characters while typing in the editor or filtering a list.
func (m Model) paneHotkeysActive() bool {
	switch m.focus {
	case FocusQuery:
		return m.editorNormalMode()
	case FocusWorkspace:
		return !m.workspace.Filtering()
	default:
		return true
	}
}
