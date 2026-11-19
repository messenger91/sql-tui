package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/messenger91/sql-tui/internal/db"
)

// WriteCSV writes a result as RFC 4180 CSV: header row followed by data rows.
// Values use the same formatting as the table view (NULL is written as the
// literal NULL).
func WriteCSV(w io.Writer, result db.QueryResult) error {
	writer := csv.NewWriter(w)
	if err := writer.Write(result.Columns); err != nil {
		return fmt.Errorf("export: write header: %w", err)
	}
	for _, row := range result.Rows {
		record := make([]string, len(result.Columns))
		for i := range record {
			if i < len(row) {
				record[i] = FormatValue(row[i])
			}
		}
		if err := writer.Write(record); err != nil {
			return fmt.Errorf("export: write row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("export: flush csv: %w", err)
	}
	return nil
}

// CSVString renders a result as CSV text.
func CSVString(result db.QueryResult) (string, error) {
	var builder strings.Builder
	if err := WriteCSV(&builder, result); err != nil {
		return "", err
	}
	return builder.String(), nil
}
