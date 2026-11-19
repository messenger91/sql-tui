package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/result"
)

// ErrRowChanged is returned when a row changed since it was loaded.
var ErrRowChanged = errors.New("row changed since load")

// ErrRowsAffected is returned when an UPDATE/DELETE affects an unexpected
// number of rows.
var ErrRowsAffected = errors.New("unexpected rows affected")

// ApplyPlan is a detached snapshot of pending changes ready for execution.
type ApplyPlan struct {
	Driver     db.Driver
	Table      string
	PrimaryKey []string
	Changes    []result.Change
	Snapshots  map[int][]result.ColumnValue
}

// ApplyOutcome reports what happened during a transactional apply.
type ApplyOutcome struct {
	Applied     int
	ConflictRow int
	FailingSQL  string
	Err         error
}

// PrepareApply snapshots the session's pending changes for execution.
func (s *Session) PrepareApply() (*ApplyPlan, error) {
	changes, err := s.Changes()
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, errors.New("session: no pending changes")
	}
	snapshots := make(map[int][]result.ColumnValue, len(s.Pending.Snapshots))
	for row, values := range s.Pending.Snapshots {
		snapshots[row] = append([]result.ColumnValue(nil), values...)
	}
	return &ApplyPlan{
		Driver:     s.Driver,
		Table:      s.Editability.Table,
		PrimaryKey: append([]string(nil), s.Editability.PrimaryKey...),
		Changes:    changes,
		Snapshots:  snapshots,
	}, nil
}

// Apply executes every change in one transaction. Any error rolls back.
func (p *ApplyPlan) Apply(ctx context.Context, handle *sql.DB) ApplyOutcome {
	tx, err := handle.BeginTx(ctx, nil)
	if err != nil {
		return ApplyOutcome{Err: fmt.Errorf("apply: begin: %w", err)}
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	applied := 0
	for index, change := range p.Changes {
		if change.Table == "" {
			change.Table = p.Table
		}
		if change.Kind != result.ChangeInsert {
			if err := p.checkConflict(ctx, tx, change); err != nil {
				outcome := ApplyOutcome{ConflictRow: change.Row, Err: err}
				if errors.Is(err, ErrRowChanged) {
					return outcome
				}
				return ApplyOutcome{Err: fmt.Errorf("apply: statement %d: %w", index+1, err)}
			}
		}

		statements, err := result.BuildStatements(p.Driver, []result.Change{change})
		if err != nil {
			return ApplyOutcome{Err: fmt.Errorf("apply: statement %d: %w", index+1, err)}
		}
		statement := statements[0]
		execResult, err := tx.ExecContext(ctx, statement.SQL, statement.Args...)
		if err != nil {
			return ApplyOutcome{
				FailingSQL: statement.SQL,
				Err:        fmt.Errorf("apply: statement %d: %w", index+1, err),
			}
		}
		if change.Kind != result.ChangeInsert {
			affected, err := execResult.RowsAffected()
			if err != nil {
				return ApplyOutcome{FailingSQL: statement.SQL, Err: fmt.Errorf("apply: rows affected: %w", err)}
			}
			if affected != 1 {
				return ApplyOutcome{
					FailingSQL: statement.SQL,
					Err:        fmt.Errorf("%w: %d rows for statement %d", ErrRowsAffected, affected, index+1),
				}
			}
		}
		applied++
	}

	if err := tx.Commit(); err != nil {
		return ApplyOutcome{Err: fmt.Errorf("apply: commit: %w", err)}
	}
	committed = true
	return ApplyOutcome{Applied: applied}
}

// checkConflict re-reads the row by primary key and compares it with the
// snapshot captured when the user first edited it.
func (p *ApplyPlan) checkConflict(ctx context.Context, tx *sql.Tx, change result.Change) error {
	snapshot, ok := p.Snapshots[change.Row]
	if !ok || len(snapshot) == 0 {
		return nil
	}
	dialect := p.Driver.Dialect()

	columns := make([]string, 0, len(snapshot))
	for _, column := range snapshot {
		columns = append(columns, dialect.QuoteIdent(column.Column))
	}
	where, args := conflictPredicate(dialect, change.Key)
	query := "SELECT " + strings.Join(columns, ", ") + " FROM " +
		dialect.QuoteIdent(p.Table) + " WHERE " + where

	targets := make([]any, len(snapshot))
	pointers := make([]any, len(snapshot))
	for i := range targets {
		pointers[i] = &targets[i]
	}
	if err := tx.QueryRowContext(ctx, query, args...).Scan(pointers...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrRowChanged
		}
		return fmt.Errorf("conflict check: %w", err)
	}

	for i, column := range snapshot {
		if !valuesEqual(column.Value, targets[i]) {
			return ErrRowChanged
		}
	}
	return nil
}

func conflictPredicate(dialect db.Dialect, key []result.ColumnValue) (string, []any) {
	parts := make([]string, 0, len(key))
	args := make([]any, 0, len(key))
	for _, column := range key {
		args = append(args, column.Value)
		parts = append(parts, dialect.QuoteIdent(column.Column)+" = "+dialect.Placeholder(len(args)))
	}
	return strings.Join(parts, " AND "), args
}

// valuesEqual compares snapshot and re-read values, normalising driver types.
func valuesEqual(expected, actual any) bool {
	expected = normalizeValue(expected)
	actual = normalizeValue(actual)
	if expectedTime, ok := expected.(time.Time); ok {
		actualTime, ok := actual.(time.Time)
		return ok && expectedTime.Equal(actualTime)
	}
	return reflect.DeepEqual(expected, actual)
}

func normalizeValue(value any) any {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	return value
}
