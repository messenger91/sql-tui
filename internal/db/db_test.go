package db

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	handle, err := Connect(context.Background(), Connection{
		ID:     "test",
		Name:   "test",
		Driver: DriverSQLite,
		DSN:    ":memory:",
	})
	if err != nil {
		t.Fatalf("Connect(:memory:): %v", err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return handle
}

func mustExec(t *testing.T, handle *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := handle.ExecContext(context.Background(), query, args...); err != nil {
		t.Fatalf("setup exec %q: %v", query, err)
	}
}

func TestNewConnectionDefaultsToReadOnly(t *testing.T) {
	conn := NewConnection("id", "name", DriverSQLite, ":memory:")
	if !conn.ReadOnly {
		t.Fatal("NewConnection must default to ReadOnly=true")
	}
}

func TestConnectUnknownDriver(t *testing.T) {
	_, err := Connect(context.Background(), Connection{Driver: Driver("oracle"), DSN: "x"})
	if !errors.Is(err, ErrUnsupportedDriver) {
		t.Fatalf("want ErrUnsupportedDriver, got %v", err)
	}
}

func TestExecuteSelectAndNull(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE people (id INTEGER, name TEXT)`)
	mustExec(t, handle, `INSERT INTO people (id, name) VALUES (1, 'ada'), (2, NULL)`)

	res := Execute(context.Background(), handle, `SELECT id, name FROM people ORDER BY id`, 0)
	if res.Err != nil {
		t.Fatalf("Execute: %v", res.Err)
	}
	if got, want := len(res.Columns), 2; got != want {
		t.Fatalf("columns = %v", res.Columns)
	}
	if res.Columns[0] != "id" || res.Columns[1] != "name" {
		t.Fatalf("columns = %v", res.Columns)
	}
	if res.RowCount != 2 || len(res.Rows) != 2 {
		t.Fatalf("row count = %d, rows = %d", res.RowCount, len(res.Rows))
	}
	if got := res.Rows[0][0]; got != int64(1) {
		t.Fatalf("row 0 id = %#v", got)
	}
	if got := res.Rows[1][1]; got != nil {
		t.Fatalf("row 1 name = %#v, want nil", got)
	}
	if res.Truncated {
		t.Fatal("unexpected truncation")
	}
}

func TestExecuteEmptyResultKeepsColumns(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE empty (id INTEGER)`)

	res := Execute(context.Background(), handle, `SELECT id FROM empty`, 0)
	if res.Err != nil {
		t.Fatalf("Execute: %v", res.Err)
	}
	if len(res.Columns) != 1 || res.Columns[0] != "id" {
		t.Fatalf("columns = %v", res.Columns)
	}
	if res.RowCount != 0 || len(res.Rows) != 0 {
		t.Fatalf("rows = %v", res.Rows)
	}
}

func TestExecuteNonQueryIsNotRetried(t *testing.T) {
	handle := openTestDB(t)
	if res := Execute(context.Background(), handle, `CREATE TABLE t (id INTEGER)`, 0); res.Err != nil {
		t.Fatalf("create: %v", res.Err)
	}
	if res := Execute(context.Background(), handle, `INSERT INTO t (id) VALUES (42)`, 0); res.Err != nil {
		t.Fatalf("insert: %v", res.Err)
	}

	var count int
	if err := handle.QueryRow(`SELECT count(*) FROM t`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("INSERT ran %d times, want exactly 1", count)
	}
}

func TestExecuteMaxRowsTruncation(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE n (id INTEGER)`)
	for i := 0; i < 5; i++ {
		mustExec(t, handle, `INSERT INTO n (id) VALUES (?)`, i)
	}

	res := Execute(context.Background(), handle, `SELECT id FROM n ORDER BY id`, 2)
	if res.Err != nil {
		t.Fatalf("Execute: %v", res.Err)
	}
	if len(res.Rows) != 2 || res.RowCount != 2 {
		t.Fatalf("rows = %d, count = %d", len(res.Rows), res.RowCount)
	}
	if !res.Truncated {
		t.Fatal("expected Truncated=true")
	}

	exact := Execute(context.Background(), handle, `SELECT id FROM n ORDER BY id`, 5)
	if exact.Err != nil {
		t.Fatalf("Execute exact: %v", exact.Err)
	}
	if exact.Truncated {
		t.Fatal("Truncated must be false when the cap equals the row count")
	}
}

func TestExecuteValueTypes(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE vals (i INTEGER, r REAL, s TEXT, b BLOB, m TEXT)`)
	mustExec(t, handle, `INSERT INTO vals VALUES (7, 1.5, 'x', x'0102', NULL)`)

	res := Execute(context.Background(), handle, `SELECT i, r, s, b, m FROM vals`, 0)
	if res.Err != nil {
		t.Fatalf("Execute: %v", res.Err)
	}
	row := res.Rows[0]
	if row[0] != int64(7) {
		t.Fatalf("integer = %#v", row[0])
	}
	if row[1] != 1.5 {
		t.Fatalf("real = %#v", row[1])
	}
	if row[2] != "x" {
		t.Fatalf("text = %#v", row[2])
	}
	if blob, ok := row[3].([]byte); !ok || !bytes.Equal(blob, []byte{1, 2}) {
		t.Fatalf("blob = %#v", row[3])
	}
	if row[4] != nil {
		t.Fatalf("null = %#v", row[4])
	}
}

func TestExecuteErrors(t *testing.T) {
	handle := openTestDB(t)

	res := Execute(context.Background(), handle, `SELECT * FROM missing`, 0)
	if res.Err == nil {
		t.Fatal("expected error for missing table")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res = Execute(ctx, handle, `SELECT 1`, 0)
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", res.Err)
	}
}

func TestManagerOpenAndCloseAll(t *testing.T) {
	manager := NewManager()
	conn := NewConnection("c1", "test", DriverSQLite, ":memory:")

	first, err := manager.Open(context.Background(), conn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	second, err := manager.Open(context.Background(), conn)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if first != second {
		t.Fatal("Open must reuse the existing handle for the same ID")
	}
	if got, ok := manager.Get("c1"); !ok || got != first {
		t.Fatal("Get did not return the open handle")
	}

	if err := manager.CloseAll(); err != nil {
		t.Fatalf("CloseAll: %v", err)
	}
	if _, ok := manager.Get("c1"); ok {
		t.Fatal("Get after CloseAll returned a handle")
	}
	if err := first.Ping(); err == nil {
		t.Fatal("handle still usable after CloseAll")
	}
}

func TestManagerCloseOne(t *testing.T) {
	manager := NewManager()
	if _, err := manager.Open(context.Background(), NewConnection("c1", "t", DriverSQLite, ":memory:")); err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := manager.Close("c1"); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := manager.Close("c1"); err != nil {
		t.Fatalf("second Close must be a no-op: %v", err)
	}
	if _, ok := manager.Get("c1"); ok {
		t.Fatal("Get after Close returned a handle")
	}
}

func TestDialect(t *testing.T) {
	tests := []struct {
		driver      Driver
		argIndex    int
		placeholder string
		quote       string
		ident       string
	}{
		{DriverMySQL, 3, "?", "`a``b`", "a`b"},
		{DriverPostgres, 3, "$3", `"a""b"`, `a"b`},
		{DriverSQLite, 2, "?", `"a""b"`, `a"b`},
	}
	for _, tc := range tests {
		t.Run(string(tc.driver), func(t *testing.T) {
			dialect := tc.driver.Dialect()
			if got := dialect.Placeholder(tc.argIndex); got != tc.placeholder {
				t.Fatalf("placeholder = %q, want %q", got, tc.placeholder)
			}
			if got := dialect.QuoteIdent(tc.ident); got != tc.quote {
				t.Fatalf("quote = %q, want %q", got, tc.quote)
			}
		})
	}
}
