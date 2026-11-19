package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/session"
)

const applyTimeout = 30 * time.Second

type applyDoneMsg struct {
	SessionID session.ID
	Outcome   session.ApplyOutcome
	Mutations []session.RowMutation
	Refresh   bool // mutations computed successfully; view can refresh in place
	Order     int  // result order version captured when the apply started
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
	order := s.Results.OrderVersion()
	// Snapshot the result for the in-place post-commit refresh.
	snapshot, hasSnapshot := s.Results.Result()
	if hasSnapshot {
		snapshot = db.QueryResult{Columns: snapshot.Columns, Rows: snapshot.Rows}
	}

	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), applyTimeout)
		defer cancel()
		outcome := plan.Apply(ctx, handle)
		msg := applyDoneMsg{SessionID: sessionID, Outcome: outcome, Order: order}
		if outcome.Err != nil {
			return msg
		}
		if hasSnapshot {
			mutations, err := plan.RefreshMutations(ctx, handle, snapshot)
			if err == nil {
				msg.Mutations = mutations
				msg.Refresh = true
			}
			// Refresh errors fall back to re-running the original query.
		}
		return msg
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

	noticeText := fmt.Sprintf("%d statement(s) applied", outcome.Applied)
	if msg.Refresh && msg.Order != s.Results.OrderVersion() {
		// The result was re-sorted while the transaction ran; the mutation
		// indexes no longer match. Fall back to re-running the query.
		msg.Refresh = false
	}
	if msg.Refresh && len(msg.Mutations) > 0 && s.LastSQL != "" {
		// In-place refresh: replay the row mutations, keep editability.
		for _, mutation := range msg.Mutations {
			switch mutation.Kind {
			case session.RowReplace:
				s.Results.ReplaceRowValues(mutation.Index, mutation.Raw)
			case session.RowRemove:
				s.Results.RemoveRowResult(mutation.Index)
			default:
				s.Results.AppendRows([][]any{mutation.Raw})
			}
		}
		// Re-apply an active sort once every mutation is replayed, so the
		// indexes stay valid throughout the loop.
		s.Results.Resort()
		cmd := m.setNotice(noticeText)
		return m, cmd
	}

	if s.LastSQL == "" {
		cmd := m.setNotice(noticeText)
		return m, cmd
	}
	// Fallback refresh: re-run the original query.
	updated, refreshCmd := m.startQuery(s.LastSQL)
	model := updated.(Model)
	noticeCmd := model.setNotice(noticeText)
	return model, tea.Batch(refreshCmd, noticeCmd)
}
