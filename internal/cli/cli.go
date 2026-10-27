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
	"github.com/cdcd/clash-vr-tui/internal/config"
	"github.com/cdcd/clash-vr-tui/internal/styles"
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
	socket     string
	server     string
	secret     string
	secretFile string
	config     string
	json       bool
	url        string
	timeout    int
	args       []string // positional args after the subcommand
}

// Maybe runs a subcommand if argv begins with one (after stripping flags).
// Returns the process exit code and whether a subcommand was handled.
func Maybe(argv []string, version string) (int, bool) {
	hasCommand := false
	for _, arg := range argv {
		if IsCommand(arg) {
			hasCommand = true
			break
		}
	}
	if !hasCommand {
		return 0, false
	}

	opts := options{timeout: 5000}
	var positional []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--json" || a == "-json":
			opts.json = true
		case a == "--socket" || a == "-socket":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.socket = value
		case strings.HasPrefix(a, "--socket="):
			opts.socket = strings.TrimPrefix(a, "--socket=")
		case a == "--server" || a == "-server":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.server = value
		case strings.HasPrefix(a, "--server="):
			opts.server = strings.TrimPrefix(a, "--server=")
		case a == "--secret" || a == "-secret":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.secret = value
		case strings.HasPrefix(a, "--secret="):
			opts.secret = strings.TrimPrefix(a, "--secret=")
		case a == "--secret-file" || a == "-secret-file":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.secretFile = value
		case strings.HasPrefix(a, "--secret-file="):
			opts.secretFile = strings.TrimPrefix(a, "--secret-file=")
		case a == "--config-path" || a == "-config-path":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.config = value
		case strings.HasPrefix(a, "--config-path="):
			opts.config = strings.TrimPrefix(a, "--config-path=")
		case a == "--url" || a == "-url":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.url = value
		case strings.HasPrefix(a, "--url="):
			opts.url = strings.TrimPrefix(a, "--url=")
		case a == "--timeout" || a == "-timeout":
			value, next, err := flagValue(argv, i, a)
			if err != nil {
				return cliUsageError(err)
			}
			i = next
			opts.timeout, err = parseTimeout(value)
			if err != nil {
				return cliUsageError(err)
			}
		case strings.HasPrefix(a, "--timeout="):
			var err error
			opts.timeout, err = parseTimeout(strings.TrimPrefix(a, "--timeout="))
			if err != nil {
				return cliUsageError(err)
			}
		case strings.HasPrefix(a, "-"):
			return cliUsageError(fmt.Errorf("unknown option %s", a))
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

// flagValue takes the next argument as the value even when it starts with "-",
// since generated secrets can.
func flagValue(argv []string, i int, name string) (string, int, error) {
	if i+1 >= len(argv) {
		return "", i, fmt.Errorf("%s requires a value", name)
	}
	return argv[i+1], i + 1, nil
}

func parseTimeout(value string) (int, error) {
	timeout, err := strconv.Atoi(value)
	if err != nil || timeout < 1 || timeout > 600000 {
		return 0, fmt.Errorf("timeout must be an integer from 1 to 600000 milliseconds")
	}
	return timeout, nil
}

func cliUsageError(err error) (int, bool) {
	fmt.Fprintf(os.Stderr, "error: %s\n", styles.SafeText(err.Error()))
	return 2, true
}

func run(cmd string, opts options, version string) int {
	if cmd == "help" {
		printUsage(version)
		return 0
	}
	resolved, err := config.Resolve(config.Flags{
		Socket:     opts.socket,
		Server:     opts.server,
		Secret:     opts.secret,
		SecretFile: opts.secretFile,
		ConfigPath: opts.config,
		TestURL:    opts.url,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", styles.SafeText(err.Error()))
		return 2
	}
	if err := api.ValidateEndpoint(resolved.Endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", styles.SafeText(err.Error()))
		return 2
	}
	if opts.url == "" {
		opts.url = resolved.TestURL
	}
	c := api.NewWith(resolved.Endpoint)
	err = nil
	switch cmd {
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
		fmt.Fprintf(os.Stderr, "error: %s\n", styles.SafeText(err.Error()))
		return 1
	}
	return 0
}

func cmdStatus(c *api.Client, opts options) error {
	ver, err := c.GetVersion()
	if err != nil {
		return fmt.Errorf("core unreachable at %s: %w", c.SocketPath(), err)
	}
	cfg, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("get runtime config: %w", err)
	}
	if cfg == nil {
		return fmt.Errorf("get runtime config: empty response")
	}
	snap, err := c.GetConnections()
	if err != nil {
		return fmt.Errorf("get connections: %w", err)
	}
	rules, err := c.GetRules()
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}

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
	fmt.Printf("Core:        mihomo %s (meta=%v)\n", styles.SafeText(ver.Version), ver.Meta)
	if cfg != nil {
		fmt.Printf("Mode:        %s\n", styles.SafeText(cfg.Mode))
		fmt.Printf("Mixed Port:  %d\n", cfg.MixedPort)
		fmt.Printf("Allow LAN:   %v\n", cfg.AllowLan)
		fmt.Printf("TUN:         %v\n", cfg.TUN.Enable)
		fmt.Printf("Log Level:   %s\n", styles.SafeText(cfg.LogLevel))
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
	groups = visibleGroups(groups)
	if opts.json {
		return printJSON(groups)
	}
	w := 0
	for _, g := range groups {
		if name := styles.SafeText(g.Name); len(name) > w {
			w = len(name)
		}
	}
	for _, g := range groups {
		fmt.Printf("%-*s  %-10s -> %-28s (%d nodes)\n", w,
			styles.SafeText(g.Name), styles.SafeText(g.Type), styles.SafeText(g.Now), len(g.All))
	}
	return nil
}

func visibleGroups(groups []api.Group) []api.Group {
	result := make([]api.Group, 0, len(groups))
	for _, g := range groups {
		if g.Hidden || g.Name == "GLOBAL" {
			continue
		}
		result = append(result, g)
	}
	return result
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
	full, err := c.GetProxies()
	if err != nil {
		return fmt.Errorf("get proxy delay history: %w", err)
	}
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
		fmt.Printf("%s%-40s %s\n", marker, truncate(styles.SafeText(n.Name), 40), delayStr(n.Delay))
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
	fmt.Printf("Switched %s -> %s\n", styles.SafeText(group), styles.SafeText(node))
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
		fmt.Printf("%-40s %s\n", truncate(styles.SafeText(it.Name), 40), delayStr(it.Delay))
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
		fmt.Println(styles.SafeText(cfg.Mode))
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
		fmt.Printf("  %-45s %-20s %s\n",
			truncate(styles.SafeText(host), 45),
			truncate(styles.SafeText(chain), 20),
			styles.SafeText(conn.Rule))
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
  --server H:P     external controller host:port (instead of socket)
  --secret S       external controller secret
  --secret-file P  read external controller secret from file
  --config-path P  mihomo config path for TCP/ICMP probes
  --json           machine-readable JSON output
  --url URL        test URL (test command)
  --timeout MS     delay-test timeout in milliseconds

Connection resolves from flags > env (CLASH_VR_TUI_SOCKET/_SERVER/_SECRET,
CLASH_VR_TUI_CONFIG_PATH, CLASH_VR_TUI_TEST_URL) > config file (%s) > defaults.

EXAMPLES:
  clash-vr-tui status --json
  clash-vr-tui switch for-test-ip 'JP-Tokyo-01'
  clash-vr-tui test for-test-ip --timeout 3000
  clash-vr-tui mode global
  clash-vr-tui status --server 127.0.0.1:9090 --secret mypw
`, version, config.Path())
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
