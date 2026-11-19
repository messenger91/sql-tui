package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Execute runs query and materialises at most maxRows rows. maxRows <= 0 uses
// DefaultMaxRows. Non-row statements are executed through ExecContext, but only
// when QueryContext reports that the statement has no result set: a successful
// QueryContext must never be retried as ExecContext, or an INSERT would run
// twice.
func Execute(ctx context.Context, handle *sql.DB, query string, maxRows int) QueryResult {
	if maxRows <= 0 {
		maxRows = DefaultMaxRows
	}
	start := time.Now()
	res := execute(ctx, handle, query, maxRows)
	res.Elapsed = time.Since(start)
	return res
}

func execute(ctx context.Context, handle *sql.DB, query string, maxRows int) QueryResult {
	rows, err := handle.QueryContext(ctx, query)
	if err != nil {
		if isNoResultSet(err) {
			return executeNonQuery(ctx, handle, query)
		}
		return QueryResult{Err: fmt.Errorf("db: query: %w", err)}
	}

	columns, err := rows.Columns()
	if err != nil {
		_ = rows.Close()
		return QueryResult{Err: fmt.Errorf("db: columns: %w", err)}
	}

	res := QueryResult{Columns: columns}
	if len(columns) == 0 {
		// A successful query with no columns is a statement that returned no
		// rows (e.g. INSERT on MySQL). Do not re-execute it.
		if err := rows.Close(); err != nil {
			return QueryResult{Columns: columns, Err: fmt.Errorf("db: close rows: %w", err)}
		}
		return res
	}
	defer rows.Close()

	for rows.Next() {
		if len(res.Rows) >= maxRows {
			res.Truncated = true
			break
		}
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			res.Err = fmt.Errorf("db: scan: %w", err)
			return res
		}
		res.Rows = append(res.Rows, values)
		res.RowCount++
	}
	if err := rows.Err(); err != nil {
		res.Err = fmt.Errorf("db: rows: %w", err)
	}
	return res
}

func executeNonQuery(ctx context.Context, handle *sql.DB, query string) QueryResult {
	result, err := handle.ExecContext(ctx, query)
	if err != nil {
		return QueryResult{Err: fmt.Errorf("db: exec: %w", err)}
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return QueryResult{Err: fmt.Errorf("db: rows affected: %w", err)}
	}
	return QueryResult{RowCount: int(affected)}
}

// isNoResultSet reports whether err means "this statement does not produce
// rows", which is the only case where retrying as ExecContext is safe.
func isNoResultSet(err error) bool {
	if errors.Is(err, driver.ErrSkip) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no result set") ||
		strings.Contains(msg, "does not return rows") ||
		strings.Contains(msg, "doesn't return rows")
}
