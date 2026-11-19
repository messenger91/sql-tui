package app

// SessionID identifies a query session (§11).
type SessionID string

// sessionEventMsg carries one asynchronous event for a specific session. The
// root model routes it by SessionID; §11 attaches the concrete event types.
type sessionEventMsg struct {
	SessionID SessionID
	Event     any
}
