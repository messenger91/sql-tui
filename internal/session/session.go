// Package session owns query tabs: one editor and result view per session,
// plus the manager that tracks their order and focus. Async execution arrives
// in §11.3.
package session

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/result"
)

// ID identifies a query session (tab).
type ID string

// State is the execution state of a session.
type State string

const (
	StateIdle      State = "idle"
	StateRunning   State = "running"
	StateDone      State = "done"
	StateCancelled State = "cancelled"
	StateError     State = "error"
)

// Session is one query tab.
type Session struct {
	ID           ID
	ConnectionID string
	Title        string

	Editor  query.Model
	Results result.Model

	State     State
	StartedAt time.Time

	ReadOnly    bool
	Driver      db.Driver
	LastSQL     string
	Editability Editability
	Pending     *Pending

	cancel context.CancelFunc
}

// New returns an idle session with an empty editor and result view.
func New(id ID, connectionID string) *Session {
	return &Session{
		ID:           id,
		ConnectionID: connectionID,
		Title:        "untitled",
		Editor:       query.New(),
		Results:      result.New(),
		State:        StateIdle,
		ReadOnly:     true,
		Pending:      NewPending(),
	}
}

// ResetPending clears pending edits (for example before a fresh query run).
func (s *Session) ResetPending() {
	if s.Pending == nil {
		s.Pending = NewPending()
		return
	}
	s.Pending.RevertAll()
}

// SetSQL replaces the editor contents and updates the tab title.
func (s *Session) SetSQL(sql string) {
	s.Editor.SetSQL(sql)
	s.Title = DeriveTitle(sql)
}

// MarkRunning records a new execution and stores its cancel function.
func (s *Session) MarkRunning(cancel context.CancelFunc) {
	s.cancel = cancel
	s.State = StateRunning
	s.StartedAt = time.Now()
}

// MarkDone records a successful execution.
func (s *Session) MarkDone() {
	s.cancel = nil
	s.State = StateDone
}

// MarkCancelled records a cancelled execution.
func (s *Session) MarkCancelled() {
	s.cancel = nil
	s.State = StateCancelled
}

// MarkFailed records a failed execution.
func (s *Session) MarkFailed() {
	s.cancel = nil
	s.State = StateError
}

// Cancel stops the running query, if any.
func (s *Session) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

// Running reports whether a query is in flight.
func (s *Session) Running() bool { return s.State == StateRunning }

// DeriveTitle summarises SQL for the tab bar. It skips leading comments,
// collapses whitespace, and truncates long statements.
func DeriveTitle(sql string) string {
	cleaned := strings.TrimSpace(sql)
	for {
		switch {
		case strings.HasPrefix(cleaned, "--"):
			if newline := strings.IndexByte(cleaned, '\n'); newline >= 0 {
				cleaned = strings.TrimSpace(cleaned[newline+1:])
				continue
			}
			cleaned = ""
		case strings.HasPrefix(cleaned, "/*"):
			if end := strings.Index(cleaned, "*/"); end >= 0 {
				cleaned = strings.TrimSpace(cleaned[end+2:])
				continue
			}
			cleaned = ""
		}
		break
	}

	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" {
		return "untitled"
	}
	runes := []rune(cleaned)
	if len(runes) > 24 {
		return string(runes[:23]) + "…"
	}
	return cleaned
}

// Label renders the tab bar entry.
func (s *Session) Label(position int) string {
	marker := ""
	if s.Running() {
		marker = "● "
	}
	return fmt.Sprintf("%d: %s%s", position, marker, s.Title)
}
