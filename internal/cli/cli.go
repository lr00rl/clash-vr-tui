// Package cli implements the non-interactive subcommands so common operations
// can be scripted over SSH without launching the full TUI.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// commands is the set of recognized subcommands.
var commands = map[string]bool{
	"status": true, "proxies": true, "groups": true, "nodes": true,
	"switch": true, "test": true, "mode": true, "restart": true,
	"conns": true, "connections": true, "help": true,
}

// IsCommand reports whether s is a recognized subcommand.
func IsCommand(s string) bool { return commands[s] }

type options struct {
	socket  string
	json    bool
	url     string
	timeout int
	args    []string // positional args after the subcommand
}

// Maybe runs a subcommand if argv begins with one (after stripping flags).
// Returns the process exit code and whether a subcommand was handled.
func Maybe(argv []string, defaultSocket, version string) (int, bool) {
	opts := options{socket: defaultSocket, timeout: 5000}
	var positional []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--json" || a == "-json":
			opts.json = true
		case a == "--socket" || a == "-socket":
			if i+1 < len(argv) {
				i++
				opts.socket = argv[i]
			}
		case strings.HasPrefix(a, "--socket="):
			opts.socket = strings.TrimPrefix(a, "--socket=")
		case a == "--url" || a == "-url":
			if i+1 < len(argv) {
				i++
				opts.url = argv[i]
			}
		case strings.HasPrefix(a, "--url="):
			opts.url = strings.TrimPrefix(a, "--url=")
		case a == "--timeout" || a == "-timeout":
			if i+1 < len(argv) {
				i++
				opts.timeout, _ = strconv.Atoi(argv[i])
			}
		case strings.HasPrefix(a, "--timeout="):
			opts.timeout, _ = strconv.Atoi(strings.TrimPrefix(a, "--timeout="))
		default:
			positional = append(positional, a)
		}
	}

	if len(positional) == 0 || !IsCommand(positional[0]) {
		return 0, false
	}
	cmd := positional[0]
	opts.args = positional[1:]
	return run(cmd, opts, version), true
}

func run(cmd string, opts options, version string) int {
	c := api.NewClient(opts.socket)
	var err error
	switch cmd {
	case "help":
		printUsage(version)
	case "status":
		err = cmdStatus(c, opts)
	case "proxies", "groups":
		err = cmdGroups(c, opts)
	case "nodes":
		err = cmdNodes(c, opts)
	case "switch":
		err = cmdSwitch(c, opts)
	case "test":
		err = cmdTest(c, opts)
	case "mode":
		err = cmdMode(c, opts)
	case "restart":
		err = cmdRestart(c, opts)
	case "conns", "connections":
		err = cmdConns(c, opts)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	return 0
}

func cmdStatus(c *api.Client, opts options) error {
	ver, err := c.GetVersion()
	if err != nil {
		return fmt.Errorf("core unreachable at %s: %w", c.SocketPath(), err)
	}
	cfg, _ := c.GetConfig()
	snap, _ := c.GetConnections()
	rules, _ := c.GetRules()

	out := map[string]any{"version": ver.Version, "meta": ver.Meta}
	if cfg != nil {
		out["mode"] = cfg.Mode
		out["mixed_port"] = cfg.MixedPort
		out["allow_lan"] = cfg.AllowLan
		out["log_level"] = cfg.LogLevel
		out["tun"] = cfg.TUN.Enable
	}
	if snap != nil {
		out["memory"] = snap.Memory
		out["active_conns"] = len(snap.Connections)
		out["download_total"] = snap.DownloadTotal
		out["upload_total"] = snap.UploadTotal
	}
	if rules != nil {
		out["rules"] = len(rules.Rules)
	}
	if opts.json {
		return printJSON(out)
	}
	fmt.Printf("Core:        mihomo %s (meta=%v)\n", ver.Version, ver.Meta)
	if cfg != nil {
		fmt.Printf("Mode:        %s\n", cfg.Mode)
		fmt.Printf("Mixed Port:  %d\n", cfg.MixedPort)
		fmt.Printf("Allow LAN:   %v\n", cfg.AllowLan)
		fmt.Printf("TUN:         %v\n", cfg.TUN.Enable)
		fmt.Printf("Log Level:   %s\n", cfg.LogLevel)
	}
	if snap != nil {
		fmt.Printf("Memory:      %s\n", humanBytes(snap.Memory))
		fmt.Printf("Active:      %d connections\n", len(snap.Connections))
	}
	if rules != nil {
		fmt.Printf("Rules:       %d\n", len(rules.Rules))
	}
	return nil
}

func cmdGroups(c *api.Client, opts options) error {
	groups, err := c.GetGroups()
	if err != nil {
		return err
	}
	if opts.json {
		return printJSON(groups)
	}
	w := 0
	for _, g := range groups {
		if len(g.Name) > w {
			w = len(g.Name)
		}
	}
	for _, g := range groups {
		fmt.Printf("%-*s  %-10s -> %-28s (%d nodes)\n", w, g.Name, g.Type, g.Now, len(g.All))
	}
	return nil
}

func cmdNodes(c *api.Client, opts options) error {
	if len(opts.args) < 1 {
		return fmt.Errorf("usage: nodes <group>")
	}
	group := opts.args[0]
	g, err := c.GetProxy(group)
	if err != nil {
		return err
	}
	type nd struct {
		Name  string `json:"name"`
		Delay int    `json:"delay"`
	}
	full, _ := c.GetProxies()
	nodes := make([]nd, 0, len(g.All))
	for _, name := range g.All {
		delay := 0
		if full != nil {
			if p, ok := full.Proxies[name]; ok && len(p.History) > 0 {
				delay = p.History[len(p.History)-1].Delay
			}
		}
		nodes = append(nodes, nd{Name: name, Delay: delay})
	}
	if opts.json {
		return printJSON(map[string]any{"group": group, "now": g.Now, "nodes": nodes})
	}
	for _, n := range nodes {
		marker := "  "
		if n.Name == g.Now {
			marker = "* "
		}
		fmt.Printf("%s%-40s %s\n", marker, truncate(n.Name, 40), delayStr(n.Delay))
	}
	return nil
}

func cmdSwitch(c *api.Client, opts options) error {
	if len(opts.args) < 2 {
		return fmt.Errorf("usage: switch <group> <node>")
	}
	group, node := opts.args[0], opts.args[1]
	if err := c.SelectProxy(group, node); err != nil {
		return err
	}
	if opts.json {
		return printJSON(map[string]string{"group": group, "selected": node})
	}
	fmt.Printf("Switched %s -> %s\n", group, node)
	return nil
}

func cmdTest(c *api.Client, opts options) error {
	if len(opts.args) < 1 {
		return fmt.Errorf("usage: test <group> [--url URL] [--timeout MS]")
	}
	group := opts.args[0]
	result, err := c.TestGroupDelay(group, opts.url, opts.timeout)
	if err != nil {
		return err
	}
	type kv struct {
		Name  string `json:"name"`
		Delay int    `json:"delay"`
	}
	items := make([]kv, 0, len(result))
	for name, d := range result {
		items = append(items, kv{name, d})
	}
	sort.Slice(items, func(i, j int) bool {
		di, dj := items[i].Delay, items[j].Delay
		if di <= 0 {
			di = 1 << 30
		}
		if dj <= 0 {
			dj = 1 << 30
		}
		return di < dj
	})
	if opts.json {
		return printJSON(map[string]any{"group": group, "results": items})
	}
	for _, it := range items {
		fmt.Printf("%-40s %s\n", truncate(it.Name, 40), delayStr(it.Delay))
	}
	return nil
}

func cmdMode(c *api.Client, opts options) error {
	if len(opts.args) == 0 {
		cfg, err := c.GetConfig()
		if err != nil {
			return err
		}
		if opts.json {
			return printJSON(map[string]string{"mode": cfg.Mode})
		}
		fmt.Println(cfg.Mode)
		return nil
	}
	mode := strings.ToLower(opts.args[0])
	if mode != "rule" && mode != "global" && mode != "direct" {
		return fmt.Errorf("mode must be rule, global, or direct")
	}
	if err := c.PatchConfig(api.ConfigPatch{Mode: &mode}); err != nil {
		return err
	}
	if !opts.json {
		fmt.Printf("Mode set to %s\n", mode)
	}
	return nil
}

func cmdRestart(c *api.Client, opts options) error {
	if err := c.RestartCore(); err != nil {
		return err
	}
	if !opts.json {
		fmt.Println("Core restart requested")
	}
	return nil
}

func cmdConns(c *api.Client, opts options) error {
	snap, err := c.GetConnections()
	if err != nil {
		return err
	}
	if opts.json {
		return printJSON(snap)
	}
	fmt.Printf("%d active connections  (DL %s / UL %s)\n",
		len(snap.Connections), humanBytes(snap.DownloadTotal), humanBytes(snap.UploadTotal))
	for _, conn := range snap.Connections {
		host := conn.Metadata.Host
		if host == "" {
			host = conn.Metadata.DstIP
		}
		if conn.Metadata.DstPort != "" {
			host += ":" + conn.Metadata.DstPort
		}
		chain := ""
		if len(conn.Chains) > 0 {
			chain = conn.Chains[0]
		}
		fmt.Printf("  %-45s %-20s %s\n", truncate(host, 45), truncate(chain, 20), conn.Rule)
	}
	return nil
}

func printUsage(version string) {
	fmt.Printf(`clash-vr-tui %s — terminal UI and CLI for the mihomo (Clash Verge Rev) core

USAGE:
  clash-vr-tui [--socket PATH]                 launch the interactive TUI
  clash-vr-tui <command> [args] [--json]       run a one-off command

COMMANDS:
  status                     core version, mode, ports, memory, active conns
  proxies | groups           list groups with their current node
  nodes <group>              list nodes in a group with last delay
  switch <group> <node>      select a node in a group
  test <group>               HTTP delay-test a group (--url, --timeout MS)
  mode [rule|global|direct]  get or set the proxy mode
  restart                    restart the mihomo core
  conns                      list active connections
  help                       show this help

GLOBAL FLAGS:
  --socket PATH    mihomo Unix socket (default: platform path)
  --json           machine-readable JSON output
  --url URL        test URL (test command)
  --timeout MS     delay-test timeout in milliseconds

EXAMPLES:
  clash-vr-tui status --json
  clash-vr-tui switch Proxy 'JP-Tokyo-01'
  clash-vr-tui test Proxy --timeout 3000
  clash-vr-tui mode global
`, version)
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func delayStr(d int) string {
	if d <= 0 {
		return "timeout"
	}
	return strconv.Itoa(d) + "ms"
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}
