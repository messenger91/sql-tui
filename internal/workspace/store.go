package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/messenger91/sql-tui/internal/db"
)

// Store persists connections as JSON. The zero path disables persistence,
// which keeps tests and read-only environments safe.
type Store struct {
	path string
}

// NewStore returns a store backed by path (normally connections.json).
func NewStore(path string) *Store { return &Store{path: path} }

// Path returns the file backing the store.
func (s *Store) Path() string { return s.path }

type fileFormat struct {
	Connections []storedConnection `json:"connections"`
}

// storedConnection mirrors db.Connection but uses a pointer for ReadOnly so a
// missing field cannot silently turn a connection writable.
type storedConnection struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Driver   db.Driver         `json:"driver"`
	DSN      string            `json:"dsn"`
	Options  map[string]string `json:"options,omitempty"`
	ReadOnly *bool             `json:"readOnly,omitempty"`
}

// Load reads the connections file. A missing file is not an error.
func (s *Store) Load() ([]db.Connection, error) {
	if s.path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", s.path, err)
	}

	var file fileFormat
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("workspace: parse %s: %w", s.path, err)
	}

	connections := make([]db.Connection, 0, len(file.Connections))
	for _, stored := range file.Connections {
		readOnly := true
		if stored.ReadOnly != nil {
			readOnly = *stored.ReadOnly
		}
		connections = append(connections, db.Connection{
			ID:       stored.ID,
			Name:     stored.Name,
			Driver:   stored.Driver,
			DSN:      stored.DSN,
			Options:  stored.Options,
			ReadOnly: readOnly,
		})
	}
	return connections, nil
}

// Save writes the connections atomically with 0600 permissions.
func (s *Store) Save(connections []db.Connection) error {
	if s.path == "" {
		return errors.New("workspace: no connections file configured")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("workspace: create %s: %w", dir, err)
	}

	file := fileFormat{Connections: make([]storedConnection, 0, len(connections))}
	for _, conn := range connections {
		readOnly := conn.ReadOnly
		file.Connections = append(file.Connections, storedConnection{
			ID:       conn.ID,
			Name:     conn.Name,
			Driver:   conn.Driver,
			DSN:      conn.DSN,
			Options:  conn.Options,
			ReadOnly: &readOnly,
		})
	}

	temp, err := os.CreateTemp(dir, ".connections-*.tmp")
	if err != nil {
		return fmt.Errorf("workspace: temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(file); err != nil {
		_ = temp.Close()
		return fmt.Errorf("workspace: encode: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("workspace: chmod: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("workspace: close: %w", err)
	}
	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("workspace: rename: %w", err)
	}
	return nil
}
