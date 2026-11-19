package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/session"
)

const applyTimeout = 30 * time.Second

type applyDoneMsg struct {
	SessionID session.ID
	Outcome   session.ApplyOutcome
}

// startApply snapshots pending changes and runs the transaction in a command.
func (m Model) startApply(s *session.Session) (tea.Model, tea.Cmd) {
	if s == nil {
		return m, nil
	}
	if s.Running() {
		cmd := m.setNotice("wait for the query to finish")
		return m, cmd
	}
	plan, err := s.PrepareApply()
	if err != nil {
		cmd := m.setNotice(err.Error())
		return m, cmd
	}
	handle, ok := m.manager.Get(s.ConnectionID)
	if !ok {
		m.openModal("Apply failed", "connection is not open")
		return m, nil
	}
	sessionID := s.ID

	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
		defer cancel()
		return applyDoneMsg{SessionID: sessionID, Outcome: plan.Apply(ctx, handle)}
	}
}

func (m Model) handleApplyDone(msg applyDoneMsg) (tea.Model, tea.Cmd) {
	s, ok := m.sessions.Get(msg.SessionID)
	if !ok {
		return m, nil
	}
	outcome := msg.Outcome

	if outcome.Err != nil {
		if errors.Is(outcome.Err, session.ErrRowChanged) {
			m.needsRefresh = true
			m.openModal("Row changed", "Row changed since load — refresh required.\n\nesc to close, then r to refresh")
			return m, nil
		}
		body := outcome.Err.Error()
		if outcome.FailingSQL != "" {
			body = outcome.FailingSQL + "\n\n" + body
		}
		m.openModal("Apply failed", body)
		return m, nil
	}

	s.Pending.RevertAll()
	m.syncPending(s)
	m.needsRefresh = false

	if s.LastSQL == "" {
		cmd := m.setNotice(fmt.Sprintf("%d statement(s) applied", outcome.Applied))
		return m, cmd
	}
	// Re-run the original query to reflect the committed data (MVP refresh).
	updated, refreshCmd := m.startQuery(s.LastSQL)
	model := updated.(Model)
	noticeCmd := model.setNotice(fmt.Sprintf("%d statement(s) applied", outcome.Applied))
	return model, tea.Batch(refreshCmd, noticeCmd)
}
