package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestExplainOpensPlanModal(t *testing.T) {
	m := activateEditable(t, false)
	m.activeSession().Editor.SetSQL("SELECT * FROM users")
	m.focus = FocusQuery

	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if cmd == nil {
		t.Fatal("x produced no explain command")
	}
	msg, ok := cmd().(explainDoneMsg)
	if !ok {
		t.Fatalf("unexpected message %T", msg)
	}
	m, _ = appUpdate(t, m, msg)
	if m.modal == nil {
		t.Fatal("plan modal did not open")
	}
	body := ansi.Strip(m.View())
	if !strings.Contains(body, "Query plan") {
		t.Fatalf("modal title missing:\n%s", body)
	}
	if !strings.Contains(strings.ToUpper(body), "SCAN") {
		t.Fatalf("plan missing scan step:\n%s", body)
	}
}

func TestExplainRequiresSQL(t *testing.T) {
	m := activateEditable(t, false)
	m.focus = FocusQuery
	m.activeSession().Editor.SetSQL("   ")

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if m.modal != nil {
		t.Fatal("modal opened without SQL")
	}
	if !strings.Contains(m.notice, "nothing to explain") {
		t.Fatalf("notice = %q", m.notice)
	}
}
