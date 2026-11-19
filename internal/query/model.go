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
	"github.com/messenger91/sql-tui/internal/schema"
)

const (
	maxHistory     = 50
	suggestDelay   = 50 * time.Millisecond
	maxSuggestions = 10
)

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

	schemaIndex *schema.Index
	popup       *popupState
	usage       map[string]int
	suggestGen  int
	baseHeight  int

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
		editor:     editor,
		spinner:    s,
		usage:      make(map[string]int),
		baseHeight: 8,
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
	m.hidePopup()
	m.editor.SetValue(sql)
	m.editor.Focus()
}

// InsertText inserts text at the editor cursor.
func (m *Model) InsertText(text string) {
	m.hidePopup()
	m.editor.InsertString(text)
	m.editor.Focus()
}

// SetSize resizes the editor, reserving one row for the status line and room
// for the autocomplete popup when it is open.
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 10)
	m.height = max(height, 2)
	m.baseHeight = m.height
	m.editor.SetWidth(m.width)
	m.applyEditorHeight()
}

// SetSchema installs the suggestion index for the active connection.
func (m *Model) SetSchema(index *schema.Index) {
	m.schemaIndex = index
	if index == nil {
		m.hidePopup()
	}
}

// PopupVisible reports whether the autocomplete popup is capturing keys.
func (m Model) PopupVisible() bool { return m.popup != nil }

func (m *Model) applyEditorHeight() {
	height := max(m.baseHeight-1, 1)
	if m.popup != nil {
		height = max(m.baseHeight-1-m.popupHeight(), 1)
	}
	m.editor.SetHeight(height)
}

func (m Model) popupHeight() int {
	if m.popup == nil {
		return 0
	}
	desired := min(len(m.popup.items), 8) + 2
	maxHeight := max(m.baseHeight-2, 1)
	return min(desired, maxHeight)
}

func (m *Model) hidePopup() {
	if m.popup == nil {
		return
	}
	m.popup = nil
	m.applyEditorHeight()
}

// cursorIndex returns the cursor position as a rune offset into Value().
func (m Model) cursorIndex() int {
	lines := strings.Split(m.editor.Value(), "\n")
	row := m.editor.Line()
	if row < 0 {
		row = 0
	}
	if row >= len(lines) {
		row = len(lines) - 1
	}
	index := 0
	for i := 0; i < row; i++ {
		index += len([]rune(lines[i])) + 1
	}
	info := m.editor.LineInfo()
	return index + info.StartColumn + info.ColumnOffset
}

type suggestTickMsg struct {
	Gen  int
	Text string
}

func (m *Model) scheduleSuggest() tea.Cmd {
	if m.schemaIndex == nil {
		return nil
	}
	m.suggestGen++
	gen := m.suggestGen
	text := m.editor.Value()
	return tea.Tick(suggestDelay, func(time.Time) tea.Msg {
		return suggestTickMsg{Gen: gen, Text: text}
	})
}

func (m Model) computeSuggestions() Model {
	if m.schemaIndex == nil {
		m.hidePopup()
		return m
	}
	text := m.editor.Value()
	cursor := m.cursorIndex()
	start, end, token := tokenSpan(text, cursor)
	if token == "" {
		m.hidePopup()
		return m
	}
	items := suggestionsFor(m.schemaIndex, text, start, end, m.usage)
	if len(items) == 0 {
		m.hidePopup()
		return m
	}
	m.popup = &popupState{items: items, start: start, end: end}
	m.applyEditorHeight()
	return m
}

func (m *Model) acceptPopup() {
	popup := m.popup
	if popup == nil {
		return
	}
	cursor := m.cursorIndex()
	if cursor < popup.start || cursor > popup.end {
		m.hidePopup()
		return
	}
	item := popup.items[popup.cursor]

	runes := []rune(m.editor.Value())
	replacement := item.Name
	if dot := strings.LastIndex(string(runes[popup.start:popup.end]), "."); dot >= 0 {
		replacement = string(runes[popup.start:popup.start+dot+1]) + item.Name
	}

	for i := cursor; i < popup.end; i++ {
		m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyRight})
	}
	for i := popup.start; i < popup.end; i++ {
		m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	}
	m.editor.InsertString(replacement)
	m.usage[strings.ToLower(item.Name)]++
	m.hidePopup()
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

	case suggestTickMsg:
		if msg.Gen == m.suggestGen && msg.Text == m.editor.Value() {
			m = m.computeSuggestions()
		}
		return m, nil

	case tea.KeyMsg:
		if m.popup != nil {
			switch msg.String() {
			case "esc":
				m.hidePopup()
				return m, nil
			case "up", "ctrl+p":
				m.movePopup(-1)
				return m, nil
			case "down", "ctrl+n":
				m.movePopup(1)
				return m, nil
			case "tab", "enter":
				m.acceptPopup()
				return m, nil
			case "ctrl+space", "ctrl+@":
				m = m.computeSuggestions()
				return m, nil
			default:
				m.hidePopup()
			}
		}

		switch msg.String() {
		case "ctrl+space", "ctrl+@":
			if m.schemaIndex != nil {
				m = m.computeSuggestions()
			}
			return m, nil
		case "ctrl+r", "f5":
			if m.running {
				return m, nil
			}
			m.hidePopup()
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
		suggestCmd := m.scheduleSuggest()
		return m, tea.Batch(cmd, suggestCmd)

	default:
		return m, nil
	}
}

func (m *Model) movePopup(delta int) {
	if m.popup == nil {
		return
	}
	m.popup.cursor = min(max(m.popup.cursor+delta, 0), len(m.popup.items)-1)
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

// View renders the editor, the autocomplete popup, and the status line.
func (m Model) View() string {
	parts := []string{m.editor.View()}
	if m.popup != nil {
		parts = append(parts, m.popupView())
	}
	parts = append(parts, m.statusLine())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m Model) popupView() string {
	height := max(m.popupHeight()-2, 1)
	start := clampInt(m.popup.cursor-height+1, 0, len(m.popup.items)-1)
	end := min(start+height, len(m.popup.items))

	dim := lipgloss.NewStyle().Faint(true)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		item := m.popup.items[i]
		label := completionIcon(item.Kind) + " " + item.Name
		switch item.Kind {
		case kindColumn:
			if item.Type != "" {
				label += "  " + item.Type
			}
			if item.Table != "" {
				label += "  " + item.Table
			}
		case kindView:
			label += "  view"
		case kindSchema:
			label += "  schema"
		}
		label = ansi.Truncate(label, max(m.width-8, 10), "…")
		if i == m.popup.cursor {
			label = lipgloss.NewStyle().Reverse(true).Bold(true).Render(label)
		} else if item.Kind == kindColumn || item.Kind == kindView || item.Kind == kindSchema {
			label = dim.Render(label)
		}
		lines = append(lines, label)
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))

	indent := 0
	if info := m.editor.LineInfo(); m.width > 20 {
		indent = min(info.ColumnOffset, max(m.width-lipgloss.Width(box)-4, 0))
	}
	return lipgloss.NewStyle().PaddingLeft(indent).Render(box)
}

func completionIcon(kind completionKind) string {
	switch kind {
	case kindTable:
		return "▦"
	case kindView:
		return "▧"
	case kindColumn:
		return "▤"
	case kindSchema:
		return "◇"
	default:
		return "K"
	}
}

func clampInt(value, low, high int) int {
	return min(max(value, low), high)
}

func (m Model) statusLine() string {
	var line string
	style := lipgloss.NewStyle().Faint(true)
	switch {
	case m.popup != nil:
		line = "↑/↓ select · tab/enter accept · esc dismiss"
	case m.running:
		line = m.spinner.View() + " running…"
	case m.status != "":
		line = m.status
		if m.statusIsError {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		}
	default:
		line = "ctrl+r run · ctrl+space complete · ctrl+↑/↓ history"
	}
	if m.width > 0 {
		line = ansi.Truncate(line, m.width, "…")
	}
	return style.Render(line)
}
