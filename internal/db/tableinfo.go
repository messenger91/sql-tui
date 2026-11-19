package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// IndexInfo describes one index on a table.
type IndexInfo struct {
	Name    string
	Unique  bool
	Columns []string
	SQL     string
}

// ConstraintInfo describes one named constraint.
type ConstraintInfo struct {
	Name    string
	Type    string // PRIMARY KEY, UNIQUE, FOREIGN KEY, CHECK
	Columns []string
}

// TableInfo is the full introspection payload for one table or view (§16).
type TableInfo struct {
	Table       string
	Kind        TableKind
	Columns     []ColumnMeta
	ForeignKeys []ForeignKey
	Indexes     []IndexInfo
	Constraints []ConstraintInfo
	DDL         string
}

// GetTableInfo collects columns, foreign keys, indexes, constraints, and the
// DDL for a single table or view.
func GetTableInfo(ctx context.Context, handle *sql.DB, driver Driver, table string) (*TableInfo, error) {
	switch driver {
	case DriverSQLite:
		return sqliteTableInfo(ctx, handle, table)
	case DriverMySQL:
		return mysqlTableInfo(ctx, handle, table)
	case DriverPostgres:
		return postgresTableInfo(ctx, handle, table)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDriver, driver)
	}
}

func (t *TableInfo) addConstraint(name, typ string, columns ...string) {
	for i := range t.Constraints {
		if t.Constraints[i].Name == name && t.Constraints[i].Type == typ {
			t.Constraints[i].Columns = append(t.Constraints[i].Columns, columns...)
			return
		}
	}
	t.Constraints = append(t.Constraints, ConstraintInfo{
		Name:    name,
		Type:    typ,
		Columns: append([]string(nil), columns...),
	})
}

func (t *TableInfo) sortAll() {
	sort.Slice(t.Columns, func(i, j int) bool { return t.Columns[i].Ordinal < t.Columns[j].Ordinal })
	for i := range t.Indexes {
		t.Indexes[i].Columns = append([]string(nil), t.Indexes[i].Columns...)
	}
}

// sqliteTableInfo -----------------------------------------------------------

func sqliteTableInfo(ctx context.Context, handle *sql.DB, table string) (*TableInfo, error) {
	info := &TableInfo{Table: table}
	quoted := quoteDouble(table)

	columns, err := sqliteColumns(ctx, handle, table)
	if err != nil {
		return nil, err
	}
	info.Columns = columns
	for _, column := range columns {
		if column.IsPrimaryKey {
			info.addConstraint("PRIMARY", "PRIMARY KEY", column.Name)
		}
	}

	fks, err := sqliteForeignKeys(ctx, handle, table)
	if err != nil {
		return nil, err
	}
	info.ForeignKeys = fks
	for _, fk := range fks {
		info.addConstraint("fk_"+fk.Column, "FOREIGN KEY", fk.Column)
	}

	rows, err := handle.QueryContext(ctx, "PRAGMA index_list("+quoted+")")
	if err != nil {
		return nil, fmt.Errorf("db: index_list %q: %w", table, err)
	}
	type indexRow struct {
		name   string
		unique bool
	}
	var indexRows []indexRow
	for rows.Next() {
		var (
			seq     int
			name    string
			unique  int
			origin  sql.NullString
			partial sql.NullInt64
		)
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan index_list %q: %w", table, err)
		}
		indexRows = append(indexRows, indexRow{name: name, unique: unique == 1})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: index_list %q: %w", table, err)
	}
	_ = rows.Close()

	for _, index := range indexRows {
		idx := IndexInfo{Name: index.name, Unique: index.unique}
		infoRows, err := handle.QueryContext(ctx, "PRAGMA index_info("+quoteDouble(index.name)+")")
		if err != nil {
			return nil, fmt.Errorf("db: index_info %q: %w", index.name, err)
		}
		for infoRows.Next() {
			var (
				seqno int
				cid   sql.NullInt64
				name  sql.NullString
			)
			if err := infoRows.Scan(&seqno, &cid, &name); err == nil && name.Valid {
				idx.Columns = append(idx.Columns, name.String)
			}
		}
		_ = infoRows.Close()
		info.Indexes = append(info.Indexes, idx)
		if index.unique && len(idx.Columns) > 0 {
			info.addConstraint(index.name, "UNIQUE", idx.Columns...)
		}
	}

	ddl, err := sqliteDDL(ctx, handle, table)
	if err != nil {
		return nil, err
	}
	info.DDL = ddl
	info.Kind = ddlKind(ddl)
	info.sortAll()
	return info, nil
}

func sqliteDDL(ctx context.Context, handle *sql.DB, table string) (string, error) {
	var ddl sql.NullString
	err := handle.QueryRowContext(ctx,
		"SELECT sql FROM sqlite_master WHERE name = ?", table).Scan(&ddl)
	if err != nil {
		return "", fmt.Errorf("db: ddl %q: %w", table, err)
	}
	return ddl.String, nil
}

func ddlKind(ddl string) TableKind {
	if strings.Contains(strings.ToUpper(ddl), "CREATE VIEW") {
		return TableKindView
	}
	return TableKindTable
}

// mysqlTableInfo ------------------------------------------------------------

func mysqlTableInfo(ctx context.Context, handle *sql.DB, table string) (*TableInfo, error) {
	info := &TableInfo{Table: table}

	rows, err := handle.QueryContext(ctx, `
		SELECT column_name, column_type, is_nullable, ordinal_position, column_key
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
		ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("db: mysql columns %q: %w", table, err)
	}
	for rows.Next() {
		var (
			name, dataType, nullable, columnKey string
			ordinal                             int
		)
		if err := rows.Scan(&name, &dataType, &nullable, &ordinal, &columnKey); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan mysql column %q: %w", table, err)
		}
		info.Columns = append(info.Columns, ColumnMeta{
			Name:         name,
			Type:         dataType,
			Nullable:     strings.EqualFold(nullable, "YES"),
			IsPrimaryKey: strings.EqualFold(columnKey, "PRI"),
			Ordinal:      ordinal - 1,
		})
		if strings.EqualFold(columnKey, "PRI") {
			info.addConstraint("PRIMARY", "PRIMARY KEY", name)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: mysql columns %q: %w", table, err)
	}
	_ = rows.Close()

	rows, err = handle.QueryContext(ctx, `
		SELECT column_name, referenced_table_name, referenced_column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE() AND table_name = ?
		  AND referenced_table_name IS NOT NULL
		ORDER BY constraint_name, ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("db: mysql foreign keys %q: %w", table, err)
	}
	for rows.Next() {
		var column, refTable, refColumn string
		if err := rows.Scan(&column, &refTable, &refColumn); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan mysql foreign key %q: %w", table, err)
		}
		info.ForeignKeys = append(info.ForeignKeys, ForeignKey{Column: column, RefTable: refTable, RefColumn: refColumn})
		info.addConstraint("fk_"+column, "FOREIGN KEY", column)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: mysql foreign keys %q: %w", table, err)
	}
	_ = rows.Close()

	rows, err = handle.QueryContext(ctx, "SHOW INDEX FROM "+quoteBacktick(table))
	if err != nil {
		return nil, fmt.Errorf("db: show index %q: %w", table, err)
	}
	indexOrder := make(map[string]int)
	for rows.Next() {
		var (
			tableName  string
			nonUnique  int
			indexName  string
			seqInIndex int
			columnName string
		)
		if err := rows.Scan(&tableName, &nonUnique, &indexName, &seqInIndex, &columnName); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan show index %q: %w", table, err)
		}
		var idx *IndexInfo
		for i := range info.Indexes {
			if info.Indexes[i].Name == indexName {
				idx = &info.Indexes[i]
			}
		}
		if idx == nil {
			info.Indexes = append(info.Indexes, IndexInfo{Name: indexName, Unique: nonUnique == 0})
			idx = &info.Indexes[len(info.Indexes)-1]
			indexOrder[indexName] = len(info.Indexes) - 1
		}
		idx.Columns = append(idx.Columns, columnName)
		if idx.Unique && indexName != "PRIMARY" && len(idx.Columns) > 0 {
			info.addConstraint(indexName, "UNIQUE", idx.Columns...)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: show index %q: %w", table, err)
	}
	_ = rows.Close()
	sort.Slice(info.Indexes, func(i, j int) bool {
		return indexNameSortKey(info.Indexes[i].Name) < indexNameSortKey(info.Indexes[j].Name)
	})

	ddl, err := showCreate(ctx, handle, table)
	if err != nil {
		return nil, err
	}
	info.DDL = ddl
	info.Kind = ddlKind(ddl)
	info.sortAll()
	return info, nil
}

func indexNameSortKey(name string) int {
	if name == "PRIMARY" {
		return 0
	}
	return 1
}

// showCreate runs SHOW CREATE TABLE/VIEW and returns the DDL column.
func showCreate(ctx context.Context, handle *sql.DB, table string) (string, error) {
	rows, err := handle.QueryContext(ctx, "SHOW CREATE TABLE "+quoteBacktick(table))
	if err != nil {
		return "", fmt.Errorf("db: show create %q: %w", table, err)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return "", fmt.Errorf("db: show create columns %q: %w", table, err)
	}
	if !rows.Next() {
		return "", fmt.Errorf("db: show create %q: no rows", table)
	}
	values := make([]sql.NullString, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}
	if err := rows.Scan(pointers...); err != nil {
		return "", fmt.Errorf("db: scan show create %q: %w", table, err)
	}
	if len(values) < 2 {
		return "", fmt.Errorf("db: show create %q: unexpected shape", table)
	}
	return values[1].String, nil
}

// postgresTableInfo ---------------------------------------------------------

func postgresTableInfo(ctx context.Context, handle *sql.DB, table string) (*TableInfo, error) {
	info := &TableInfo{Table: table}

	rows, err := handle.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, ordinal_position
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ?
		ORDER BY ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("db: postgres columns %q: %w", table, err)
	}
	for rows.Next() {
		var (
			name, dataType, nullable string
			ordinal                  int
		)
		if err := rows.Scan(&name, &dataType, &nullable, &ordinal); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan postgres column %q: %w", table, err)
		}
		info.Columns = append(info.Columns, ColumnMeta{
			Name:     name,
			Type:     dataType,
			Nullable: strings.EqualFold(nullable, "YES"),
			Ordinal:  ordinal - 1,
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: postgres columns %q: %w", table, err)
	}
	_ = rows.Close()

	rows, err = handle.QueryContext(ctx, `
		SELECT tc.constraint_name, tc.constraint_type, kcu.column_name
		FROM information_schema.table_constraints tc
		LEFT JOIN information_schema.key_column_usage kcu
		  ON kcu.constraint_name = tc.constraint_name
		 AND kcu.constraint_schema = tc.constraint_schema
		WHERE tc.table_schema = current_schema() AND tc.table_name = ?
		ORDER BY tc.constraint_type, tc.constraint_name, kcu.ordinal_position`, table)
	if err != nil {
		return nil, fmt.Errorf("db: postgres constraints %q: %w", table, err)
	}
	for rows.Next() {
		var name, typ string
		column := sql.NullString{}
		if err := rows.Scan(&name, &typ, &column); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan postgres constraint %q: %w", table, err)
		}
		if column.Valid {
			info.addConstraint(name, typ, column.String)
		} else {
			info.addConstraint(name, typ)
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: postgres constraints %q: %w", table, err)
	}
	_ = rows.Close()

	rows, err = handle.QueryContext(ctx, `
		SELECT att.attname, ref_cls.relname, ref_att.attname
		FROM pg_constraint c
		JOIN pg_class cls ON cls.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = cls.relnamespace
		JOIN LATERAL unnest(c.conkey, c.confkey) WITH ORDINALITY AS cols(attnum, refattnum, ord) ON true
		JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = cols.attnum
		JOIN pg_class ref_cls ON ref_cls.oid = c.confrelid
		JOIN pg_attribute ref_att ON ref_att.attrelid = c.confrelid AND ref_att.attnum = cols.refattnum
		WHERE c.contype = 'f' AND n.nspname = current_schema() AND cls.relname = ?
		ORDER BY cols.ord`, table)
	if err != nil {
		return nil, fmt.Errorf("db: postgres foreign keys %q: %w", table, err)
	}
	for rows.Next() {
		var column, refTable, refColumn string
		if err := rows.Scan(&column, &refTable, &refColumn); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan postgres foreign key %q: %w", table, err)
		}
		info.ForeignKeys = append(info.ForeignKeys, ForeignKey{Column: column, RefTable: refTable, RefColumn: refColumn})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: postgres foreign keys %q: %w", table, err)
	}
	_ = rows.Close()

	rows, err = handle.QueryContext(ctx, `
		SELECT indexname, indexdef
		FROM pg_indexes
		WHERE schemaname = current_schema() AND tablename = ?
		ORDER BY indexname`, table)
	if err != nil {
		return nil, fmt.Errorf("db: postgres indexes %q: %w", table, err)
	}
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("db: scan postgres index %q: %w", table, err)
		}
		idx := IndexInfo{Name: name, SQL: definition, Unique: strings.Contains(definition, "UNIQUE")}
		idx.Columns = indexColumnsFromDef(definition)
		info.Indexes = append(info.Indexes, idx)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("db: postgres indexes %q: %w", table, err)
	}
	_ = rows.Close()

	var kind string
	err = handle.QueryRowContext(ctx, `
		SELECT table_type FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = ?`, table).Scan(&kind)
	if err != nil {
		return nil, fmt.Errorf("db: postgres table kind %q: %w", table, err)
	}
	info.Kind = tableKind(kind)

	if info.Kind == TableKindView {
		var definition string
		if err := handle.QueryRowContext(ctx, `
			SELECT definition FROM pg_views
			WHERE schemaname = current_schema() AND viewname = ?`, table).Scan(&definition); err != nil {
			return nil, fmt.Errorf("db: postgres view ddl %q: %w", table, err)
		}
		info.DDL = "CREATE VIEW " + quoteDouble(table) + " AS\n" + strings.TrimSpace(definition)
	} else {
		info.DDL = postgresReconstructDDL(info)
	}
	info.sortAll()
	return info, nil
}

// indexColumnsFromDef extracts the column list from a CREATE INDEX statement.
func indexColumnsFromDef(definition string) []string {
	open := strings.LastIndex(definition, "(")
	closeIdx := strings.LastIndex(definition, ")")
	if open < 0 || closeIdx <= open {
		return nil
	}
	parts := strings.Split(definition[open+1:closeIdx], ",")
	columns := make([]string, 0, len(parts))
	for _, part := range parts {
		column := strings.TrimSpace(part)
		if column != "" {
			columns = append(columns, column)
		}
	}
	return columns
}

// postgresReconstructDDL synthesizes a CREATE TABLE from introspected metadata.
func postgresReconstructDDL(info *TableInfo) string {
	var b strings.Builder
	b.WriteString("CREATE TABLE " + quoteDouble(info.Table) + " (\n")
	for i, column := range info.Columns {
		line := "  " + quoteDouble(column.Name) + " " + column.Type
		if !column.Nullable {
			line += " NOT NULL"
		}
		if i < len(info.Columns)-1 || len(info.Constraints) > 0 {
			line += ","
		}
		b.WriteString(line + "\n")
	}
	for i, constraint := range info.Constraints {
		if len(constraint.Columns) == 0 {
			continue
		}
		quoted := make([]string, 0, len(constraint.Columns))
		for _, column := range constraint.Columns {
			quoted = append(quoted, quoteDouble(column))
		}
		line := "  " + constraint.Type + " (" + strings.Join(quoted, ", ") + ")"
		if constraint.Type == "FOREIGN KEY" {
			for _, fk := range info.ForeignKeys {
				if fk.Column == constraint.Columns[0] {
					line += " REFERENCES " + quoteDouble(fk.RefTable) + " (" + quoteDouble(fk.RefColumn) + ")"
					break
				}
			}
		}
		if i < len(info.Constraints)-1 {
			line += ","
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(");")
	return b.String()
}
