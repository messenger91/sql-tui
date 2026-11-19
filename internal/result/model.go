package result

import (
	"fmt"
	"strings"
	"time"

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

	result    db.QueryResult
	has       bool
	streaming bool

	cellCol      int
	pendingRows  map[int]bool
	pendingCount int
	hidden       map[string]bool
	widthDelta   map[string]int

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
		table:      t,
		viewport:   vp,
		format:     FormatTable,
		width:      80,
		height:     10,
		hidden:     make(map[string]bool),
		widthDelta: make(map[string]int),
	}
}

// SetPending updates the row markers and pending count.
func (m *Model) SetPending(rows map[int]bool, count int) {
	m.pendingRows = make(map[int]bool, len(rows))
	for row := range rows {
		m.pendingRows[row] = true
	}
	m.pendingCount = count
	m.rebuild()
}

// SelectedCell returns the focused cell (row index, column name, raw value).
func (m Model) SelectedCell() (int, string, any, bool) {
	visible := m.visibleColumns()
	if !m.has || len(visible) == 0 {
		return 0, "", nil, false
	}
	row := m.table.Cursor()
	if row < 0 || row >= len(m.result.Rows) {
		return 0, "", nil, false
	}
	position := min(max(m.cellCol, 0), len(visible)-1)
	column := visible[position]
	var value any
	if column < len(m.result.Rows[row]) {
		value = m.result.Rows[row][column]
	}
	return row, m.result.Columns[column], value, true
}

// visibleColumns returns the original indexes of columns not hidden.
func (m Model) visibleColumns() []int {
	visible := make([]int, 0, len(m.result.Columns))
	for i, name := range m.result.Columns {
		if m.hidden[strings.ToLower(name)] {
			continue
		}
		visible = append(visible, i)
	}
	return visible
}

func (m *Model) moveCell(delta int) {
	visible := m.visibleColumns()
	if len(visible) == 0 {
		return
	}
	m.cellCol = min(max(m.cellCol+delta, 0), len(visible)-1)
}

// ToggleColumnHidden hides or shows the selected column.
func (m *Model) ToggleColumnHidden() {
	_, name, _, ok := m.SelectedCell()
	if !ok {
		return
	}
	key := strings.ToLower(name)
	visible := m.visibleColumns()
	if !m.hidden[key] && len(visible) <= 1 {
		return
	}
	m.hidden[key] = !m.hidden[key]
	m.cellCol = min(m.cellCol, max(len(m.visibleColumns())-1, 0))
	m.rebuild()
}

// ResizeColumn widens/narrows the selected column by delta cells.
func (m *Model) ResizeColumn(delta int) {
	_, name, _, ok := m.SelectedCell()
	if !ok {
		return
	}
	key := strings.ToLower(name)
	m.widthDelta[key] = min(max(m.widthDelta[key]+delta, -20), 40)
	m.rebuild()
}

// ResetColumnLayout clears hidden columns and width overrides.
func (m *Model) ResetColumnLayout() {
	m.hidden = make(map[string]bool)
	m.widthDelta = make(map[string]int)
	m.rebuild()
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
	m.cellCol = 0
	m.pendingRows = make(map[int]bool)
	m.pendingCount = 0
	m.rebuild()
	// Set the cursor after rows exist: table clamps the cursor against the
	// current row count, so doing this before rebuild leaves it at -1.
	m.table.SetCursor(0)
	m.table.GotoTop()
	m.viewport.SetYOffset(0)
}

// UpdateStats refreshes the completion statistics after streaming.
func (m *Model) UpdateStats(rowCount, fetched int, elapsed time.Duration, truncated bool) {
	m.result.RowCount = rowCount
	m.result.Fetched = fetched
	m.result.Elapsed = elapsed
	m.result.Truncated = truncated
	m.rebuild()
}

// ReplaceRowValues swaps a raw result row in place (post-apply refresh).
func (m *Model) ReplaceRowValues(index int, raw []any) {
	if index < 0 || index >= len(m.result.Rows) {
		return
	}
	m.result.Rows[index] = append([]any(nil), raw...)
	m.rebuild()
}

// RemoveRowResult deletes a raw result row in place (post-apply refresh).
func (m *Model) RemoveRowResult(index int) {
	if index < 0 || index >= len(m.result.Rows) {
		return
	}
	m.result.Rows = append(m.result.Rows[:index], m.result.Rows[index+1:]...)
	m.result.RowCount = len(m.result.Rows)
	m.rebuild()
}

// SetStreaming toggles the streaming status label. Turning it off rebuilds
// deferred CSV/JSON views once the result is complete.
func (m *Model) SetStreaming(streaming bool) {
	m.streaming = streaming
	if !streaming {
		m.rebuild()
	}
}

// AppendRows streams more rows into the current result without resetting the
// cursor (§11). It follows the bottom if the cursor was already there.
func (m *Model) AppendRows(rows [][]any) {
	if !m.has {
		return
	}
	oldCount := len(m.result.Rows)
	atBottom := oldCount == 0 || m.table.Cursor() >= oldCount-1
	m.result.Rows = append(m.result.Rows, rows...)
	m.result.RowCount = len(m.result.Rows)
	m.rebuild()
	if atBottom && len(m.result.Rows) > 0 {
		m.table.SetCursor(len(m.result.Rows) - 1)
	}
}

// Result returns the raw result and whether one is present.
func (m Model) Result() (db.QueryResult, bool) { return m.result, m.has }

// Format returns the active result format.
func (m Model) Format() Format { return m.format }

// SetFormat switches the rendering (§9 cycles this) and resets scroll state.
func (m *Model) SetFormat(format Format) {
	m.format = format
	m.resetScroll()
	m.rebuild()
}

// CycleFormat advances table → CSV → JSON → table.
func (m *Model) CycleFormat() Format {
	switch m.format {
	case FormatTable:
		m.SetFormat(FormatCSV)
	case FormatCSV:
		m.SetFormat(FormatJSON)
	default:
		m.SetFormat(FormatTable)
	}
	return m.format
}

func (m *Model) resetScroll() {
	m.viewport.SetYOffset(0)
	m.table.SetCursor(0)
	m.table.GotoTop()
}

// Update handles navigation for the active view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.format {
	case FormatCSV, FormatJSON:
		m.viewport, cmd = m.viewport.Update(msg)
	default:
		if key, ok := msg.(tea.KeyMsg); ok {
			switch key.String() {
			case "left", "h":
				m.moveCell(-1)
				return m, nil
			case "right", "l":
				m.moveCell(1)
				return m, nil
			case "H":
				m.ToggleColumnHidden()
				return m, nil
			case "]":
				m.ResizeColumn(2)
				return m, nil
			case "[":
				m.ResizeColumn(-2)
				return m, nil
			case "0":
				m.ResetColumnLayout()
				return m, nil
			}
		}
		m.table, cmd = m.table.Update(msg)
	}
	return m, cmd
}

// View renders the active view plus the status line.
func (m Model) View() string {
	if !m.has {
		return "no results yet\n\nctrl+r in the query pane to run something"
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
	if m.streaming {
		return fmt.Sprintf("Streaming… %d rows · %s", m.result.RowCount, m.format)
	}
	line := export.ResultStats(m.result) + " · " + string(m.format)
	if m.result.Truncated {
		line += " · truncated"
	}
	if m.pendingCount > 0 {
		line += fmt.Sprintf(" · %d pending", m.pendingCount)
	}
	if hidden := m.hiddenCount(); hidden > 0 {
		line += fmt.Sprintf(" · %d hidden", hidden)
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
	// CSV/JSON rendering is deferred until streaming completes to avoid
	// re-serializing the whole result on every batch.
	if m.streaming && m.format != FormatTable {
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
		m.viewport.SetContent(export.HighlightJSON(strings.TrimRight(text, "\n")))
	default:
		m.rebuildTable()
	}
}

func (m *Model) rebuildTable() {
	visible := m.visibleColumns()
	if len(visible) == 0 {
		return
	}
	columns := make([]table.Column, len(visible))
	natural := make([]int, len(visible))
	for position, index := range visible {
		natural[position] = ansi.StringWidth(m.result.Columns[index])
	}

	rows := export.ValueRows(m.result)
	tableRows := make([]table.Row, 0, len(rows))
	for rowIndex, row := range rows {
		filtered := make([]string, 0, len(visible))
		for _, index := range visible {
			if index < len(row) {
				filtered = append(filtered, row[index])
			} else {
				filtered = append(filtered, "")
			}
		}
		if m.pendingRows[rowIndex] && len(filtered) > 0 {
			filtered[0] = "* " + filtered[0]
		}
		for i, cell := range filtered {
			if width := ansi.StringWidth(cell); width > natural[i] {
				natural[i] = width
			}
		}
		tableRows = append(tableRows, table.Row(filtered))
	}
	for position, index := range visible {
		delta := m.widthDelta[strings.ToLower(m.result.Columns[index])]
		natural[position] = max(natural[position], minColumnWidth) + delta
	}

	widths := fitWidths(natural, m.width)
	for position, index := range visible {
		columns[position] = table.Column{Title: m.result.Columns[index], Width: widths[position]}
	}
	// Reset rows first: table.SetColumns/SetRows re-render against the
	// previous shape and would panic when the visible column count changes.
	cursor := m.table.Cursor()
	m.table.SetRows(nil)
	m.table.SetColumns(columns)
	m.table.SetRows(tableRows)
	if len(tableRows) > 0 {
		m.table.SetCursor(min(max(cursor, 0), len(tableRows)-1))
	}
	m.table.SetWidth(m.width)
	m.table.SetHeight(max(m.height-2, 1))
	m.cellCol = min(m.cellCol, len(visible)-1)
}

func (m Model) hiddenCount() int {
	count := 0
	for _, name := range m.result.Columns {
		if m.hidden[strings.ToLower(name)] {
			count++
		}
	}
	return count
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
