package session

import (
	"sort"
	"strings"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/schema"
)

// Editability describes whether a result can be edited in place (§12).
type Editability struct {
	Editable   bool
	Table      string
	PrimaryKey []string
	Columns    map[string]db.ColumnMeta
	Reason     string
}

// ComputeEditability decides whether the result of sqlText is editable. The
// gate requires a writable connection, a simple single-table SELECT, every
// result column mapped to a table column, and a primary key present in the
// result.
func ComputeEditability(sqlText string, resultColumns []string, index *schema.Index, readOnly bool) Editability {
	if readOnly {
		return Editability{Reason: "connection read-only"}
	}
	tableName, ok := query.SimpleSelectTable(sqlText)
	if !ok {
		return Editability{Reason: "complex query"}
	}
	if index == nil {
		return Editability{Reason: "schema unavailable"}
	}
	table, ok := index.Table(tableName)
	if !ok {
		return Editability{Reason: "table not in schema"}
	}

	var primaryKey []db.ColumnMeta
	byName := make(map[string]db.ColumnMeta, len(table.Columns))
	for _, column := range table.Columns {
		byName[strings.ToLower(column.Name)] = column
		if column.IsPrimaryKey {
			primaryKey = append(primaryKey, column)
		}
	}
	sort.Slice(primaryKey, func(i, j int) bool {
		return primaryKey[i].Ordinal < primaryKey[j].Ordinal
	})
	if len(primaryKey) == 0 {
		return Editability{Table: table.Name, Reason: "no primary key"}
	}

	mapped := make(map[string]db.ColumnMeta, len(resultColumns))
	seen := make(map[string]bool, len(resultColumns))
	for _, name := range resultColumns {
		key := strings.ToLower(name)
		if seen[key] {
			return Editability{Table: table.Name, Reason: "duplicate column: " + name}
		}
		seen[key] = true
		column, ok := byName[key]
		if !ok {
			return Editability{Table: table.Name, Reason: "unmapped column: " + name}
		}
		mapped[name] = column
	}

	names := make([]string, 0, len(primaryKey))
	for _, column := range primaryKey {
		if !seen[strings.ToLower(column.Name)] {
			return Editability{Table: table.Name, Reason: "primary key not selected"}
		}
		names = append(names, column.Name)
	}

	return Editability{
		Editable:   true,
		Table:      table.Name,
		PrimaryKey: names,
		Columns:    mapped,
	}
}
