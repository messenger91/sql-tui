package session

import (
	"strings"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/schema"
)

func editIndex() *schema.Index {
	return schema.BuildIndex([]db.TableMeta{
		{
			Name: "users",
			Columns: []db.ColumnMeta{
				{Name: "id", Type: "INTEGER", IsPrimaryKey: true, Ordinal: 0},
				{Name: "email", Type: "TEXT", Ordinal: 1},
				{Name: "name", Type: "TEXT", Ordinal: 2},
			},
		},
		{
			Name: "composite",
			Columns: []db.ColumnMeta{
				{Name: "a", IsPrimaryKey: true, Ordinal: 0},
				{Name: "b", IsPrimaryKey: true, Ordinal: 1},
				{Name: "note", Ordinal: 2},
			},
		},
		{
			Name: "no_pk",
			Columns: []db.ColumnMeta{
				{Name: "value", Ordinal: 0},
			},
		},
	})
}

func TestComputeEditability(t *testing.T) {
	index := editIndex()

	editable := ComputeEditability("SELECT id, email FROM users", []string{"id", "email"}, index, false)
	if !editable.Editable || editable.Table != "users" {
		t.Fatalf("editable = %+v", editable)
	}
	if len(editable.PrimaryKey) != 1 || editable.PrimaryKey[0] != "id" {
		t.Fatalf("pk = %v", editable.PrimaryKey)
	}
	if editable.Columns["email"].Name != "email" {
		t.Fatalf("columns = %+v", editable.Columns)
	}

	composite := ComputeEditability("SELECT a, b, note FROM composite", []string{"a", "b", "note"}, index, false)
	if !composite.Editable || strings.Join(composite.PrimaryKey, ",") != "a,b" {
		t.Fatalf("composite = %+v", composite)
	}

	cases := []struct {
		name     string
		sql      string
		columns  []string
		index    *schema.Index
		readOnly bool
		reason   string
	}{
		{"read-only", "SELECT id FROM users", []string{"id"}, index, true, "connection read-only"},
		{"complex", "SELECT * FROM users JOIN orders ON 1=1", []string{"id"}, index, false, "complex query"},
		{"nil index", "SELECT id FROM users", []string{"id"}, nil, false, "schema unavailable"},
		{"missing table", "SELECT id FROM missing", []string{"id"}, index, false, "table not in schema"},
		{"no pk", "SELECT value FROM no_pk", []string{"value"}, index, false, "no primary key"},
		{"pk missing", "SELECT email FROM users", []string{"email"}, index, false, "primary key not selected"},
		{"alias mismatch", "SELECT email AS mail FROM users", []string{"mail"}, index, false, "unmapped column: mail"},
		{"duplicate", "SELECT id, id FROM users", []string{"id", "id"}, index, false, "duplicate column: id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeEditability(tc.sql, tc.columns, tc.index, tc.readOnly)
			if got.Editable || got.Reason != tc.reason {
				t.Fatalf("editability = %+v, want reason %q", got, tc.reason)
			}
		})
	}
}

func TestComputeEditabilityCaseInsensitive(t *testing.T) {
	got := ComputeEditability("select ID from USERS", []string{"ID"}, editIndex(), false)
	if !got.Editable || strings.Join(got.PrimaryKey, ",") != "id" {
		t.Fatalf("editability = %+v", got)
	}
}
