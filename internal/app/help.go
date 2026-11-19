package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// helpText renders the full keybinding reference as a context menu body.
func helpText() string {
	section := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	faint := lipgloss.NewStyle().Faint(true)
	sections := []string{
		"global",
		"  tab / shift+tab   cycle panes",
		"  ctrl+t / ctrl+w   new / close tab",
		"  ctrl+→ / ctrl+←   next / previous tab",
		"  alt+1..9          jump to tab",
		"  ctrl+c            cancel running query, then quit",
		"  esc               cancel query / close overlay",
		"  x                 command menu (outside the editor)",
		"",
		"connections",
		"  a / e / d         add / edit / delete",
		"  enter             connect",
		"  /                 filter",
		"  t                 table dropdown (fetch with LIMIT 100)",
		"  i                 table info (in the table dropdown)",
		"  D                 view DBML for the connection",
		"",
		"query",
		"  ctrl+r / F5       execute",
		"  ctrl+space        autocomplete",
		"  ctrl+↑ / ctrl+↓   history",
		"  E (results)       explain plan",
		"",
		"results",
		"  ↑ / ↓ / h / l     navigate cells",
		"  v                 table → CSV → JSON",
		"  ctrl+e            export",
		"  e / ctrl+n / m    edit cell / NULL / row menu",
		"  p                 preview & apply changes",
		"  H / [ / ] / 0     hide / resize / reset columns",
		"  ctrl+z / ctrl+⇧z  revert row / all",
		"  r                 refresh after a conflict",
		"",
		"documents",
		"  esc / q / enter   close (table info, DBML)",
		"  c                 copy document to clipboard",
		"",
		faint.Render("esc closes this menu"),
	}
	var b strings.Builder
	for _, line := range sections {
		switch line {
		case "global", "connections", "query", "results", "documents":
			b.WriteString(section.Render(line))
		default:
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "enter", "q", "?":
		m.helpOpen = false
	}
	return m, nil
}
