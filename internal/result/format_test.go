package result

import (
	"testing"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestFormatValue(t *testing.T) {
	stamp := time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC)
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "NULL"},
		{"integer", int64(42), "42"},
		{"float", 1.5, "1.5"},
		{"string", "hello", "hello"},
		{"newline", "a\nb", `a\nb`},
		{"tab", "a\tb", `a\tb`},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"time", stamp, "2026-11-19T12:30:00Z"},
		{"blob text", []byte("hi"), "hi"},
		{"blob binary", []byte{0x01, 0x02}, "0x0102"},
		{"control bytes", []byte{0x01}, "0x01"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FormatValue(tc.value); got != tc.want {
				t.Fatalf("FormatValue(%#v) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}
}

func TestValueRows(t *testing.T) {
	res := db.QueryResult{
		Columns: []string{"id", "name"},
		Rows: [][]any{
			{int64(1), "ada"},
			{int64(2), nil},
		},
	}
	rows := ValueRows(res)
	if len(rows) != 2 || len(rows[0]) != 2 {
		t.Fatalf("rows = %#v", rows)
	}
	if rows[0][1] != "ada" || rows[1][1] != "NULL" {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestBinaryBlobIsHex(t *testing.T) {
	if got := FormatValue([]byte{0xff, 0x00}); got != "0xff00" {
		t.Fatalf("blob = %q", got)
	}
}
