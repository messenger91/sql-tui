package session

import "fmt"

// Manager tracks sessions in tab order and the active session. It is not
// goroutine-safe by itself: all mutations happen in the root model's update
// loop (the runner only emits messages).
type Manager struct {
	sessions map[ID]*Session
	order    []ID
	active   ID
	nextID   int
}

// NewManager returns an empty session manager.
func NewManager() *Manager {
	return &Manager{
		sessions: make(map[ID]*Session),
		nextID:   1,
	}
}

// New creates a session for a connection, appends it, and makes it active.
func (m *Manager) New(connectionID string) *Session {
	id := ID(fmt.Sprintf("s%d", m.nextID))
	m.nextID++
	s := New(id, connectionID)
	m.sessions[id] = s
	m.order = append(m.order, id)
	m.active = id
	return s
}

// Get returns a session by ID.
func (m *Manager) Get(id ID) (*Session, bool) {
	s, ok := m.sessions[id]
	return s, ok
}

// Active returns the focused session, if any.
func (m *Manager) Active() (*Session, bool) {
	if m.active == "" {
		return nil, false
	}
	return m.Get(m.active)
}

// SetActive focuses a session. Unknown IDs are ignored.
func (m *Manager) SetActive(id ID) bool {
	if _, ok := m.sessions[id]; !ok {
		return false
	}
	m.active = id
	return true
}

// Sessions returns sessions in tab order.
func (m *Manager) Sessions() []*Session {
	sessions := make([]*Session, 0, len(m.order))
	for _, id := range m.order {
		if s, ok := m.sessions[id]; ok {
			sessions = append(sessions, s)
		}
	}
	return sessions
}

// Len returns the number of open sessions.
func (m *Manager) Len() int { return len(m.order) }

// Close cancels and removes a session. It reports whether the session existed.
// The caller decides whether to create a replacement for the last tab.
func (m *Manager) Close(id ID) bool {
	s, ok := m.sessions[id]
	if !ok {
		return false
	}
	s.Cancel()
	delete(m.sessions, id)

	removedIndex := -1
	for i, existing := range m.order {
		if existing == id {
			removedIndex = i
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	if m.active == id {
		m.active = ""
		if len(m.order) > 0 && removedIndex >= 0 {
			// Prefer the tab that took the closed tab's place, else the last.
			index := min(removedIndex, len(m.order)-1)
			m.active = m.order[index]
		}
	}
	return true
}

// Next advances to the next tab (wrapping) and returns it.
func (m *Manager) Next() (*Session, bool) {
	return m.step(1)
}

// Prev moves to the previous tab (wrapping) and returns it.
func (m *Manager) Prev() (*Session, bool) {
	return m.step(-1)
}

func (m *Manager) step(delta int) (*Session, bool) {
	if len(m.order) == 0 {
		return nil, false
	}
	current := 0
	for i, id := range m.order {
		if id == m.active {
			current = i
			break
		}
	}
	next := (current + delta + len(m.order)) % len(m.order)
	m.active = m.order[next]
	return m.Get(m.active)
}
