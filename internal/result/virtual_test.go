package result

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
)

func virtualResult(rows int) db.QueryResult {
	res := db.QueryResult{Columns: []string{"id", "name"}}
	for i := 0; i < rows; i++ {
		res.Rows = append(res.Rows, []any{int64(i), "name-" + itoa(i)})
	}
	res.RowCount = rows
	res.Elapsed = 250 * time.Millisecond
	return res
}

func itoa(i int) string {
	return string(rune('a' + i%26))
}

func TestVirtualModeActivatesAboveThreshold(t *testing.T) {
	small := New()
	small.SetSize(60, 10)
	small.SetResult(virtualResult(virtualThreshold))
	if small.virtual {
		t.Fatal("virtual mode active at the threshold")
	}

	large := New()
	large.SetSize(60, 10)
	large.SetResult(virtualResult(virtualThreshold + 1))
	if !large.virtual {
		t.Fatal("virtual mode not active above the threshold")
	}
}

func TestVirtualNavigation(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(virtualResult(virtualThreshold + 10))

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := m.virtualCursor; got != 1 {
		t.Fatalf("cursor = %d", got)
	}
	if got := m.selectedRow(); got != 1 {
		t.Fatalf("selectedRow = %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := m.virtualCursor; got != 1 {
		t.Fatalf("cursor after up = %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if got := m.virtualCursor; got != len(m.result.Rows)-1 {
		t.Fatalf("end cursor = %d", got)
	}
	view := ansi.Strip(m.virtualView())
	if !strings.Contains(view, itoaLast(m)) {
		t.Fatalf("last row not visible:\n%s", view)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyHome})
	if got := m.virtualCursor; got != 0 {
		t.Fatalf("home cursor = %d", got)
	}
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if got := m.virtualCursor; got != m.virtualVisibleHeight() {
		t.Fatalf("pgdown cursor = %d", got)
	}
}

func itoaLast(m Model) string {
	return m.cachedRow(len(m.result.Rows) - 1)[1]
}

func TestVirtualWindowRenders(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(virtualResult(virtualThreshold + 50))
	m.Focus()

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	view := ansi.Strip(m.virtualView())
	for _, want := range []string{"id", "name", "name-b"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "row 2 of") {
		t.Fatalf("position missing:\n%s", view)
	}
}

func TestVirtualCacheBounds(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(virtualResult(virtualThreshold + 20))

	for i := 0; i < 50; i++ {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
		_ = m.virtualView()
	}
	if len(m.rowCache) > virtualCacheLimit {
		t.Fatalf("cache size = %d", len(m.rowCache))
	}
	if _, ok := m.rowCache[m.virtualCursor]; !ok {
		t.Fatal("the cursor row is not cached")
	}
}

func TestVirtualPendingMarkerAndStats(t *testing.T) {
	m := New()
	m.SetSize(90, 10)
	res := virtualResult(virtualThreshold + 5)
	res.Truncated = true
	res.Fetched = res.RowCount + 42
	m.SetResult(res)
	m.SetPending(map[int]bool{3: true}, 1)

	m.moveVirtualCursor(3)
	view := ansi.Strip(m.virtualView())
	if !strings.Contains(view, "* 3") {
		t.Fatalf("pending marker missing:\n%s", view)
	}
	if !strings.Contains(m.View(), "truncated") || !strings.Contains(m.View(), "fetched") {
		t.Fatalf("stats missing:\n%s", m.View())
	}
}

func TestVirtualInPlaceMutations(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(virtualResult(virtualThreshold + 10))
	before := len(m.result.Rows)

	m.ReplaceRowValues(5, []any{int64(999), "replaced"})
	if got := m.result.Rows[5][0]; got != int64(999) {
		t.Fatalf("replace = %v", got)
	}

	m.RemoveRowResult(2)
	if len(m.result.Rows) != before-1 {
		t.Fatalf("rows = %d", len(m.result.Rows))
	}
	if got := m.result.Rows[3][0]; got != int64(4) {
		t.Fatalf("rows shifted incorrectly: %v", got)
	}
	if got := m.result.Rows[4][0]; got != int64(999) {
		t.Fatalf("replaced row moved incorrectly: %v", got)
	}
	if got := m.virtualCursor; got != 0 {
		t.Fatalf("cursor = %d", got)
	}

	m.AppendRows([][]any{{int64(777), "appended"}})
	if len(m.result.Rows) != before {
		t.Fatalf("rows after append = %d", len(m.result.Rows))
	}
	if got := m.result.Rows[len(m.result.Rows)-1][0]; got != int64(777) {
		t.Fatalf("append = %v", got)
	}
}

func TestVirtualHiddenColumnsAndCellCursor(t *testing.T) {
	m := New()
	m.SetSize(60, 10)
	m.SetResult(virtualResult(virtualThreshold + 5))
	m.Focus()

	m.moveCell(1) // select "name"
	_, column, _, _ := m.SelectedCell()
	if column != "name" {
		t.Fatalf("column = %q", column)
	}
	m.ToggleColumnHidden()
	if got := m.hiddenCount(); got != 1 {
		t.Fatalf("hidden = %d", got)
	}
	view := ansi.Strip(m.virtualView())
	if strings.Contains(view, "name-a") {
		t.Fatalf("hidden column still rendered:\n%s", view)
	}
	m.ResetColumnLayout()
	if got := m.hiddenCount(); got != 0 {
		t.Fatalf("hidden after reset = %d", got)
	}
}
