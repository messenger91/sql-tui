package app

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/db"
	"github.com/messenger91/sql-tui/internal/query"
	"github.com/messenger91/sql-tui/internal/workspace"
)

func activateConnection(t *testing.T, m Model, id string) Model {
	t.Helper()
	connection := db.Connection{ID: id, Name: id, Driver: db.DriverSQLite, DSN: ":memory:"}
	if _, err := m.manager.Open(context.Background(), connection); err != nil {
		t.Fatalf("open %s: %v", id, err)
	}
	t.Cleanup(func() { _ = m.manager.CloseAll() })
	m, _ = appUpdate(t, m, workspace.ActivatedMsg{Connection: connection})
	return m
}

func TestInitialSessionAndConnectionReuse(t *testing.T) {
	m := newSized(t, 80, 24)
	if m.sessions.Len() != 1 {
		t.Fatalf("initial sessions = %d", m.sessions.Len())
	}
	first := m.activeSession().ID

	m = activateConnection(t, m, "c1")
	if m.sessions.Len() != 1 {
		t.Fatalf("sessions after activation = %d", m.sessions.Len())
	}
	if active := m.activeSession(); active.ID != first || active.ConnectionID != "c1" {
		t.Fatalf("active = %+v", active)
	}

	m = activateConnection(t, m, "c1")
	if m.sessions.Len() != 1 {
		t.Fatalf("re-activation created a tab: %d", m.sessions.Len())
	}

	m = activateConnection(t, m, "c2")
	if m.sessions.Len() != 2 {
		t.Fatalf("new connection sessions = %d", m.sessions.Len())
	}
	if active := m.activeSession(); active.ConnectionID != "c2" {
		t.Fatalf("active connection = %q", active.ConnectionID)
	}
}

func TestNewTabInheritsConnectionAndKeepsBuffersSeparate(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	first := m.activeSession()

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	if m.sessions.Len() != 2 {
		t.Fatalf("sessions = %d", m.sessions.Len())
	}
	second := m.activeSession()
	if second.ID == first.ID || second.ConnectionID != "c1" {
		t.Fatalf("new session = %+v", second)
	}
	if m.focus != FocusQuery {
		t.Fatalf("focus = %v", m.focus)
	}

	m = typeRunes(t, m, "SELECT 2")
	if second.Editor.SQL() != "SELECT 2" || first.Editor.SQL() != "" {
		t.Fatalf("buffers not isolated: first=%q second=%q", first.Editor.SQL(), second.Editor.SQL())
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlLeft})
	if active := m.activeSession(); active.ID != first.ID {
		t.Fatalf("active after ctrl+left = %s", active.ID)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlRight})
	if active := m.activeSession(); active.ID != second.ID {
		t.Fatalf("active after ctrl+right = %s", active.ID)
	}
}

func TestJumpToTab(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	first := m.activeSession().ID
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	second := m.activeSession().ID

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alt+1")})
	if active := m.activeSession(); active.ID != first {
		t.Fatalf("alt+1 active = %s", active.ID)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alt+2")})
	if active := m.activeSession(); active.ID != second {
		t.Fatalf("alt+2 active = %s", active.ID)
	}
	// Out-of-range digit is ignored.
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alt+9")})
	if active := m.activeSession(); active.ID != second {
		t.Fatalf("alt+9 changed active = %s", active.ID)
	}
}

func TestCloseLastTabKeepsOneAlive(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	old := m.activeSession().ID

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if m.sessions.Len() != 1 {
		t.Fatalf("sessions = %d", m.sessions.Len())
	}
	if active := m.activeSession(); active.ID == old {
		t.Fatal("closed session still active")
	}
	if active := m.activeSession(); active.ConnectionID != "c1" {
		t.Fatalf("replacement connection = %q", active.ConnectionID)
	}
}

func TestCloseRunningTabRequiresConfirmation(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	s := m.activeSession()
	cancelled := false
	s.MarkRunning(func() { cancelled = true })

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlW})
	if m.confirmClose != s.ID || m.sessions.Len() != 2 {
		t.Fatalf("confirm=%q sessions=%d", m.confirmClose, m.sessions.Len())
	}
	if !strings.Contains(ansi.Strip(m.View()), "running") {
		t.Fatalf("confirm modal missing:\n%s", m.View())
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.confirmClose != "" || m.sessions.Len() != 2 {
		t.Fatalf("cancel failed: confirm=%q sessions=%d", m.confirmClose, m.sessions.Len())
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlW})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if m.sessions.Len() != 1 {
		t.Fatalf("sessions after confirm = %d", m.sessions.Len())
	}
	if !cancelled {
		t.Fatal("running session was not cancelled")
	}
}

func TestTabBarShowsTitlesAndRunningMarker(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	m = typeRunes(t, m, "SELECT 1")

	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
	for _, msg := range collectCmds(cmd) {
		if _, ok := msg.(query.ExecuteRequestedMsg); ok {
			m, _ = appUpdate(t, m, msg)
		}
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "1: ") || !strings.Contains(view, "SELECT 1") {
		t.Fatalf("tab bar missing title:\n%s", view)
	}
	if !strings.Contains(view, "●") {
		t.Fatalf("running marker missing:\n%s", view)
	}
}

func TestQueryExecutionUpdatesTabTitle(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	s := m.activeSession()
	s.Editor.SetSQL("SELECT 42 AS answer")

	updated, _ := m.Update(query.ExecuteRequestedMsg{SQL: s.Editor.SQL()})
	m = updated.(Model)

	if got := m.activeSession().Title; got != "SELECT 42 AS answer" {
		t.Fatalf("title = %q", got)
	}
}
