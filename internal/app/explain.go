package app

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/session"
)

type explainDoneMsg struct {
	SessionID session.ID
	Title     string
	Plan      string
	Err       error
}

func (m Model) startExplain() (tea.Model, tea.Cmd) {
	s := m.activeSession()
	if s == nil {
		return m, nil
	}
	statements := query.SplitStatements(s.Editor.SQL())
	if len(statements) == 0 {
		cmd := m.setNotice("nothing to explain")
		return m, cmd
	}
	connectionID := s.ConnectionID
	driver := s.Driver
	if connectionID == "" && m.active != nil {
		connectionID = m.active.ID
		driver = m.active.Driver
		s.ConnectionID = connectionID
		s.Driver = driver
	}
	handle, ok := m.manager.Get(connectionID)
	if !ok {
		cmd := m.setNotice("connection is not open")
		return m, cmd
	}

	m.notice = "explaining…"
	sessionID := s.ID
	title := s.Title
	statement := statements[0]
	return m, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		plan, err := db.Explain(ctx, handle, driver, statement, 500)
		return explainDoneMsg{SessionID: sessionID, Title: title, Plan: plan, Err: err}
	}
}
