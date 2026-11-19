// Package db is the driver-agnostic database layer: connection management,
// query execution, and schema introspection.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql" // registers the "mysql" driver
	_ "github.com/lib/pq"              // registers the "postgres" driver
	_ "modernc.org/sqlite"             // registers the "sqlite" driver
)

// DefaultMaxRows is the row cap used by Execute when the caller does not
// supply a limit (§6.4).
const DefaultMaxRows = 10000

// ErrUnsupportedDriver is returned for connection drivers this build does not
// know how to open.
var ErrUnsupportedDriver = errors.New("db: unsupported driver")

// ErrUnsupportedSchema is returned by LoadSchema for drivers whose
// introspection path has not been implemented yet.
var ErrUnsupportedSchema = errors.New("db: schema introspection not implemented")

// Driver identifies a supported SQL backend.
type Driver string

const (
	DriverMySQL    Driver = "mysql"
	DriverPostgres Driver = "postgres"
	DriverSQLite   Driver = "sqlite"
)

// Valid reports whether d is a known driver.
func (d Driver) Valid() bool {
	switch d {
	case DriverMySQL, DriverPostgres, DriverSQLite:
		return true
	default:
		return false
	}
}

// Dialect describes how to build parameterised SQL for a driver (§12.3).
type Dialect struct {
	// Placeholder returns the bind parameter for the 1-based argument index n.
	Placeholder func(n int) string
	// QuoteIdent quotes an identifier, escaping embedded quote characters.
	QuoteIdent func(name string) string
}

// Dialect returns the SQL dialect for d. Unknown drivers return a zero
// Dialect; callers should only use it for connections that opened successfully.
func (d Driver) Dialect() Dialect {
	switch d {
	case DriverMySQL:
		return Dialect{
			Placeholder: func(int) string { return "?" },
			QuoteIdent:  quoteBacktick,
		}
	case DriverPostgres:
		return Dialect{
			Placeholder: func(n int) string { return "$" + strconv.Itoa(n) },
			QuoteIdent:  quoteDouble,
		}
	case DriverSQLite:
		return Dialect{
			Placeholder: func(int) string { return "?" },
			QuoteIdent:  quoteDouble,
		}
	default:
		return Dialect{}
	}
}

func quoteDouble(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteBacktick(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

// Connection is a stored connection definition. ReadOnly connections must not
// enable inline editing (§12); new connections default to read-only via
// NewConnection.
type Connection struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Driver   Driver            `json:"driver"`
	DSN      string            `json:"dsn"`
	Options  map[string]string `json:"options,omitempty"`
	ReadOnly bool              `json:"readOnly"`
}

// NewConnection builds a connection with the safe default ReadOnly=true.
func NewConnection(id, name string, driver Driver, dsn string) Connection {
	return Connection{
		ID:       id,
		Name:     name,
		Driver:   driver,
		DSN:      dsn,
		ReadOnly: true,
	}
}

// QueryResult is the outcome of Execute. Err is non-nil when the statement
// failed; Rows holds the materialised result (empty for non-row statements).
type QueryResult struct {
	Columns   []string
	Rows      [][]any
	RowCount  int
	Truncated bool
	Elapsed   time.Duration
	Err       error
}

// Connect opens and pings a database handle for conn. The caller owns the
// returned handle and must close it.
func Connect(ctx context.Context, conn Connection) (*sql.DB, error) {
	name, ok := driverName(conn.Driver)
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDriver, conn.Driver)
	}
	handle, err := sql.Open(name, conn.DSN)
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", conn.Driver, err)
	}
	if conn.Driver == DriverSQLite {
		// A SQLite :memory: database lives per physical connection, so a
		// single connection keeps schema and data consistent. It also
		// serialises queries, which is required for SQLite writers anyway.
		handle.SetMaxOpenConns(1)
		handle.SetMaxIdleConns(1)
	}
	if err := handle.PingContext(ctx); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("db: ping %s: %w", conn.Driver, err)
	}
	return handle, nil
}

func driverName(d Driver) (string, bool) {
	switch d {
	case DriverMySQL:
		return "mysql", true
	case DriverPostgres:
		return "postgres", true
	case DriverSQLite:
		return "sqlite", true
	default:
		return "", false
	}
}

// Manager owns the open database handles keyed by connection ID. It keeps
// package-level state out of the application and provides one shutdown point
// (CloseAll) for the TUI.
type Manager struct {
	mu  sync.Mutex
	dbs map[string]*sql.DB
}

// NewManager returns an empty connection manager.
func NewManager() *Manager {
	return &Manager{dbs: make(map[string]*sql.DB)}
}

// Open connects conn and records the handle. Calling Open for an ID that is
// already open returns the existing handle.
func (m *Manager) Open(ctx context.Context, conn Connection) (*sql.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if handle, ok := m.dbs[conn.ID]; ok {
		return handle, nil
	}
	handle, err := Connect(ctx, conn)
	if err != nil {
		return nil, err
	}
	m.dbs[conn.ID] = handle
	return handle, nil
}

// Get returns the open handle for a connection ID.
func (m *Manager) Get(id string) (*sql.DB, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	handle, ok := m.dbs[id]
	return handle, ok
}

// Close closes one connection and forgets it.
func (m *Manager) Close(id string) error {
	m.mu.Lock()
	handle, ok := m.dbs[id]
	if ok {
		delete(m.dbs, id)
	}
	m.mu.Unlock()
	if !ok {
		return nil
	}
	return handle.Close()
}

// CloseAll closes every open connection, returning all close errors joined.
func (m *Manager) CloseAll() error {
	m.mu.Lock()
	handles := make([]*sql.DB, 0, len(m.dbs))
	for id, handle := range m.dbs {
		handles = append(handles, handle)
		delete(m.dbs, id)
	}
	m.mu.Unlock()

	var errs []error
	for _, handle := range handles {
		if err := handle.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
