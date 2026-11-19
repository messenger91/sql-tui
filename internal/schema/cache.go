// Package schema caches database metadata per connection and builds the
// suggestion index used by autocomplete (§10).
package schema

import (
	"strings"
	"sync"
	"time"

	"github.com/messenger91/sql-tui/internal/db"
)

// Kind identifies a suggestion entry.
type Kind string

const (
	KindTable  Kind = "table"
	KindView   Kind = "view"
	KindColumn Kind = "column"
)

// Suggestion is one autocomplete candidate.
type Suggestion struct {
	Name  string
	Kind  Kind
	Table string // owning table for columns
	Type  string // column type, when known
}

// Index is an immutable, case-insensitive lookup over a schema snapshot.
type Index struct {
	tables      []db.TableMeta
	suggestions []Suggestion
	byName      map[string][]int
	tablesLower map[string]int
}

// BuildIndex constructs the lookup tables for a schema snapshot.
func BuildIndex(tables []db.TableMeta) *Index {
	index := &Index{
		tables:      append([]db.TableMeta(nil), tables...),
		byName:      make(map[string][]int),
		tablesLower: make(map[string]int),
	}
	for i, table := range index.tables {
		kind := KindTable
		if table.Kind == db.TableKindView {
			kind = KindView
		}
		index.tablesLower[strings.ToLower(table.Name)] = i
		index.add(Suggestion{Name: table.Name, Kind: kind})
		for _, column := range table.Columns {
			index.add(Suggestion{
				Name:  column.Name,
				Kind:  KindColumn,
				Table: table.Name,
				Type:  column.Type,
			})
		}
	}
	return index
}

func (i *Index) add(suggestion Suggestion) {
	key := strings.ToLower(suggestion.Name)
	i.byName[key] = append(i.byName[key], len(i.suggestions))
	i.suggestions = append(i.suggestions, suggestion)
}

// Suggestions returns every candidate in schema order (tables, then their
// columns).
func (i *Index) Suggestions() []Suggestion {
	if i == nil {
		return nil
	}
	return i.suggestions
}

// Find returns exact case-insensitive matches for name.
func (i *Index) Find(name string) []Suggestion {
	if i == nil {
		return nil
	}
	positions := i.byName[strings.ToLower(name)]
	matches := make([]Suggestion, 0, len(positions))
	for _, position := range positions {
		matches = append(matches, i.suggestions[position])
	}
	return matches
}

// Table returns a table or view by case-insensitive name.
func (i *Index) Table(name string) (db.TableMeta, bool) {
	if i == nil {
		return db.TableMeta{}, false
	}
	position, ok := i.tablesLower[strings.ToLower(name)]
	if !ok {
		return db.TableMeta{}, false
	}
	return i.tables[position], true
}

// Tables returns the cached schema snapshot.
func (i *Index) Tables() []db.TableMeta {
	if i == nil {
		return nil
	}
	return i.tables
}

// Entry is the cached schema for one connection.
type Entry struct {
	Tables   []db.TableMeta
	Index    *Index
	Err      error
	LoadedAt time.Time
}

// Cache stores schema entries keyed by connection ID. It is safe for
// concurrent use: §11 may update it from background commands.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]Entry)}
}

// Store records a schema snapshot. Partial results (tables with err) are kept.
func (c *Cache) Store(connectionID string, tables []db.TableMeta, err error) {
	entry := Entry{
		Tables:   tables,
		Index:    BuildIndex(tables),
		Err:      err,
		LoadedAt: time.Now(),
	}
	c.mu.Lock()
	c.entries[connectionID] = entry
	c.mu.Unlock()
}

// Get returns the cached entry for a connection.
func (c *Cache) Get(connectionID string) (Entry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[connectionID]
	c.mu.RUnlock()
	return entry, ok
}

// Delete removes a connection's entry.
func (c *Cache) Delete(connectionID string) {
	c.mu.Lock()
	delete(c.entries, connectionID)
	c.mu.Unlock()
}

// Len returns the number of cached connections.
func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
