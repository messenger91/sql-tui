package favorites

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "favorites.json")
	store := NewStore(path)
	want := []Favorite{
		{ID: "a", Name: "one", SQL: "SELECT 1", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{ID: "b", Name: "two", SQL: "SELECT 2", ConnectionID: "c1", CreatedAt: time.Now().UTC().Truncate(time.Second)},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("favorites = %+v", got)
	}
	for i := range want {
		got[i].CreatedAt = got[i].CreatedAt.UTC()
		want[i].CreatedAt = want[i].CreatedAt.UTC()
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestStorePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.json")
	if err := NewStore(path).Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("permissions = %o", mode)
	}
}

func TestStoreMissingAndInvalid(t *testing.T) {
	missing, err := NewStore(filepath.Join(t.TempDir(), "nope.json")).Load()
	if err != nil || len(missing) != 0 {
		t.Fatalf("missing: %v, %v", missing, err)
	}
	path := filepath.Join(t.TempDir(), "favorites.json")
	if err := os.WriteFile(path, []byte("{bad"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := NewStore(path).Load(); err == nil {
		t.Fatal("expected parse error")
	}
}
