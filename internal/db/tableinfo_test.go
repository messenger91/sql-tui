package db

import (
	"context"
	"strings"
	"testing"
)

func TestGetTableInfoSQLite(t *testing.T) {
	handle := openTestDB(t)
	mustExec(t, handle, `CREATE TABLE users (
		id INTEGER PRIMARY KEY,
		email TEXT
	)`)
	mustExec(t, handle, `CREATE TABLE child (
		parent_id INTEGER REFERENCES users(id),
		code TEXT NOT NULL
	)`)
	mustExec(t, handle, `CREATE INDEX idx_child_code ON child (code)`)
	mustExec(t, handle, `CREATE VIEW active AS SELECT id FROM users`)

	child, err := GetTableInfo(context.Background(), handle, DriverSQLite, "child")
	if err != nil {
		t.Fatalf("GetTableInfo: %v", err)
	}
	if len(child.Columns) != 2 || child.Columns[0].Name != "parent_id" {
		t.Fatalf("columns = %+v", child.Columns)
	}
	if len(child.ForeignKeys) != 1 || child.ForeignKeys[0].RefTable != "users" {
		t.Fatalf("foreign keys = %+v", child.ForeignKeys)
	}
	foundIndex := false
	for _, idx := range child.Indexes {
		if strings.Contains(idx.Name, "idx_child_code") {
			foundIndex = true
		}
	}
	if !foundIndex {
		t.Fatalf("indexes = %+v", child.Indexes)
	}
	hasFK := false
	for _, constraint := range child.Constraints {
		if constraint.Type == "FOREIGN KEY" {
			hasFK = true
		}
	}
	if !hasFK {
		t.Fatalf("constraints = %+v", child.Constraints)
	}
	if !strings.Contains(child.DDL, "CREATE TABLE child") {
		t.Fatalf("ddl = %q", child.DDL)
	}

	users, err := GetTableInfo(context.Background(), handle, DriverSQLite, "users")
	if err != nil {
		t.Fatalf("GetTableInfo users: %v", err)
	}
	hasPK := false
	for _, constraint := range users.Constraints {
		if constraint.Type == "PRIMARY KEY" {
			hasPK = true
		}
	}
	if !hasPK {
		t.Fatalf("users constraints = %+v", users.Constraints)
	}

	view, err := GetTableInfo(context.Background(), handle, DriverSQLite, "active")
	if err != nil {
		t.Fatalf("GetTableInfo view: %v", err)
	}
	if view.Kind != TableKindView {
		t.Fatalf("view kind = %q", view.Kind)
	}
	if !strings.Contains(view.DDL, "CREATE VIEW active") {
		t.Fatalf("view ddl = %q", view.DDL)
	}
}

func TestDBML(t *testing.T) {
	tables := []TableMeta{
		{Name: "users", Columns: []ColumnMeta{
			{Name: "id", Type: "integer", IsPrimaryKey: true},
			{Name: "email", Type: "text", Nullable: true},
		}},
		{Name: "orders", Kind: TableKindTable, Columns: []ColumnMeta{
			{Name: "user_id", Type: "integer"},
		}, ForeignKeys: []ForeignKey{{Column: "user_id", RefTable: "users", RefColumn: "id"}}},
		{Name: "active_users", Kind: TableKindView, Columns: []ColumnMeta{{Name: "id", Type: "integer"}}},
	}

	ml := DBML(tables)
	if !strings.Contains(ml, "Table users {") {
		t.Fatalf("dbml = %q", ml)
	}
	if !strings.Contains(ml, "id integer [pk, not null]") {
		t.Fatalf("dbml missing pk: %q", ml)
	}
	if !strings.Contains(ml, "email text\n") {
		t.Fatalf("dbml missing nullable column: %q", ml)
	}
	if !strings.Contains(ml, "user_id integer [not null]") {
		t.Fatalf("dbml missing fk column: %q", ml)
	}
	if !strings.Contains(ml, "Ref: orders.user_id > users.id") {
		t.Fatalf("dbml missing ref: %q", ml)
	}
	if !strings.Contains(ml, "// view") || !strings.Contains(ml, "Table active_users {") {
		t.Fatalf("dbml missing view: %q", ml)
	}
	if DBML(nil) != "" {
		t.Fatal("empty schema produced output")
	}
}
