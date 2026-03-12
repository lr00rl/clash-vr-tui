package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/app"
)

var version = "dev"

func main() {
	socketPath := flag.String("socket", api.DefaultSocketPath(), "mihomo Unix socket path")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("clash-vr-tui %s\n", version)
		os.Exit(0)
	}

	client := api.NewClient(*socketPath)
	model := app.NewModel(client)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
