package session

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/result"
)

// RowOperation marks a whole-row pending change.
type RowOperation string

const (
	RowNone      RowOperation = ""
	RowDelete    RowOperation = "delete"
	RowDuplicate RowOperation = "duplicate"
)

// CellEdit is one pending cell change. Original* capture the value as loaded
// so §12.4 can detect out-of-band changes.
type CellEdit struct {
	Value        any
	IsNull       bool
	Original     any
	OriginalNull bool
}

// Pending holds uncommitted edits for one session, keyed by result row index.
type Pending struct {
	Cells     map[int]map[string]CellEdit
	Ops       map[int]RowOperation
	Snapshots map[int][]result.ColumnValue
}

// NewPending returns an empty pending-edit set.
func NewPending() *Pending {
	return &Pending{
		Cells:     make(map[int]map[string]CellEdit),
		Ops:       make(map[int]RowOperation),
		Snapshots: make(map[int][]result.ColumnValue),
	}
}

// EnsureSnapshot records the row as loaded before its first change, for
// conflict detection during apply.
func (p *Pending) EnsureSnapshot(row int, values []result.ColumnValue) {
	if _, ok := p.Snapshots[row]; ok {
		return
	}
	copied := append([]result.ColumnValue(nil), values...)
	p.Snapshots[row] = copied
}

// SetCell records a pending edit.
func (p *Pending) SetCell(row int, column string, edit CellEdit) {
	if p.Cells[row] == nil {
		p.Cells[row] = make(map[string]CellEdit)
	}
	p.Cells[row][column] = edit
}

// Cell returns a pending edit for a cell.
func (p *Pending) Cell(row int, column string) (CellEdit, bool) {
	rowEdits, ok := p.Cells[row]
	if !ok {
		return CellEdit{}, false
	}
	edit, ok := rowEdits[column]
	return edit, ok
}

// SetRowOp records a row-level operation.
func (p *Pending) SetRowOp(row int, op RowOperation) {
	if op == RowNone {
		delete(p.Ops, row)
		return
	}
	p.Ops[row] = op
}

// RowOp returns the row-level operation for a row.
func (p *Pending) RowOp(row int) RowOperation { return p.Ops[row] }

// HasRow reports whether a row has any pending change.
func (p *Pending) HasRow(row int) bool {
	if _, ok := p.Ops[row]; ok {
		return true
	}
	_, ok := p.Cells[row]
	return ok
}

// RowSet returns the rows with pending changes (for table markers).
func (p *Pending) RowSet() map[int]bool {
	rows := make(map[int]bool, len(p.Cells)+len(p.Ops))
	for row := range p.Cells {
		rows[row] = true
	}
	for row := range p.Ops {
		rows[row] = true
	}
	return rows
}

// Count returns the number of pending changes (cells plus row operations).
func (p *Pending) Count() int {
	count := 0
	for _, edits := range p.Cells {
		count += len(edits)
	}
	count += len(p.Ops)
	return count
}

// Empty reports whether there are no pending changes.
func (p *Pending) Empty() bool { return p.Count() == 0 }

// RevertRow removes all pending changes for one row.
func (p *Pending) RevertRow(row int) {
	delete(p.Cells, row)
	delete(p.Ops, row)
	delete(p.Snapshots, row)
}

// RevertAll clears every pending change.
func (p *Pending) RevertAll() {
	p.Cells = make(map[int]map[string]CellEdit)
	p.Ops = make(map[int]RowOperation)
	p.Snapshots = make(map[int][]result.ColumnValue)
}

// ParseCellValue validates and converts text according to the column type.
// NULL is handled by the caller (ctrl+n); empty text is invalid for non-text
// columns.
func ParseCellValue(column db.ColumnMeta, text string) (any, error) {
	kind := strings.ToLower(column.Type)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" && !isTextType(kind) {
		return nil, fmt.Errorf("value required for %s", column.Type)
	}

	switch {
	case isBoolType(kind):
		switch strings.ToLower(trimmed) {
		case "t", "true", "1", "yes", "y":
			return true, nil
		case "f", "false", "0", "no", "n":
			return false, nil
		default:
			return nil, fmt.Errorf("expected a boolean, got %q", text)
		}
	case isIntegerType(kind):
		value, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("expected an integer, got %q", text)
		}
		return value, nil
	case isFloatType(kind):
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("expected a number, got %q", text)
		}
		return value, nil
	case isTimeType(kind):
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
			"2006-01-02 15:04",
			"2006-01-02",
		} {
			if parsed, err := time.Parse(layout, trimmed); err == nil {
				return parsed, nil
			}
		}
		return nil, fmt.Errorf("expected a timestamp, got %q", text)
	default:
		return text, nil
	}
}

func isTextType(kind string) bool {
	return strings.Contains(kind, "char") || strings.Contains(kind, "text") ||
		strings.Contains(kind, "clob") || strings.Contains(kind, "json") ||
		strings.Contains(kind, "uuid") || kind == ""
}

func isBoolType(kind string) bool {
	return kind == "bool" || kind == "boolean" || strings.HasPrefix(kind, "tinyint(1)")
}

func isIntegerType(kind string) bool {
	return strings.Contains(kind, "int")
}

func isFloatType(kind string) bool {
	for _, marker := range []string{"real", "float", "double", "numeric", "decimal", "money"} {
		if strings.Contains(kind, marker) {
			return true
		}
	}
	return false
}

func isTimeType(kind string) bool {
	return strings.Contains(kind, "timestamp") || strings.Contains(kind, "datetime") ||
		kind == "date" || kind == "time" || strings.HasPrefix(kind, "time ")
}
