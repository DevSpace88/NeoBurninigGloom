package main

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"NeoBurningGoom/internal/tui"
)

func main() {
	app := tui.New()
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		os.Exit(1)
	}
}
