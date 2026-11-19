package workspace

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/messenger91/sql-tui/internal/db"
)

const connectTimeout = 10 * time.Second

// ActivatedMsg tells the root model that a connection is now active.
type ActivatedMsg struct {
	Connection db.Connection
}

// DeactivatedMsg tells the root model that the active connection went away.
type DeactivatedMsg struct{}

type connectionsLoadedMsg struct {
	connections []db.Connection
	err         error
}

type connectedMsg struct {
	connection  db.Connection
	connections []db.Connection
	wasActive   bool
	err         error
	saveErr     error
}

type activationFailedMsg struct {
	err error
}

type deletedMsg struct {
	connections []db.Connection
	deactivated bool
	saveErr     error
}

type confirmDelete struct {
	connection db.Connection
}

// Model is the workspace pane: connection list plus add/edit/delete flows.
type Model struct {
	store   *Store
	manager *db.Manager

	list    list.Model
	spinner spinner.Model

	width, height int
	connections   []db.Connection
	activeID      string

	form     *form
	confirm  *confirmDelete
	modalErr error
	busy     bool
	loaded   bool
	status   string
}

// New returns a workspace model. A nil store keeps changes in memory only.
func New(store *Store, manager *db.Manager) Model {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true

	l := list.New(nil, delegate, 20, 5)
	l.SetShowTitle(false)
	l.SetShowHelp(false)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()

	s := spinner.New()
	s.Spinner = spinner.Dot

	return Model{
		store:   store,
		manager: manager,
		list:    l,
		spinner: s,
	}
}

// Init loads persisted connections.
func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return connectionsLoadedMsg{}
		}
		connections, err := m.store.Load()
		return connectionsLoadedMsg{connections: connections, err: err}
	}
}

// SetSize updates the list viewport. The last row is reserved for status.
func (m *Model) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.list.SetSize(max(width, 1), max(height-1, 1))
}

// SetActive marks the connection that the query pane is using.
func (m *Model) SetActive(id string) {
	m.activeID = id
}

// ActivationSucceeded marks the connection active and clears the connecting
// state; the root model calls it when it receives ActivatedMsg.
func (m *Model) ActivationSucceeded(id string) {
	m.activeID = id
	m.busy = false
}

// CapturesInput reports whether a form or dialog must receive keys before
// global pane navigation.
func (m Model) CapturesInput() bool {
	return m.form != nil || m.confirm != nil || m.modalErr != nil
}

// Modal returns the active dialog, if any, so the root model can centre it.
func (m Model) Modal() (title, body string, ok bool) {
	switch {
	case m.form != nil:
		return m.form.title(), m.form.view(m.width - 10), true
	case m.confirm != nil:
		return "Delete connection",
			fmt.Sprintf("Delete %q?\n\ny confirm · n cancel", m.confirm.connection.Name), true
	case m.modalErr != nil:
		return "Connection error", m.modalErr.Error() + "\n\nesc to close", true
	default:
		return "", "", false
	}
}

// Update handles workspace messages and keys.
func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.busy {
			return m, cmd
		}
		return m, nil

	case connectionsLoadedMsg:
		m.loaded = true
		if msg.err != nil {
			m.modalErr = msg.err
			return m, nil
		}
		m.connections = msg.connections
		m.rebuildList()
		return m, nil

	case connectedMsg:
		m.busy = false
		if msg.err != nil {
			m.modalErr = msg.err
			return m, nil
		}
		m.connections = msg.connections
		m.rebuildList()
		m.status = "connected: " + msg.connection.Name
		if msg.saveErr != nil {
			m.modalErr = msg.saveErr
		}
		if msg.wasActive {
			connection := msg.connection
			return m, func() tea.Msg { return ActivatedMsg{Connection: connection} }
		}
		return m, nil

	case deletedMsg:
		if msg.saveErr != nil {
			m.modalErr = msg.saveErr
		}
		m.connections = msg.connections
		m.rebuildList()
		m.status = "connection deleted"
		if msg.deactivated {
			return m, func() tea.Msg { return DeactivatedMsg{} }
		}
		return m, nil

	case activationFailedMsg:
		m.busy = false
		m.modalErr = msg.err
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	default:
		return m, nil
	}
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	if m.form != nil {
		if msg.Type == tea.KeyEsc {
			m.form = nil
			return m, nil
		}
		if m.form.update(msg) {
			return m.submitForm()
		}
		return m, nil
	}

	if m.confirm != nil {
		switch msg.String() {
		case "y", "Y":
			return m.deleteConfirmed()
		case "n", "N", "esc":
			m.confirm = nil
		}
		return m, nil
	}

	if m.modalErr != nil {
		if key.Matches(msg, key.NewBinding(key.WithKeys("esc", "enter"))) {
			m.modalErr = nil
		}
		return m, nil
	}

	if m.busy {
		return m, nil
	}

	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "a":
		f := newForm(formAdd)
		f.setWidth(m.width - 10)
		m.form = f
		return m, nil
	case "e":
		connection, ok := m.selected()
		if !ok {
			return m, nil
		}
		f := newForm(formEdit)
		f.setConnection(connection)
		f.setWidth(m.width - 10)
		m.form = f
		return m, nil
	case "d":
		connection, ok := m.selected()
		if !ok {
			return m, nil
		}
		m.confirm = &confirmDelete{connection: connection}
		return m, nil
	case "enter", " ":
		return m.activateSelected()
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) submitForm() (Model, tea.Cmd) {
	form := m.form
	connection, ok := form.validate()
	if !ok {
		return m, nil
	}

	next := make([]db.Connection, len(m.connections))
	copy(next, m.connections)

	reconnectID := ""
	wasActive := false
	if form.mode == formAdd {
		connection.ID = newID()
		next = append(next, connection)
	} else {
		reconnectID = connection.ID
		wasActive = m.activeID == connection.ID
		for i, existing := range next {
			if existing.ID == connection.ID {
				next[i] = connection
				break
			}
		}
	}
	m.form = nil
	m.busy = true

	store := m.store
	manager := m.manager
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		if reconnectID != "" {
			_ = manager.Close(reconnectID)
		}
		if _, err := manager.Open(ctx, connection); err != nil {
			return connectedMsg{err: fmt.Errorf("connect %s: %w", connection.Name, err)}
		}
		var saveErr error
		if store != nil {
			saveErr = store.Save(next)
		}
		return connectedMsg{
			connection:  connection,
			connections: next,
			wasActive:   wasActive,
			saveErr:     saveErr,
		}
	})
}

func (m Model) activateSelected() (Model, tea.Cmd) {
	connection, ok := m.selected()
	if !ok {
		return m, nil
	}
	m.busy = true
	manager := m.manager
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
		defer cancel()
		if _, err := manager.Open(ctx, connection); err != nil {
			return activationFailedMsg{err: fmt.Errorf("connect %s: %w", connection.Name, err)}
		}
		return ActivatedMsg{Connection: connection}
	})
}

func (m Model) deleteConfirmed() (Model, tea.Cmd) {
	connection := m.confirm.connection
	m.confirm = nil

	remaining := make([]db.Connection, 0, len(m.connections))
	for _, existing := range m.connections {
		if existing.ID != connection.ID {
			remaining = append(remaining, existing)
		}
	}
	deactivated := m.activeID == connection.ID
	if deactivated {
		m.activeID = ""
	}
	m.busy = true

	store := m.store
	manager := m.manager
	return m, tea.Batch(m.spinner.Tick, func() tea.Msg {
		_ = manager.Close(connection.ID)
		var saveErr error
		if store != nil {
			saveErr = store.Save(remaining)
		}
		return deletedMsg{connections: remaining, deactivated: deactivated, saveErr: saveErr}
	})
}

func (m Model) selected() (db.Connection, bool) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return db.Connection{}, false
	}
	entry, ok := selected.(item)
	if !ok {
		return db.Connection{}, false
	}
	return entry.connection, true
}

func (m *Model) rebuildList() {
	m.list.SetItems(items(m.connections))
}

// View renders the list or its empty state, plus a status line.
func (m Model) View() string {
	if !m.loaded {
		return "loading connections…"
	}

	var body string
	if len(m.connections) == 0 {
		body = "no connections yet\n\npress a to add one"
	} else {
		body = m.list.View()
	}

	status := ""
	if m.busy {
		status = m.spinner.View() + " connecting…"
	} else if m.status != "" {
		status = m.status
	}

	view := lipgloss.JoinVertical(lipgloss.Left, body, status)
	return view
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("conn-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
