package result

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/export"
)

// Virtualization (§15): above virtualThreshold rows the table view stops
// relying on bubbles/table (which pre-formats every row) and renders only the
// visible window, formatting rows on demand through a bounded cache.

var (
	virtualHeaderStyle  = lipgloss.NewStyle().Bold(true).Padding(0, 1)
	virtualRowStyle     = lipgloss.NewStyle().Padding(0, 1)
	virtualSelectedStyl = lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1)
)

// syncVirtualMode turns virtual rendering on/off based on the row count.
func (m *Model) syncVirtualMode() {
	should := m.has && m.result.RowCount > virtualThreshold
	if should == m.virtual {
		return
	}
	if should {
		// Carry the bubbles/table cursor over when crossing mid-stream.
		m.virtualCursor = max(m.table.Cursor(), 0)
	}
	m.virtual = should
	m.invalidateRowCache()
}

// invalidateRowCache drops formatted-row and width caches.
func (m *Model) invalidateRowCache() {
	m.rowCache = make(map[int][]string)
	m.widthsCache = nil
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
	if len(m.rowCache) >= virtualCacheLimit {
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

// virtualWidths computes column widths from a bounded sample once per result
// shape. Pending markers and hidden columns are accounted for at render time.
func (m *Model) virtualWidths() []int {
	if m.widthsCache != nil {
		return m.widthsCache
	}
	visible := m.visibleColumns()
	natural := make([]int, len(visible))
	sample := min(virtualSampleRows, len(m.result.Rows))
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
	widths := fitWidths(natural, m.width)
	m.widthsCache = widths
	return widths
}

// pendingRowStrings returns the formatted row with a pending marker applied to
// the first cell.
func (m *Model) pendingRowStrings(rowIndex int) []string {
	row := m.cachedRow(rowIndex)
	if !m.pendingRows[rowIndex] && len(row) > 0 && strings.HasPrefix(row[0], "* ") {
		adjusted := append([]string(nil), row...)
		adjusted[0] = strings.TrimPrefix(adjusted[0], "* ")
		return adjusted
	}
	if m.pendingRows[rowIndex] && len(row) > 0 && !strings.HasPrefix(row[0], "* ") {
		adjusted := append([]string(nil), row...)
		adjusted[0] = "* " + adjusted[0]
		return adjusted
	}
	return row
}

// virtualVisibleHeight returns how many data rows fit under the header.
func (m Model) virtualVisibleHeight() int {
	return max(m.height-3, 1) // header + separator + status line
}

// virtualWindow returns the first visible row index for the cursor.
func (m Model) virtualWindow() int {
	height := m.virtualVisibleHeight()
	if m.virtualCursor < height {
		return 0
	}
	return m.virtualCursor - height + 1
}

func (m *Model) moveVirtualCursor(delta int) {
	last := max(len(m.result.Rows)-1, 0)
	m.virtualCursor = min(max(m.virtualCursor+delta, 0), last)
}

func (m *Model) moveVirtualPage(delta int) {
	m.moveVirtualCursor(delta * m.virtualVisibleHeight())
}

// virtualUpdate consumes navigation keys in virtual mode; it reports whether
// the key was handled.
func (m *Model) virtualUpdate(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "k", "ctrl+p":
		m.moveVirtualCursor(-1)
	case "down", "j", "ctrl+n":
		m.moveVirtualCursor(1)
	case "pgup", "ctrl+u":
		m.moveVirtualPage(-1)
	case "pgdown", "ctrl+d", "f":
		m.moveVirtualPage(1)
	case "home", "g":
		m.virtualCursor = 0
	case "end", "G":
		m.moveVirtualCursor(len(m.result.Rows))
	default:
		return false
	}
	return true
}

// virtualView renders the header plus the visible row window.
func (m Model) virtualView() string {
	visible := m.visibleColumns()
	if len(visible) == 0 || len(m.result.Rows) == 0 {
		return faintLine("no rows")
	}
	widths := m.virtualWidths()

	header := make([]string, len(visible))
	separator := make([]string, len(visible))
	for position, index := range visible {
		title := ansi.Truncate(m.result.Columns[index], max(widths[position], 1), "…")
		header[position] = virtualHeaderStyle.Render(padCell(title, widths[position]))
		separator[position] = strings.Repeat("─", widths[position]+2)
	}

	height := m.virtualVisibleHeight()
	top := m.virtualWindow()
	lines := make([]string, 0, height+2)
	lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, header...))
	lines = append(lines, strings.Join(separator, "┬"))

	for rowIndex := top; rowIndex < min(top+height, len(m.result.Rows)); rowIndex++ {
		cells := make([]string, len(visible))
		row := m.pendingRowStrings(rowIndex)
		for position, index := range visible {
			cell := ""
			if index < len(row) {
				cell = row[index]
			}
			cells[position] = padCell(ansi.Truncate(cell, max(widths[position], 1), "…"), widths[position])
		}
		joined := lipgloss.JoinHorizontal(lipgloss.Top, cells...)
		if m.pendingRows[rowIndex] {
			// Re-pad the first cell so the "* " marker fits without truncation.
			first := strings.TrimPrefix(strings.TrimRight(cells[0], " "), "* ")
			cells[0] = "* " + padCell(first, widths[0])
			joined = lipgloss.JoinHorizontal(lipgloss.Top, cells...)
		}
		if rowIndex == m.virtualCursor {
			lines = append(lines, virtualSelectedStyl.Render(joined))
		} else {
			lines = append(lines, virtualRowStyle.Render(joined))
		}
	}
	lines = append(lines, positionLine(m.virtualCursor+1, len(m.result.Rows)))
	return strings.Join(lines, "\n")
}

func padCell(text string, width int) string {
	if gap := width - ansi.StringWidth(text); gap > 0 {
		return text + strings.Repeat(" ", gap)
	}
	return text
}

func faintLine(text string) string {
	return lipgloss.NewStyle().Faint(true).Render(text)
}

func positionLine(cursor, total int) string {
	rows := "rows"
	if total == 1 {
		rows = "row"
	}
	return faintLine(fmt.Sprintf("row %d of %d %s", cursor, total, rows))
}
