// Package export owns value formatting and result serialization shared by the
// on-screen views and the export dialog (§8).
package export

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/messenger91/sql-tui/internal/db"
)

// FormatValue converts a driver value into a single-line display/export cell.
func FormatValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "NULL"
	case string:
		return sanitize(v)
	case []byte:
		if isPrintableUTF8(v) {
			return sanitize(string(v))
		}
		return "0x" + hex.EncodeToString(v)
	case time.Time:
		return v.Format(time.RFC3339)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return sanitize(fmt.Sprint(v))
	}
}

// ValueRows converts a raw result into display rows.
func ValueRows(result db.QueryResult) [][]string {
	rows := make([][]string, 0, len(result.Rows))
	for _, raw := range result.Rows {
		row := make([]string, len(result.Columns))
		for i := range result.Columns {
			if i < len(raw) {
				row[i] = FormatValue(raw[i])
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// FormatElapsed renders a duration, avoiding "0s" for quick queries.
func FormatElapsed(elapsed time.Duration) string {
	if elapsed < time.Millisecond {
		return "<1ms"
	}
	return elapsed.Round(time.Millisecond).String()
}

func isPrintableUTF8(data []byte) bool {
	if !utf8.Valid(data) {
		return false
	}
	for _, r := range string(data) {
		if r < 0x20 && r != '\t' && r != '\n' && r != '\r' {
			return false
		}
	}
	return true
}

// sanitize keeps table rows on one line.
func sanitize(value string) string {
	if !strings.ContainsAny(value, "\n\r\t") {
		return value
	}
	return strings.NewReplacer("\n", "\\n", "\r", "\\r", "\t", "\\t").Replace(value)
}
