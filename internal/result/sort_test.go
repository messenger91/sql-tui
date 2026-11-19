package result

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
)

func sortTestResult() db.QueryResult {
	return db.QueryResult{
		Columns: []string{"id", "name", "score"},
		Rows: [][]any{
			{int64(3), "carol", 2.5},
			{int64(1), "alice", nil},
			{int64(2), "Bob", 10.0},
			{int64(4), "alice", 1.0},
		},
		RowCount: 4,
	}
}

func columnValues(m Model, index int) []any {
	values := make([]any, 0, len(m.result.Rows))
	for _, row := range m.result.Rows {
		values = append(values, columnValue(row, index))
	}
	return values
}

func TestCompareForSort(t *testing.T) {
	earlier := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	later := earlier.Add(24 * time.Hour)

	cases := []struct {
		name string
		a    any
		b    any
		want int
	}{
		{"both null", nil, nil, 0},
		{"null last", nil, int64(1), 1},
		{"null last reversed", int64(1), nil, -1},
		{"ints", int64(2), int64(10), -1},
		{"signed vs unsigned", int64(-1), uint64(1), -1},
		{"ints vs floats", int64(2), float64(2.5), -1},
		{"floats", 1.5, 1.25, 1},
		{"strings case-insensitive", "ada", "Grace", -1},
		{"byte strings", []byte("b"), "a", 1},
		{"times", earlier, later, -1},
		{"bools", false, true, -1},
		{"mixed fallback", int64(5), "5", 0},
	}
	for _, tc := range cases {
		if got := compareForSort(tc.a, tc.b); got != tc.want {
			t.Fatalf("%s: compare(%v, %v) = %d, want %d", tc.name, tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSortColumnAscending(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())

	perm, ok := m.SortColumn("score")
	if !ok {
		t.Fatal("sort not applied")
	}
	if m.sortName != "score" || !m.sortAsc {
		t.Fatalf("sort state = %q asc=%v", m.sortName, m.sortAsc)
	}
	// 1.0, 2.5, 10.0, then the NULL row.
	want := []any{1.0, 2.5, 10.0, nil}
	got := columnValues(m, 2)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ascending order = %v, want %v", got, want)
		}
	}
	// The permutation maps old row indexes to their new positions.
	if perm[0] != 1 || perm[1] != 3 || perm[3] != 0 {
		t.Fatalf("permutation = %v", perm)
	}
	if !strings.Contains(ansi.Strip(m.View()), "score ▲") {
		t.Fatalf("header marker missing:\n%s", m.View())
	}
	if !strings.Contains(ansi.Strip(m.View()), "sort score ▲") {
		t.Fatalf("status sort indicator missing:\n%s", m.View())
	}
}

func TestSortColumnDescendingKeepsNullsLast(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())

	m.SortColumn("score")
	if _, ok := m.SortColumn("score"); !ok {
		t.Fatal("second sort not applied")
	}
	if m.sortAsc {
		t.Fatal("sort did not flip to descending")
	}
	want := []any{10.0, 2.5, 1.0, nil}
	got := columnValues(m, 2)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("descending order = %v, want %v", got, want)
		}
	}
	if got := columnValues(m, 2)[3]; got != nil {
		t.Fatalf("NULL must stay last in descending order, got %v", got)
	}
	if !strings.Contains(ansi.Strip(m.View()), "score ▼") {
		t.Fatalf("descending marker missing:\n%s", m.View())
	}
}

func TestSortIsStableForEqualKeys(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())

	m.SortColumn("name")
	// Both alice rows keep their original relative order (ids 1 then 4).
	want := []any{int64(1), int64(4), int64(2), int64(3)}
	got := columnValues(m, 0)
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("stable order = %v, want %v", got, want)
		}
	}
}

func TestSortFollowsSelectedRow(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())
	m.table.SetCursor(2) // row with id=2

	m.SortColumn("score")
	row, column, value, ok := m.SelectedCell()
	if !ok {
		t.Fatal("no selected cell")
	}
	if row != 2 || column != "id" || value != int64(2) {
		t.Fatalf("cursor = %d %q %v, want row 2 id=2", row, column, value)
	}
}

func TestSortUnknownColumnAndStreaming(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())

	if _, ok := m.SortColumn("missing"); ok {
		t.Fatal("sorted by an unknown column")
	}
	if m.Sorted() {
		t.Fatal("unknown column activated a sort")
	}

	m.SetStreaming(true)
	if _, ok := m.SortColumn("id"); ok {
		t.Fatal("sort ran while streaming")
	}
	m.SetStreaming(false)
}

func TestSetResultClearsSort(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())
	m.SortColumn("name")

	m.SetResult(sortTestResult())
	if m.Sorted() {
		t.Fatal("new result kept the old sort")
	}
}

func TestSortVirtualMode(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	res := db.QueryResult{Columns: []string{"id"}}
	for i := virtualThreshold + 10; i > 0; i-- {
		res.Rows = append(res.Rows, []any{int64(i)})
	}
	res.RowCount = len(res.Rows)
	m.SetResult(res)
	_ = m.View() // virtual widths are computed lazily

	m.virtualCursor = 0
	perm, ok := m.SortColumn("id")
	if !ok {
		t.Fatal("virtual sort not applied")
	}
	if m.result.Rows[0][0] != int64(1) || m.result.Rows[len(m.result.Rows)-1][0] != int64(virtualThreshold+10) {
		t.Fatalf("virtual sort order = %v..%v", m.result.Rows[0][0], m.result.Rows[len(m.result.Rows)-1][0])
	}
	if m.virtualCursor != perm[0] {
		t.Fatalf("virtual cursor = %d, want %d", m.virtualCursor, perm[0])
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "id ▲") {
		t.Fatalf("virtual header marker missing:\n%s", view)
	}
}

func TestColumnAt(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())
	_ = m.View()

	if len(m.columnWidths) != 3 {
		t.Fatalf("column widths = %v", m.columnWidths)
	}
	if name, ok := m.ColumnAt(0); !ok || name != "id" {
		t.Fatalf("offset 0 = %q %v", name, ok)
	}
	second := m.columnWidths[0] + 2
	if name, ok := m.ColumnAt(second); !ok || name != "name" {
		t.Fatalf("offset %d = %q %v", second, name, ok)
	}
	third := second + m.columnWidths[1] + 2
	if name, ok := m.ColumnAt(third); !ok || name != "score" {
		t.Fatalf("offset %d = %q %v", third, name, ok)
	}
	if _, ok := m.ColumnAt(third + m.columnWidths[2] + 2); ok {
		t.Fatal("offset past the last column matched")
	}

	m.hidden["id"] = true
	m.rebuild()
	if name, ok := m.ColumnAt(0); !ok || name != "name" {
		t.Fatalf("hidden column mapping = %q %v", name, ok)
	}
}

func TestSelectColumnAndToggleSort(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())

	m.SelectColumn("score")
	if _, column, _, ok := m.SelectedCell(); !ok || column != "score" {
		t.Fatalf("selected column = %q", column)
	}
	if _, ok := m.ToggleSort(); !ok {
		t.Fatal("toggle sort failed")
	}
	if m.sortName != "score" {
		t.Fatalf("sort name = %q", m.sortName)
	}
}

func TestResortAfterAppendAndReplace(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())
	m.SortColumn("id")

	m.AppendRows([][]any{{int64(0), "zoe", 3.0}})
	m.Resort()
	if got := m.result.Rows[0][0]; got != int64(0) {
		t.Fatalf("resort after append: first id = %v", got)
	}

	m.ReplaceRowValues(len(m.result.Rows)-1, []any{int64(-1), "first", 3.0})
	m.Resort()
	if got := m.result.Rows[0][0]; got != int64(-1) {
		t.Fatalf("resort after replace: first id = %v", got)
	}
}

func TestSortKeyDoesNotChangeFormatCycle(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())
	m.SortColumn("id")

	m.SetFormat(FormatCSV)
	if !strings.Contains(m.viewport.View(), "alice") {
		t.Fatal("CSV view missing sorted rows")
	}
	m.SetFormat(FormatTable)
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "id ▲") {
		t.Fatalf("sort marker lost after format cycle:\n%s", view)
	}
}

func TestSortKeyIgnoredOutsideTableNavigation(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())

	// "s" is handled by the app, not the result pane itself; the pane must
	// keep forwarding unknown keys without changing the sort.
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	if m.Sorted() {
		t.Fatal("result pane sorted on its own")
	}
}

func TestResetColumnLayoutKeepsSort(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(sortTestResult())
	m.SortColumn("name")
	m.ToggleColumnHidden()
	m.ResetColumnLayout()

	if !m.Sorted() {
		t.Fatal("resetting the column layout cleared the sort")
	}
	if !strings.Contains(ansi.Strip(m.View()), "sort name ▲") {
		t.Fatalf("sort indicator missing after layout reset:\n%s", m.View())
	}
}
