package result

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func checkGolden(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(want) != content {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, content)
	}
}

func goldenResult() db.QueryResult {
	return db.QueryResult{
		Columns: []string{"id", "name", "note"},
		Rows: [][]any{
			{int64(1), "ada", nil},
			{int64(2), "grace hopper", "compiler"},
			{int64(3), strings.Repeat("wide", 20), ""},
		},
		RowCount: 3,
	}
}

func TestGoldenResultViews(t *testing.T) {
	m := New()
	m.SetSize(50, 10)
	m.SetResult(goldenResult())

	checkGolden(t, "table_view", ansi.Strip(m.View()))

	m.SetFormat(FormatCSV)
	checkGolden(t, "csv_view", ansi.Strip(m.View()))

	m.SetFormat(FormatJSON)
	checkGolden(t, "json_view", ansi.Strip(m.View()))

	m.SetResult(db.QueryResult{Columns: []string{"id", "name"}})
	checkGolden(t, "empty_view", ansi.Strip(m.View()))

	m = New()
	checkGolden(t, "no_result_view", ansi.Strip(m.View()))
}
