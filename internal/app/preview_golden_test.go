package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestGoldenDMLPreview(t *testing.T) {
	m := editableWithRows(t, false)
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.cellEdit.input.SetValue("42")
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m, _ = appUpdate(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}})
	if m.preview == nil {
		t.Fatal("preview did not open")
	}
	checkGolden(t, "dml_preview", ansi.Strip(m.preview.view()))
}
