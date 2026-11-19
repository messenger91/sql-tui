package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// TableKind distinguishes base tables from views.
type TableKind string

const (
	TableKindTable TableKind = "table"
	TableKindView  TableKind = "view"
)

// ColumnMeta describes one column of a table or view.
type ColumnMeta struct {
	Name         string
	Type         string
	Nullable     bool
	IsPrimaryKey bool
	Ordinal      int
}

// ForeignKey is a single column-level foreign-key reference.
type ForeignKey struct {
	Column    string
	RefTable  string
	RefColumn string
}

// TableMeta describes a table or view and its columns.
type TableMeta struct {
	Schema      string
	Name        string
	Kind        TableKind
	Columns     []ColumnMeta
	ForeignKeys []ForeignKey
}

// LoadSchema introspects the database behind handle. Results are sorted by
// kind then name; columns are ordered by ordinal. Drivers without an
// introspection path return ErrUnsupportedSchema. Partial results may be
// returned together with a non-nil error when only some objects failed.
func LoadSchema(ctx context.Context, handle *sql.DB, driver Driver) ([]TableMeta, error) {
	switch driver {
	case DriverSQLite:
		return loadSQLiteSchema(ctx, handle)
	case DriverMySQL, DriverPostgres:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedSchema, driver)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDriver, driver)
	}
}

func loadSQLiteSchema(ctx context.Context, handle *sql.DB) ([]TableMeta, error) {
	rows, err := handle.QueryContext(ctx,
		`SELECT name, type FROM sqlite_master WHERE type IN ('table', 'view') ORDER BY type, name`)
	if err != nil {
		return nil, fmt.Errorf("db: list sqlite objects: %w", err)
	}

	type object struct {
		name string
		kind TableKind
	}
	var objects []object
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan sqlite object: %w", err)
		}
		// Internal tables (sqlite_sequence, FTS shadow tables, ...) are not
		// part of the user schema.
		if strings.HasPrefix(name, "sqlite_") {
			continue
		}
		objects = append(objects, object{name: name, kind: TableKind(kind)})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: list sqlite objects: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("db: close sqlite objects: %w", err)
	}

	// The sqlite handle is limited to one physical connection (:memory:
	// correctness), so PRAGMAs must run only after the listing rows above
	// have been closed.
	tables := make([]TableMeta, 0, len(objects))
	var errs []error
	for _, obj := range objects {
		table := TableMeta{
			Schema: "main",
			Name:   obj.name,
			Kind:   obj.kind,
		}
		columns, err := sqliteColumns(ctx, handle, obj.name)
		if err != nil {
			errs = append(errs, err)
		} else {
			table.Columns = columns
		}
		fks, err := sqliteForeignKeys(ctx, handle, obj.name)
		if err != nil {
			errs = append(errs, err)
		} else {
			table.ForeignKeys = fks
		}
		tables = append(tables, table)
	}
	return tables, errors.Join(errs...)
}

func sqliteColumns(ctx context.Context, handle *sql.DB, table string) ([]ColumnMeta, error) {
	rows, err := handle.QueryContext(ctx, "PRAGMA table_info("+quoteDouble(table)+")")
	if err != nil {
		return nil, fmt.Errorf("db: table_info %q: %w", table, err)
	}
	defer rows.Close()

	var columns []ColumnMeta
	for rows.Next() {
		var (
			column   ColumnMeta
			notNull  int
			defaultV sql.NullString
			pk       int
		)
		if err := rows.Scan(&column.Ordinal, &column.Name, &column.Type, &notNull, &defaultV, &pk); err != nil {
			return nil, fmt.Errorf("db: scan table_info %q: %w", table, err)
		}
		column.Nullable = notNull == 0
		column.IsPrimaryKey = pk > 0
		columns = append(columns, column)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: table_info %q: %w", table, err)
	}
	sort.Slice(columns, func(i, j int) bool { return columns[i].Ordinal < columns[j].Ordinal })
	return columns, nil
}

func sqliteForeignKeys(ctx context.Context, handle *sql.DB, table string) ([]ForeignKey, error) {
	rows, err := handle.QueryContext(ctx, "PRAGMA foreign_key_list("+quoteDouble(table)+")")
	if err != nil {
		return nil, fmt.Errorf("db: foreign_key_list %q: %w", table, err)
	}
	defer rows.Close()

	var fks []ForeignKey
	for rows.Next() {
		var (
			id, seq            int
			refTable, from     string
			to                 sql.NullString
			onUpdate, onDelete string
			match              string
		)
		if err := rows.Scan(&id, &seq, &refTable, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return nil, fmt.Errorf("db: scan foreign_key_list %q: %w", table, err)
		}
		fks = append(fks, ForeignKey{
			Column:    from,
			RefTable:  refTable,
			RefColumn: to.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("db: foreign_key_list %q: %w", table, err)
	}
	sort.Slice(fks, func(i, j int) bool {
		if fks[i].Column != fks[j].Column {
			return fks[i].Column < fks[j].Column
		}
		return fks[i].RefTable < fks[j].RefTable
	})
	return fks, nil
}
