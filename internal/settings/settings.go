// Package settings persists user preferences (§6.4). Currently only the
// in-memory result cap is configurable.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// FileName is the settings file name under the config directory.
	FileName = "settings.json"
	// DefaultMaxRows is the default in-memory result cap.
	DefaultMaxRows = 10000
	// MinMaxRows and MaxMaxRows bound the configurable cap.
	MinMaxRows = 100
	MaxMaxRows = 1000000
)

// Settings holds user preferences.
type Settings struct {
	MaxRows int `json:"max_rows"`
}

// Default returns the default settings.
func Default() Settings {
	return Settings{MaxRows: DefaultMaxRows}
}

// Sanitized clamps MaxRows into the supported range and fills defaults.
func (s Settings) Sanitized() Settings {
	if s.MaxRows <= 0 {
		s.MaxRows = DefaultMaxRows
	}
	if s.MaxRows < MinMaxRows {
		s.MaxRows = MinMaxRows
	}
	if s.MaxRows > MaxMaxRows {
		s.MaxRows = MaxMaxRows
	}
	return s
}

// Load reads settings from path. A missing file returns the defaults.
func Load(path string) (Settings, error) {
	if path == "" {
		return Default(), nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), fmt.Errorf("settings: read %s: %w", path, err)
	}
	var loaded Settings
	if err := json.Unmarshal(data, &loaded); err != nil {
		return Default(), fmt.Errorf("settings: parse %s: %w", path, err)
	}
	return loaded.Sanitized(), nil
}

// Save writes settings atomically with 0600 permissions.
func Save(path string, settings Settings) error {
	if path == "" {
		return nil
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("settings: create %s: %w", dir, err)
	}

	temp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("settings: temp file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	encoder := json.NewEncoder(temp)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(settings.Sanitized()); err != nil {
		_ = temp.Close()
		return fmt.Errorf("settings: encode: %w", err)
	}
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("settings: chmod: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("settings: close: %w", err)
	}
	if err := os.Rename(tempName, path); err != nil {
		return fmt.Errorf("settings: rename: %w", err)
	}
	return nil
}
