//go:build integration

package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
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

func openIntegration(t *testing.T, driver Driver, dsn string) *sql.DB {
	t.Helper()
	handle, err := Connect(context.Background(), Connection{
		ID:     "integration",
		Name:   "integration",
		Driver: driver,
		DSN:    dsn,
	})
	if err != nil {
		t.Fatalf("connect %s (is docker compose up?): %v", driver, err)
	}
	t.Cleanup(func() { _ = handle.Close() })
	return handle
}

var integrationDDL = []string{
	"DROP VIEW IF EXISTS sql_tui_view",
	"DROP TABLE IF EXISTS sql_tui_child",
	"DROP TABLE IF EXISTS sql_tui_composite",
	"DROP TABLE IF EXISTS sql_tui_parent",
	`CREATE TABLE sql_tui_parent (
		id INTEGER PRIMARY KEY,
		name VARCHAR(50) NOT NULL,
		note TEXT
	)`,
	`CREATE TABLE sql_tui_child (
		parent_id INTEGER,
		code VARCHAR(20) NOT NULL,
		CONSTRAINT fk_sql_tui_child_parent
			FOREIGN KEY (parent_id) REFERENCES sql_tui_parent(id)
	)`,
	`CREATE TABLE sql_tui_composite (
		a INTEGER NOT NULL,
		b INTEGER NOT NULL,
		PRIMARY KEY (a, b)
	)`,
	"CREATE VIEW sql_tui_view AS SELECT id FROM sql_tui_parent",
}

func setupIntegration(t *testing.T, driver Driver) *sql.DB {
	t.Helper()
	dsn := integrationDSN("MYSQL_TEST_DSN", defaultMySQLDSN)
	if driver == DriverPostgres {
		dsn = integrationDSN("POSTGRES_TEST_DSN", defaultPostgresDSN)
	}
	handle := openIntegration(t, driver, dsn)
	for _, statement := range integrationDDL {
		if _, err := handle.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("setup %q: %v", statement, err)
		}
	}
	if _, err := handle.ExecContext(context.Background(),
		"INSERT INTO sql_tui_parent (id, name) VALUES (1, 'ada')"); err != nil {
		t.Fatalf("insert fixture: %v", err)
	}
	t.Cleanup(func() {
		for _, statement := range []string{
			"DROP VIEW IF EXISTS sql_tui_view",
			"DROP TABLE IF EXISTS sql_tui_child",
			"DROP TABLE IF EXISTS sql_tui_composite",
			"DROP TABLE IF EXISTS sql_tui_parent",
		} {
			_, _ = handle.ExecContext(context.Background(), statement)
		}
	})
	return handle
}

func runSchemaIntegration(t *testing.T, driver Driver) {
	t.Helper()
	handle := setupIntegration(t, driver)

	result := Execute(context.Background(), handle, "SELECT id, name FROM sql_tui_parent", 0)
	if result.Err != nil {
		t.Fatalf("Execute select: %v", result.Err)
	}
	if len(result.Columns) != 2 || result.Columns[0] != "id" || result.Columns[1] != "name" {
		t.Fatalf("columns = %v", result.Columns)
	}
	if result.RowCount != 1 || len(result.Rows) != 1 || integrationString(result.Rows[0][1]) != "ada" {
		t.Fatalf("rows = %+v", result.Rows)
	}

	tables, err := LoadSchema(context.Background(), handle, driver)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}

	parent, ok := findTable(tables, "sql_tui_parent")
	if !ok {
		t.Fatalf("parent missing from %+v", tableNames(tables))
	}
	if parent.Schema == "" {
		t.Fatal("parent schema is empty")
	}
	if parent.Kind != TableKindTable {
		t.Fatalf("parent kind = %q", parent.Kind)
	}
	columns := columnMap(parent)
	if id, ok := columns["id"]; !ok || !id.IsPrimaryKey || id.Ordinal != 0 {
		t.Fatalf("id column = %+v", id)
	}
	if name, ok := columns["name"]; !ok || name.Nullable {
		t.Fatalf("name column = %+v", name)
	}
	if note, ok := columns["note"]; !ok || !note.Nullable {
		t.Fatalf("note column = %+v", note)
	}

	child, ok := findTable(tables, "sql_tui_child")
	if !ok {
		t.Fatalf("child missing from %+v", tableNames(tables))
	}
	wantFK := ForeignKey{Column: "parent_id", RefTable: "sql_tui_parent", RefColumn: "id"}
	if len(child.ForeignKeys) != 1 || child.ForeignKeys[0] != wantFK {
		t.Fatalf("child foreign keys = %+v, want %+v", child.ForeignKeys, wantFK)
	}

	composite, ok := findTable(tables, "sql_tui_composite")
	if !ok {
		t.Fatalf("composite missing from %+v", tableNames(tables))
	}
	for _, column := range composite.Columns {
		if !column.IsPrimaryKey {
			t.Fatalf("composite column %q is not a primary key", column.Name)
		}
	}

	view, ok := findTable(tables, "sql_tui_view")
	if !ok {
		t.Fatalf("view missing from %+v", tableNames(tables))
	}
	if view.Kind != TableKindView {
		t.Fatalf("view kind = %q", view.Kind)
	}
}

func TestMySQLSchemaAndQuery(t *testing.T) {
	runSchemaIntegration(t, DriverMySQL)
}

func TestPostgresSchemaAndQuery(t *testing.T) {
	runSchemaIntegration(t, DriverPostgres)
}

func findTable(tables []TableMeta, name string) (TableMeta, bool) {
	for _, table := range tables {
		if table.Name == name {
			return table, true
		}
	}
	return TableMeta{}, false
}

func columnMap(table TableMeta) map[string]ColumnMeta {
	columns := make(map[string]ColumnMeta, len(table.Columns))
	for _, column := range table.Columns {
		columns[column.Name] = column
	}
	return columns
}

func tableNames(tables []TableMeta) []string {
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.Name)
	}
	return names
}

// integrationString normalises driver value types (MySQL returns []byte for
// VARCHAR columns).
func integrationString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}
