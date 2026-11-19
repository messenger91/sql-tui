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
	case DriverMySQL:
		return loadMySQLSchema(ctx, handle)
	case DriverPostgres:
		return loadPostgresSchema(ctx, handle)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDriver, driver)
	}
}

// schemaBuilder assembles introspection rows into TableMeta values.
type schemaBuilder struct {
	tables []TableMeta
	index  map[string]int
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{index: make(map[string]int)}
}

func (b *schemaBuilder) add(name string, kind TableKind, schema string) {
	if _, ok := b.index[name]; ok {
		return
	}
	b.index[name] = len(b.tables)
	b.tables = append(b.tables, TableMeta{Schema: schema, Name: name, Kind: kind})
}

func (b *schemaBuilder) column(table string, column ColumnMeta) {
	position, ok := b.index[table]
	if !ok {
		return
	}
	b.tables[position].Columns = append(b.tables[position].Columns, column)
}

func (b *schemaBuilder) foreignKey(table string, fk ForeignKey) {
	position, ok := b.index[table]
	if !ok {
		return
	}
	b.tables[position].ForeignKeys = append(b.tables[position].ForeignKeys, fk)
}

func (b *schemaBuilder) finish() []TableMeta {
	for i := range b.tables {
		sort.Slice(b.tables[i].Columns, func(a, c int) bool {
			return b.tables[i].Columns[a].Ordinal < b.tables[i].Columns[c].Ordinal
		})
		sort.Slice(b.tables[i].ForeignKeys, func(a, c int) bool {
			if b.tables[i].ForeignKeys[a].Column != b.tables[i].ForeignKeys[c].Column {
				return b.tables[i].ForeignKeys[a].Column < b.tables[i].ForeignKeys[c].Column
			}
			return b.tables[i].ForeignKeys[a].RefTable < b.tables[i].ForeignKeys[c].RefTable
		})
	}
	return b.tables
}

func tableKind(raw string) TableKind {
	if strings.EqualFold(raw, "VIEW") {
		return TableKindView
	}
	return TableKindTable
}

func loadMySQLSchema(ctx context.Context, handle *sql.DB) ([]TableMeta, error) {
	builder := newSchemaBuilder()
	var errs []error

	rows, err := handle.QueryContext(ctx, `
		SELECT table_name, table_type, table_schema
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_type IN ('BASE TABLE', 'VIEW')
		ORDER BY table_type, table_name`)
	if err != nil {
		return nil, fmt.Errorf("db: list mysql tables: %w", err)
	}
	for rows.Next() {
		var name, kind, schema string
		if err := rows.Scan(&name, &kind, &schema); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan mysql table: %w", err)
		}
		builder.add(name, tableKind(kind), schema)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: list mysql tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("db: close mysql tables: %w", err)
	}

	rows, err = handle.QueryContext(ctx, `
		SELECT table_name, column_name, column_type, is_nullable, ordinal_position, column_key
		FROM information_schema.columns
		WHERE table_schema = DATABASE()
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		errs = append(errs, fmt.Errorf("db: mysql columns: %w", err))
	} else {
		for rows.Next() {
			var (
				table, name, dataType, nullable, columnKey string
				ordinal                                    int
			)
			if err := rows.Scan(&table, &name, &dataType, &nullable, &ordinal, &columnKey); err != nil {
				errs = append(errs, fmt.Errorf("db: scan mysql column: %w", err))
				break
			}
			builder.column(table, ColumnMeta{
				Name:         name,
				Type:         dataType,
				Nullable:     strings.EqualFold(nullable, "YES"),
				IsPrimaryKey: strings.EqualFold(columnKey, "PRI"),
				Ordinal:      ordinal - 1,
			})
		}
		if err := rows.Err(); err != nil {
			errs = append(errs, fmt.Errorf("db: mysql columns: %w", err))
		}
		_ = rows.Close()
	}

	rows, err = handle.QueryContext(ctx, `
		SELECT table_name, column_name, referenced_table_name, referenced_column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE() AND referenced_table_name IS NOT NULL
		ORDER BY table_name, constraint_name, ordinal_position`)
	if err != nil {
		errs = append(errs, fmt.Errorf("db: mysql foreign keys: %w", err))
	} else {
		for rows.Next() {
			var table, column, refTable, refColumn string
			if err := rows.Scan(&table, &column, &refTable, &refColumn); err != nil {
				errs = append(errs, fmt.Errorf("db: scan mysql foreign key: %w", err))
				break
			}
			builder.foreignKey(table, ForeignKey{
				Column:    column,
				RefTable:  refTable,
				RefColumn: refColumn,
			})
		}
		if err := rows.Err(); err != nil {
			errs = append(errs, fmt.Errorf("db: mysql foreign keys: %w", err))
		}
		_ = rows.Close()
	}

	return builder.finish(), errors.Join(errs...)
}

func loadPostgresSchema(ctx context.Context, handle *sql.DB) ([]TableMeta, error) {
	builder := newSchemaBuilder()
	var errs []error

	rows, err := handle.QueryContext(ctx, `
		SELECT table_name, table_type, table_schema
		FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type IN ('BASE TABLE', 'VIEW')
		ORDER BY table_type, table_name`)
	if err != nil {
		return nil, fmt.Errorf("db: list postgres tables: %w", err)
	}
	for rows.Next() {
		var name, kind, schema string
		if err := rows.Scan(&name, &kind, &schema); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan postgres table: %w", err)
		}
		builder.add(name, tableKind(kind), schema)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: list postgres tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("db: close postgres tables: %w", err)
	}

	rows, err = handle.QueryContext(ctx, `
		SELECT table_name, column_name, data_type, is_nullable, ordinal_position
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		ORDER BY table_name, ordinal_position`)
	if err != nil {
		errs = append(errs, fmt.Errorf("db: postgres columns: %w", err))
	} else {
		for rows.Next() {
			var (
				table, name, dataType, nullable string
				ordinal                         int
			)
			if err := rows.Scan(&table, &name, &dataType, &nullable, &ordinal); err != nil {
				errs = append(errs, fmt.Errorf("db: scan postgres column: %w", err))
				break
			}
			builder.column(table, ColumnMeta{
				Name:     name,
				Type:     dataType,
				Nullable: strings.EqualFold(nullable, "YES"),
				Ordinal:  ordinal - 1,
			})
		}
		if err := rows.Err(); err != nil {
			errs = append(errs, fmt.Errorf("db: postgres columns: %w", err))
		}
		_ = rows.Close()
	}

	rows, err = handle.QueryContext(ctx, `
		SELECT tc.table_name, kcu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON kcu.constraint_name = tc.constraint_name
		 AND kcu.constraint_schema = tc.constraint_schema
		WHERE tc.constraint_type = 'PRIMARY KEY' AND tc.table_schema = current_schema()`)
	if err != nil {
		errs = append(errs, fmt.Errorf("db: postgres primary keys: %w", err))
	} else {
		for rows.Next() {
			var table, column string
			if err := rows.Scan(&table, &column); err != nil {
				errs = append(errs, fmt.Errorf("db: scan postgres primary key: %w", err))
				break
			}
			position, ok := builder.index[table]
			if !ok {
				continue
			}
			for i := range builder.tables[position].Columns {
				if builder.tables[position].Columns[i].Name == column {
					builder.tables[position].Columns[i].IsPrimaryKey = true
				}
			}
		}
		if err := rows.Err(); err != nil {
			errs = append(errs, fmt.Errorf("db: postgres primary keys: %w", err))
		}
		_ = rows.Close()
	}

	rows, err = handle.QueryContext(ctx, `
		SELECT cls.relname, att.attname, ref_cls.relname, ref_att.attname
		FROM pg_constraint c
		JOIN pg_class cls ON cls.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = cls.relnamespace
		JOIN LATERAL unnest(c.conkey, c.confkey) WITH ORDINALITY AS cols(attnum, refattnum, ord) ON true
		JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = cols.attnum
		JOIN pg_class ref_cls ON ref_cls.oid = c.confrelid
		JOIN pg_attribute ref_att ON ref_att.attrelid = c.confrelid AND ref_att.attnum = cols.refattnum
		WHERE c.contype = 'f' AND n.nspname = current_schema()
		ORDER BY cls.relname, cols.ord`)
	if err != nil {
		errs = append(errs, fmt.Errorf("db: postgres foreign keys: %w", err))
	} else {
		for rows.Next() {
			var table, column, refTable, refColumn string
			if err := rows.Scan(&table, &column, &refTable, &refColumn); err != nil {
				errs = append(errs, fmt.Errorf("db: scan postgres foreign key: %w", err))
				break
			}
			builder.foreignKey(table, ForeignKey{
				Column:    column,
				RefTable:  refTable,
				RefColumn: refColumn,
			})
		}
		if err := rows.Err(); err != nil {
			errs = append(errs, fmt.Errorf("db: postgres foreign keys: %w", err))
		}
		_ = rows.Close()
	}

	return builder.finish(), errors.Join(errs...)
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
