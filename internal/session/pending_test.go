package session

import (
	"testing"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestPendingEdits(t *testing.T) {
	pending := NewPending()
	if !pending.Empty() || pending.Count() != 0 {
		t.Fatalf("pending = %+v", pending)
	}

	pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})
	pending.SetCell(0, "id", CellEdit{Value: int64(2), Original: int64(1)})
	pending.SetCell(2, "email", CellEdit{Value: "z"})
	pending.SetRowOp(1, RowDelete)

	if pending.Count() != 4 {
		t.Fatalf("count = %d", pending.Count())
	}
	if !pending.HasRow(0) || !pending.HasRow(1) || pending.HasRow(3) {
		t.Fatalf("has row = %+v", pending.RowSet())
	}
	if edit, ok := pending.Cell(0, "email"); !ok || edit.Value != "x" {
		t.Fatalf("cell = %+v", edit)
	}
	if pending.RowOp(1) != RowDelete {
		t.Fatalf("op = %q", pending.RowOp(1))
	}
	if rows := pending.RowSet(); len(rows) != 3 {
		t.Fatalf("row set = %+v", rows)
	}

	pending.RevertRow(0)
	if pending.Count() != 2 || pending.HasRow(0) {
		t.Fatalf("after revert row: %+v", pending)
	}

	pending.RevertAll()
	if !pending.Empty() {
		t.Fatalf("after revert all: %+v", pending)
	}
}

func TestPendingRowOpReplacement(t *testing.T) {
	pending := NewPending()
	pending.SetRowOp(0, RowDuplicate)
	pending.SetRowOp(0, RowNone)
	if pending.Count() != 0 {
		t.Fatalf("pending = %+v", pending)
	}
	pending.SetRowOp(0, RowDelete)
	pending.SetRowOp(0, RowDuplicate)
	if pending.RowOp(0) != RowDuplicate {
		t.Fatalf("op = %q", pending.RowOp(0))
	}
}

func TestParseCellValue(t *testing.T) {
	tests := []struct {
		name    string
		column  db.ColumnMeta
		input   string
		want    any
		wantErr bool
	}{
		{"int", db.ColumnMeta{Type: "INTEGER"}, "42", int64(42), false},
		{"int error", db.ColumnMeta{Type: "INTEGER"}, "abc", nil, true},
		{"int empty", db.ColumnMeta{Type: "INTEGER"}, "  ", nil, true},
		{"bigint", db.ColumnMeta{Type: "bigint"}, "-7", int64(-7), false},
		{"real", db.ColumnMeta{Type: "REAL"}, "1.5", 1.5, false},
		{"numeric", db.ColumnMeta{Type: "numeric(10,2)"}, "2.25", 2.25, false},
		{"float error", db.ColumnMeta{Type: "double precision"}, "x", nil, true},
		{"bool true", db.ColumnMeta{Type: "boolean"}, "T", true, false},
		{"bool false", db.ColumnMeta{Type: "boolean"}, "0", false, false},
		{"bool error", db.ColumnMeta{Type: "boolean"}, "maybe", nil, true},
		{"tinyint bool", db.ColumnMeta{Type: "tinyint(1)"}, "true", true, false},
		{"text", db.ColumnMeta{Type: "varchar(50)"}, "hello", "hello", false},
		{"text empty", db.ColumnMeta{Type: "TEXT"}, "", "", false},
		{"timestamp", db.ColumnMeta{Type: "timestamp"}, "2026-01-02 03:04:05", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), false},
		{"date", db.ColumnMeta{Type: "date"}, "2026-01-02", time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), false},
		{"timestamp error", db.ColumnMeta{Type: "timestamp"}, "yesterday", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseCellValue(tc.column, tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("value = %#v, want %#v", got, tc.want)
			}
		})
	}
}
