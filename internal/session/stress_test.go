package session

import (
	"context"
	"database/sql"
	"sync"
	"testing"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

func openStressDB(t *testing.T) *sql.DB {
	t.Helper()
	handle, err := db.Connect(context.Background(), db.Connection{
		ID: "stress", Name: "stress", Driver: db.DriverSQLite, DSN: ":memory:",
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return handle
}

func TestRunnerConcurrentSessionsComplete(t *testing.T) {
	const sessions = 10
	completed := make([]bool, sessions)
	var wg sync.WaitGroup

	for i := 0; i < sessions; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			handle := openStressDB(t)
			defer handle.Close()
			Run(context.Background(), handle, "SELECT 1 AS n", 0, func(event Event) bool {
				if event.Kind == EventDone {
					completed[index] = true
				}
				return true
			})
		}(i)
	}
	wg.Wait()

	for i, done := range completed {
		if !done {
			t.Fatalf("session %d did not complete", i)
		}
	}
}

func TestRunnerConcurrentSessionsCancelCleanly(t *testing.T) {
	const sessions = 10
	finished := make([]bool, sessions)
	var wg sync.WaitGroup

	for i := 0; i < sessions; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			handle := openStressDB(t)

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				defer close(done)
				Run(ctx, handle,
					"WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x<2000000) SELECT x FROM c",
					0, func(event Event) bool {
						if event.Kind == EventBatch {
							cancel()
						}
						if event.Kind == EventCancelled {
							finished[index] = true
						}
						return true
					})
			}()

			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Errorf("session %d did not stop", index)
			}
			cancel()
			_ = handle.Close()
		}(i)
	}
	wg.Wait()

	for i, ok := range finished {
		if !ok {
			t.Fatalf("session %d did not report cancellation", i)
		}
	}
}
