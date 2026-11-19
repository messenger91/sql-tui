// Package history stores persistent query history shared by all sessions.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MaxEntries caps the on-disk history.
const MaxEntries = 200

// Store persists executed SQL strings as JSON.
type Store struct {
	path string
}

// NewStore returns a store backed by path (normally history.json).
func NewStore(path string) *Store { return &Store{path: path} }

type fileFormat struct {
	Queries []string `json:"queries"`
}

// Load reads the history file. A missing file is not an error.
func (s *Store) Load() ([]string, error) {
	if s.path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("history: read %s: %w", s.path, err)
	}
	var file fileFormat
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("history: parse %s: %w", s.path, err)
	}
	return file.Queries, nil
}

// Save writes the history atomically with 0600 permissions.
func (s *Store) Save(queries []string) error {
	if s.path == "" {
		return nil
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("history: create %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".history-*.tmp")
	if err != nil {
		return fmt.Errorf("history: temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(fileFormat{Queries: queries}); err != nil {
		_ = temp.Close()
		return fmt.Errorf("history: encode: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("history: chmod: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("history: close: %w", err)
	}
	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("history: rename: %w", err)
	}
	return nil
}

// Append adds sql to queries (chronological order), collapsing consecutive
// duplicates and enforcing MaxEntries.
func Append(queries []string, sql string) []string {
	sql = strings.TrimSpace(sql)
	if sql == "" {
		return queries
	}
	if len(queries) > 0 && queries[len(queries)-1] == sql {
		return queries
	}
	queries = append(queries, sql)
	if len(queries) > MaxEntries {
		queries = queries[len(queries)-MaxEntries:]
	}
	return queries
}
