package app

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type exportDestination int

const (
	exportToFile exportDestination = iota
	exportToClipboard
)

// exportDialog is the ctrl+e overlay. It owns only UI state; the root model
// performs the actual write in a tea.Cmd.
type exportDialog struct {
	format      string // "csv" or "json"
	destination exportDestination
	focus       int // 0 format, 1 destination, 2 path
	path        textinput.Model
	err         string
}

func newExportDialog() *exportDialog {
	path := textinput.New()
	path.Prompt = ""
	path.CharLimit = 512
	path.Width = 48
	path.SetValue(defaultExportPath("csv"))

	return &exportDialog{
		format: "csv",
		path:   path,
	}
}

func defaultExportPath(format string) string {
	return "results-" + time.Now().Format("20060102-150405") + "." + format
}

func (d *exportDialog) setFormat(format string) {
	d.format = format
	value := d.path.Value()
	if dot := strings.LastIndex(value, "."); dot > 0 && !strings.Contains(value[dot:], "/") {
		value = value[:dot]
	}
	d.path.SetValue(value + "." + format)
}

func (d *exportDialog) syncFocus() {
	if d.focus == 2 && d.destination == exportToFile {
		d.path.Focus()
		return
	}
	d.path.Blur()
}

// update handles a key press and reports whether the dialog should submit.
func (d *exportDialog) update(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab":
		d.focus = (d.focus + 1) % 3
		if d.destination == exportToClipboard && d.focus == 2 {
			d.focus = 0
		}
		d.syncFocus()
		return false
	case "shift+tab":
		d.focus = (d.focus + 2) % 3
		if d.destination == exportToClipboard && d.focus == 2 {
			d.focus = 1
		}
		d.syncFocus()
		return false
	}

	if msg.Type == tea.KeyEnter {
		return true
	}

	switch d.focus {
	case 0:
		if toggleKey(msg) {
			if d.format == "csv" {
				d.setFormat("json")
			} else {
				d.setFormat("csv")
			}
		}
	case 1:
		if toggleKey(msg) {
			if d.destination == exportToFile {
				d.destination = exportToClipboard
				if d.focus == 2 {
					d.focus = 1
				}
			} else {
				d.destination = exportToFile
			}
			d.syncFocus()
		}
	case 2:
		if d.destination == exportToFile {
			var cmd tea.Cmd
			d.path, cmd = d.path.Update(msg)
			_ = cmd
		}
	}
	return false
}

func toggleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "left", "right", "h", "l", " ":
		return true
	default:
		return false
	}
}

func (d *exportDialog) validate() bool {
	if d.destination == exportToFile && strings.TrimSpace(d.path.Value()) == "" {
		d.err = "file path is required"
		return false
	}
	d.err = ""
	return true
}

func (d *exportDialog) view() string {
	label := lipgloss.NewStyle().Bold(true)
	faint := lipgloss.NewStyle().Faint(true)

	var b strings.Builder
	b.WriteString(label.Render("Format"))
	b.WriteString("\n◀ ")
	b.WriteString(strings.ToUpper(d.format))
	b.WriteString(" ▶\n\n")
	b.WriteString(label.Render("Destination"))
	b.WriteString("\n◀ ")
	if d.destination == exportToFile {
		b.WriteString("File")
	} else {
		b.WriteString("Clipboard")
	}
	b.WriteString(" ▶")

	if d.destination == exportToFile {
		b.WriteString("\n\n")
		b.WriteString(label.Render("File path"))
		b.WriteString("\n")
		b.WriteString(d.path.View())
	}

	if d.err != "" {
		b.WriteString("\n\n")
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(d.err))
	}
	b.WriteString("\n\n")
	b.WriteString(faint.Render("tab next · ←/→ change · enter export · esc cancel"))
	return b.String()
}
