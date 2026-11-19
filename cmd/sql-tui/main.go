package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/messenger91/sql-tui/internal/app"
)

func main() {
	model := app.New()
	program := tea.NewProgram(model)
	model.SetSender(program.Send)
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "sql-tui: %v\n", err)
		os.Exit(1)
	}
	if err := model.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "sql-tui: close databases: %v\n", err)
	}
}
