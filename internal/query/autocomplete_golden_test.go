package query

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func checkGolden(t *testing.T, name, content string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden (run with -update to create): %v", err)
	}
	if string(want) != content {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", name, want, content)
	}
}

func TestGoldenAutocompletePopup(t *testing.T) {
	m := New()
	m.SetSize(60, 16)
	m.SetSchema(testIndex())
	m.editor.SetValue("SELECT * FROM us")
	m.editor.CursorEnd()
	m = m.computeSuggestions()
	if !m.PopupVisible() {
		t.Fatal("popup did not open")
	}

	checkGolden(t, "autocomplete_popup", ansi.Strip(m.popupView()))

	// Golden for a selected second row as well.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = updated
	checkGolden(t, "autocomplete_popup_selected", ansi.Strip(m.popupView()))
}
