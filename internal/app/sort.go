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
