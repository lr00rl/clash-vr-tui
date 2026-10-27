package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

// now is replaced in tests so relative dates are stable.
var now = time.Now

func newTable() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0) }

func cmdRules(c *api.Client, opts options) error {
	resp, err := c.GetRules()
	if err != nil {
		return fmt.Errorf("get rules: %w", err)
	}
	grep := strings.ToLower(opts.grep)
	type row struct {
		Index   int    `json:"index"`
		Type    string `json:"type"`
		Payload string `json:"payload"`
		Proxy   string `json:"proxy"`
	}
	var rows []row
	for i, r := range resp.Rules {
		if grep != "" && !strings.Contains(strings.ToLower(r.Type+" "+r.Payload+" "+r.Proxy), grep) {
			continue
		}
		rows = append(rows, row{Index: i + 1, Type: r.Type, Payload: r.Payload, Proxy: r.Proxy})
	}
	if opts.json {
		return printJSON(rows)
	}
	w := newTable()
	fmt.Fprintln(w, "#\tTYPE\tPAYLOAD\tTARGET")
	for _, r := range rows {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", r.Index, styles.SafeText(r.Type), styles.SafeText(truncate(r.Payload, 60)), styles.SafeText(r.Proxy))
	}
	w.Flush()
	fmt.Fprintf(os.Stderr, "%d of %d rules\n", len(rows), len(resp.Rules))
	return nil
}

type proxyProviderRow struct {
	Name      string     `json:"name"`
	Vehicle   string     `json:"vehicle"`
	Nodes     int        `json:"nodes"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
	Used      int64      `json:"used_bytes,omitempty"`
	Total     int64      `json:"total_bytes,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type ruleProviderRow struct {
	Name      string     `json:"name"`
	Behavior  string     `json:"behavior"`
	Vehicle   string     `json:"vehicle"`
	Rules     int        `json:"rules"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
}

func cmdProviders(c *api.Client, opts options) error {
	if len(opts.args) > 0 {
		if opts.args[0] != "update" {
			return usagef("usage: providers [update [name...]]")
		}
		return updateProviders(c, opts.args[1:])
	}
	proxies, err := c.GetProxyProviders()
	if err != nil {
		return fmt.Errorf("get proxy providers: %w", err)
	}
	rules, err := c.GetRuleProviders()
	if err != nil {
		return fmt.Errorf("get rule providers: %w", err)
	}

	var prows []proxyProviderRow
	for _, p := range proxies {
		if p.IsGroup() {
			continue
		}
		r := proxyProviderRow{Name: p.Name, Vehicle: p.VehicleType, Nodes: len(p.Proxies), UpdatedAt: knownTime(p.UpdatedAt)}
		if si := p.SubscriptionInfo; si != nil {
			r.Used, r.Total = si.Upload+si.Download, si.Total
			if si.Expire > 0 {
				t := time.Unix(si.Expire, 0)
				r.ExpiresAt = &t
			}
		}
		prows = append(prows, r)
	}
	sort.Slice(prows, func(i, j int) bool { return prows[i].Name < prows[j].Name })
	var rrows []ruleProviderRow
	for _, p := range rules {
		rrows = append(rrows, ruleProviderRow{Name: p.Name, Behavior: p.Behavior, Vehicle: p.VehicleType, Rules: p.RuleCount, UpdatedAt: knownTime(p.UpdatedAt)})
	}
	sort.Slice(rrows, func(i, j int) bool { return rrows[i].Name < rrows[j].Name })

	if opts.json {
		return printJSON(map[string]any{"proxy_providers": prows, "rule_providers": rrows})
	}
	fmt.Println("Proxy providers")
	if len(prows) == 0 {
		fmt.Println("  none: nodes are defined inline in the profile")
	} else {
		w := newTable()
		fmt.Fprintln(w, "  NAME\tTYPE\tNODES\tUPDATED\tUSAGE\tEXPIRES")
		for _, r := range prows {
			fmt.Fprintf(w, "  %s\t%s\t%d\t%s\t%s\t%s\n", styles.SafeText(r.Name), r.Vehicle, r.Nodes, ago(r.UpdatedAt), usage(r.Used, r.Total), expiry(r.ExpiresAt))
		}
		w.Flush()
	}
	fmt.Println("\nRule providers")
	if len(rrows) == 0 {
		fmt.Println("  none")
		return nil
	}
	w := newTable()
	fmt.Fprintln(w, "  NAME\tBEHAVIOR\tTYPE\tRULES\tUPDATED")
	for _, r := range rrows {
		fmt.Fprintf(w, "  %s\t%s\t%s\t%d\t%s\n", styles.SafeText(r.Name), styles.SafeText(r.Behavior), r.Vehicle, r.Rules, ago(r.UpdatedAt))
	}
	return w.Flush()
}

// updateProviders re-fetches the named providers, or every HTTP provider when
// none are named. Each result is printed; any failure fails the command.
func updateProviders(c *api.Client, names []string) error {
	proxies, err := c.GetProxyProviders()
	if err != nil {
		return fmt.Errorf("get proxy providers: %w", err)
	}
	rules, err := c.GetRuleProviders()
	if err != nil {
		return fmt.Errorf("get rule providers: %w", err)
	}
	type target struct {
		name   string
		update func(string) error
	}
	var targets []target
	if len(names) == 0 {
		for _, p := range proxies {
			if p.VehicleType == "HTTP" {
				targets = append(targets, target{p.Name, c.UpdateProxyProvider})
			}
		}
		for _, p := range rules {
			if p.VehicleType == "HTTP" {
				targets = append(targets, target{p.Name, c.UpdateRuleProvider})
			}
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].name < targets[j].name })
		if len(targets) == 0 {
			fmt.Println("No HTTP providers to update.")
			return nil
		}
	}
	for _, n := range names {
		if p, ok := proxies[n]; ok && !p.IsGroup() {
			targets = append(targets, target{n, c.UpdateProxyProvider})
		} else if _, ok := rules[n]; ok {
			targets = append(targets, target{n, c.UpdateRuleProvider})
		} else {
			return usagef("no provider named %q; run `clash-vr-tui providers` to list them", n)
		}
	}
	failed := 0
	for _, t := range targets {
		if err := t.update(t.name); err != nil {
			failed++
			fmt.Printf("failed   %s: %s\n", styles.SafeText(t.name), styles.SafeText(styles.RedactURLs(err.Error())))
			continue
		}
		fmt.Printf("updated  %s\n", styles.SafeText(t.name))
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d providers failed to update", failed, len(targets))
	}
	return nil
}

func cmdConfig(c *api.Client, opts options) error {
	if len(opts.args) > 0 {
		if opts.args[0] != "set" || len(opts.args) != 3 {
			return usagef("usage: config set tun|lan on|off, or config set log-level %s", strings.Join(logLevels, "|"))
		}
		return setConfig(c, opts.args[1], opts.args[2])
	}
	cfg, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("get runtime config: %w", err)
	}
	// Built by hand so the Authentication credentials never reach output.
	out := map[string]any{
		"mode": cfg.Mode, "mixed_port": cfg.MixedPort, "port": cfg.Port, "socks_port": cfg.SocksPort,
		"allow_lan": cfg.AllowLan, "bind_address": cfg.BindAddress, "log_level": cfg.LogLevel,
		"tun":          map[string]any{"enable": cfg.TUN.Enable, "stack": cfg.TUN.Stack, "device": cfg.TUN.Device},
		"inbound_auth": len(cfg.Authentication) > 0,
	}
	if opts.json {
		return printJSON(out)
	}
	w := newTable()
	fmt.Fprintf(w, "Mode\t%s\n", styles.SafeText(cfg.Mode))
	fmt.Fprintf(w, "Mixed port\t%d\n", cfg.MixedPort)
	if cfg.Port > 0 || cfg.SocksPort > 0 {
		fmt.Fprintf(w, "HTTP / SOCKS port\t%d / %d\n", cfg.Port, cfg.SocksPort)
	}
	lan := onOff(cfg.AllowLan)
	if cfg.AllowLan {
		lan += fmt.Sprintf(" (bind %s, inbound auth %s)", orDash(cfg.BindAddress), onOff(len(cfg.Authentication) > 0))
	}
	fmt.Fprintf(w, "Allow LAN\t%s\n", styles.SafeText(lan))
	tun := onOff(cfg.TUN.Enable)
	if cfg.TUN.Enable {
		tun += fmt.Sprintf(" (stack %s, device %s)", orDash(cfg.TUN.Stack), orDash(cfg.TUN.Device))
	}
	fmt.Fprintf(w, "TUN\t%s\n", styles.SafeText(tun))
	fmt.Fprintf(w, "Log level\t%s\n", styles.SafeText(cfg.LogLevel))
	return w.Flush()
}

// setConfig patches one setting and reads the config back, failing when the
// core did not keep the value (for example TUN without the needed privileges).
func setConfig(c *api.Client, key, value string) error {
	var patch api.ConfigPatch
	var held func(*api.Config) bool
	switch key {
	case "tun", "lan":
		on, err := parseOnOff(value)
		if err != nil {
			return err
		}
		if key == "tun" {
			patch.TUN = &api.TUNConfig{Enable: on}
			held = func(cfg *api.Config) bool { return cfg.TUN.Enable == on }
		} else {
			patch.AllowLan = &on
			held = func(cfg *api.Config) bool { return cfg.AllowLan == on }
		}
	case "log-level":
		level, err := parseLevel(value)
		if err != nil {
			return usageError{err}
		}
		patch.LogLevel = &level
		held = func(cfg *api.Config) bool { return strings.EqualFold(cfg.LogLevel, level) }
	default:
		return usagef("unknown setting %q: want tun, lan, or log-level", key)
	}
	if err := c.PatchConfig(patch); err != nil {
		return fmt.Errorf("set %s: %w", key, err)
	}
	cfg, err := c.GetConfig()
	if err != nil {
		return fmt.Errorf("set %s was sent but reading it back failed: %w", key, err)
	}
	if !held(cfg) {
		return fmt.Errorf("the core did not keep %s %s; check its log with `clash-vr-tui logs --level warning`", key, value)
	}
	fmt.Printf("%s is now %s\n", key, value)
	return nil
}

func parseOnOff(v string) (bool, error) {
	switch strings.ToLower(v) {
	case "on", "true", "yes", "1":
		return true, nil
	case "off", "false", "no", "0":
		return false, nil
	}
	return false, usagef("want on or off, got %q", v)
}

func cmdUnpin(c *api.Client, opts options) error {
	if len(opts.args) != 1 {
		return usagef("usage: unpin <group>")
	}
	if err := c.UnfixProxy(opts.args[0]); err != nil {
		return fmt.Errorf("unpin %s: %w", opts.args[0], err)
	}
	fmt.Printf("%s returns to automatic selection\n", styles.SafeText(opts.args[0]))
	return nil
}

func cmdClose(c *api.Client, opts options) error {
	switch {
	case opts.all:
		if len(opts.args) > 0 || opts.host != "" {
			return usagef("--all closes every connection; do not combine it with ids or --host")
		}
		if err := c.CloseAllConnections(); err != nil {
			return fmt.Errorf("close all: %w", err)
		}
		fmt.Println("closed all connections")
		return nil
	case opts.host != "":
		snap, err := c.GetConnections()
		if err != nil {
			return fmt.Errorf("get connections: %w", err)
		}
		for _, conn := range snap.Connections {
			if hostMatches(conn.Metadata, opts.host) {
				opts.args = append(opts.args, conn.ID)
			}
		}
		if len(opts.args) == 0 {
			fmt.Printf("no connection matches %q\n", opts.host)
			return nil
		}
	case len(opts.args) == 0:
		return usagef("usage: close <id>... | --all | --host S")
	}
	var errs []error
	for _, id := range opts.args {
		if err := c.CloseConnection(id); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", id, err))
		}
	}
	fmt.Printf("closed %d of %d connections\n", len(opts.args)-len(errs), len(opts.args))
	return errors.Join(errs...)
}

// hostMatches is deliberately narrow because close is destructive: the host
// itself or a subdomain of it ("github.com" matches "api.github.com" but not
// "notgithub.com"), or exactly the destination IP.
func hostMatches(m api.ConnMetadata, want string) bool {
	want = strings.TrimSuffix(strings.ToLower(want), ".")
	host := strings.TrimSuffix(strings.ToLower(m.Host), ".")
	return host == want || strings.HasSuffix(host, "."+want) || m.DstIP == want
}

func cmdFlush(c *api.Client, opts options) error {
	if len(opts.args) != 1 {
		return usagef("usage: flush dns|fakeip")
	}
	var err error
	switch opts.args[0] {
	case "dns":
		err = c.FlushDNS()
	case "fakeip":
		err = c.FlushFakeIP()
	default:
		return usagef("usage: flush dns|fakeip")
	}
	if err != nil {
		return fmt.Errorf("flush %s: %w", opts.args[0], err)
	}
	fmt.Printf("flushed the %s cache\n", opts.args[0])
	return nil
}

// knownTime treats mihomo's zero time ("0001-01-01") as unknown.
func knownTime(t time.Time) *time.Time {
	if t.IsZero() || t.Year() < 2000 {
		return nil
	}
	return &t
}

func ago(t *time.Time) string {
	if t == nil {
		return "-"
	}
	d := now().Sub(*t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

func usage(used, total int64) string {
	if total <= 0 {
		return "-"
	}
	return fmt.Sprintf("%s / %s (%d%%)", humanBytes(used), humanBytes(total), used*100/total)
}

func expiry(t *time.Time) string {
	if t == nil {
		return "-"
	}
	d := t.Sub(now())
	if d < 0 {
		return "expired " + t.Format("2006-01-02")
	}
	return fmt.Sprintf("%s (in %dd)", t.Format("2006-01-02"), int(d.Hours()/24))
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
