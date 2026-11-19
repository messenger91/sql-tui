package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestMarkdownString(t *testing.T) {
	result := db.QueryResult{
		Columns: []string{"id", "name"},
		Rows: [][]any{
			{int64(1), "ada"},
			{int64(2), nil},
			{int64(3), "a|b"},
		},
	}
	got := MarkdownString(result)
	want := "| id | name |\n| --- | --- |\n| 1 | ada |\n| 2 | NULL |\n| 3 | a\\|b |\n"
	if got != want {
		t.Fatalf("markdown = %q, want %q", got, want)
	}
}

func TestMarkdownEmptyColumns(t *testing.T) {
	if got := MarkdownString(db.QueryResult{}); got != "" {
		t.Fatalf("markdown = %q", got)
	}
}

func TestSerializeMarkdown(t *testing.T) {
	text, err := Serialize("md", db.QueryResult{Columns: []string{"a"}, Rows: [][]any{{int64(1)}}})
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	if !strings.HasPrefix(text, "| a |") {
		t.Fatalf("markdown = %q", text)
	}
}

func TestWriteFileMarkdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	if _, err := WriteFile(path, "markdown", db.QueryResult{Columns: []string{"a"}, Rows: [][]any{{"x"}}}); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "| a |\n| --- |\n| x |\n" {
		t.Fatalf("file = %q", data)
	}
}
