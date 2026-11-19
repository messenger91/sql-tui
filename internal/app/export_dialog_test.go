package app

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestExportDialogDefaultPath(t *testing.T) {
	dialog := newExportDialog()
	if dialog.format != "csv" {
		t.Fatalf("format = %q", dialog.format)
	}
	if dialog.destination != exportToFile {
		t.Fatalf("destination = %v", dialog.destination)
	}
	if !regexp.MustCompile(`^results-\d{8}-\d{6}\.csv$`).MatchString(dialog.path.Value()) {
		t.Fatalf("path = %q", dialog.path.Value())
	}
}

func TestExportDialogFormatChangesExtension(t *testing.T) {
	dialog := newExportDialog()
	base := dialog.path.Value()
	dialog.setFormat("json")
	if got := dialog.path.Value(); got != base[:len(base)-4]+".json" {
		t.Fatalf("path = %q", got)
	}
}

func TestExportDialogCyclesFormats(t *testing.T) {
	dialog := newExportDialog()
	dialog.update(tea.KeyMsg{Type: tea.KeyRight})
	if dialog.format != "json" || !strings.HasSuffix(dialog.path.Value(), ".json") {
		t.Fatalf("format = %q path = %q", dialog.format, dialog.path.Value())
	}
	dialog.update(tea.KeyMsg{Type: tea.KeyRight})
	if dialog.format != "markdown" || !strings.HasSuffix(dialog.path.Value(), ".markdown") {
		t.Fatalf("format = %q path = %q", dialog.format, dialog.path.Value())
	}
	dialog.update(tea.KeyMsg{Type: tea.KeyRight})
	if dialog.format != "csv" || !strings.HasSuffix(dialog.path.Value(), ".csv") {
		t.Fatalf("format = %q path = %q", dialog.format, dialog.path.Value())
	}
}

func TestExportDialogTabSkipsPathForClipboard(t *testing.T) {
	dialog := newExportDialog() // focus format
	dialog.update(tea.KeyMsg{Type: tea.KeyTab})
	if dialog.focus != 1 {
		t.Fatalf("focus = %d", dialog.focus)
	}
	// Switch to clipboard, then tab must skip the path field.
	dialog.update(tea.KeyMsg{Type: tea.KeyRight})
	if dialog.destination != exportToClipboard {
		t.Fatalf("destination = %v", dialog.destination)
	}
	dialog.update(tea.KeyMsg{Type: tea.KeyTab})
	if dialog.focus != 0 {
		t.Fatalf("focus = %d, want wrap to format", dialog.focus)
	}
}

func TestExportDialogValidation(t *testing.T) {
	dialog := newExportDialog()
	dialog.path.SetValue("   ")
	if dialog.validate() {
		t.Fatal("blank path validated")
	}
	dialog.destination = exportToClipboard
	if !dialog.validate() {
		t.Fatal("clipboard export should not require a path")
	}
}
