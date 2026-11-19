package export

import (
	"fmt"
	"io"
	"strings"

	"github.com/messenger91/sql-tui/internal/db"
)

// MarkdownString renders a result as a GitHub-flavored Markdown table.
func MarkdownString(result db.QueryResult) string {
	var builder strings.Builder
	if err := WriteMarkdown(&builder, result); err != nil {
		return ""
	}
	return builder.String()
}

// WriteMarkdown writes a Markdown table with a header and separator row.
func WriteMarkdown(w io.Writer, result db.QueryResult) error {
	if len(result.Columns) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(w, markdownRow(result.Columns)); err != nil {
		return fmt.Errorf("export: write markdown header: %w", err)
	}
	separators := make([]string, len(result.Columns))
	for i := range separators {
		separators[i] = "---"
	}
	if _, err := fmt.Fprintln(w, markdownRow(separators)); err != nil {
		return fmt.Errorf("export: write markdown separator: %w", err)
	}
	for _, row := range result.Rows {
		values := make([]string, len(result.Columns))
		for i := range values {
			if i < len(row) {
				values[i] = FormatValue(row[i])
			}
		}
		if _, err := fmt.Fprintln(w, markdownRow(values)); err != nil {
			return fmt.Errorf("export: write markdown row: %w", err)
		}
	}
	return nil
}

func markdownRow(values []string) string {
	escaped := make([]string, len(values))
	for i, value := range values {
		escaped[i] = escapeMarkdown(value)
	}
	return "| " + strings.Join(escaped, " | ") + " |"
}

func escapeMarkdown(value string) string {
	return strings.ReplaceAll(value, "|", `\|`)
}
