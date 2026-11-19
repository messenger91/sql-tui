package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
)

func TestStoreRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "connections.json")
	store := NewStore(path)

	want := []db.Connection{
		{ID: "a", Name: "Alpha", Driver: db.DriverSQLite, DSN: ":memory:", ReadOnly: true},
		{ID: "b", Name: "Beta", Driver: db.DriverPostgres, DSN: "postgres://u@h/db", ReadOnly: false},
	}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestStorePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	store := NewStore(path)
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("permissions = %o, want 600", mode)
	}
}

func TestStoreMissingFileIsEmpty(t *testing.T) {
	connections, err := NewStore(filepath.Join(t.TempDir(), "nope.json")).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(connections) != 0 {
		t.Fatalf("connections = %+v", connections)
	}
}

func TestStoreDefaultsMissingReadOnlyToTrue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	raw := `{"connections":[{"id":"x","name":"X","driver":"sqlite","dsn":":memory:"}]}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	connections, err := NewStore(path).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(connections) != 1 || !connections[0].ReadOnly {
		t.Fatalf("connections = %+v, want ReadOnly=true", connections)
	}
}

func TestStoreInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := NewStore(path).Load(); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestStoreWithoutPath(t *testing.T) {
	store := NewStore("")
	if connections, err := store.Load(); err != nil || connections != nil {
		t.Fatalf("Load = %v, %v", connections, err)
	}
	if err := store.Save(nil); err == nil {
		t.Fatal("Save without a path must fail")
	}
}

func TestSaveShapeUsesConnectionsKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "connections.json")
	if err := NewStore(path).Save([]db.Connection{{ID: "x", Name: "X", Driver: db.DriverSQLite, DSN: ":memory:"}}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var file map[string]json.RawMessage
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := file["connections"]; !ok {
		t.Fatalf("missing connections key: %s", data)
	}
}

func TestMaskDSN(t *testing.T) {
	tests := []struct {
		dsn  string
		want string
	}{
		{"user:secret@tcp(localhost:3306)/db", "user:***@tcp(localhost:3306)/db"},
		{"postgres://user:secret@localhost:5432/db?sslmode=disable", "postgres://user:***@localhost:5432/db?sslmode=disable"},
		{":memory:", ":memory:"},
		{"user@tcp(host)/db", "user@tcp(host)/db"},
	}
	for _, tc := range tests {
		if got := maskDSN(tc.dsn); got != tc.want {
			t.Fatalf("maskDSN(%q) = %q, want %q", tc.dsn, got, tc.want)
		}
	}
}
