// Package cli implements the non-interactive subcommands so common operations
// can be scripted over SSH without launching the full TUI.
package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/config"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

// command is one subcommand. The first name is canonical; the rest are
// aliases. Local commands run without resolving a controller.
type command struct {
	names   []string
	args    string
	summary string
	group   string
	local   bool
	run     func(c *api.Client, opts options) error
}

// commandGroups orders the help output.
var commandGroups = []string{"Find", "Inspect", "Live", "Control", "Diagnose"}

// commands is the registry. "help" is handled in run so this table does not
// refer to printUsage, which reads it.
var commands = []command{
	{names: []string{"discover", "list-socket", "sockets"}, group: "Find", local: true, run: cmdDiscover,
		summary: "find local mihomo controllers and show which one is used"},
	{names: []string{"status"}, group: "Inspect", run: cmdStatus,
		summary: "core version, endpoint, mode, ports, memory, active conns"},
	{names: []string{"proxies", "groups"}, group: "Inspect", run: cmdGroups,
		summary: "list groups with their current node"},
	{names: []string{"nodes"}, args: "<group>", group: "Inspect", run: cmdNodes,
		summary: "list nodes in a group with last delay"},
	{names: []string{"conns", "connections"}, group: "Inspect", run: cmdConns,
		summary: "list active connections"},
	{names: []string{"rules"}, args: "[--grep S]", group: "Inspect", run: cmdRules,
		summary: "list routing rules"},
	{names: []string{"providers"}, args: "[update [name...]]", group: "Inspect", run: cmdProviders,
		summary: "subscriptions and rule sets; update re-fetches them"},
	{names: []string{"config"}, args: "[set tun|lan on|off | set log-level L]", group: "Inspect", run: cmdConfig,
		summary: "show or change runtime config"},
	{names: []string{"logs", "log"}, args: "[--level L] [--grep S] [--for DUR]", group: "Live", run: cmdLogs,
		summary: "stream core logs live"},
	{names: []string{"switch"}, args: "<group> <node>", group: "Control", run: cmdSwitch,
		summary: "select a node in a group"},
	{names: []string{"test"}, args: "<group>", group: "Control", run: cmdTest,
		summary: "HTTP delay-test a group (--url, --timeout MS)"},
	{names: []string{"unpin"}, args: "<group>", group: "Control", run: cmdUnpin,
		summary: "return a URLTest/Fallback group to automatic selection"},
	{names: []string{"mode"}, args: "[rule|global|direct]", group: "Control", run: cmdMode,
		summary: "get or set the proxy mode"},
	{names: []string{"close"}, args: "<id>... | --all | --host S", group: "Control", run: cmdClose,
		summary: "close connections"},
	{names: []string{"flush"}, args: "dns|fakeip", group: "Control", run: cmdFlush,
		summary: "clear the core's DNS or fake-ip cache"},
	{names: []string{"restart"}, group: "Control", run: cmdRestart,
		summary: "restart the mihomo core"},
	{names: []string{"doctor", "autodebug"}, args: "[--logs DUR] [--level L] [--probe] [--out DIR]", group: "Diagnose", run: cmdDoctor,
		summary: "check the setup end to end and explain problems"},
}

func findCommand(name string) (command, bool) {
	for _, c := range commands {
		for _, n := range c.names {
			if n == name {
				return c, true
			}
		}
	}
	return command{}, false
}

// IsCommand reports whether s is a recognized subcommand or alias.
func IsCommand(s string) bool {
	_, ok := findCommand(s)
	return ok || s == "help"
}

type options struct {
	socket     string
	server     string
	secret     string
	secretFile string
	config     string
	json       bool
	url        string
	timeout    int
	level      string
	grep       string
	forDur     time.Duration
	all        bool
	host       string
	out        string
	logsDur    time.Duration // doctor log capture; negative means the default
	probe      bool
	resolved   config.Resolved
	args       []string // positional args after the subcommand
}

// usageError marks a command-line mistake; run exits 2 for it.
type usageError struct{ error }

func usagef(format string, a ...any) error { return usageError{fmt.Errorf(format, a...)} }

// Maybe runs a subcommand if argv contains one (after stripping flags).
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

	opts := options{timeout: 5000, logsDur: -1}
	valueFlags := map[string]func(string) error{
		"socket":      func(v string) error { opts.socket = v; return nil },
		"server":      func(v string) error { opts.server = v; return nil },
		"secret":      func(v string) error { opts.secret = v; return nil },
		"secret-file": func(v string) error { opts.secretFile = v; return nil },
		"config-path": func(v string) error { opts.config = v; return nil },
		"url":         func(v string) error { opts.url = v; return nil },
		"grep":        func(v string) error { opts.grep = v; return nil },
		"host":        func(v string) error { opts.host = v; return nil },
		"out":         func(v string) error { opts.out = v; return nil },
		"timeout":     func(v string) (err error) { opts.timeout, err = parseTimeout(v); return err },
		"level":       func(v string) (err error) { opts.level, err = parseLevel(v); return err },
		"for":         func(v string) (err error) { opts.forDur, err = parseDuration("--for", v); return err },
		"logs":        func(v string) (err error) { opts.logsDur, err = parseDuration("--logs", v); return err },
	}
	boolFlags := map[string]*bool{"json": &opts.json, "all": &opts.all, "probe": &opts.probe}

	var positional []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if !strings.HasPrefix(a, "-") || a == "-" {
			positional = append(positional, a)
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if b, ok := boolFlags[name]; ok && !hasValue {
			*b = true
			continue
		}
		set, ok := valueFlags[name]
		if !ok {
			return cliUsageError(fmt.Errorf("unknown option %s", a))
		}
		if !hasValue {
			var err error
			if value, i, err = flagValue(argv, i, a); err != nil {
				return cliUsageError(err)
			}
		}
		if err := set(value); err != nil {
			return cliUsageError(err)
		}
	}

	if len(positional) == 0 || !IsCommand(positional[0]) {
		return 0, false
	}
	opts.args = positional[1:]
	return run(positional[0], opts, version), true
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

// parseDuration accepts Go durations ("30s", "2m") or plain seconds ("30").
func parseDuration(flag, value string) (time.Duration, error) {
	if n, err := strconv.Atoi(value); err == nil && n >= 0 {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s wants a duration such as 30s or 2m", flag)
	}
	return d, nil
}

// logLevels are the levels mihomo's /logs endpoint accepts, most verbose first.
var logLevels = []string{"debug", "info", "warning", "error"}

func parseLevel(value string) (string, error) {
	value = strings.ToLower(value)
	if value == "warn" {
		value = "warning"
	}
	for _, l := range logLevels {
		if l == value {
			return value, nil
		}
	}
	return "", fmt.Errorf("--level must be one of %s", strings.Join(logLevels, ", "))
}

func cliUsageError(err error) (int, bool) {
	fmt.Fprintf(os.Stderr, "error: %s\n", styles.SafeText(err.Error()))
	return 2, true
}

func run(name string, opts options, version string) int {
	if name == "help" {
		printUsage(version)
		return 0
	}
	cmd, _ := findCommand(name)
	if cmd.local {
		return exitCode(cmd.run(nil, opts), config.Resolved{})
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
	opts.resolved = resolved
	return exitCode(cmd.run(api.NewWith(resolved.Endpoint), opts), resolved)
}

// exitCode prints err and maps it to an exit status. When the endpoint was a
// guess and nothing answered, it points at discover.
func exitCode(err error, resolved config.Resolved) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(os.Stderr, "error: %s\n", styles.SafeText(err.Error()))
	if errors.As(err, new(usageError)) {
		return 2
	}
	var se *api.StatusError
	if resolved.Source == "default path" && !errors.As(err, &se) {
		fmt.Fprintln(os.Stderr, "hint: no running controller was found; run `clash-vr-tui discover` to see what was checked")
	}
	return 1
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

	out := map[string]any{"version": ver.Version, "meta": ver.Meta,
		"endpoint": map[string]string{"address": c.SocketPath(), "source": opts.resolved.Source}}
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
	fmt.Printf("Endpoint:    %s\n", styles.SafeText(endpointLabel(opts.resolved)))
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
	var b strings.Builder
	fmt.Fprintf(&b, "clash-vr-tui %s: terminal UI and CLI for the mihomo (Clash Verge Rev) core\n\n", version)
	b.WriteString("USAGE:\n")
	b.WriteString("  clash-vr-tui [--socket PATH | --server H:P]   launch the interactive TUI\n")
	b.WriteString("  clash-vr-tui <command> [args] [--json]         run a one-off command\n")
	for _, group := range commandGroups {
		fmt.Fprintf(&b, "\n%s:\n", strings.ToUpper(group))
		for _, c := range commands {
			if c.group != group {
				continue
			}
			usage := strings.TrimSpace(c.names[0] + " " + c.args)
			line := fmt.Sprintf("  %-34s %s", usage, c.summary)
			if len(usage) > 34 {
				line = fmt.Sprintf("  %s\n  %-34s %s", usage, "", c.summary)
			}
			if len(c.names) > 1 {
				line += " (alias: " + strings.Join(c.names[1:], ", ") + ")"
			}
			b.WriteString(line + "\n")
		}
	}
	fmt.Fprintf(&b, `
GLOBAL FLAGS:
  --socket PATH    mihomo Unix socket
  --server H:P     external controller host:port (loopback when a secret is set)
  --secret S       external controller secret
  --secret-file P  read external controller secret from file
  --config-path P  mihomo config path for TCP/ICMP probes
  --json           machine-readable output (NDJSON for logs)

The controller resolves from flags > env (CLASH_VR_TUI_SOCKET/_SERVER/_SECRET/
_SECRET_FILE) > config file (%s) > auto-detect > %s.
Auto-detect checks running cores, their config files, and Clash Verge Rev paths.

EXAMPLES:
  clash-vr-tui discover
  clash-vr-tui logs --level warning --grep timeout
  clash-vr-tui doctor --logs 30s --out ~/cvt-report
  clash-vr-tui switch Proxy 'JP-Tokyo-01'
  clash-vr-tui config set tun off
  clash-vr-tui status --json | jq .mode
`, config.Path(), api.DefaultSocketPath())
	fmt.Print(b.String())
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
