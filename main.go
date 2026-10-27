package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/app"
	"github.com/cdcd/clash-vr-tui/internal/cli"
	"github.com/cdcd/clash-vr-tui/internal/config"
	"github.com/cdcd/clash-vr-tui/internal/probe"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

var version = "dev"

func main() {
	// If the args contain a known subcommand, run it non-interactively.
	if code, handled := cli.Maybe(os.Args[1:], version); handled {
		os.Exit(code)
	}

	socketPath := flag.String("socket", "", "mihomo Unix socket path")
	server := flag.String("server", "", "external controller host:port (instead of socket)")
	secret := flag.String("secret", "", "external controller secret")
	secretFile := flag.String("secret-file", "", "read external controller secret from file")
	configPath := flag.String("config-path", "", "mihomo config path for TCP/ICMP node probes")
	showVersion := flag.Bool("version", false, "show version")
	flag.Parse()
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "Error: unknown command %s\n", styles.SafeText(flag.Arg(0)))
		os.Exit(2)
	}

	if *showVersion {
		fmt.Printf("clash-vr-tui %s\n", version)
		os.Exit(0)
	}

	resolved, err := config.Resolve(config.Flags{
		Socket:     *socketPath,
		Server:     *server,
		Secret:     *secret,
		SecretFile: *secretFile,
		ConfigPath: *configPath,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", styles.SafeText(err.Error()))
		os.Exit(2)
	}
	if err := api.ValidateEndpoint(resolved.Endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", styles.SafeText(err.Error()))
		os.Exit(2)
	}
	if resolved.ConfigPath != "" {
		probe.SetDefaultConfigPath(resolved.ConfigPath)
	}

	client := api.NewWith(resolved.Endpoint)
	model := app.NewModel(client, resolved.TestURL)

	p := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
