package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
)

func exportResult() db.QueryResult {
	return db.QueryResult{
		Columns: []string{"id", "name"},
		Rows:    [][]any{{int64(1), "ada"}, {int64(2), nil}},
	}
}

func TestWriteFileCSV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.csv")
	absolute, err := WriteFile(path, "csv", exportResult())
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !filepath.IsAbs(absolute) {
		t.Fatalf("path = %q, want absolute", absolute)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	want := "id,name\n1,ada\n2,NULL\n"
	if string(data) != want {
		t.Fatalf("file = %q, want %q", data, want)
	}
}

func TestWriteFileJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.json")
	if _, err := WriteFile(path, "json", exportResult()); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), `"id": 1`) || !strings.Contains(string(data), `"name": "ada"`) {
		t.Fatalf("json = %s", data)
	}
}

func TestWriteFileErrors(t *testing.T) {
	if _, err := WriteFile("", "csv", exportResult()); err == nil {
		t.Fatal("empty path accepted")
	}
	if _, err := WriteFile(filepath.Join(t.TempDir(), "out.csv"), "xml", exportResult()); err == nil {
		t.Fatal("unknown format accepted")
	}
	if _, err := WriteFile(filepath.Join(t.TempDir(), "missing", "out.csv"), "csv", exportResult()); err == nil {
		t.Fatal("missing directory accepted")
	}
}

func TestSerialize(t *testing.T) {
	csvText, err := Serialize("csv", exportResult())
	if err != nil || !strings.HasPrefix(csvText, "id,name\n") {
		t.Fatalf("csv = %q, %v", csvText, err)
	}
	jsonText, err := Serialize("json", exportResult())
	if err != nil || !strings.Contains(jsonText, `"id": 1`) {
		t.Fatalf("json = %q, %v", jsonText, err)
	}
	if _, err := Serialize("yaml", exportResult()); err == nil {
		t.Fatal("unknown format accepted")
	}
}
