package app

import "github.com/messenger91/sql-tui/internal/session"

// sessionEventMsg carries one asynchronous runner event for a specific
// session. The root model routes it by SessionID.
type sessionEventMsg struct {
	SessionID session.ID
	Event     any
}
