package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/workspace"
)

func mouseClick(x, y int) tea.MouseMsg {
	return tea.MouseMsg{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: x, Y: y}
}

func TestLayoutCollapseThresholds(t *testing.T) {
	cases := []struct {
		width  int
		ws     int
		hidden bool
	}{
		{120, 30, false},
		{80, 30, false},
		{79, 30, false},
		{60, 24, false},
		{40, 16, false},
		{25, 10, false},
		{24, 0, true},
	}
	for _, tc := range cases {
		m := newSized(t, tc.width, 24)
		lay := m.layout()
		if lay.workspaceW != tc.ws || m.wsHidden != tc.hidden {
			t.Fatalf("width %d: ws = %d (hidden %v), want %d (%v)", tc.width, lay.workspaceW, m.wsHidden, tc.ws, tc.hidden)
		}
	}
}

func TestFocusSkipsHiddenWorkspace(t *testing.T) {
	m := newSized(t, 20, 10)
	if !m.wsHidden || m.focus != FocusQuery {
		t.Fatalf("hidden=%v focus=%v", m.wsHidden, m.focus)
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusResult {
		t.Fatalf("focus = %v", m.focus)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.focus != FocusQuery {
		t.Fatalf("focus did not skip hidden workspace: %v", m.focus)
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.focus != FocusResult {
		t.Fatalf("shift+tab focus = %v", m.focus)
	}
}

func TestMouseFocusPanes(t *testing.T) {
	m := newSized(t, 80, 24)

	m, _ = appUpdate(t, m, mouseClick(10, 5))
	if m.focus != FocusWorkspace {
		t.Fatalf("workspace click focus = %v", m.focus)
	}
	m, _ = appUpdate(t, m, mouseClick(40, 5))
	if m.focus != FocusQuery {
		t.Fatalf("query click focus = %v", m.focus)
	}
	m, _ = appUpdate(t, m, mouseClick(40, 15))
	if m.focus != FocusResult {
		t.Fatalf("results click focus = %v", m.focus)
	}
}

func TestMouseTabBarClick(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	first := m.activeSession().ID
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	second := m.activeSession().ID

	// Click the first tab label, then the second.
	m, _ = appUpdate(t, m, mouseClick(30+2+14, 2))
	if active := m.activeSession(); active.ID != second {
		t.Fatalf("clicked second tab, active = %s", active.ID)
	}
	m, _ = appUpdate(t, m, mouseClick(30+2+0, 2))
	if active := m.activeSession(); active.ID != first {
		t.Fatalf("clicked first tab, active = %s", active.ID)
	}

	// "+" affordance creates a new tab.
	before := m.sessions.Len()
	m, _ = appUpdate(t, m, mouseClick(30+2+28, 2))
	if m.sessions.Len() != before+1 {
		t.Fatalf("sessions = %d", m.sessions.Len())
	}
}

func TestMouseIgnoredWhileModalOpen(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	m.openModal("test", "body")

	m, _ = appUpdate(t, m, mouseClick(40, 15))
	if m.focus != FocusQuery {
		t.Fatalf("click changed focus behind a modal: %v", m.focus)
	}
}

func TestMouseWheelEventsDoNothing(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery
	m, _ = appUpdate(t, m, tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress, X: 10, Y: 5})
	if m.focus != FocusQuery {
		t.Fatalf("wheel event changed focus: %v", m.focus)
	}
}

func TestQuestionMarkTypesInEditor(t *testing.T) {
	m := newSized(t, 80, 24)
	m.focus = FocusQuery

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if got := m.activeSession().Editor.SQL(); got != "?" {
		t.Fatalf("editor = %q", got)
	}
	if m.helpOpen {
		t.Fatal("'?' opened the help menu while the editor was focused")
	}
}

func addTestConnection(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = typeRunes(t, m, "local")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeRunes(t, m, ":memory:")
	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	for _, msg := range collectCmds(cmd) {
		m, _ = appUpdate(t, m, msg)
	}
	if got := len(m.workspace.WorkspaceConnections()); got != 1 {
		t.Fatalf("connections = %d", got)
	}
	return m
}

func TestMouseExplorerClickExpandsTables(t *testing.T) {
	m := newSized(t, 80, 24)
	m = addTestConnection(t, m)
	connection := m.workspace.WorkspaceConnections()[0]
	m.workspace.ActivationSucceeded(connection.ID)
	m.workspace.SetTables([]string{"users", "orders"})

	// Click the first connection row (content starts at y=2).
	m, _ = appUpdate(t, m, mouseClick(2, 2))
	if m.focus != FocusWorkspace {
		t.Fatalf("focus = %v", m.focus)
	}
	if !m.workspace.TablesVisible() {
		t.Fatal("clicking the active connection did not expand tables")
	}

	// Click the first table row to fetch it.
	m, cmd := appUpdate(t, m, mouseClick(2, 2))
	if cmd == nil {
		t.Fatal("table click produced no command")
	}
	msg, ok := cmd().(workspace.FetchTableMsg)
	if !ok || msg.Table != "users" {
		t.Fatalf("table click message = %#v", msg)
	}
	if m.workspace.TablesVisible() {
		t.Fatal("fetching a table did not close the list")
	}
}
