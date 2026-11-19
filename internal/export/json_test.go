package export

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestJSONStringShapeAndOrder(t *testing.T) {
	res := db.QueryResult{
		Columns: []string{"z", "a"},
		Rows: [][]any{
			{int64(1), "ada"},
			{int64(2), nil},
		},
	}
	got, err := JSONString(res)
	if err != nil {
		t.Fatalf("JSONString: %v", err)
	}
	want := `[
  {
    "z": 1,
    "a": "ada"
  },
  {
    "z": 2,
    "a": null
  }
]
`
	if got != want {
		t.Fatalf("JSON = %q, want %q", got, want)
	}
}

func TestJSONStringTypes(t *testing.T) {
	stamp := time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC)
	res := db.QueryResult{
		Columns: []string{"number", "flag", "when", "blob", "text_blob"},
		Rows: [][]any{{
			1.5,
			true,
			stamp,
			[]byte{0x01, 0x02},
			[]byte("hi"),
		}},
	}
	got, err := JSONString(res)
	if err != nil {
		t.Fatalf("JSONString: %v", err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	row := decoded[0]
	if row["number"] != 1.5 {
		t.Fatalf("number = %#v", row["number"])
	}
	if row["flag"] != true {
		t.Fatalf("flag = %#v", row["flag"])
	}
	if row["when"] != "2026-11-19T12:30:00Z" {
		t.Fatalf("when = %#v", row["when"])
	}
	if row["blob"] != "0x0102" {
		t.Fatalf("blob = %#v", row["blob"])
	}
	if row["text_blob"] != "hi" {
		t.Fatalf("text_blob = %#v", row["text_blob"])
	}
}

func TestJSONEmptyResult(t *testing.T) {
	got, err := JSONString(db.QueryResult{Columns: []string{"id"}})
	if err != nil {
		t.Fatalf("JSONString: %v", err)
	}
	if strings.TrimSpace(got) != "[]" {
		t.Fatalf("JSON = %q", got)
	}
}

func TestJSONDuplicateColumnsAreKept(t *testing.T) {
	got, err := JSONString(db.QueryResult{
		Columns: []string{"a", "a"},
		Rows:    [][]any{{int64(1), int64(2)}},
	})
	if err != nil {
		t.Fatalf("JSONString: %v", err)
	}
	if strings.Count(got, `"a"`) != 2 {
		t.Fatalf("JSON = %q", got)
	}
}
