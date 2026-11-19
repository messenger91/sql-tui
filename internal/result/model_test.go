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
