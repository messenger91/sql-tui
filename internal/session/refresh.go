package session

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/result"
)

// RowMutationKind identifies an in-place result row update after apply.
type RowMutationKind string

const (
	RowReplace RowMutationKind = "replace"
	RowRemove  RowMutationKind = "remove"
	RowAppend  RowMutationKind = "append"
)

// RowMutation is a UI-safe instruction to update the result view after a
// successful apply, so the view refreshes in place without re-running the
// original query.
type RowMutation struct {
	Kind  RowMutationKind
	Index int   // result row index for replace/remove
	Raw   []any // re-read values ordered by the result columns
}

// RefreshMutations re-reads the affected rows by primary key after a
// successful apply and returns view mutations. The result snapshot carries the
// columns and rows the changes were applied to. Any failure means the caller
// should fall back to re-running the original query.
func (p *ApplyPlan) RefreshMutations(ctx context.Context, handle *sql.DB, snapshot db.QueryResult) ([]RowMutation, error) {
	if p.Table == "" || len(p.PrimaryKey) == 0 {
		return nil, errors.New("refresh: missing table or primary key")
	}
	dialect := p.Driver.Dialect()

	for _, pk := range p.PrimaryKey {
		missing := true
		for _, column := range snapshot.Columns {
			if strings.EqualFold(column, pk) {
				missing = false
				break
			}
		}
		if missing {
			return nil, fmt.Errorf("refresh: pk column %q missing from result", pk)
		}
	}

	columnPosition := func(name string) int {
		for i, column := range snapshot.Columns {
			if strings.EqualFold(column, name) {
				return i
			}
		}
		return -1
	}

	findRowByKey := func(key []result.ColumnValue) (int, bool) {
		for i, row := range snapshot.Rows {
			match := true
			for _, kv := range key {
				position := columnPosition(kv.Column)
				if position < 0 || position >= len(row) || !valuesEqual(kv.Value, row[position]) {
					match = false
					break
				}
			}
			if match {
				return i, true
			}
		}
		return -1, false
	}

	quotedColumns := make([]string, 0, len(snapshot.Columns))
	for _, column := range snapshot.Columns {
		quotedColumns = append(quotedColumns, dialect.QuoteIdent(column))
	}
	selectSQL := "SELECT " + strings.Join(quotedColumns, ", ") + " FROM " + dialect.QuoteIdent(p.Table)
	readRow := func(where string, args []any) ([]any, error) {
		row := make([]any, len(snapshot.Columns))
		pointers := make([]any, len(row))
		for i := range row {
			pointers[i] = &row[i]
		}
		if err := handle.QueryRowContext(ctx, selectSQL+" WHERE "+where, args...).Scan(pointers...); err != nil {
			return nil, fmt.Errorf("refresh: re-read row: %w", err)
		}
		return row, nil
	}

	pkValuesFrom := func(values []result.ColumnValue) ([]any, error) {
		out := make([]any, 0, len(p.PrimaryKey))
		for _, pk := range p.PrimaryKey {
			for _, cv := range values {
				if strings.EqualFold(cv.Column, pk) {
					out = append(out, cv.Value)
					break
				}
			}
		}
		if len(out) != len(p.PrimaryKey) {
			return nil, errors.New("refresh: primary key values missing")
		}
		return out, nil
	}

	pkWhere := func() string {
		parts := make([]string, 0, len(p.PrimaryKey))
		for i, pk := range p.PrimaryKey {
			parts = append(parts, dialect.QuoteIdent(pk)+" = "+dialect.Placeholder(i+1))
		}
		return strings.Join(parts, " AND ")
	}()

	var mutations []RowMutation
	for _, change := range p.Changes {
		switch change.Kind {
		case result.ChangeUpdate:
			index, ok := findRowByKey(change.Key)
			if !ok {
				return nil, errors.New("refresh: updated row not found in the result")
			}
			// The primary key itself may have been edited.
			key := make([]result.ColumnValue, len(change.Key))
			copy(key, change.Key)
			for _, set := range change.Set {
				for i := range key {
					if strings.EqualFold(key[i].Column, set.Column) {
						key[i].Value = set.Value
					}
				}
			}
			pkValues, err := pkValuesFrom(key)
			if err != nil {
				return nil, err
			}
			row, err := readRow(pkWhere, pkValues)
			if err != nil {
				return nil, err
			}
			mutations = append(mutations, RowMutation{Kind: RowReplace, Index: index, Raw: row})

		case result.ChangeDelete:
			index, ok := findRowByKey(change.Key)
			if !ok {
				return nil, errors.New("refresh: deleted row not found in the result")
			}
			mutations = append(mutations, RowMutation{Kind: RowRemove, Index: index})

		case result.ChangeInsert:
			pkValues, err := pkValuesFrom(change.Values)
			if err != nil {
				return nil, err
			}
			row, err := readRow(pkWhere, pkValues)
			if err != nil {
				return nil, err
			}
			mutations = append(mutations, RowMutation{Kind: RowAppend, Raw: row})

		default:
			return nil, fmt.Errorf("refresh: unknown change kind %q", change.Kind)
		}
	}

	return orderMutations(mutations), nil
}

// orderMutations sorts mutations so they can be replayed safely: replaces use
// pre-removal indexes, removes go last-first, appends come after.
func orderMutations(mutations []RowMutation) []RowMutation {
	var replaces, removes, appends []RowMutation
	for _, mutation := range mutations {
		switch mutation.Kind {
		case RowReplace:
			replaces = append(replaces, mutation)
		case RowRemove:
			removes = append(removes, mutation)
		default:
			appends = append(appends, mutation)
		}
	}
	sort.Slice(removes, func(i, j int) bool { return removes[i].Index > removes[j].Index })
	out := make([]RowMutation, 0, len(mutations))
	out = append(out, replaces...)
	out = append(out, removes...)
	out = append(out, appends...)
	return out
}
