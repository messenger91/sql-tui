package export

import (
	"fmt"
	"strings"

	"github.com/messenger91/sql-tui/internal/db"
)

// ResultStats renders "N rows · speed · elapsed" for a completed result.
func ResultStats(result db.QueryResult) string {
	rows := "rows"
	if result.RowCount == 1 {
		rows = "row"
	}
	parts := []string{fmt.Sprintf("%d %s", result.RowCount, rows)}
	if result.Fetched > 0 && result.Elapsed > 0 {
		parts = append(parts, fmt.Sprintf("%.0f rows/s", float64(result.Fetched)/result.Elapsed.Seconds()))
	}
	parts = append(parts, FormatElapsed(result.Elapsed))
	if result.Fetched > result.RowCount && result.Fetched > 0 {
		parts = append(parts, fmt.Sprintf("fetched %s", formatCount(result.Fetched)))
	}
	return strings.Join(parts, " · ")
}

func formatCount(count int) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	return fmt.Sprintf("%d,%03d", count/1000, count%1000)
}
