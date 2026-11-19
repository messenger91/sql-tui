package main

import (
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var quitKey = key.NewBinding(
	key.WithKeys("q", "ctrl+c"),
	key.WithHelp("q", "quit"),
)

// placeholderModel is a temporary bootstrap model. It is replaced by the
// root application model in internal/app (§3).
type placeholderModel struct{}

func (placeholderModel) Init() tea.Cmd { return nil }

func (m placeholderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok && key.Matches(msg, quitKey) {
		return m, tea.Quit
	}
	return m, nil
}

func (placeholderModel) View() string {
	body := lipgloss.NewStyle().Padding(1, 2).Render("sql-tui\n\nscaffolding only")
	footer := lipgloss.NewStyle().Faint(true).Padding(0, 2).Render(
		quitKey.Help().Key + " " + quitKey.Help().Desc,
	)
	return lipgloss.JoinVertical(lipgloss.Left, body, footer)
}

func main() {
	if _, err := tea.NewProgram(placeholderModel{}).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sql-tui: %v\n", err)
		os.Exit(1)
	}
}
