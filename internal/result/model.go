package result

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/export"
)

const (
	maxColumnWidth = 40
	minColumnWidth = 3
)

// Format identifies the active rendering of a result set.
type Format string

const (
	FormatTable Format = "table"
	FormatCSV   Format = "csv"
	FormatJSON  Format = "json"
)

// Model is the results pane. It keeps the raw result for export (§8) and
// renders the active format: table, CSV, or JSON.
type Model struct {
	table    table.Model
	viewport viewport.Model

	result db.QueryResult
	has    bool

	format Format
	width  int
	height int
}

// New returns an empty results model.
func New() Model {
	styles := table.DefaultStyles()
	t := table.New(
		table.WithColumns(nil),
		table.WithRows(nil),
		table.WithFocused(false),
		table.WithStyles(styles),
	)
	t.SetWidth(80)
	t.SetHeight(8)

	vp := viewport.New(80, 8)

	return Model{
		table:    t,
		viewport: vp,
		format:   FormatTable,
		width:    80,
		height:   10,
	}
}

// SetSize updates the available content area (excluding pane border/title).
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 4)
	m.height = max(height, 2)
	m.table.SetWidth(m.width)
	m.table.SetHeight(max(m.height-2, 1)) // header + status line
	m.viewport.Width = m.width
	m.viewport.Height = max(m.height-1, 1) // status line
	m.rebuild()
}

// Focus enables row navigation.
func (m *Model) Focus() { m.table.Focus() }

// Blur disables row navigation.
func (m *Model) Blur() { m.table.Blur() }

// SetResult installs a new result set and resets scroll/cursor.
func (m *Model) SetResult(result db.QueryResult) {
	m.result = result
	m.has = true
	m.rebuild()
	// Set the cursor after rows exist: table clamps the cursor against the
	// current row count, so doing this before rebuild leaves it at -1.
	m.table.SetCursor(0)
	m.table.GotoTop()
	m.viewport.SetYOffset(0)
}

// AppendRows streams more rows into the current result without resetting the
// cursor (§11).
func (m *Model) AppendRows(rows [][]any) {
	if !m.has {
		return
	}
	for _, row := range rows {
		m.result.Rows = append(m.result.Rows, row)
	}
	m.result.RowCount = len(m.result.Rows)
	m.rebuild()
}

// Result returns the raw result and whether one is present.
func (m Model) Result() (db.QueryResult, bool) { return m.result, m.has }

// Format returns the active result format.
func (m Model) Format() Format { return m.format }

// SetFormat switches the rendering (§9 cycles this).
func (m *Model) SetFormat(format Format) {
	m.format = format
	m.rebuild()
}

// Update handles navigation for the active view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.format {
	case FormatCSV, FormatJSON:
		m.viewport, cmd = m.viewport.Update(msg)
	default:
		m.table, cmd = m.table.Update(msg)
	}
	return m, cmd
}

// View renders the active view plus the status line.
func (m Model) View() string {
	if !m.has {
		return "no results yet"
	}
	if len(m.result.Columns) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left, "statement executed", m.statusLine())
	}

	var body string
	switch m.format {
	case FormatCSV, FormatJSON:
		body = m.viewport.View()
	default:
		body = m.table.View()
	}
	body = lipgloss.NewStyle().Height(max(m.height-1, 1)).Render(body)
	return lipgloss.JoinVertical(lipgloss.Left, body, m.statusLine())
}

func (m Model) statusLine() string {
	return lipgloss.NewStyle().Faint(true).Render(ansi.Truncate(m.summary(), m.width, "…"))
}

func (m Model) summary() string {
	rows := "rows"
	if m.result.RowCount == 1 {
		rows = "row"
	}
	line := fmt.Sprintf("%d %s · %s · %s",
		m.result.RowCount, rows, export.FormatElapsed(m.result.Elapsed), m.format)
	if m.result.Truncated {
		line += " · truncated"
	}
	return line
}

func (m *Model) rebuild() {
	if !m.has {
		return
	}
	if len(m.result.Columns) == 0 {
		m.table.SetColumns(nil)
		m.table.SetRows(nil)
		m.viewport.SetContent("")
		return
	}

	switch m.format {
	case FormatCSV:
		text, err := export.CSVString(m.result)
		if err != nil {
			text = "error: " + err.Error()
		}
		m.viewport.SetContent(strings.TrimRight(text, "\n"))
	case FormatJSON:
		text, err := export.JSONString(m.result)
		if err != nil {
			text = "error: " + err.Error()
		}
		m.viewport.SetContent(strings.TrimRight(text, "\n"))
	default:
		m.rebuildTable()
	}
}

func (m *Model) rebuildTable() {
	columns := make([]table.Column, len(m.result.Columns))
	natural := make([]int, len(m.result.Columns))
	for i, name := range m.result.Columns {
		natural[i] = ansi.StringWidth(name)
	}
	rows := export.ValueRows(m.result)
	for _, row := range rows {
		for i, cell := range row {
			if width := ansi.StringWidth(cell); width > natural[i] {
				natural[i] = width
			}
		}
	}
	tableRows := make([]table.Row, len(rows))
	for i, row := range rows {
		tableRows[i] = table.Row(row)
	}
	widths := fitWidths(natural, m.width)
	for i, name := range m.result.Columns {
		columns[i] = table.Column{Title: name, Width: widths[i]}
	}
	m.table.SetColumns(columns)
	m.table.SetRows(tableRows)
	m.table.SetWidth(m.width)
	m.table.SetHeight(max(m.height-2, 1))
}

// fitWidths caps natural widths so the rendered table (cells plus 1-column
// padding on each side) fits the available width.
func fitWidths(natural []int, available int) []int {
	widths := make([]int, len(natural))
	total := 0
	for i, width := range natural {
		widths[i] = min(max(width, minColumnWidth), maxColumnWidth)
		total += widths[i]
	}
	budget := available - 2*len(natural)
	if budget < len(natural) {
		budget = len(natural)
	}
	for total > budget {
		widest := 0
		for i := range widths {
			if widths[i] > widths[widest] {
				widest = i
			}
		}
		if widths[widest] <= 1 {
			break
		}
		widths[widest]--
		total--
	}
	return widths
}
