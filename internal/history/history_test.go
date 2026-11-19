package history

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestAppendDedupesAndCaps(t *testing.T) {
	var queries []string
	queries = Append(queries, "SELECT 1")
	queries = Append(queries, "SELECT 1")
	queries = Append(queries, "  SELECT 2  ")
	queries = Append(queries, "")
	if !reflect.DeepEqual(queries, []string{"SELECT 1", "SELECT 2"}) {
		t.Fatalf("queries = %#v", queries)
	}

	for i := 0; i < MaxEntries+10; i++ {
		queries = Append(queries, "SELECT "+string(rune('a'+i%26)))
	}
	if len(queries) != MaxEntries {
		t.Fatalf("len = %d", len(queries))
	}
}

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "history.json")
	store := NewStore(path)
	want := []string{"SELECT 1", "SELECT 2"}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %#v", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("mode = %o", mode)
	}
}

func TestStoreMissingAndNoPath(t *testing.T) {
	queries, err := NewStore(filepath.Join(t.TempDir(), "nope.json")).Load()
	if err != nil || len(queries) != 0 {
		t.Fatalf("missing: %v, %v", queries, err)
	}
	if err := NewStore("").Save([]string{"x"}); err != nil {
		t.Fatalf("no-path save: %v", err)
	}
}
