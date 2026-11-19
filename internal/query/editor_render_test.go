package query

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderEditorShowsCursorAndPrompt(t *testing.T) {
	m := New()
	m.SetSize(40, 6)
	m.editor.SetValue("SELECT 1")
	m.editor.Focus()
	m.editor.CursorEnd()

	view := m.EditorView()
	if !strings.Contains(view, editorPrompt) {
		t.Fatalf("prompt missing:\n%s", view)
	}
	if !strings.Contains(view, editorCaret) {
		t.Fatalf("caret missing:\n%s", view)
	}
	if !strings.Contains(ansi.Strip(view), "SELECT 1") {
		t.Fatalf("text missing:\n%s", view)
	}
	lines := strings.Split(view, "\n")
	if len(lines) != m.editor.Height() {
		t.Fatalf("lines = %d, want %d", len(lines), m.editor.Height())
	}
}

func TestRenderEditorPlaceholder(t *testing.T) {
	m := New()
	m.SetSize(40, 6)
	view := m.EditorView()
	if !strings.Contains(view, "SELECT * FROM …") {
		t.Fatalf("placeholder missing:\n%s", view)
	}
	if strings.Contains(view, editorCaret) {
		t.Fatalf("caret shown for empty editor:\n%s", view)
	}
}

func TestHighlightLinePreservesText(t *testing.T) {
	inputs := []string{
		"SELECT * FROM users WHERE id = 1",
		"INSERT INTO t VALUES ('a''b', 1.5)",
		"-- comment\nSELECT 1",
		"/* block */ SELECT lower(email) FROM users",
	}
	for _, input := range inputs {
		for _, line := range strings.Split(input, "\n") {
			if got := ansi.Strip(HighlightLine(line)); got != line {
				t.Fatalf("HighlightLine(%q) = %q", line, got)
			}
		}
	}
}
