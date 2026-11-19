package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAndSanitize(t *testing.T) {
	if got := (Settings{}).Sanitized().MaxRows; got != 10000 {
		t.Fatalf("zero sanitized = %d", got)
	}
	if got := (Settings{MaxRows: 50}).Sanitized().MaxRows; got != 100 {
		t.Fatalf("low clamp = %d", got)
	}
	if got := (Settings{MaxRows: 5000000}).Sanitized().MaxRows; got != 1000000 {
		t.Fatalf("high clamp = %d", got)
	}
	if got := (Settings{MaxRows: 250000}).Sanitized().MaxRows; got != 250000 {
		t.Fatalf("passthrough = %d", got)
	}
}

func TestLoadMissingAndNoPath(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file: %v", err)
	}
	if got.MaxRows != 10000 {
		t.Fatalf("missing file max rows = %d", got.MaxRows)
	}
	if got, err := Load(""); err != nil || got.MaxRows != 10000 {
		t.Fatalf("empty path: %v, %d", err, got.MaxRows)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	if err := Save(path, Settings{MaxRows: 250000}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o", mode)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.MaxRows != 250000 {
		t.Fatalf("MaxRows = %d", got.MaxRows)
	}
}

func TestLoadInvalidFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := Load(path)
	if err == nil {
		t.Fatal("expected a parse error")
	}
	if got.MaxRows != 10000 {
		t.Fatalf("invalid file max rows = %d", got.MaxRows)
	}
}
