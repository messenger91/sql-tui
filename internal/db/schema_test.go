package db

import (
	"context"
	"errors"
	"testing"
)

func loadTestSchema(t *testing.T) []TableMeta {
	t.Helper()
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE parent (
		id INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		note TEXT
	)`)
	mustExec(t, handle, `CREATE TABLE child (
		parent_id INTEGER REFERENCES parent(id),
		code TEXT NOT NULL
	)`)
	mustExec(t, handle, `CREATE TABLE composite (
		a INTEGER,
		b INTEGER,
		PRIMARY KEY (a, b)
	)`)
	mustExec(t, handle, `CREATE VIEW parent_view AS SELECT id FROM parent`)

	tables, err := LoadSchema(context.Background(), handle, DriverSQLite)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	return tables
}

func tableByName(tables []TableMeta, name string) TableMeta {
	for _, table := range tables {
		if table.Name == name {
			return table
		}
	}
	return TableMeta{}
}

func TestLoadSchemaSQLiteOrderAndKinds(t *testing.T) {
	tables := loadTestSchema(t)

	var names []string
	for _, table := range tables {
		names = append(names, table.Name)
	}
	want := []string{"child", "composite", "parent", "parent_view"}
	if len(names) != len(want) {
		t.Fatalf("tables = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("tables = %v, want %v", names, want)
		}
	}

	for _, table := range tables {
		if table.Schema != "main" {
			t.Fatalf("%s schema = %q", table.Name, table.Schema)
		}
		wantKind := TableKindTable
		if table.Name == "parent_view" {
			wantKind = TableKindView
		}
		if table.Kind != wantKind {
			t.Fatalf("%s kind = %q, want %q", table.Name, table.Kind, wantKind)
		}
	}
}

func TestLoadSchemaSQLiteColumns(t *testing.T) {
	tables := loadTestSchema(t)

	parent := tableByName(tables, "parent")
	if parent.Name == "" {
		t.Fatal("parent table missing")
	}
	wantColumns := []ColumnMeta{
		{Name: "id", Type: "INTEGER", Nullable: true, IsPrimaryKey: true, Ordinal: 0},
		{Name: "name", Type: "TEXT", Nullable: false, Ordinal: 1},
		{Name: "note", Type: "TEXT", Nullable: true, Ordinal: 2},
	}
	if len(parent.Columns) != len(wantColumns) {
		t.Fatalf("parent columns = %+v", parent.Columns)
	}
	for i, want := range wantColumns {
		if parent.Columns[i] != want {
			t.Fatalf("parent column %d = %+v, want %+v", i, parent.Columns[i], want)
		}
	}

	composite := tableByName(tables, "composite")
	if len(composite.Columns) != 2 {
		t.Fatalf("composite columns = %+v", composite.Columns)
	}
	for _, column := range composite.Columns {
		if !column.IsPrimaryKey {
			t.Fatalf("composite column %q not marked as PK", column.Name)
		}
	}
}

func TestLoadSchemaSQLiteForeignKeys(t *testing.T) {
	tables := loadTestSchema(t)

	child := tableByName(tables, "child")
	want := ForeignKey{Column: "parent_id", RefTable: "parent", RefColumn: "id"}
	if len(child.ForeignKeys) != 1 || child.ForeignKeys[0] != want {
		t.Fatalf("foreign keys = %+v, want %+v", child.ForeignKeys, want)
	}
}

func TestLoadSchemaSQLiteSkipsInternalTables(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE auto (id INTEGER PRIMARY KEY AUTOINCREMENT)`)

	tables, err := LoadSchema(context.Background(), handle, DriverSQLite)
	if err != nil {
		t.Fatalf("LoadSchema: %v", err)
	}
	for _, table := range tables {
		if table.Name == "sqlite_sequence" {
			t.Fatal("internal sqlite_sequence table leaked into the schema")
		}
	}
}

func TestLoadSchemaUnsupportedDrivers(t *testing.T) {
	if _, err := LoadSchema(context.Background(), nil, DriverMySQL); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("mysql: want ErrUnsupportedSchema, got %v", err)
	}
	if _, err := LoadSchema(context.Background(), nil, DriverPostgres); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("postgres: want ErrUnsupportedSchema, got %v", err)
	}
	if _, err := LoadSchema(context.Background(), nil, Driver("oracle")); !errors.Is(err, ErrUnsupportedDriver) {
		t.Fatalf("unknown: want ErrUnsupportedDriver, got %v", err)
	}
}
