package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/session"
)

func TestEscapeCancelsRunningSession(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	s := m.activeSession()
	cancelled := false
	s.MarkRunning(func() { cancelled = true })
	m.focus = FocusQuery

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !cancelled {
		t.Fatal("esc did not cancel the running query")
	}
	if s.State != session.StateCancelled || s.Editor.Running() {
		t.Fatalf("state = %v, editor running = %v", s.State, s.Editor.Running())
	}
	if !strings.Contains(m.notice, "cancelling") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestCtrlCCancelsThenQuits(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	s := m.activeSession()
	cancelled := false
	s.MarkRunning(func() { cancelled = true })

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if !cancelled {
		t.Fatal("first ctrl+c did not cancel")
	}
	if s.State != session.StateCancelled {
		t.Fatalf("state = %v", s.State)
	}

	_, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("second ctrl+c produced no command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("second ctrl+c did not quit")
	}
}

func TestShutdownWaitsForRunners(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	s := m.activeSession()
	s.MarkRunning(func() {})

	m.runners.begin(1)
	finished := make(chan struct{})
	runners := m.runners
	go func() {
		time.Sleep(20 * time.Millisecond)
		close(finished)
		runners.done()
	}()

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlC}) // cancel
	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("shutdown produced no command")
	}
	if !m.quitting {
		t.Fatal("quitting flag not set")
	}

	msg := cmd()
	if _, ok := msg.(shutdownDoneMsg); !ok {
		t.Fatalf("unexpected shutdown message %T", msg)
	}
	select {
	case <-finished:
	default:
		t.Fatal("shutdown returned before runners finished")
	}
}
