package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/app"
)

func main() {
	if _, err := tea.NewProgram(app.New()).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sql-tui: %v\n", err)
		os.Exit(1)
	}
}
