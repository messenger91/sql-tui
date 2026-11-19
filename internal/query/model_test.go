package query

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/db"
)

func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		updated, _ := m.Update(keyRunes(string(r)))
		m = updated
	}
	return m
}

func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var messages []tea.Msg
		for _, nested := range batch {
			messages = append(messages, collect(nested)...)
		}
		return messages
	}
	return []tea.Msg{msg}
}

func TestEmptyExecuteDoesNothing(t *testing.T) {
	m := New()
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated
	if cmd != nil {
		t.Fatal("empty editor emitted a command")
	}
	if !strings.Contains(m.View(), "nothing to run") {
		t.Fatalf("status not reported:\n%s", m.View())
	}
}

func TestExecuteEmitsRequestAndBlocksReentry(t *testing.T) {
	m := New()
	m = typeText(t, m, "SELECT 1")

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated
	if !m.Running() {
		t.Fatal("editor is not marked running")
	}

	var requested *ExecuteRequestedMsg
	for _, msg := range collect(cmd) {
		if msg, ok := msg.(ExecuteRequestedMsg); ok {
			copied := msg
			requested = &copied
		}
	}
	if requested == nil || requested.SQL != "SELECT 1" {
		t.Fatalf("request = %+v", requested)
	}

	updated, second := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated
	if second != nil {
		t.Fatal("second ctrl+r while running emitted a command")
	}
}

func TestHistoryRecall(t *testing.T) {
	m := New()

	m.editor.SetValue("SELECT 1")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated
	m.ReportSuccess(1, db.QueryResult{})

	m.editor.SetValue("SELECT 2")
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m = updated
	m.ReportSuccess(1, db.QueryResult{})

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	m = updated
	if got := m.editor.Value(); got != "SELECT 2" {
		t.Fatalf("history back = %q", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlUp})
	m = updated
	if got := m.editor.Value(); got != "SELECT 1" {
		t.Fatalf("history back twice = %q", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	m = updated
	if got := m.editor.Value(); got != "SELECT 2" {
		t.Fatalf("history forward = %q", got)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlDown})
	m = updated
	if got := m.editor.Value(); got != "SELECT 2" {
		t.Fatalf("history draft = %q", got)
	}
}

func TestReportStatus(t *testing.T) {
	m := New()
	m.ReportSuccess(2, db.QueryResult{RowCount: 3})
	view := m.View()
	if !strings.Contains(view, "2 statements") || !strings.Contains(view, "3 rows") {
		t.Fatalf("success status missing:\n%s", view)
	}

	m.ReportError(errors.New("boom"))
	if !strings.Contains(m.View(), "error: boom") {
		t.Fatalf("error status missing:\n%s", m.View())
	}

	m.SetStatus("no connection selected")
	if !strings.Contains(m.View(), "no connection selected") {
		t.Fatalf("custom status missing:\n%s", m.View())
	}
}

func TestSetSizeReservesStatusRow(t *testing.T) {
	m := New()
	m.SetSize(40, 6)
	if m.width != 40 {
		t.Fatalf("model width = %d", m.width)
	}
	if m.editor.Height() != 5 {
		t.Fatalf("editor height = %d", m.editor.Height())
	}
}
