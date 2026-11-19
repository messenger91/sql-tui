package result

import (
	"errors"
	"fmt"
	"strings"

	"github.com/messenger91/sql-tui/internal/db"
)

// ChangeKind identifies the DML operation.
type ChangeKind string

const (
	ChangeUpdate ChangeKind = "update"
	ChangeInsert ChangeKind = "insert"
	ChangeDelete ChangeKind = "delete"
)

// ColumnValue is one named value.
type ColumnValue struct {
	Column string
	Value  any
}

// Change is a row-level pending operation ready for DML generation.
type Change struct {
	Kind   ChangeKind
	Table  string
	Key    []ColumnValue // primary-key predicates
	Set    []ColumnValue // updated/inserted columns
	Values []ColumnValue // insert values
}

// Statement is a parameterised SQL statement.
type Statement struct {
	SQL  string
	Args []any
}

// BuildStatements generates placeholder-only DML for the pending changes.
// Values are never interpolated into SQL.
func BuildStatements(driver db.Driver, changes []Change) ([]Statement, error) {
	if !driver.Valid() {
		return nil, fmt.Errorf("result: unsupported driver %q", driver)
	}
	dialect := driver.Dialect()
	statements := make([]Statement, 0, len(changes))

	for _, change := range changes {
		statement, err := buildStatement(driver, dialect, change)
		if err != nil {
			return nil, err
		}
		statements = append(statements, statement)
	}
	return statements, nil
}

func buildStatement(driver db.Driver, dialect db.Dialect, change Change) (Statement, error) {
	table := dialect.QuoteIdent(change.Table)
	switch change.Kind {
	case ChangeUpdate:
		if len(change.Set) == 0 {
			return Statement{}, errors.New("result: update without columns")
		}
		args := make([]any, 0, len(change.Set)+len(change.Key))
		var setParts []string
		for _, column := range change.Set {
			args = append(args, marshalValue(driver, column.Value))
			setParts = append(setParts, dialect.QuoteIdent(column.Column)+" = "+dialect.Placeholder(len(args)))
		}
		where, whereArgs := predicate(dialect, driver, change.Key, len(args))
		args = append(args, whereArgs...)
		sql := "UPDATE " + table + " SET " + strings.Join(setParts, ", ") + " WHERE " + where
		return Statement{SQL: sql, Args: args}, nil

	case ChangeDelete:
		if len(change.Key) == 0 {
			return Statement{}, errors.New("result: delete without a primary key")
		}
		where, args := predicate(dialect, driver, change.Key, 0)
		return Statement{SQL: "DELETE FROM " + table + " WHERE " + where, Args: args}, nil

	case ChangeInsert:
		values := change.Values
		if len(values) == 0 {
			values = change.Set
		}
		if len(values) == 0 {
			return Statement{}, errors.New("result: insert without columns")
		}
		args := make([]any, 0, len(values))
		columns := make([]string, 0, len(values))
		placeholders := make([]string, 0, len(values))
		for _, column := range values {
			args = append(args, marshalValue(driver, column.Value))
			columns = append(columns, dialect.QuoteIdent(column.Column))
			placeholders = append(placeholders, dialect.Placeholder(len(args)))
		}
		sql := "INSERT INTO " + table + " (" + strings.Join(columns, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
		return Statement{SQL: sql, Args: args}, nil

	default:
		return Statement{}, fmt.Errorf("result: unknown change kind %q", change.Kind)
	}
}

func predicate(dialect db.Dialect, driver db.Driver, key []ColumnValue, placeholderOffset int) (string, []any) {
	parts := make([]string, 0, len(key))
	args := make([]any, 0, len(key))
	for _, column := range key {
		args = append(args, marshalValue(driver, column.Value))
		parts = append(parts, dialect.QuoteIdent(column.Column)+" = "+dialect.Placeholder(placeholderOffset+len(args)))
	}
	return strings.Join(parts, " AND "), args
}

// marshalValue adapts values the driver cannot bind directly. Time values and
// byte slices are passed through to database/sql.
func marshalValue(driver db.Driver, value any) any {
	if boolean, ok := value.(bool); ok && driver == db.DriverMySQL {
		if boolean {
			return int64(1)
		}
		return int64(0)
	}
	return value
}
