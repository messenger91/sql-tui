package workspace

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"github.com/messenger91/sql-tui/internal/db"
)

type item struct {
	connection db.Connection
	active     bool
}

func (i item) Title() string {
	if i.active {
		return "● " + i.connection.Name
	}
	return i.connection.Name
}
func (i item) Description() string { return description(i.connection) }
func (i item) FilterValue() string {
	return strings.ToLower(i.connection.Name + " " + string(i.connection.Driver))
}

func description(conn db.Connection) string {
	if conn.ReadOnly {
		return fmt.Sprintf("%s · %s · read-only", conn.Driver, maskDSN(conn.DSN))
	}
	return fmt.Sprintf("%s · %s", conn.Driver, maskDSN(conn.DSN))
}

// maskDSN hides passwords before a DSN reaches the screen.
func maskDSN(dsn string) string {
	if schemeEnd := strings.Index(dsn, "://"); schemeEnd >= 0 {
		rest := dsn[schemeEnd+3:]
		authority, suffix := rest, ""
		if cut := strings.IndexAny(rest, "/?#"); cut >= 0 {
			authority, suffix = rest[:cut], rest[cut:]
		}
		if at := strings.LastIndex(authority, "@"); at > 0 {
			userinfo := authority[:at]
			if colon := strings.Index(userinfo, ":"); colon >= 0 {
				authority = userinfo[:colon+1] + "***" + authority[at:]
			}
		}
		return dsn[:schemeEnd+3] + authority + suffix
	}
	// MySQL form: user:password@tcp(host:port)/db
	at := strings.LastIndex(dsn, "@")
	if at <= 0 {
		return dsn
	}
	colon := strings.Index(dsn[:at], ":")
	if colon < 0 {
		return dsn
	}
	return dsn[:colon+1] + "***" + dsn[at:]
}

func items(connections []db.Connection, activeID string) []list.Item {
	listItems := make([]list.Item, 0, len(connections))
	for _, conn := range connections {
		listItems = append(listItems, item{connection: conn, active: conn.ID == activeID})
	}
	return listItems
}
