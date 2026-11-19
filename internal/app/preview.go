package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/messenger91/sql-tui/internal/export"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/session"
)

// previewState shows generated DML before it is applied (§12.3/§12.4).
type previewState struct {
	statements []result.Statement
	err        string
}

func (p *previewState) view() string {
	var b strings.Builder
	for i, statement := range p.statements {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(fmt.Sprintf("%d. ", i+1))
		b.WriteString(statement.SQL)
		if len(statement.Args) > 0 {
			b.WriteString("\n   args: ")
			parts := make([]string, 0, len(statement.Args))
			for _, arg := range statement.Args {
				parts = append(parts, previewArg(arg))
			}
			b.WriteString(strings.Join(parts, ", "))
		}
	}
	if p.err != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(p.err))
	}
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Faint(true).Render("enter apply · esc close"))
	return b.String()
}

func previewArg(value any) string {
	switch value.(type) {
	case nil:
		return "NULL"
	case string:
		return fmt.Sprintf("%q", value)
	default:
		return export.FormatValue(value)
	}
}

func (m Model) openPreview(s *session.Session) (tea.Model, tea.Cmd) {
	if s == nil || s.Pending == nil || s.Pending.Empty() {
		cmd := m.setNotice("no pending changes")
		return m, cmd
	}
	changes, err := s.Changes()
	if err != nil {
		cmd := m.setNotice(err.Error())
		return m, cmd
	}
	statements, err := result.BuildStatements(s.Driver, changes)
	if err != nil {
		cmd := m.setNotice(err.Error())
		return m, cmd
	}
	if len(statements) == 0 {
		cmd := m.setNotice("no effective changes")
		return m, cmd
	}
	m.preview = &previewState{statements: statements}
	return m, nil
}

func (m Model) handlePreviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.preview = nil
		return m, nil
	case "enter":
		m.preview = nil
		cmd := m.setNotice("transactional apply arrives with §12.4")
		return m, cmd
	}
	return m, nil
}
