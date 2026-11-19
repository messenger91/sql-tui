package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/export"
)

type fakeClipboard struct {
	text string
	err  error
}

func (f *fakeClipboard) WriteAll(text string) error {
	if f.err != nil {
		return f.err
	}
	f.text = text
	return nil
}

func newExportModel(t *testing.T, clipboard export.Clipboard) Model {
	t.Helper()
	dir := t.TempDir()
	m := NewWithConfig(Config{
		ConnectionsPath: filepath.Join(dir, "connections.json"),
		FavoritesPath:   filepath.Join(dir, "favorites.json"),
		Clipboard:       clipboard,
	})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return updated.(Model)
}

func executeSQL(t *testing.T, m Model, sql string) Model {
	t.Helper()
	return runQuerySync(t, m, sql)
}

func TestExportToFile(t *testing.T) {
	m := openActive(t, newSized(t, 80, 24))
	m = executeSQL(t, m, "SELECT 42 AS answer")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)
	if m.export == nil {
		t.Fatal("ctrl+e did not open the export dialog")
	}

	path := filepath.Join(t.TempDir(), "out.csv")
	m.export.path.SetValue(path)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.export != nil {
		t.Fatal("dialog still open after enter")
	}
	if cmd == nil {
		t.Fatal("no export command")
	}

	updated, _ = m.Update(cmd())
	m = updated.(Model)
	if m.modal != nil {
		t.Fatalf("export failed: %s", m.modal.Body)
	}
	if !strings.Contains(m.notice, "Export complete") {
		t.Fatalf("notice = %q", m.notice)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if string(data) != "answer\n42\n" {
		t.Fatalf("file = %q", data)
	}
}

func TestExportToClipboardAsJSON(t *testing.T) {
	clipboard := &fakeClipboard{}
	m := openActive(t, newExportModel(t, clipboard))
	m = executeSQL(t, m, "SELECT 42 AS answer")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)
	// format -> JSON, destination -> clipboard
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if cmd == nil {
		t.Fatal("no export command")
	}
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if !strings.Contains(clipboard.text, `"answer": 42`) {
		t.Fatalf("clipboard = %q", clipboard.text)
	}
	if !strings.Contains(m.notice, "Copied to clipboard") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestExportWithoutResult(t *testing.T) {
	m := newSized(t, 80, 24)
	// workspace -> query -> results
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)
	if m.export != nil {
		t.Fatal("dialog opened without a result")
	}
	if !strings.Contains(m.notice, "no results to export") {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestExportIgnoresCtrlEOutsideResults(t *testing.T) {
	m := newSized(t, 80, 24) // focus workspace
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)
	if m.export != nil {
		t.Fatal("dialog opened outside the results pane")
	}
}

func TestExportFailureShowsModal(t *testing.T) {
	clipboard := &fakeClipboard{err: errors.New("no clipboard utility")}
	m := openActive(t, newExportModel(t, clipboard))
	m = executeSQL(t, m, "SELECT 1")

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlE})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = updated.(Model)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	updated, _ = m.Update(cmd())
	m = updated.(Model)

	if m.modal == nil || !strings.Contains(m.modal.Body, "no clipboard utility") {
		t.Fatalf("modal = %+v", m.modal)
	}
}

func TestStatusNoticeTimeout(t *testing.T) {
	m := newSized(t, 80, 24)
	cmd := m.setNotice("Export complete")
	if m.notice == "" {
		t.Fatal("notice not set")
	}
	if cmd == nil {
		t.Fatal("no expiry command scheduled")
	}
	updated, _ := m.Update(statusTimeoutMsg{Seq: m.noticeSeq})
	m = updated.(Model)
	if m.notice != "" {
		t.Fatalf("notice = %q", m.notice)
	}
}

func TestStaleNoticeTimeoutIsIgnored(t *testing.T) {
	m := newSized(t, 80, 24)
	stale := m.noticeSeq
	m.setNotice("new notice")
	updated, _ := m.Update(statusTimeoutMsg{Seq: stale})
	m = updated.(Model)
	if m.notice != "new notice" {
		t.Fatalf("stale timeout cleared the notice: %q", m.notice)
	}
}
