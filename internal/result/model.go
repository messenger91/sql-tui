package result

import (
	"fmt"
	"sort"
	"strings"
	"time"

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

	// virtualThreshold switches the table view to the virtualized renderer
	// above this row count (§15).
	virtualThreshold = 5000
	// virtualCacheLimit bounds the formatted-row cache.
	virtualCacheLimit = 1024
	// virtualSampleRows bounds the width-sampling scan in virtual mode.
	virtualSampleRows = 200
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
	viewport viewport.Model

	result    db.QueryResult
	has       bool
	streaming bool

	cellCol      int
	pendingRows  map[int]bool
	pendingCount int
	hidden       map[string]bool
	widthDelta   map[string]int

	virtual      bool
	cursor       int
	rowCache     map[int][]string
	widthsCache  []int
	columnWidths []int

	// Sort state is applied by reordering result.Rows in place; callers must
	// remap pending row indexes through the returned permutation.
	sortName     string
	sortAsc      bool
	orderVersion int

	format Format
	width  int
	height int
}

// New returns an empty results model.
func New() Model {
	vp := viewport.New(80, 8)

	return Model{
		viewport:   vp,
		format:     FormatTable,
		width:      80,
		height:     10,
		hidden:     make(map[string]bool),
		widthDelta: make(map[string]int),
		rowCache:   make(map[int][]string),
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
	row := m.selectedRow()
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

// selectedRow returns the focused result row.
func (m Model) selectedRow() int { return m.cursor }

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
	m.invalidateRowCache()
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
	m.invalidateRowCache()
	m.rebuild()
}

// ResetColumnLayout clears hidden columns and width overrides.
func (m *Model) ResetColumnLayout() {
	m.hidden = make(map[string]bool)
	m.widthDelta = make(map[string]int)
	m.invalidateRowCache()
	m.rebuild()
}

// SortColumn sorts rows by the named column. Sorting the active column again
// flips the direction; a new column starts ascending. It returns the old→new
// row index permutation so callers can remap pending edits, plus whether the
// sort was applied.
func (m *Model) SortColumn(name string) ([]int, bool) {
	if !m.has || m.streaming || len(m.result.Rows) == 0 {
		return nil, false
	}
	index := m.columnIndex(name)
	if index < 0 {
		return nil, false
	}
	if strings.EqualFold(m.sortName, m.result.Columns[index]) {
		m.sortAsc = !m.sortAsc
	} else {
		m.sortName = m.result.Columns[index]
		m.sortAsc = true
	}
	selected := max(m.selectedRow(), 0)
	perm := m.applySort(index, m.sortAsc)
	m.selectRow(perm, selected)
	m.orderVersion++
	m.rebuild()
	return perm, true
}

// ToggleSort sorts by the selected column (ascending first, then descending).
func (m *Model) ToggleSort() ([]int, bool) {
	_, name, _, ok := m.SelectedCell()
	if !ok {
		return nil, false
	}
	return m.SortColumn(name)
}

// Resort re-applies the active sort after rows were mutated in place. It is
// used after post-apply row mutations, when pending edits have been cleared,
// so no permutation is returned.
func (m *Model) Resort() {
	if m.sortName == "" || !m.has {
		return
	}
	index := m.columnIndex(m.sortName)
	if index < 0 {
		return
	}
	m.applySort(index, m.sortAsc)
	m.cursor = min(max(m.cursor, 0), max(len(m.result.Rows)-1, 0))
	m.invalidateRowCache()
	m.rebuild()
}

// Sorted reports whether a sort is active.
func (m Model) Sorted() bool { return m.sortName != "" }

// OrderVersion increases whenever the row order changes due to a user sort.
// Async apply handlers compare it to detect a sort during the transaction.
func (m Model) OrderVersion() int { return m.orderVersion }

// SelectColumn moves the cell cursor to the named visible column.
func (m *Model) SelectColumn(name string) {
	for position, index := range m.visibleColumns() {
		if strings.EqualFold(m.result.Columns[index], name) {
			m.cellCol = position
			return
		}
	}
}

// ColumnAt maps an offset inside the rendered header row (content-relative,
// zero-based) to a column name. Cells are padded by one space on each side.
// Widths are computed on demand so virtual-mode headers are clickable before
// the next render.
func (m *Model) ColumnAt(offset int) (string, bool) {
	if !m.has || offset < 0 {
		return "", false
	}
	widths := m.tableWidths()
	visible := m.visibleColumns()
	position := 0
	for i, width := range widths {
		block := width + 2
		if offset < position+block {
			if i < len(visible) {
				return m.result.Columns[visible[i]], true
			}
			return "", false
		}
		position += block
	}
	return "", false
}

// columnIndex finds a result column by name (case-insensitive).
func (m Model) columnIndex(name string) int {
	for index, column := range m.result.Columns {
		if strings.EqualFold(column, name) {
			return index
		}
	}
	return -1
}

// sortIndicator renders the header arrow for the active sort column.
func (m Model) sortIndicator(index int) string {
	if m.sortName == "" || index < 0 || index >= len(m.result.Columns) {
		return ""
	}
	if !strings.EqualFold(m.sortName, m.result.Columns[index]) {
		return ""
	}
	if m.sortAsc {
		return " ▲"
	}
	return " ▼"
}

// applySort reorders result.Rows by the given column and returns the
// old→new index permutation.
func (m *Model) applySort(index int, ascending bool) []int {
	count := len(m.result.Rows)
	order := make([]int, count)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		a := columnValue(m.result.Rows[order[i]], index)
		b := columnValue(m.result.Rows[order[j]], index)
		if a == nil || b == nil {
			// Keep NULLs last in both directions.
			return compareForSort(a, b) < 0
		}
		compare := compareForSort(a, b)
		if !ascending {
			compare = -compare
		}
		return compare < 0
	})
	rows := make([][]any, count)
	perm := make([]int, count)
	for position, previous := range order {
		rows[position] = m.result.Rows[previous]
		perm[previous] = position
	}
	m.result.Rows = rows
	m.invalidateRowCache()
	return perm
}

// selectRow moves the cursor to the new position of a previously selected row.
func (m *Model) selectRow(perm []int, previous int) {
	if previous < 0 || previous >= len(perm) {
		return
	}
	m.cursor = perm[previous]
}

func columnValue(row []any, index int) any {
	if index < 0 || index >= len(row) {
		return nil
	}
	return row[index]
}

// SetSize updates the available content area (excluding pane border/title).
func (m *Model) SetSize(width, height int) {
	m.width = max(width, 4)
	m.height = max(height, 2)
	m.viewport.Width = m.width
	m.viewport.Height = max(m.height-1, 1) // status line
	m.rebuild()
}

// Focus and Blur are kept for the pane-focus wiring; the custom table renderer
// always shows the selected row.
func (m *Model) Focus() {}

// Blur is a no-op for the custom renderer.
func (m *Model) Blur() {}

// SetResult installs a new result set and resets scroll/cursor.
func (m *Model) SetResult(result db.QueryResult) {
	m.result = result
	m.has = true
	m.cellCol = 0
	m.pendingRows = make(map[int]bool)
	m.pendingCount = 0
	m.sortName = ""
	m.sortAsc = true
	m.orderVersion++
	m.syncVirtualMode()
	m.cursor = 0
	m.invalidateRowCache()
	m.rebuild()
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
	m.invalidateRowCache()
	m.cursor = min(max(m.cursor, 0), len(m.result.Rows)-1)
	m.rebuild()
}

// RemoveRowResult deletes a raw result row in place (post-apply refresh).
func (m *Model) RemoveRowResult(index int) {
	if index < 0 || index >= len(m.result.Rows) {
		return
	}
	m.result.Rows = append(m.result.Rows[:index], m.result.Rows[index+1:]...)
	m.result.RowCount = len(m.result.Rows)
	if m.cursor >= index && m.cursor > 0 {
		m.cursor--
	}
	m.cursor = min(max(m.cursor, 0), max(len(m.result.Rows)-1, 0))
	m.invalidateRowCache()
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
	atBottom := oldCount == 0 || m.cursor >= oldCount-1
	m.result.Rows = append(m.result.Rows, rows...)
	m.result.RowCount = len(m.result.Rows)
	m.invalidateRowCache()
	m.rebuild()
	if atBottom && len(m.result.Rows) > 0 {
		m.cursor = len(m.result.Rows) - 1
	}
	m.cursor = min(max(m.cursor, 0), max(len(m.result.Rows)-1, 0))
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
	m.cursor = 0
}

// Update handles navigation for the active view.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd
	switch m.format {
	case FormatCSV, FormatJSON:
		m.viewport, cmd = m.viewport.Update(msg)
	default:
		if key, ok := msg.(tea.KeyMsg); ok {
			if m.tableUpdate(key) {
				return m, nil
			}
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
	}
	return m, cmd
}

// View renders the active view plus the status line. It uses a pointer
// receiver so width/row caches computed while rendering persist for header
// hit-testing and the next frame.
func (m *Model) View() string {
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
		body = m.tableView()
	}
	body = lipgloss.NewStyle().Width(m.width).Height(max(m.height-1, 1)).Render(body)
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
	if m.sortName != "" {
		arrow := "▲"
		if !m.sortAsc {
			arrow = "▼"
		}
		line += fmt.Sprintf(" · sort %s %s", m.sortName, arrow)
	}
	return line
}

func (m *Model) rebuild() {
	if !m.has {
		return
	}
	m.syncVirtualMode()
	if len(m.result.Columns) == 0 {
		m.invalidateRowCache()
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
		m.invalidateRowCache()
		if !m.virtual {
			// Small results are measured eagerly so header hit-testing and
			// resize keys work before the next render.
			m.tableWidths()
		}
	}
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
