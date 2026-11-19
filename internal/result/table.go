package result

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/export"
)

// The results table is rendered by hand for every result size. Above
// virtualThreshold only the visible window is formatted (through a bounded
// cache) and a position line is added; below it the whole result is formatted
// once. Using one renderer keeps column highlighting, sorting, pending
// markers, and mouse hit-testing identical in both modes.

var (
	tableHeaderStyle       = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	tableHeaderActiveStyle = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("39")).Padding(0, 1)
	tableSeparatorStyle    = lipgloss.NewStyle().Faint(true).Padding(0, 1)
	tableCellStyle         = lipgloss.NewStyle().Padding(0, 1)
	tableCellActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Background(lipgloss.Color("236")).Padding(0, 1)
	tableRowStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212")).Padding(0, 1)
	tableRowActiveStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")).Background(lipgloss.Color("237")).Underline(true).Padding(0, 1)
	tablePositionStyle     = lipgloss.NewStyle().Faint(true)
)

// syncVirtualMode turns windowed rendering on/off based on the row count.
func (m *Model) syncVirtualMode() {
	should := m.has && m.result.RowCount > virtualThreshold
	if should == m.virtual {
		return
	}
	m.virtual = should
	m.invalidateRowCache()
}

// invalidateRowCache drops formatted-row and width caches.
func (m *Model) invalidateRowCache() {
	m.rowCache = make(map[int][]string)
	m.widthsCache = nil
	m.columnWidths = nil
}

// cachedRow formats one row on demand.
func (m *Model) cachedRow(index int) []string {
	if index < 0 || index >= len(m.result.Rows) {
		return nil
	}
	if m.rowCache == nil {
		m.rowCache = make(map[int][]string)
	}
	if cached, ok := m.rowCache[index]; ok {
		return cached
	}
	formatted := exportRow(m.result.Columns, m.result.Rows[index])
	if m.virtual && len(m.rowCache) >= virtualCacheLimit {
		m.rowCache = make(map[int][]string)
	}
	m.rowCache[index] = formatted
	return formatted
}

func exportRow(columns []string, raw []any) []string {
	values := make([]string, len(columns))
	for i := range values {
		if i < len(raw) {
			values[i] = export.FormatValue(raw[i])
		} else {
			values[i] = ""
		}
	}
	return values
}

// tableWidths computes column widths once per result shape. Small results are
// measured in full; virtual mode samples the first rows. Pending markers and
// hidden columns are accounted for.
func (m *Model) tableWidths() []int {
	if m.widthsCache != nil {
		return m.widthsCache
	}
	visible := m.visibleColumns()
	natural := make([]int, len(visible))
	for position, index := range visible {
		natural[position] = ansi.StringWidth(m.result.Columns[index] + m.sortIndicator(index))
	}
	sample := len(m.result.Rows)
	if m.virtual {
		sample = min(virtualSampleRows, sample)
	}
	for rowIndex := 0; rowIndex < sample; rowIndex++ {
		row := m.pendingRowStrings(rowIndex)
		for position, index := range visible {
			cell := ""
			if index < len(row) {
				cell = row[index]
			}
			if width := ansi.StringWidth(cell); width > natural[position] {
				natural[position] = width
			}
		}
	}
	for position, index := range visible {
		delta := m.widthDelta[strings.ToLower(m.result.Columns[index])]
		natural[position] = max(natural[position], minColumnWidth) + delta
	}

	widths := fitWidths(natural, m.width)
	m.widthsCache = widths
	m.columnWidths = widths
	return widths
}

// pendingRowStrings returns the formatted row with a pending marker applied to
// the first cell.
func (m *Model) pendingRowStrings(rowIndex int) []string {
	row := m.cachedRow(rowIndex)
	if len(row) == 0 {
		return row
	}
	hasMarker := strings.HasPrefix(row[0], "* ")
	if m.pendingRows[rowIndex] == hasMarker {
		return row
	}
	adjusted := append([]string(nil), row...)
	if hasMarker {
		adjusted[0] = strings.TrimPrefix(adjusted[0], "* ")
	} else {
		adjusted[0] = "* " + adjusted[0]
	}
	return adjusted
}

// visibleRows returns how many data rows fit between the header/separator and
// the row-position and status lines.
func (m Model) visibleRows() int {
	reserved := 3 // header + separator + status
	if m.virtual {
		reserved++ // position line
	}
	return max(m.height-reserved, 1)
}

// windowStart returns the first visible row index for the cursor.
func (m Model) windowStart() int {
	height := m.visibleRows()
	if m.cursor < height {
		return 0
	}
	return m.cursor - height + 1
}

func (m *Model) moveCursor(delta int) {
	last := max(len(m.result.Rows)-1, 0)
	m.cursor = min(max(m.cursor+delta, 0), last)
}

func (m *Model) movePage(delta int) {
	m.moveCursor(delta * m.visibleRows())
}

// tableUpdate consumes table navigation keys; it reports whether the key was
// handled. Cell-column movement (h/l/left/right) is handled by the caller.
func (m *Model) tableUpdate(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k", "ctrl+p":
		m.moveCursor(-1)
	case "down", "j", "ctrl+n":
		m.moveCursor(1)
	case "pgup", "ctrl+u":
		m.movePage(-1)
	case "pgdown", "ctrl+d", "f":
		m.movePage(1)
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.moveCursor(len(m.result.Rows))
	default:
		return false
	}
	return true
}

// tableView renders the header, the visible row window, and (in virtual mode)
// a row-position line. The current cell column is highlighted across the whole
// column, including its header.
func (m *Model) tableView() string {
	visible := m.visibleColumns()
	if len(visible) == 0 {
		return tablePositionStyle.Render("no rows")
	}
	widths := m.tableWidths()
	active := min(max(m.cellCol, 0), len(visible)-1)

	header := make([]string, len(visible))
	separator := make([]string, len(visible))
	for position, index := range visible {
		title := ansi.Truncate(m.result.Columns[index]+m.sortIndicator(index), max(widths[position], 1), "…")
		style := tableHeaderStyle
		if position == active {
			style = tableHeaderActiveStyle
		}
		header[position] = style.Render(padCell(title, widths[position]))
		separator[position] = tableSeparatorStyle.Render(strings.Repeat("─", widths[position]))
	}

	height := m.visibleRows()
	top := m.windowStart()
	lines := make([]string, 0, height+3)
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, header...))
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, separator...))

	for rowIndex := top; rowIndex < min(top+height, len(m.result.Rows)); rowIndex++ {
		row := m.pendingRowStrings(rowIndex)
		cells := make([]string, len(visible))
		for position, index := range visible {
			cell := ""
			if index < len(row) {
				cell = row[index]
			}
			text := padCell(ansi.Truncate(cell, max(widths[position], 1), "…"), widths[position])
			cells[position] = m.cellStyle(position == active, rowIndex == m.cursor).Render(text)
		}
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	if m.virtual {
		lines = append(lines, m.positionLine())
	}
	return strings.Join(lines, "\n")
}

// cellStyle combines the current-column and selected-row highlights.
func (m Model) cellStyle(active, selected bool) lipgloss.Style {
	switch {
	case selected && active:
		return tableRowActiveStyle
	case selected:
		return tableRowStyle
	case active:
		return tableCellActiveStyle
	default:
		return tableCellStyle
	}
}

func (m Model) positionLine() string {
	rows := "rows"
	if len(m.result.Rows) == 1 {
		rows = "row"
	}
	return tablePositionStyle.Render(fmt.Sprintf("row %d of %d %s", m.cursor+1, len(m.result.Rows), rows))
}

func padCell(text string, width int) string {
	if gap := width - ansi.StringWidth(text); gap > 0 {
		return text + strings.Repeat(" ", gap)
	}
	return text
}
