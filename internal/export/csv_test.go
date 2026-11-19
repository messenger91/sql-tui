package export

import (
	"encoding/csv"
	"strings"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
)

func sampleCSVResult() db.QueryResult {
	return db.QueryResult{
		Columns: []string{"id", "name"},
		Rows: [][]any{
			{int64(1), "ada"},
			{int64(2), nil},
			{int64(3), "a,b"},
			{int64(4), `he said "hi"`},
		},
	}
}

func TestCSVStringHeaderAndQuoting(t *testing.T) {
	got, err := CSVString(sampleCSVResult())
	if err != nil {
		t.Fatalf("CSVString: %v", err)
	}
	want := "id,name\n1,ada\n2,NULL\n3,\"a,b\"\n4,\"he said \"\"hi\"\"\"\n"
	if got != want {
		t.Fatalf("CSV = %q, want %q", got, want)
	}
}

func TestCSVStringRoundTrip(t *testing.T) {
	text, err := CSVString(sampleCSVResult())
	if err != nil {
		t.Fatalf("CSVString: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(text)).ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	if len(records) != 5 {
		t.Fatalf("records = %d", len(records))
	}
	if records[0][0] != "id" || records[0][1] != "name" {
		t.Fatalf("header = %v", records[0])
	}
	if records[4][1] != `he said "hi"` {
		t.Fatalf("quoted value = %q", records[4][1])
	}
}

func TestCSVEmptyResult(t *testing.T) {
	text, err := CSVString(db.QueryResult{Columns: []string{"id"}})
	if err != nil {
		t.Fatalf("CSVString: %v", err)
	}
	if text != "id\n" {
		t.Fatalf("CSV = %q", text)
	}
}

func TestCSVShortRowsArePadded(t *testing.T) {
	text, err := CSVString(db.QueryResult{
		Columns: []string{"a", "b"},
		Rows:    [][]any{{int64(1)}},
	})
	if err != nil {
		t.Fatalf("CSVString: %v", err)
	}
	if text != "a,b\n1,\n" {
		t.Fatalf("CSV = %q", text)
	}
}
