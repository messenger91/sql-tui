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
	if row := m.table.SelectedRow(); row == nil || row[1] != "NULL" {
		t.Fatalf("selected row = %v", row)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if row := m.table.SelectedRow(); row == nil || row[1] != "ada" {
		t.Fatalf("selected row = %v", row)
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
	if m.table.Cursor() == 0 {
		t.Fatal("table cursor did not move")
	}

	m.CycleFormat() // CSV
	m.viewport.SetYOffset(5)
	m.CycleFormat() // JSON
	if m.viewport.YOffset != 0 {
		t.Fatalf("viewport offset = %d after switch", m.viewport.YOffset)
	}
	m.CycleFormat() // table
	if m.table.Cursor() != 0 {
		t.Fatalf("table cursor = %d after switch", m.table.Cursor())
	}
}
