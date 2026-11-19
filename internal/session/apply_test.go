package session

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/result"
	"github.com/messenger91/sql-tui/internal/schema"
)

func applySession(t *testing.T) (*Session, *sql.DB) {
	t.Helper()
	handle := openRunnerDB(t)
	if _, err := handle.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, email TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := handle.Exec(`INSERT INTO users (id, email) VALUES (1, 'a'), (2, 'b')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	data := db.Execute(context.Background(), handle, "SELECT id, email FROM users ORDER BY id", 0)
	if data.Err != nil {
		t.Fatalf("select: %v", data.Err)
	}
	s := New("s1", "runner")
	s.Driver = db.DriverSQLite
	s.Results.SetResult(data)
	s.Editability = ComputeEditability("SELECT id, email FROM users", data.Columns,
		schema.BuildIndex([]db.TableMeta{{
			Name: "users",
			Columns: []db.ColumnMeta{
				{Name: "id", IsPrimaryKey: true, Ordinal: 0},
				{Name: "email", Ordinal: 1},
			},
		}}), false)
	return s, handle
}

func snapshotCurrent(s *Session, row int) []result.ColumnValue {
	data, _ := s.Results.Result()
	values := make([]result.ColumnValue, len(data.Columns))
	for i, column := range data.Columns {
		var value any
		if i < len(data.Rows[row]) {
			value = data.Rows[row][i]
		}
		values[i] = result.ColumnValue{Column: column, Value: value}
	}
	return values
}

func TestApplyUpdateCommits(t *testing.T) {
	s, handle := applySession(t)
	s.Pending.EnsureSnapshot(0, snapshotCurrent(s, 0))
	s.Pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})

	plan, err := s.PrepareApply()
	if err != nil {
		t.Fatalf("PrepareApply: %v", err)
	}
	outcome := plan.Apply(context.Background(), handle)
	if outcome.Err != nil || outcome.Applied != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}

	var email string
	if err := handle.QueryRow(`SELECT email FROM users WHERE id = 1`).Scan(&email); err != nil {
		t.Fatalf("query: %v", err)
	}
	if email != "x" {
		t.Fatalf("email = %q", email)
	}
}

func TestApplyConflictAborts(t *testing.T) {
	s, handle := applySession(t)
	s.Pending.EnsureSnapshot(0, snapshotCurrent(s, 0))
	s.Pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})
	if _, err := handle.Exec(`UPDATE users SET email = 'outside' WHERE id = 1`); err != nil {
		t.Fatalf("external update: %v", err)
	}

	plan, _ := s.PrepareApply()
	outcome := plan.Apply(context.Background(), handle)
	if !errors.Is(outcome.Err, ErrRowChanged) {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.ConflictRow != 0 {
		t.Fatalf("conflict row = %d", outcome.ConflictRow)
	}
}

func TestApplyRollsBackOnError(t *testing.T) {
	s, handle := applySession(t)
	s.Pending.EnsureSnapshot(0, snapshotCurrent(s, 0))
	s.Pending.EnsureSnapshot(1, snapshotCurrent(s, 1))
	s.Pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})
	// Duplicate primary key: UPDATE id=1 WHERE id=2 violates id=1.
	s.Pending.SetCell(1, "id", CellEdit{Value: int64(1), Original: int64(2)})

	plan, _ := s.PrepareApply()
	outcome := plan.Apply(context.Background(), handle)
	if outcome.Err == nil || outcome.FailingSQL == "" {
		t.Fatalf("outcome = %+v", outcome)
	}

	var email string
	if err := handle.QueryRow(`SELECT email FROM users WHERE id = 1`).Scan(&email); err != nil {
		t.Fatalf("query: %v", err)
	}
	if email != "a" {
		t.Fatalf("first statement was not rolled back: %q", email)
	}
}

func TestApplyRowsAffectedMismatch(t *testing.T) {
	_, handle := applySession(t)
	plan := &ApplyPlan{
		Driver: db.DriverSQLite,
		Table:  "users",
		Changes: []result.Change{{
			Kind: result.ChangeUpdate,
			Row:  0,
			Key:  []result.ColumnValue{{Column: "id", Value: int64(999)}},
			Set:  []result.ColumnValue{{Column: "email", Value: "x"}},
		}},
	}
	outcome := plan.Apply(context.Background(), handle)
	if !errors.Is(outcome.Err, ErrRowsAffected) {
		t.Fatalf("outcome = %+v", outcome)
	}
}

func TestApplyDelete(t *testing.T) {
	s, handle := applySession(t)
	s.Pending.EnsureSnapshot(1, snapshotCurrent(s, 1))
	s.Pending.SetRowOp(1, RowDelete)

	plan, _ := s.PrepareApply()
	outcome := plan.Apply(context.Background(), handle)
	if outcome.Err != nil || outcome.Applied != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}
	var count int
	if err := handle.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d", count)
	}
}

func TestApplyInsertFromDuplicate(t *testing.T) {
	s, handle := applySession(t)
	s.Pending.EnsureSnapshot(0, snapshotCurrent(s, 0))
	s.Pending.SetRowOp(0, RowDuplicate)
	s.Pending.SetCell(0, "id", CellEdit{Value: int64(3), Original: int64(1)})
	s.Pending.SetCell(0, "email", CellEdit{Value: "dup", Original: "a"})

	plan, err := s.PrepareApply()
	if err != nil {
		t.Fatalf("PrepareApply: %v", err)
	}
	outcome := plan.Apply(context.Background(), handle)
	if outcome.Err != nil || outcome.Applied != 1 {
		t.Fatalf("outcome = %+v", outcome)
	}

	var count int
	if err := handle.QueryRow(`SELECT count(*) FROM users WHERE email = 'dup'`).Scan(&count); err != nil {
		t.Fatalf("query: %v", err)
	}
	if count != 1 {
		t.Fatalf("inserted rows = %d", count)
	}
}

func TestPrepareApplyErrors(t *testing.T) {
	s, _ := applySession(t)
	if _, err := s.PrepareApply(); err == nil {
		t.Fatal("expected an error with no pending changes")
	}

	s.Pending.SetCell(0, "email", CellEdit{Value: "x"})
	s.Editability = Editability{Reason: "no primary key"}
	if _, err := s.PrepareApply(); err == nil {
		t.Fatal("expected an error for a non-editable result")
	}
}
