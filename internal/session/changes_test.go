package session

import (
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/schema"
)

func changeSession(t *testing.T) *Session {
	t.Helper()
	s := New("s1", "c1")
	s.Driver = db.DriverSQLite
	s.Results.SetResult(db.QueryResult{
		Columns: []string{"id", "email"},
		Rows: [][]any{
			{int64(1), "a"},
			{int64(2), "b"},
		},
		RowCount: 2,
	})
	s.Editability = ComputeEditability("SELECT id, email FROM users", []string{"id", "email"},
		schema.BuildIndex([]db.TableMeta{{
			Name: "users",
			Columns: []db.ColumnMeta{
				{Name: "id", IsPrimaryKey: true, Ordinal: 0},
				{Name: "email", Ordinal: 1},
			},
		}}), false)
	return s
}

func TestChangesFromPendingEdits(t *testing.T) {
	s := changeSession(t)
	s.Pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})

	changes, err := s.Changes()
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(changes) != 1 || changes[0].Kind != result.ChangeUpdate {
		t.Fatalf("changes = %+v", changes)
	}
	if len(changes[0].Key) != 1 || changes[0].Key[0].Column != "id" || changes[0].Key[0].Value != int64(1) {
		t.Fatalf("key = %+v", changes[0].Key)
	}
	if len(changes[0].Set) != 1 || changes[0].Set[0].Value != "x" {
		t.Fatalf("set = %+v", changes[0].Set)
	}
}

func TestChangesSkipNoOps(t *testing.T) {
	s := changeSession(t)
	s.Pending.SetCell(0, "email", CellEdit{Value: "a", Original: "a"})
	s.Pending.SetCell(0, "email", CellEdit{Value: nil, IsNull: true, OriginalNull: true})

	changes, err := s.Changes()
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestChangesDeleteAndDuplicate(t *testing.T) {
	s := changeSession(t)
	s.Pending.SetRowOp(1, RowDelete)
	s.Pending.SetRowOp(0, RowDuplicate)
	s.Pending.SetCell(0, "email", CellEdit{Value: "dup", Original: "a"})

	changes, err := s.Changes()
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("changes = %+v", changes)
	}
	if changes[0].Kind != result.ChangeInsert || changes[0].Values[1].Value != "dup" {
		t.Fatalf("insert = %+v", changes[0])
	}
	if changes[1].Kind != result.ChangeDelete || changes[1].Key[0].Value != int64(2) {
		t.Fatalf("delete = %+v", changes[1])
	}
}

func TestChangesDeleteWins(t *testing.T) {
	s := changeSession(t)
	s.Pending.SetRowOp(0, RowDelete)
	s.Pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})

	changes, err := s.Changes()
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(changes) != 1 || changes[0].Kind != result.ChangeDelete {
		t.Fatalf("changes = %+v", changes)
	}
}

func TestChangesRequireEditableResult(t *testing.T) {
	s := changeSession(t)
	s.Editability = Editability{Reason: "no primary key"}
	s.Pending.SetCell(0, "email", CellEdit{Value: "x"})
	if _, err := s.Changes(); err == nil {
		t.Fatal("expected an error for a non-editable result")
	}
}

func TestChangesEmptyPending(t *testing.T) {
	s := changeSession(t)
	changes, err := s.Changes()
	if err != nil || len(changes) != 0 {
		t.Fatalf("changes = %+v, err = %v", changes, err)
	}
}
