// Package query owns the SQL editor pane: input, history, and execution
// requests. Execution itself is performed by the root model (later by the
// session manager in §11) so the editor never blocks on I/O.
package query

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
)

const maxHistory = 50

// ExecuteRequestedMsg asks the root model to run the given SQL.
type ExecuteRequestedMsg struct {
	SQL string
}

// Model is the query editor pane.
type Model struct {
	editor  textarea.Model
	spinner spinner.Model

	running       bool
	status        string
	statusIsError bool

	history      []string
	historyIndex int
	draft        string

	width, height int
}

// New returns an empty query editor.
func New() Model {
	editor := textarea.New()
	editor.Placeholder = "SELECT * FROM …"
	editor.ShowLineNumbers = false
	editor.CharLimit = 0
	editor.Focus()
	editor.SetWidth(80)
	editor.SetHeight(8)

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		editor:  editor,
		spinner: s,
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

// SQL returns the current editor contents.
func (m Model) SQL() string { return m.editor.Value() }

// SetSQL replaces the editor contents and focuses it.
func (m *Model) SetSQL(sql string) {
	m.editor.SetValue(sql)
	m.editor.Focus()
}

// SetSize resizes the editor, reserving one row for the status line.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 10)
	m.height = max(height, 2)
	m.editor.SetWidth(m.width)
	m.editor.SetHeight(max(m.height-1, 1))
}

// Running reports whether a query is in flight.
func (m Model) Running() bool { return m.running }

// MarkRunning blocks a second execution while the root model runs the query.
// It complements the state set by requestExecute for callers that inject an
// ExecuteRequestedMsg directly.
func (m *Model) MarkRunning() {
	m.running = true
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.running {
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+r", "f5":
			if m.running {
				return m, nil
			}
			return m.requestExecute()
		case "ctrl+up":
			m.historyBack()
			return m, nil
		case "ctrl+down":
			m.historyForward()
			return m, nil
		}
		var cmd tea.Cmd
		m.editor, cmd = m.editor.Update(msg)
		return m, cmd

	default:
		return m, nil
	}
}

func (m Model) requestExecute() (Model, tea.Cmd) {
	sql := strings.TrimSpace(m.editor.Value())
	if sql == "" {
		m.status = "nothing to run"
		m.statusIsError = false
		return m, nil
	}
	m.pushHistory(sql)
	m.running = true
	m.status = ""
	m.statusIsError = false
	return m, tea.Batch(
		func() tea.Msg { return ExecuteRequestedMsg{SQL: sql} },
		m.spinner.Tick,
	)
}

// ReportSuccess clears the running state and summarises the execution.
func (m *Model) ReportSuccess(statements int, result db.QueryResult) {
	m.running = false
	m.statusIsError = false

	summary := fmt.Sprintf("%s · %s · %s",
		countLabel(statements, "statement"), countLabel(result.RowCount, "row"), FormatElapsed(result.Elapsed))
	if result.Truncated {
		summary += " · truncated"
	}
	m.status = summary
}

func countLabel(count int, singular string) string {
	if count == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %ss", count, singular)
}

// FormatElapsed renders a duration for status lines, avoiding "0s" for
// sub-millisecond queries.
func FormatElapsed(elapsed time.Duration) string {
	if elapsed < time.Millisecond {
		return "<1ms"
	}
	return elapsed.Round(time.Millisecond).String()
}

// ReportError records a failed execution.
func (m *Model) ReportError(err error) {
	m.running = false
	m.statusIsError = true
	m.status = "error: " + err.Error()
}

// SetStatus sets an informational message (for example, no active connection).
func (m *Model) SetStatus(message string) {
	m.running = false
	m.statusIsError = false
	m.status = message
}

func (m *Model) pushHistory(sql string) {
	if len(m.history) > 0 && m.history[len(m.history)-1] == sql {
		m.historyIndex = len(m.history)
		return
	}
	m.history = append(m.history, sql)
	if len(m.history) > maxHistory {
		m.history = m.history[len(m.history)-maxHistory:]
	}
	m.historyIndex = len(m.history)
	m.draft = ""
}

func (m *Model) historyBack() {
	if len(m.history) == 0 || m.historyIndex == 0 {
		return
	}
	if m.historyIndex == len(m.history) {
		m.draft = m.editor.Value()
	}
	m.historyIndex--
	m.editor.SetValue(m.history[m.historyIndex])
}

func (m *Model) historyForward() {
	if len(m.history) == 0 || m.historyIndex >= len(m.history) {
		return
	}
	m.historyIndex++
	if m.historyIndex == len(m.history) {
		m.editor.SetValue(m.draft)
		return
	}
	m.editor.SetValue(m.history[m.historyIndex])
}

// View renders the editor and its status line.
func (m Model) View() string {
	status := m.statusLine()
	return lipgloss.JoinVertical(lipgloss.Left, m.editor.View(), status)
}

func (m Model) statusLine() string {
	var line string
	style := lipgloss.NewStyle().Faint(true)
	switch {
	case m.running:
		line = m.spinner.View() + " running…"
	case m.status != "":
		line = m.status
		if m.statusIsError {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		}
	default:
		line = "ctrl+r run · ctrl+↑/↓ history"
	}
	if m.width > 0 {
		line = ansi.Truncate(line, m.width, "…")
	}
	return style.Render(line)
}
