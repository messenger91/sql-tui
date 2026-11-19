package db

import (
	"context"
	"database/sql"
	"fmt"
)

// StreamResult is the outcome of Stream: either open rows (Rows non-nil) or a
// non-row statement with its affected-row count.
type StreamResult struct {
	Rows         *sql.Rows
	RowsAffected int64
}

// Stream starts a statement for incremental consumption. The caller must close
// Rows. Like Execute, a successful QueryContext is never retried as
// ExecContext; ExecContext is used only when the driver reports no result set.
func Stream(ctx context.Context, handle *sql.DB, query string) (StreamResult, error) {
	rows, err := handle.QueryContext(ctx, query)
	if err != nil {
		if !isNoResultSet(err) {
			return StreamResult{}, fmt.Errorf("db: query: %w", err)
		}
		result, execErr := handle.ExecContext(ctx, query)
		if execErr != nil {
			return StreamResult{}, fmt.Errorf("db: exec: %w", execErr)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return StreamResult{}, fmt.Errorf("db: rows affected: %w", err)
		}
		return StreamResult{RowsAffected: affected}, nil
	}

	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return StreamResult{}, fmt.Errorf("db: columns: %w", err)
	}
	if len(columns) == 0 {
		if err := rows.Close(); err != nil {
			return StreamResult{}, fmt.Errorf("db: close rows: %w", err)
		}
		return StreamResult{}, nil
	}
	return StreamResult{Rows: rows}, nil
}
