package export

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/atotto/clipboard"

	"github.com/messenger91/sql-tui/internal/db"
)

// Clipboard is the clipboard write surface, injectable for tests.
type Clipboard interface {
	WriteAll(text string) error
}

// SystemClipboard writes through github.com/atotto/clipboard. On Linux this
// needs xclip or xsel installed; the underlying error names the requirement.
type SystemClipboard struct{}

// WriteAll implements Clipboard.
func (SystemClipboard) WriteAll(text string) error {
	if err := clipboard.WriteAll(text); err != nil {
		return fmt.Errorf("export: write clipboard: %w", err)
	}
	return nil
}

// Serialize renders a result in the named format ("csv" or "json").
func Serialize(format string, result db.QueryResult) (string, error) {
	switch strings.ToLower(format) {
	case "csv":
		return CSVString(result)
	case "json":
		return JSONString(result)
	case "markdown", "md":
		return MarkdownString(result), nil
	default:
		return "", fmt.Errorf("export: unknown format %q", format)
	}
}

// WriteFile writes the result to path in the named format with 0644
// permissions and returns the absolute path.
func WriteFile(path, format string, result db.QueryResult) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("export: empty path")
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return "", fmt.Errorf("export: create %s: %w", path, err)
	}
	writeErr := writeTo(file, format, result)
	closeErr := file.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if closeErr != nil {
		return "", fmt.Errorf("export: close %s: %w", path, closeErr)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return path, nil
	}
	return absolute, nil
}

func writeTo(file *os.File, format string, result db.QueryResult) error {
	switch strings.ToLower(format) {
	case "csv":
		return WriteCSV(file, result)
	case "json":
		return WriteJSON(file, result)
	case "markdown", "md":
		return WriteMarkdown(file, result)
	default:
		return fmt.Errorf("export: unknown format %q", format)
	}
}
