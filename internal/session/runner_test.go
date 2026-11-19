package session

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

func openRunnerDB(t *testing.T) *sql.DB {
	t.Helper()
	handle, err := db.Connect(context.Background(), db.Connection{
		ID: "runner", Name: "runner", Driver: db.DriverSQLite, DSN: ":memory:",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return handle
}

func seedRows(t *testing.T, handle *sql.DB, count int) {
	t.Helper()
	if _, err := handle.Exec(`CREATE TABLE numbers (id INTEGER)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	tx, err := handle.Begin()
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	for i := 0; i < count; i++ {
		if _, err := tx.Exec(`INSERT INTO numbers (id) VALUES (?)`, i); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func collectRun(t *testing.T, handle *sql.DB, sqlText string, maxRows int) []Event {
	t.Helper()
	var events []Event
	Run(context.Background(), handle, sqlText, maxRows, func(event Event) bool {
		events = append(events, event)
		return true
	})
	return events
}

func rowTotal(events []Event) int {
	total := 0
	for _, event := range events {
		if event.Kind == EventBatch {
			total += len(event.Rows)
		}
	}
	return total
}

func TestRunnerStreamsBatchesAndDone(t *testing.T) {
	handle := openRunnerDB(t)
	seedRows(t, handle, 450)

	events := collectRun(t, handle, "SELECT id FROM numbers ORDER BY id", 0)
	if len(events) == 0 {
		t.Fatal("no events")
	}
	if events[0].Kind != EventStarted || len(events[0].Columns) != 1 || events[0].Columns[0] != "id" {
		t.Fatalf("started = %+v", events[0])
	}

	var batches int
	for _, event := range events {
		if event.Kind == EventBatch {
			if len(event.Rows) > runnerBatchSize {
				t.Fatalf("batch too large: %d", len(event.Rows))
			}
			batches++
		}
	}
	if rowTotal(events) != 450 {
		t.Fatalf("rows = %d", rowTotal(events))
	}
	if batches < 3 {
		t.Fatalf("batches = %d, want at least 3", batches)
	}

	done := events[len(events)-1]
	if done.Kind != EventDone || done.RowCount != 450 || done.Statements != 1 || done.Truncated {
		t.Fatalf("done = %+v", done)
	}
	if done.Elapsed <= 0 {
		t.Fatalf("elapsed = %v", done.Elapsed)
	}
}

func TestRunnerTruncatesAndDrains(t *testing.T) {
	handle := openRunnerDB(t)
	seedRows(t, handle, 250)

	events := collectRun(t, handle, "SELECT id FROM numbers", 100)
	if rows := rowTotal(events); rows != 100 {
		t.Fatalf("streamed rows = %d, want cap 100", rows)
	}
	done := events[len(events)-1]
	if done.Kind != EventDone || done.RowCount != 100 || !done.Truncated {
		t.Fatalf("done = %+v", done)
	}
}

func TestRunnerNonQueryStatements(t *testing.T) {
	handle := openRunnerDB(t)
	events := collectRun(t, handle,
		"CREATE TABLE t (x INTEGER); INSERT INTO t VALUES (1); INSERT INTO t VALUES (2)", 0)
	for _, event := range events {
		if event.Kind == EventStarted {
			t.Fatalf("unexpected started event for non-query: %+v", event)
		}
	}
	done := events[len(events)-1]
	if done.Kind != EventDone || done.Statements != 3 || done.RowCount != 0 {
		t.Fatalf("done = %+v", done)
	}
}

func TestRunnerLastResultWins(t *testing.T) {
	handle := openRunnerDB(t)
	events := collectRun(t, handle,
		"CREATE TABLE t (x INTEGER); INSERT INTO t VALUES (7); SELECT x FROM t", 0)
	if rowTotal(events) != 1 {
		t.Fatalf("rows = %d", rowTotal(events))
	}
	done := events[len(events)-1]
	if done.Kind != EventDone || done.Statements != 3 || done.RowCount != 1 {
		t.Fatalf("done = %+v", done)
	}
}

func TestRunnerError(t *testing.T) {
	handle := openRunnerDB(t)
	events := collectRun(t, handle, "SELECT * FROM missing", 0)
	last := events[len(events)-1]
	if last.Kind != EventError || last.Err == nil || !strings.Contains(last.Err.Error(), "statement 1") {
		t.Fatalf("last = %+v", last)
	}
}

func TestRunnerCancellationStopsPromptly(t *testing.T) {
	handle := openRunnerDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan Event, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		Run(ctx, handle,
			"WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x<200000) SELECT x FROM c",
			0, func(event Event) bool {
				events <- event
				return true
			})
	}()

	var cancelled bool
	timeout := time.After(5 * time.Second)
	for !cancelled {
		select {
		case event := <-events:
			if event.Kind == EventBatch {
				cancel()
			}
			if event.Kind == EventCancelled {
				cancelled = true
			}
			if event.Kind == EventError || event.Kind == EventDone {
				t.Fatalf("unexpected terminal event: %+v", event)
			}
		case <-timeout:
			t.Fatal("runner did not cancel in time")
		}
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runner goroutine did not exit after cancellation")
	}
}
