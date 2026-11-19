package result

import (
	"reflect"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestBuildUpdateSQLite(t *testing.T) {
	statements, err := BuildStatements(db.DriverSQLite, []Change{{
		Kind:  ChangeUpdate,
		Table: "users",
		Key:   []ColumnValue{{Column: "id", Value: int64(1)}},
		Set:   []ColumnValue{{Column: "email", Value: "x"}},
	}})
	if err != nil {
		t.Fatalf("BuildStatements: %v", err)
	}
	if len(statements) != 1 {
		t.Fatalf("statements = %d", len(statements))
	}
	wantSQL := `UPDATE "users" SET "email" = ? WHERE "id" = ?`
	if statements[0].SQL != wantSQL {
		t.Fatalf("sql = %q, want %q", statements[0].SQL, wantSQL)
	}
	if !reflect.DeepEqual(statements[0].Args, []any{"x", int64(1)}) {
		t.Fatalf("args = %#v", statements[0].Args)
	}
}

func TestBuildUpdatePostgresNumbering(t *testing.T) {
	statements, err := BuildStatements(db.DriverPostgres, []Change{{
		Kind:  ChangeUpdate,
		Table: "users",
		Key: []ColumnValue{
			{Column: "tenant", Value: int64(9)},
			{Column: "id", Value: int64(1)},
		},
		Set: []ColumnValue{
			{Column: "email", Value: "x"},
			{Column: "name", Value: "n"},
		},
	}})
	if err != nil {
		t.Fatalf("BuildStatements: %v", err)
	}
	wantSQL := `UPDATE "users" SET "email" = $1, "name" = $2 WHERE "tenant" = $3 AND "id" = $4`
	if statements[0].SQL != wantSQL {
		t.Fatalf("sql = %q", statements[0].SQL)
	}
}

func TestBuildDeleteCompositeKey(t *testing.T) {
	statements, err := BuildStatements(db.DriverSQLite, []Change{{
		Kind:  ChangeDelete,
		Table: "composite",
		Key: []ColumnValue{
			{Column: "a", Value: int64(1)},
			{Column: "b", Value: int64(2)},
		},
	}})
	if err != nil {
		t.Fatalf("BuildStatements: %v", err)
	}
	wantSQL := `DELETE FROM "composite" WHERE "a" = ? AND "b" = ?`
	if statements[0].SQL != wantSQL {
		t.Fatalf("sql = %q", statements[0].SQL)
	}
}

func TestBuildInsert(t *testing.T) {
	statements, err := BuildStatements(db.DriverPostgres, []Change{{
		Kind:  ChangeInsert,
		Table: "users",
		Values: []ColumnValue{
			{Column: "id", Value: int64(3)},
			{Column: "email", Value: nil},
		},
	}})
	if err != nil {
		t.Fatalf("BuildStatements: %v", err)
	}
	wantSQL := `INSERT INTO "users" ("id", "email") VALUES ($1, $2)`
	if statements[0].SQL != wantSQL {
		t.Fatalf("sql = %q", statements[0].SQL)
	}
	if !reflect.DeepEqual(statements[0].Args, []any{int64(3), nil}) {
		t.Fatalf("args = %#v", statements[0].Args)
	}
}

func TestBuildMySQLQuotingAndBooleans(t *testing.T) {
	statements, err := BuildStatements(db.DriverMySQL, []Change{{
		Kind:  ChangeUpdate,
		Table: "we`ird",
		Key:   []ColumnValue{{Column: "i`d", Value: true}},
		Set:   []ColumnValue{{Column: "active", Value: true}},
	}})
	if err != nil {
		t.Fatalf("BuildStatements: %v", err)
	}
	wantSQL := "UPDATE `we``ird` SET `active` = ? WHERE `i``d` = ?"
	if statements[0].SQL != wantSQL {
		t.Fatalf("sql = %q", statements[0].SQL)
	}
	if !reflect.DeepEqual(statements[0].Args, []any{int64(1), int64(1)}) {
		t.Fatalf("args = %#v", statements[0].Args)
	}
}

func TestBuildPostgresEmbeddedQuotes(t *testing.T) {
	statements, err := BuildStatements(db.DriverPostgres, []Change{{
		Kind:  ChangeDelete,
		Table: `we"ird`,
		Key:   []ColumnValue{{Column: `i"d`, Value: int64(1)}},
	}})
	if err != nil {
		t.Fatalf("BuildStatements: %v", err)
	}
	wantSQL := `DELETE FROM "we""ird" WHERE "i""d" = $1`
	if statements[0].SQL != wantSQL {
		t.Fatalf("sql = %q", statements[0].SQL)
	}
}

func TestBuildErrors(t *testing.T) {
	if _, err := BuildStatements(db.Driver("oracle"), nil); err == nil {
		t.Fatal("unsupported driver accepted")
	}
	if _, err := BuildStatements(db.DriverSQLite, []Change{{
		Kind: ChangeUpdate, Table: "t", Key: []ColumnValue{{Column: "id", Value: 1}},
	}}); err == nil {
		t.Fatal("empty update accepted")
	}
	if _, err := BuildStatements(db.DriverSQLite, []Change{{
		Kind: ChangeDelete, Table: "t",
	}}); err == nil {
		t.Fatal("delete without key accepted")
	}
}
