package session

import (
	"context"
	"strings"
	"testing"
)

func TestDeriveTitle(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want string
	}{
		{"empty", "", "untitled"},
		{"whitespace", "   \n\t ", "untitled"},
		{"simple", "SELECT * FROM users", "SELECT * FROM users"},
		{"line comment", "-- note\nSELECT 1", "SELECT 1"},
		{"block comment", "/* note */ SELECT 1", "SELECT 1"},
		{"multi line", "SELECT\n  *\nFROM users", "SELECT * FROM users"},
		{"long", strings.Repeat("a", 40), strings.Repeat("a", 23) + "…"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveTitle(tc.sql); got != tc.want {
				t.Fatalf("DeriveTitle(%q) = %q, want %q", tc.sql, got, tc.want)
			}
		})
	}
}

func TestSessionDefaultsAndSetSQL(t *testing.T) {
	s := New("s1", "c1")
	if s.State != StateIdle || s.Running() || s.Title != "untitled" {
		t.Fatalf("session = %+v", s)
	}
	s.SetSQL("SELECT 42")
	if s.Title != "SELECT 42" {
		t.Fatalf("title = %q", s.Title)
	}
	if s.Editor.SQL() != "SELECT 42" {
		t.Fatalf("editor = %q", s.Editor.SQL())
	}
}

func TestSessionStateTransitionsAndCancel(t *testing.T) {
	s := New("s1", "c1")
	ctx, cancel := context.WithCancel(context.Background())
	s.MarkRunning(cancel)
	if !s.Running() || s.State != StateRunning || s.StartedAt.IsZero() {
		t.Fatalf("running state = %+v", s)
	}

	s.Cancel()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("cancel function was not called")
	}

	s.MarkDone()
	if s.Running() || s.State != StateDone {
		t.Fatalf("state = %v", s.State)
	}
	s.MarkFailed()
	if s.State != StateError {
		t.Fatalf("state = %v", s.State)
	}
	s.MarkCancelled()
	if s.State != StateCancelled {
		t.Fatalf("state = %v", s.State)
	}
}

func TestSessionLabel(t *testing.T) {
	s := New("s1", "c1")
	s.SetSQL("SELECT 1")
	if got := s.Label(1); got != "1: SELECT 1" {
		t.Fatalf("label = %q", got)
	}
	s.MarkRunning(func() {})
	if got := s.Label(2); got != "2: ● SELECT 1" {
		t.Fatalf("running label = %q", got)
	}
}

func TestManagerOrderAndActivation(t *testing.T) {
	m := NewManager()
	first := m.New("c1")
	second := m.New("c1")

	if first.ID == second.ID {
		t.Fatal("duplicate session IDs")
	}
	if m.Len() != 2 {
		t.Fatalf("len = %d", m.Len())
	}
	if active, ok := m.Active(); !ok || active.ID != second.ID {
		t.Fatalf("active = %+v", active)
	}
	if !m.SetActive(first.ID) {
		t.Fatal("SetActive failed")
	}
	if m.SetActive("missing") {
		t.Fatal("SetActive accepted an unknown ID")
	}
	if got := m.Sessions(); len(got) != 2 || got[0].ID != first.ID {
		t.Fatalf("sessions = %+v", got)
	}
}

func TestManagerNextPrevWrap(t *testing.T) {
	m := NewManager()
	a := m.New("c1")
	b := m.New("c1")

	m.SetActive(a.ID)
	if next, _ := m.Next(); next.ID != b.ID {
		t.Fatalf("next = %s", next.ID)
	}
	if next, _ := m.Next(); next.ID != a.ID {
		t.Fatalf("next did not wrap: %s", next.ID)
	}
	if prev, _ := m.Prev(); prev.ID != b.ID {
		t.Fatalf("prev = %s", prev.ID)
	}
}

func TestManagerClose(t *testing.T) {
	m := NewManager()
	a := m.New("c1")
	b := m.New("c1")
	c := m.New("c1")
	m.SetActive(b.ID)

	if !m.Close(b.ID) {
		t.Fatal("Close returned false")
	}
	if _, ok := m.Get(b.ID); ok {
		t.Fatal("session still present")
	}
	if active, _ := m.Active(); active.ID != c.ID {
		t.Fatalf("active after close = %s, want %s", active.ID, c.ID)
	}
	if m.Close("missing") {
		t.Fatal("Close accepted an unknown ID")
	}

	m.Close(a.ID)
	m.Close(c.ID)
	if m.Len() != 0 {
		t.Fatalf("len = %d", m.Len())
	}
	if _, ok := m.Active(); ok {
		t.Fatal("active session after closing all")
	}
	if _, ok := m.Next(); ok {
		t.Fatal("Next on empty manager")
	}
	if _, ok := m.Prev(); ok {
		t.Fatal("Prev on empty manager")
	}
}

func TestManagerCloseCancelsRunningSession(t *testing.T) {
	m := NewManager()
	s := m.New("c1")
	ctx, cancel := context.WithCancel(context.Background())
	s.MarkRunning(cancel)

	m.Close(s.ID)
	select {
	case <-ctx.Done():
	default:
		t.Fatal("closing a running session did not cancel it")
	}
}
