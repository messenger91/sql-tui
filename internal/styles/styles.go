// Package styles holds the Lip Gloss theme shared by all panes. Themes are
// values, not globals, so tests and future light/dark modes stay isolated.
package styles

import "github.com/charmbracelet/lipgloss"

// Theme is the full set of styles used by the app shell and its panes.
type Theme struct {
	Pane        lipgloss.Style
	PaneFocused lipgloss.Style
	PaneTitle   lipgloss.Style
	Status      lipgloss.Style
	Help        lipgloss.Style
	Modal       lipgloss.Style
	ModalTitle  lipgloss.Style
	Dim         lipgloss.Style
	Error       lipgloss.Style
}

// Default returns the default dark-terminal theme.
func Default() Theme {
	accent := lipgloss.Color("62")
	muted := lipgloss.Color("240")
	border := lipgloss.RoundedBorder()

	pane := lipgloss.NewStyle().
		Border(border).
		BorderForeground(muted).
		Padding(0, 1)
	focused := pane.BorderForeground(accent)

	return Theme{
		Pane:        pane,
		PaneFocused: focused,
		PaneTitle:   lipgloss.NewStyle().Bold(true).Foreground(accent),
		Status:      lipgloss.NewStyle().Foreground(muted),
		Help:        lipgloss.NewStyle().Foreground(muted),
		Modal: lipgloss.NewStyle().
			Border(border).
			BorderForeground(accent).
			Padding(1, 2),
		ModalTitle: lipgloss.NewStyle().Bold(true).Foreground(accent),
		Dim:        lipgloss.NewStyle().Faint(true),
		Error:      lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
	}
}
