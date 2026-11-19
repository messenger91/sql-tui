package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
)

// EventKind identifies a runner event.
type EventKind string

const (
	EventStarted   EventKind = "started"
	EventBatch     EventKind = "batch"
	EventDone      EventKind = "done"
	EventError     EventKind = "error"
	EventCancelled EventKind = "cancelled"
)

// Event is one asynchronous execution update for a session.
type Event struct {
	Kind       EventKind
	Columns    []string
	Rows       [][]any
	RowCount   int
	Statements int
	Truncated  bool
	Elapsed    time.Duration
	Err        error
}

const (
	runnerBatchSize   = 200
	runnerFlushWindow = 50 * time.Millisecond
)

// Run executes SQL for a session, emitting events until completion. It blocks
// until every statement has finished (or ctx is cancelled) and must run in its
// own goroutine. emit may block; it reports false when the consumer is gone.
func Run(ctx context.Context, handle *sql.DB, sqlText string, maxRows int, emit func(Event) bool) {
	if maxRows <= 0 {
		maxRows = db.DefaultMaxRows
	}
	statements := query.SplitStatements(sqlText)
	if len(statements) == 0 {
		emit(Event{Kind: EventError, Err: errors.New("no executable SQL")})
		return
	}

	started := time.Now()
	stored := 0
	truncated := false
	statementsRun := 0

	for index, statement := range statements {
		if ctx.Err() != nil {
			emit(Event{Kind: EventCancelled})
			return
		}
		statementsRun++

		stream, err := db.Stream(ctx, handle, statement)
		if err != nil {
			if ctx.Err() != nil {
				emit(Event{Kind: EventCancelled})
			} else {
				emit(Event{Kind: EventError, Err: fmt.Errorf("statement %d: %w", index+1, err)})
			}
			return
		}
		if stream.Rows == nil {
			continue
		}

		rows := stream.Rows
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			emit(Event{Kind: EventError, Err: fmt.Errorf("statement %d: columns: %w", index+1, err)})
			return
		}

		// Each result-producing statement replaces the previous result view.
		stored = 0
		truncated = false
		if !emit(Event{Kind: EventStarted, Columns: columns}) {
			_ = rows.Close()
			return
		}

		batch := make([][]any, 0, runnerBatchSize)
		lastFlush := time.Now()

		flush := func() bool {
			if len(batch) == 0 {
				return true
			}
			ok := emit(Event{Kind: EventBatch, Rows: batch})
			batch = make([][]any, 0, runnerBatchSize)
			lastFlush = time.Now()
			return ok
		}

		cancelled := false
		for rows.Next() {
			if ctx.Err() != nil {
				cancelled = true
				break
			}
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for i := range values {
				targets[i] = &values[i]
			}
			if err := rows.Scan(targets...); err != nil {
				_ = rows.Close()
				if ctx.Err() != nil {
					emit(Event{Kind: EventCancelled})
				} else {
					emit(Event{Kind: EventError, Err: fmt.Errorf("statement %d: scan: %w", index+1, err)})
				}
				return
			}
			if stored >= maxRows {
				truncated = true
				continue // keep draining so the connection is released cleanly
			}
			stored++
			batch = append(batch, values)
			if len(batch) >= runnerBatchSize || time.Since(lastFlush) >= runnerFlushWindow {
				if !flush() {
					_ = rows.Close()
					return
				}
			}
		}
		if err := rows.Err(); err != nil && !cancelled {
			_ = rows.Close()
			if ctx.Err() != nil {
				emit(Event{Kind: EventCancelled})
			} else {
				emit(Event{Kind: EventError, Err: fmt.Errorf("statement %d: rows: %w", index+1, err)})
			}
			return
		}
		if !flush() {
			_ = rows.Close()
			return
		}
		if err := rows.Close(); err != nil {
			emit(Event{Kind: EventError, Err: fmt.Errorf("statement %d: close: %w", index+1, err)})
			return
		}
		if cancelled || ctx.Err() != nil {
			emit(Event{Kind: EventCancelled})
			return
		}
	}

	emit(Event{
		Kind:       EventDone,
		RowCount:   stored,
		Statements: statementsRun,
		Truncated:  truncated,
		Elapsed:    time.Since(started),
	})
}
