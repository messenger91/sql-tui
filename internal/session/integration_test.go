//go:build integration

package session

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/schema"
)

const (
	defaultMySQLDSN    = "root:root@tcp(127.0.0.1:3307)/sql_tui_test?parseTime=true"
	defaultPostgresDSN = "postgres://postgres:postgres@127.0.0.1:5433/sql_tui_test?sslmode=disable"
)

func integrationDSN(env, fallback string) string {
	if value := os.Getenv(env); value != "" {
		return value
	}
	return fallback
}

func openIntegration(t *testing.T, driver db.Driver, dsn string) *sql.DB {
	t.Helper()
	handle, err := db.Connect(context.Background(), db.Connection{
		ID: "integration", Name: "integration", Driver: driver, DSN: dsn,
	})
	if err != nil {
		t.Fatalf("connect %s (is docker compose up?): %v", driver, err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return handle
}

func integrationSession(t *testing.T, driver db.Driver) (*Session, *sql.DB) {
	t.Helper()
	dsn := integrationDSN("MYSQL_TEST_DSN", defaultMySQLDSN)
	if driver == db.DriverPostgres {
		dsn = integrationDSN("POSTGRES_TEST_DSN", defaultPostgresDSN)
	}
	handle := openIntegration(t, driver, dsn)

	ctx := context.Background()
	for _, statement := range []string{
		"DROP TABLE IF EXISTS sql_tui_edit",
		"CREATE TABLE sql_tui_edit (id INTEGER PRIMARY KEY, email VARCHAR(50))",
		"INSERT INTO sql_tui_edit (id, email) VALUES (1, 'a'), (2, 'b')",
	} {
		if _, err := handle.ExecContext(ctx, statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}
	t.Cleanup(func() {
		_, _ = handle.ExecContext(context.Background(), "DROP TABLE IF EXISTS sql_tui_edit")
	})

	tables, err := db.LoadSchema(ctx, handle, driver)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	table, ok := findIntegrationTable(tables, "sql_tui_edit")
	if !ok {
		t.Fatalf("table missing from schema: %+v", tables)
	}

	result := db.Execute(ctx, handle, "SELECT id, email FROM sql_tui_edit ORDER BY id", 0)
	if result.Err != nil {
		t.Fatalf("select: %v", result.Err)
	}

	s := New("integration", "integration")
	s.Driver = driver
	s.Results.SetResult(result)
	s.Editability = ComputeEditability("SELECT id, email FROM sql_tui_edit", result.Columns,
		schema.BuildIndex([]db.TableMeta{table}), false)
	return s, handle
}

func findIntegrationTable(tables []db.TableMeta, name string) (db.TableMeta, bool) {
	for _, table := range tables {
		if table.Name == name {
			return table, true
		}
	}
	return db.TableMeta{}, false
}

func TestIntegrationApplyCommit(t *testing.T) {
	for _, driver := range []db.Driver{db.DriverMySQL, db.DriverPostgres} {
		t.Run(string(driver), func(t *testing.T) {
			s, handle := integrationSession(t, driver)
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
			if err := handle.QueryRow(`SELECT email FROM sql_tui_edit WHERE id = 1`).Scan(&email); err != nil {
				t.Fatalf("query: %v", err)
			}
			if email != "x" {
				t.Fatalf("email = %q", email)
			}
		})
	}
}

func TestIntegrationApplyConflict(t *testing.T) {
	for _, driver := range []db.Driver{db.DriverMySQL, db.DriverPostgres} {
		t.Run(string(driver), func(t *testing.T) {
			s, handle := integrationSession(t, driver)
			s.Pending.EnsureSnapshot(0, snapshotCurrent(s, 0))
			s.Pending.SetCell(0, "email", CellEdit{Value: "x", Original: "a"})
			if _, err := handle.Exec(`UPDATE sql_tui_edit SET email = 'outside' WHERE id = 1`); err != nil {
				t.Fatalf("external update: %v", err)
			}

			plan, _ := s.PrepareApply()
			outcome := plan.Apply(context.Background(), handle)
			if !errors.Is(outcome.Err, ErrRowChanged) {
				t.Fatalf("outcome = %+v", outcome)
			}
		})
	}
}

func TestIntegrationLongQueryCancellation(t *testing.T) {
	tests := []struct {
		driver db.Driver
		query  string
	}{
		{db.DriverPostgres, "SELECT pg_sleep(5)"},
		{db.DriverMySQL, "SELECT SLEEP(5)"},
	}
	for _, tc := range tests {
		t.Run(string(tc.driver), func(t *testing.T) {
			dsn := integrationDSN("MYSQL_TEST_DSN", defaultMySQLDSN)
			if tc.driver == db.DriverPostgres {
				dsn = integrationDSN("POSTGRES_TEST_DSN", defaultPostgresDSN)
			}
			handle := openIntegration(t, tc.driver, dsn)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			events := make(chan Event, 16)
			done := make(chan struct{})
			go func() {
				defer close(done)
				Run(ctx, handle, tc.query, 0, func(event Event) bool {
					events <- event
					return true
				})
			}()

			time.AfterFunc(150*time.Millisecond, cancel)
			deadline := time.After(5 * time.Second)
			started := time.Now()
			for {
				select {
				case event := <-events:
					if event.Kind == EventCancelled {
						if elapsed := time.Since(started); elapsed > 3*time.Second {
							t.Fatalf("cancellation took %v", elapsed)
						}
						return
					}
					if event.Kind == EventError || event.Kind == EventDone {
						t.Fatalf("unexpected terminal event: %+v", event)
					}
				case <-done:
					t.Fatal("runner exited without a cancelled event")
				case <-deadline:
					t.Fatal("runner did not cancel in time")
				}
			}
		})
	}
}
