package result

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
)

func sampleResult() db.QueryResult {
	return db.QueryResult{
		Columns:  []string{"id", "name"},
		Rows:     [][]any{{int64(1), "ada"}, {int64(2), nil}},
		RowCount: 2,
		Elapsed:  12 * time.Millisecond,
	}
}

func TestSetResultRendersTableAndStatus(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "id") || !strings.Contains(view, "name") {
		t.Fatalf("headers missing:\n%s", view)
	}
	if !strings.Contains(view, "ada") || !strings.Contains(view, "NULL") {
		t.Fatalf("cells missing:\n%s", view)
	}
	if !strings.Contains(view, "2 rows") || !strings.Contains(view, "table") {
		t.Fatalf("status missing:\n%s", view)
	}
}

func TestEmptyResultKeepsHeaders(t *testing.T) {
	m := New()
	m.SetSize(40, 6)
	m.SetResult(db.QueryResult{Columns: []string{"id"}})

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "id") {
		t.Fatalf("header missing:\n%s", view)
	}
	if !strings.Contains(view, "0 rows") {
		t.Fatalf("status missing:\n%s", view)
	}
}

func TestNonRowResult(t *testing.T) {
	m := New()
	m.SetSize(40, 6)
	m.SetResult(db.QueryResult{RowCount: 0})

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "statement executed") {
		t.Fatalf("view = %q", view)
	}
}

func TestNoResult(t *testing.T) {
	m := New()
	if got := ansi.Strip(m.View()); !strings.Contains(got, "no results yet") {
		t.Fatalf("view = %q", got)
	}
}

func TestColumnsFitWidth(t *testing.T) {
	res := db.QueryResult{
		Columns: []string{"id", "description", "extra", "another", "last"},
		Rows: [][]any{{
			int64(1),
			strings.Repeat("long text ", 10),
			"some value",
			"more value",
			"tail",
		}},
		RowCount: 1,
	}
	m := New()
	m.SetSize(30, 8)
	m.SetResult(res)

	for i, line := range strings.Split(ansi.Strip(m.View()), "\n") {
		if width := ansi.StringWidth(line); width > 30 {
			t.Fatalf("line %d width %d > 30: %q", i, width, line)
		}
	}
}

func TestSetSizeReflowsColumns(t *testing.T) {
	m := New()
	m.SetSize(80, 10)
	m.SetResult(sampleResult())
	wide := ansi.StringWidth(ansi.Strip(strings.Split(m.View(), "\n")[0]))

	m.SetSize(24, 10)
	narrow := ansi.StringWidth(ansi.Strip(strings.Split(m.View(), "\n")[0]))
	if narrow > 24 {
		t.Fatalf("narrow header width = %d", narrow)
	}
	if wide == narrow {
		t.Fatalf("column widths did not reflow (both %d)", wide)
	}
}

func TestAppendRowsPreservesResult(t *testing.T) {
	m := New()
	m.SetSize(40, 8)
	m.SetResult(sampleResult())
	m.AppendRows([][]any{{int64(3), "grace"}})

	res, ok := m.Result()
	if !ok {
		t.Fatal("result missing")
	}
	if len(res.Rows) != 3 || res.RowCount != 3 {
		t.Fatalf("rows = %d, count = %d", len(res.Rows), res.RowCount)
	}
	if !strings.Contains(ansi.Strip(m.View()), "grace") {
		t.Fatalf("appended row missing:\n%s", m.View())
	}
}

func TestRowNavigation(t *testing.T) {
	m := New()
	m.SetSize(40, 8)
	m.SetResult(sampleResult())
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if row, _, _, ok := m.SelectedCell(); !ok || row != 1 {
		t.Fatalf("selected row = %d ok=%v", row, ok)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if row, _, _, ok := m.SelectedCell(); !ok || row != 0 {
		t.Fatalf("selected row = %d ok=%v", row, ok)
	}
}

func TestSelectedCellNavigation(t *testing.T) {
	m := New()
	m.SetSize(40, 8)
	m.SetResult(sampleResult())
	m.Focus()

	row, column, value, ok := m.SelectedCell()
	if !ok || row != 0 || column != "id" || value != int64(1) {
		t.Fatalf("cell = %d %q %#v", row, column, value)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	row, column, value, ok = m.SelectedCell()
	if !ok || column != "name" || value != "ada" {
		t.Fatalf("cell = %d %q %#v", row, column, value)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	row, column, value, ok = m.SelectedCell()
	if !ok || row != 1 || column != "name" || value != nil {
		t.Fatalf("cell = %d %q %#v", row, column, value)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if _, column, _, _ = m.SelectedCell(); column != "id" {
		t.Fatalf("column = %q", column)
	}
	// Left must not wrap or go negative.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if _, column, _, _ = m.SelectedCell(); column != "id" {
		t.Fatalf("column = %q", column)
	}
}

func TestPendingMarkersAndStatus(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())
	m.SetPending(map[int]bool{1: true}, 2)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "* ") {
		t.Fatalf("pending marker missing:\n%s", view)
	}
	if !strings.Contains(view, "2 pending") {
		t.Fatalf("pending count missing:\n%s", view)
	}
}

func TestTruncatedStatus(t *testing.T) {
	res := sampleResult()
	res.Truncated = true
	m := New()
	m.SetSize(60, 8)
	m.SetResult(res)
	if !strings.Contains(ansi.Strip(m.View()), "truncated") {
		t.Fatalf("view missing truncation:\n%s", m.View())
	}
}

func TestStreamingStatus(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(db.QueryResult{Columns: []string{"id"}})
	m.SetStreaming(true)
	m.AppendRows([][]any{{int64(1)}, {int64(2)}})

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Streaming… 2 rows") {
		t.Fatalf("view missing streaming status:\n%s", view)
	}
}

func TestCSVViewIsDeferredWhileStreaming(t *testing.T) {
	m := New()
	m.SetSize(60, 12)
	m.SetResult(db.QueryResult{Columns: []string{"id"}})
	m.SetFormat(FormatCSV)
	m.SetStreaming(true)

	m.AppendRows([][]any{{int64(1)}})
	if strings.Contains(m.viewport.View(), "1") {
		t.Fatalf("CSV rebuilt while streaming:\n%s", m.viewport.View())
	}
	if !strings.Contains(ansi.Strip(m.View()), "Streaming… 1 rows") {
		t.Fatalf("streaming status missing:\n%s", m.View())
	}

	m.SetStreaming(false)
	if view := m.viewport.View(); !strings.Contains(view, "id") || !strings.Contains(view, "1") {
		t.Fatalf("CSV not rebuilt after streaming:\n%s", view)
	}
}

func TestAppendRowsFollowsBottomOnly(t *testing.T) {
	res := db.QueryResult{Columns: []string{"id"}}
	for i := 0; i < 20; i++ {
		res.Rows = append(res.Rows, []any{int64(i)})
	}
	res.RowCount = len(res.Rows)

	m := New()
	m.SetSize(40, 8)
	m.SetResult(res)
	m.Focus()

	// Cursor starts at the top (SetResult), so appends must not move it.
	m.AppendRows([][]any{{int64(20)}})
	if m.cursor != 0 {
		t.Fatalf("cursor moved while at top: %d", m.cursor)
	}

	m.cursor = len(m.result.Rows) - 1
	m.AppendRows([][]any{{int64(21)}})
	if got := m.cursor; got != len(m.result.Rows)-1 {
		t.Fatalf("cursor did not follow bottom: %d", got)
	}
}

func TestCSVView(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())
	m.SetFormat(FormatCSV)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, "id,name") {
		t.Fatalf("header missing:\n%s", view)
	}
	if !strings.Contains(view, "1,ada") || !strings.Contains(view, "2,NULL") {
		t.Fatalf("rows missing:\n%s", view)
	}
	if !strings.Contains(view, "csv") {
		t.Fatalf("format missing from status:\n%s", view)
	}
}

func TestSetFormatSwitchesBackToTable(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())

	m.SetFormat(FormatCSV)
	if m.Format() != FormatCSV {
		t.Fatalf("format = %q", m.Format())
	}
	m.SetFormat(FormatTable)
	if m.Format() != FormatTable {
		t.Fatalf("format = %q", m.Format())
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "id") || strings.Contains(view, "id,name") {
		t.Fatalf("table not restored:\n%s", view)
	}
}

func TestCSVViewFitsHeight(t *testing.T) {
	res := sampleResult()
	m := New()
	m.SetSize(40, 8)
	m.SetResult(res)
	m.SetFormat(FormatCSV)

	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if len(lines) != 8 {
		t.Fatalf("lines = %d, want 8", len(lines))
	}
}

func TestJSONView(t *testing.T) {
	m := New()
	m.SetSize(60, 12)
	m.SetResult(sampleResult())
	m.SetFormat(FormatJSON)

	view := ansi.Strip(m.View())
	if !strings.Contains(view, `"id"`) || !strings.Contains(view, `"name"`) {
		t.Fatalf("keys missing:\n%s", view)
	}
	if !strings.Contains(view, `1`) || !strings.Contains(view, `"ada"`) || !strings.Contains(view, `null`) {
		t.Fatalf("values missing:\n%s", view)
	}
	if !strings.Contains(view, "json") {
		t.Fatalf("format missing from status:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != 12 {
		t.Fatalf("lines = %d, want 12", len(lines))
	}
}

func TestCycleFormat(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())

	steps := []Format{FormatCSV, FormatJSON, FormatTable}
	for _, want := range steps {
		if got := m.CycleFormat(); got != want {
			t.Fatalf("cycle = %q, want %q", got, want)
		}
		if m.Format() != want {
			t.Fatalf("format = %q, want %q", m.Format(), want)
		}
		if !strings.Contains(ansi.Strip(m.View()), string(want)) {
			t.Fatalf("status missing %q:\n%s", want, m.View())
		}
	}
}

func TestCycleFormatResetsScroll(t *testing.T) {
	res := db.QueryResult{Columns: []string{"id"}}
	for i := 0; i < 40; i++ {
		res.Rows = append(res.Rows, []any{int64(i)})
	}
	res.RowCount = len(res.Rows)

	m := New()
	m.SetSize(40, 8)
	m.SetResult(res)
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor == 0 {
		t.Fatal("table cursor did not move")
	}

	m.CycleFormat() // CSV
	m.viewport.SetYOffset(5)
	m.CycleFormat() // JSON
	if m.viewport.YOffset != 0 {
		t.Fatalf("viewport offset = %d after switch", m.viewport.YOffset)
	}
	m.CycleFormat() // table
	if m.cursor != 0 {
		t.Fatalf("table cursor = %d after switch", m.cursor)
	}
}

func TestHideColumn(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())
	m.Focus()

	m.ToggleColumnHidden()
	visible := m.visibleColumns()
	if len(visible) != 1 || m.result.Columns[visible[0]] != "name" {
		t.Fatalf("visible columns = %v", visible)
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "name") {
		t.Fatalf("visible column missing:\n%s", view)
	}
	if !strings.Contains(view, "1 hidden") {
		t.Fatalf("hidden count missing:\n%s", view)
	}

	// Cannot hide the last visible column.
	m.ToggleColumnHidden()
	if got := m.hiddenCount(); got != 1 {
		t.Fatalf("hidden = %d, want 1", got)
	}

	m.ResetColumnLayout()
	if got := m.hiddenCount(); got != 0 {
		t.Fatalf("hidden = %d, want 0", got)
	}
}

func TestResizeColumn(t *testing.T) {
	m := New()
	m.SetSize(80, 10)
	m.SetResult(db.QueryResult{
		Columns:  []string{"description"},
		Rows:     [][]any{{"a fairly long description value"}},
		RowCount: 1,
	})

	before := m.columnWidths[0]
	m.ResizeColumn(2)
	if got := m.columnWidths[0]; got != before+2 {
		t.Fatalf("width = %d, want %d", got, before+2)
	}
	m.ResizeColumn(-4)
	if got := m.columnWidths[0]; got >= before+2 {
		t.Fatalf("width did not shrink: %d", got)
	}
	m.ResetColumnLayout()
	if got := m.columnWidths[0]; got != before {
		t.Fatalf("reset width = %d, want %d", got, before)
	}
}

func TestColumnLayoutPersistsAcrossFormats(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())
	m.ToggleColumnHidden()

	m.SetFormat(FormatCSV)
	m.SetFormat(FormatTable)
	if got := m.hiddenCount(); got != 1 {
		t.Fatalf("hidden after format switch = %d", got)
	}

	// A new result with the same column names keeps the layout.
	m.SetResult(sampleResult())
	if got := m.hiddenCount(); got != 1 {
		t.Fatalf("hidden after new result = %d", got)
	}
}

func TestActiveColumnHighlight(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sampleResult())

	view := m.View()
	header := tableHeaderActiveStyle.Render(padCell("id", m.columnWidths[0]))
	if !strings.Contains(view, header) {
		t.Fatalf("active column header is not highlighted:\n%q", view)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	view = m.View()
	header = tableHeaderActiveStyle.Render(padCell("name", m.columnWidths[1]))
	if !strings.Contains(view, header) {
		t.Fatalf("active column highlight did not move:\n%q", view)
	}

	active := m.cellStyle(true, false)
	normal := m.cellStyle(false, false)
	if active.GetForeground() == normal.GetForeground() || active.GetBackground() == normal.GetBackground() {
		t.Fatal("active column cells are not visually distinct")
	}
	selectedActive := m.cellStyle(true, true)
	if selectedActive.GetBackground() == active.GetBackground() {
		t.Fatal("selected-row active cell keeps the plain active background")
	}
}
