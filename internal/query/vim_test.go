package query

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func typeVimText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func pressVim(t *testing.T, m Model, key string) Model {
	t.Helper()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return m
}

func TestVimStartsInInsertMode(t *testing.T) {
	m := New()
	if m.NormalMode() {
		t.Fatal("editor must start in insert mode")
	}
	m = typeVimText(t, m, "select 1")
	if got := m.SQL(); got != "select 1" {
		t.Fatalf("SQL = %q", got)
	}
	if m.ModeLabel() != "INSERT" {
		t.Fatalf("mode label = %q", m.ModeLabel())
	}
}

func TestVimEscEntersNormalAndIInserts(t *testing.T) {
	m := New()
	m = typeVimText(t, m, "select 1")

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.NormalMode() || m.ModeLabel() != "NORMAL" {
		t.Fatalf("esc did not enter normal mode: %q", m.ModeLabel())
	}
	// Normal-mode runes must not reach the textarea.
	m = typeVimText(t, m, "ix")
	if got := m.SQL(); got != "select 1x" {
		// "i" switches to insert, "x" is then typed.
		t.Fatalf("SQL = %q", got)
	}
	if m.NormalMode() {
		t.Fatal("i did not return to insert mode")
	}
}

func TestVimNormalMovementAndEdits(t *testing.T) {
	m := New()
	m.SetSQL("abc\ndef")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	m = pressVim(t, m, "g")
	m = pressVim(t, m, "g")
	if got := m.cursorIndex(); got != 0 {
		t.Fatalf("gg cursor = %d", got)
	}
	m = pressVim(t, m, "l")
	if got := m.cursorIndex(); got != 1 {
		t.Fatalf("l cursor = %d", got)
	}
	m = pressVim(t, m, "0")
	if got := m.cursorIndex(); got != 0 {
		t.Fatalf("0 cursor = %d", got)
	}
	m = pressVim(t, m, "$")
	if got := m.cursorIndex(); got != 3 {
		t.Fatalf("$ cursor = %d", got)
	}

	m = pressVim(t, m, "j")
	if got := m.editor.Line(); got != 1 {
		t.Fatalf("j line = %d", got)
	}
	m = pressVim(t, m, "k")
	if got := m.editor.Line(); got != 0 {
		t.Fatalf("k line = %d", got)
	}

	m = pressVim(t, m, "G")
	if m.editor.Line() != 1 {
		t.Fatalf("G line = %d", m.editor.Line())
	}

	m = pressVim(t, m, "0")
	m = pressVim(t, m, "x")
	if got := m.SQL(); got != "abc\nef" {
		t.Fatalf("x SQL = %q", got)
	}
}

func TestVimDeleteLine(t *testing.T) {
	m := New()
	m.SetSQL("one\ntwo\nthree")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = pressVim(t, m, "g")
	m = pressVim(t, m, "g")
	m = pressVim(t, m, "d")
	m = pressVim(t, m, "d")

	if got := m.SQL(); got != "two\nthree" {
		t.Fatalf("dd SQL = %q", got)
	}
	if m.NormalMode() == false {
		t.Fatal("dd must stay in normal mode")
	}
}

func TestVimInsertVariants(t *testing.T) {
	m := New()
	m.SetSQL("abc")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = pressVim(t, m, "0")
	m = pressVim(t, m, "A")
	m = typeVimText(t, m, "!")
	if got := m.SQL(); got != "abc!" {
		t.Fatalf("A SQL = %q", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = pressVim(t, m, "g")
	m = pressVim(t, m, "g")
	m = pressVim(t, m, "I")
	m = typeVimText(t, m, ">")
	if got := m.SQL(); got != ">abc!" {
		t.Fatalf("I SQL = %q", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = pressVim(t, m, "o")
	m = typeVimText(t, m, "next")
	if got := m.SQL(); got != ">abc!\nnext" {
		t.Fatalf("o SQL = %q", got)
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = pressVim(t, m, "O")
	m = typeVimText(t, m, "before")
	if got := m.SQL(); got != ">abc!\nbefore\nnext" {
		t.Fatalf("O SQL = %q", got)
	}
}

func TestVimWordMovement(t *testing.T) {
	m := New()
	m.SetSQL("one two three")
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = pressVim(t, m, "g")
	m = pressVim(t, m, "g")
	m = pressVim(t, m, "w")
	if got := m.cursorIndex(); got != 4 {
		t.Fatalf("w cursor = %d", got)
	}
	m = pressVim(t, m, "b")
	if got := m.cursorIndex(); got != 0 {
		t.Fatalf("b cursor = %d", got)
	}
}

func TestVimSetSQLReturnsToInsert(t *testing.T) {
	m := New()
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !m.NormalMode() {
		t.Fatal("not normal")
	}
	m.SetSQL("select 2")
	if m.NormalMode() {
		t.Fatal("SetSQL did not return to insert mode")
	}
	m.InsertText(" x")
	if m.NormalMode() || !strings.HasSuffix(m.SQL(), " x") {
		t.Fatalf("InsertText mode/SQL = %q normal=%v", m.SQL(), m.NormalMode())
	}
}
