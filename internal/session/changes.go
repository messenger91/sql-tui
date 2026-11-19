package session

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/messenger91/sql-tui/internal/result"
)

// Changes converts pending edits into row-level DML changes. The result must
// still be editable and loaded.
func (s *Session) Changes() ([]result.Change, error) {
	if s.Pending == nil || s.Pending.Empty() {
		return nil, nil
	}
	if !s.Editability.Editable {
		return nil, fmt.Errorf("session: result is not editable")
	}
	data, ok := s.Results.Result()
	if !ok {
		return nil, fmt.Errorf("session: no result loaded")
	}

	index := make(map[string]int, len(data.Columns))
	for i, name := range data.Columns {
		index[strings.ToLower(name)] = i
	}

	valueAt := func(row int, column string) any {
		position, ok := index[strings.ToLower(column)]
		if !ok || row < 0 || row >= len(data.Rows) || position >= len(data.Rows[row]) {
			return nil
		}
		return data.Rows[row][position]
	}
	primaryKey := func(row int) []result.ColumnValue {
		key := make([]result.ColumnValue, 0, len(s.Editability.PrimaryKey))
		for _, column := range s.Editability.PrimaryKey {
			key = append(key, result.ColumnValue{Column: column, Value: valueAt(row, column)})
		}
		return key
	}

	var changes []result.Change
	for row := 0; row < len(data.Rows); row++ {
		operation := s.Pending.RowOp(row)
		edits := s.Pending.Cells[row]
		if operation == RowNone && len(edits) == 0 {
			continue
		}

		switch operation {
		case RowDelete:
			changes = append(changes, result.Change{
				Kind:  result.ChangeDelete,
				Table: s.Editability.Table,
				Row:   row,
				Key:   primaryKey(row),
			})

		case RowDuplicate:
			values := make([]result.ColumnValue, 0, len(data.Columns))
			for _, column := range data.Columns {
				value := valueAt(row, column)
				if edit, ok := edits[column]; ok {
					value = edit.Value
					if edit.IsNull {
						value = nil
					}
				}
				values = append(values, result.ColumnValue{Column: column, Value: value})
			}
			changes = append(changes, result.Change{
				Kind:   result.ChangeInsert,
				Table:  s.Editability.Table,
				Row:    row,
				Values: values,
			})

		default:
			set := make([]result.ColumnValue, 0, len(edits))
			for column, edit := range edits {
				if !edit.IsNull && !edit.OriginalNull && reflect.DeepEqual(edit.Value, edit.Original) {
					continue
				}
				if edit.IsNull && edit.OriginalNull {
					continue
				}
				set = append(set, result.ColumnValue{Column: column, Value: edit.Value})
			}
			if len(set) == 0 {
				continue
			}
			changes = append(changes, result.Change{
				Kind:  result.ChangeUpdate,
				Table: s.Editability.Table,
				Row:   row,
				Key:   primaryKey(row),
				Set:   set,
			})
		}
	}
	return changes, nil
}
