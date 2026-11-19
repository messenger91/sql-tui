package schema

import (
	"errors"
	"sync"
	"testing"

	"github.com/messenger91/sql-tui/internal/db"
)

func sampleTables() []db.TableMeta {
	return []db.TableMeta{
		{
			Schema: "main",
			Name:   "Users",
			Kind:   db.TableKindTable,
			Columns: []db.ColumnMeta{
				{Name: "ID", Type: "INTEGER", IsPrimaryKey: true, Ordinal: 0},
				{Name: "email", Type: "TEXT", Nullable: true, Ordinal: 1},
			},
		},
		{
			Schema: "main",
			Name:   "active_users",
			Kind:   db.TableKindView,
			Columns: []db.ColumnMeta{
				{Name: "id", Type: "INTEGER", Ordinal: 0},
			},
		},
	}
}

func TestBuildIndex(t *testing.T) {
	index := BuildIndex(sampleTables())

	if got := len(index.Suggestions()); got != 5 {
		t.Fatalf("suggestions = %d", got)
	}
	tables := index.Find("users")
	if len(tables) != 1 || tables[0].Kind != KindTable || tables[0].Name != "Users" {
		t.Fatalf("Find(users) = %+v", tables)
	}
	views := index.Find("active_users")
	if len(views) != 1 || views[0].Kind != KindView {
		t.Fatalf("Find(view) = %+v", views)
	}
	columns := index.Find("EMAIL")
	if len(columns) != 1 || columns[0].Kind != KindColumn || columns[0].Table != "Users" || columns[0].Type != "TEXT" {
		t.Fatalf("Find(email) = %+v", columns)
	}
	if _, ok := index.Table("USERS"); !ok {
		t.Fatal("Table lookup is not case-insensitive")
	}
	if _, ok := index.Table("missing"); ok {
		t.Fatal("unexpected table match")
	}
	if len(index.Find("missing")) != 0 {
		t.Fatal("unexpected suggestion match")
	}
}

func TestDuplicateColumnNamesAcrossTables(t *testing.T) {
	index := BuildIndex([]db.TableMeta{
		{Name: "a", Columns: []db.ColumnMeta{{Name: "id"}}},
		{Name: "b", Columns: []db.ColumnMeta{{Name: "id"}}},
	})
	matches := index.Find("id")
	if len(matches) != 2 {
		t.Fatalf("matches = %+v", matches)
	}
}

func TestCacheStoreGetDelete(t *testing.T) {
	cache := NewCache()
	if _, ok := cache.Get("c1"); ok {
		t.Fatal("unexpected empty cache hit")
	}

	cache.Store("c1", sampleTables(), nil)
	entry, ok := cache.Get("c1")
	if !ok {
		t.Fatal("missing entry")
	}
	if len(entry.Tables) != 2 || entry.Index == nil || entry.Err != nil || entry.LoadedAt.IsZero() {
		t.Fatalf("entry = %+v", entry)
	}
	if _, ok := cache.Get("c2"); ok {
		t.Fatal("entries leaked between connections")
	}

	cache.Delete("c1")
	if _, ok := cache.Get("c1"); ok {
		t.Fatal("entry not deleted")
	}
}

func TestCacheKeepsPartialSchema(t *testing.T) {
	cache := NewCache()
	wantErr := errors.New("permission denied")
	cache.Store("c1", sampleTables()[:1], wantErr)

	entry, ok := cache.Get("c1")
	if !ok || len(entry.Tables) != 1 || !errors.Is(entry.Err, wantErr) {
		t.Fatalf("entry = %+v", entry)
	}
}

func TestCacheConcurrentAccess(t *testing.T) {
	cache := NewCache()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Store("c1", sampleTables(), nil)
			_, _ = cache.Get("c1")
		}()
	}
	wg.Wait()
	if cache.Len() != 1 {
		t.Fatalf("len = %d", cache.Len())
	}
}
