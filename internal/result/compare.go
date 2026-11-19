package result

import (
	"cmp"
	"strings"
	"time"

	"github.com/messenger91/sql-tui/internal/export"
)

// compareForSort orders two driver values for the table sort. NULLs always
// sort last, regardless of the direction, so they never interleave with data.
func compareForSort(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return 1
	}
	if b == nil {
		return -1
	}
	if order, ok := compareValues(a, b); ok {
		return order
	}
	// Mixed or unsupported types: fall back to the rendered text so the order
	// matches what the table shows.
	left := strings.ToLower(export.FormatValue(a))
	right := strings.ToLower(export.FormatValue(b))
	return strings.Compare(left, right)
}

// compareValues orders two non-nil driver values of comparable types. The bool
// reports whether the values shared a comparison domain.
func compareValues(a, b any) (int, bool) {
	if left, ok := numeric(a); ok {
		if right, ok := numeric(b); ok {
			return left.compare(right), true
		}
	}
	if left, ok := a.(time.Time); ok {
		if right, ok := b.(time.Time); ok {
			switch {
			case left.Before(right):
				return -1, true
			case left.After(right):
				return 1, true
			default:
				return 0, true
			}
		}
	}
	if left, ok := a.(bool); ok {
		if right, ok := b.(bool); ok {
			switch {
			case !left && right:
				return -1, true
			case left && !right:
				return 1, true
			default:
				return 0, true
			}
		}
	}
	if left, ok := stringValue(a); ok {
		if right, ok := stringValue(b); ok {
			return strings.Compare(strings.ToLower(left), strings.ToLower(right)), true
		}
	}
	return 0, false
}

// number is a comparison-friendly view of any Go numeric driver value.
type number struct {
	unsigned bool
	float    bool
	i        int64
	u        uint64
	f        float64
}

func numeric(value any) (number, bool) {
	switch typed := value.(type) {
	case int:
		return number{i: int64(typed)}, true
	case int8:
		return number{i: int64(typed)}, true
	case int16:
		return number{i: int64(typed)}, true
	case int32:
		return number{i: int64(typed)}, true
	case int64:
		return number{i: typed}, true
	case uint:
		return number{unsigned: true, u: uint64(typed)}, true
	case uint8:
		return number{unsigned: true, u: uint64(typed)}, true
	case uint16:
		return number{unsigned: true, u: uint64(typed)}, true
	case uint32:
		return number{unsigned: true, u: uint64(typed)}, true
	case uint64:
		return number{unsigned: true, u: typed}, true
	case float32:
		return number{float: true, f: float64(typed)}, true
	case float64:
		return number{float: true, f: typed}, true
	default:
		return number{}, false
	}
}

func (n number) compare(other number) int {
	switch {
	case n.float || other.float:
		return cmpFloat(n.asFloat(), other.asFloat())
	case n.unsigned && other.unsigned:
		return cmp.Compare(n.u, other.u)
	case n.unsigned:
		if other.i < 0 {
			return 1
		}
		return cmp.Compare(n.u, uint64(other.i))
	case other.unsigned:
		if n.i < 0 {
			return -1
		}
		return cmp.Compare(uint64(n.i), other.u)
	default:
		return cmp.Compare(n.i, other.i)
	}
}

func (n number) asFloat() float64 {
	switch {
	case n.float:
		return n.f
	case n.unsigned:
		return float64(n.u)
	default:
		return float64(n.i)
	}
}

func cmpFloat(a, b float64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

// stringValue accepts the text types the drivers return, including MySQL's
// []byte for VARCHAR columns.
func stringValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, true
	case []byte:
		return string(typed), true
	default:
		return "", false
	}
}
