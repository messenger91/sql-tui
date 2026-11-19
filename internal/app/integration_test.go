package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/messenger91/sql-tui/internal/result"
)

func TestViewToggleIsPerSession(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	m = runQuerySync(t, m, "SELECT 1 AS answer")
	first := m.activeSession()
	if m.focus != FocusResult {
		t.Fatalf("focus = %v", m.focus)
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if first.Results.Format() != result.FormatCSV {
		t.Fatalf("first format = %s", first.Results.Format())
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	second := m.activeSession()
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab}) // results pane
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if second.Results.Format() != result.FormatCSV {
		t.Fatalf("second format = %s", second.Results.Format())
	}
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	if second.Results.Format() != result.FormatJSON {
		t.Fatalf("second format = %s", second.Results.Format())
	}
	if first.Results.Format() != result.FormatCSV {
		t.Fatalf("first session format changed to %s", first.Results.Format())
	}
}

func TestExportUsesActiveSessionResults(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	m = runQuerySync(t, m, "SELECT 1 AS answer")

	// New tab has no results.
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyTab}) // results pane
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.export != nil {
		t.Fatal("export opened for a tab without results")
	}
	if !strings.Contains(m.notice, "no results") {
		t.Fatalf("notice = %q", m.notice)
	}

	// Back to the first tab: export sees its result.
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlLeft})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.export == nil {
		t.Fatal("export did not open for the tab with results")
	}
}

func TestExportBlockedWhileStreaming(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	s := m.activeSession()
	s.SetSQL("SELECT 1")
	s.MarkRunning(func() {})
	m.focus = FocusResult

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.export != nil {
		t.Fatal("export opened while streaming")
	}
	if !strings.Contains(m.notice, "wait for the query") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestDialogTitlesIncludeSessionName(t *testing.T) {
	m := activateConnection(t, newSized(t, 80, 24), "c1")
	m.activeSession().Editor.SetSQL("SELECT 1 AS answer")
	m = runQuerySync(t, m, "SELECT 1 AS answer")

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlE})
	if m.export == nil {
		t.Fatal("export did not open")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Export results · SELECT 1 AS answer") {
		t.Fatalf("export title missing session name:\n%s", view)
	}

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyShiftTab}) // results -> query
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if _, _, ok := m.favorites.Modal(); !ok {
		t.Fatal("save dialog did not open")
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Save favorite · SELECT 1 AS answer") {
		t.Fatalf("favorite title missing session name:\n%s", view)
	}
}

func TestFavoriteLoadsIntoActiveSessionOnly(t *testing.T) {
	m, _ := newFavoritesModel(t)
	m = saveFavoriteFromQuery(t, m, "one", "SELECT 1")
	first := m.activeSession()
	first.Editor.SetSQL("SELECT 0")

	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlT})
	second := m.activeSession()
	if second.ID == first.ID {
		t.Fatal("new tab missing")
	}
	m.focus = FocusQuery
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyCtrlF})
	if _, _, ok := m.favorites.Modal(); !ok {
		t.Fatal("favorites did not open")
	}
	m, cmd := appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("load produced no command")
	}
	m, _ = appUpdate(t, m, cmd())

	if second.Editor.SQL() != "SELECT 1" {
		t.Fatalf("active editor = %q", second.Editor.SQL())
	}
	if first.Editor.SQL() != "SELECT 0" {
		t.Fatalf("first tab changed: %q", first.Editor.SQL())
	}
}
