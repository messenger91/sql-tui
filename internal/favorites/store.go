package favorites

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Favorite is a named SQL script.
type Favorite struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	SQL          string    `json:"sql"`
	ConnectionID string    `json:"connectionId,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

// Store persists favorites as JSON. The zero path disables persistence.
type Store struct {
	path string
}

// NewStore returns a store backed by path (normally favorites.json).
func NewStore(path string) *Store { return &Store{path: path} }

type fileFormat struct {
	Favorites []Favorite `json:"favorites"`
}

// Load reads the favorites file. A missing file is not an error.
func (s *Store) Load() ([]Favorite, error) {
	if s.path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("favorites: read %s: %w", s.path, err)
	}
	var file fileFormat
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("favorites: parse %s: %w", s.path, err)
	}
	return file.Favorites, nil
}

// Save writes favorites atomically with 0600 permissions.
func (s *Store) Save(favorites []Favorite) error {
	if s.path == "" {
		return errors.New("favorites: no favorites file configured")
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("favorites: create %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".favorites-*.tmp")
	if err != nil {
		return fmt.Errorf("favorites: temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(fileFormat{Favorites: favorites}); err != nil {
		_ = temp.Close()
		return fmt.Errorf("favorites: encode: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("favorites: chmod: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("favorites: close: %w", err)
	}
	if err := os.Rename(tempName, s.path); err != nil {
		return fmt.Errorf("favorites: rename: %w", err)
	}
	return nil
}
