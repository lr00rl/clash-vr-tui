package doctor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/discover"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

func controllerFindings(endpoint, source string, ver *api.VersionInfo, err error) []Finding {
	where := endpoint
	if source != "" {
		where += " (" + source + ")"
	}
	if err != nil {
		return []Finding{{Check: checkController, ID: "controller.unreachable", Severity: Fail,
			Title: "The controller does not answer", Detail: where + "\n" + err.Error(),
			Hint: "Start the core (open Clash Verge Rev), or run `clash-vr-tui discover` to find the right --socket/--server"}}
	}
	out := []Finding{{Check: checkController, ID: "controller.ok", Severity: OK,
		Title: "mihomo " + ver.Version + " answers", Detail: where}}
	if !ver.Meta {
		out = append(out, Finding{Check: checkController, ID: "controller.not-meta", Severity: Info,
			Title:  "The core does not report itself as mihomo (Meta)",
			Detail: "Some checks rely on mihomo endpoints and may not apply to Clash Premium or older cores"})
	}
	return out
}

// otherControllerFindings warns when discovery finds a second reachable
// controller that is not the same core on another address. Two cores cannot
// share a non-zero mixed port, so a matching port and version means "same core".
func otherControllerFindings(ctx context.Context, c *api.Client, cfg *api.Config, ver *api.VersionInfo, sys System) []Finding {
	if sys.Discover == nil {
		return nil
	}
	current := c.SocketPath()
	var others []string
	for _, cand := range sys.Discover(ctx) {
		if cand.State != discover.StateOK || cand.Address == current {
			continue
		}
		if cfg != nil && ver != nil && cfg.MixedPort != 0 && cand.Version == ver.Version {
			other, err := api.NewWith(cand.Endpoint()).WithTimeout(discover.ProbeTimeout).GetConfig()
			if err == nil && other.MixedPort == cfg.MixedPort {
				continue
			}
		}
		label := cand.Address
		if cand.Version != "" {
			label += " (" + cand.Version + ")"
		}
		if len(cand.Sources) > 0 {
			label += ", from " + cand.Sources[0]
		}
		others = append(others, label)
	}
	if len(others) == 0 {
		return nil
	}
	return []Finding{{Check: checkController, ID: "controller.multiple", Severity: Warn,
		Title:  fmt.Sprintf("%d other reachable controller(s) besides the one in use", len(others)),
		Detail: strings.Join(others, "\n"),
		Hint:   "Two cores fight over ports, TUN and the system proxy; quit the client you do not use, or pin one with --socket/--server"}}
}

func configFindings(cfg *api.Config, sp sysProxy) []Finding {
	var out []Finding
	add := func(sev Severity, id, title, detail, hint string) {
		out = append(out, Finding{Check: checkConfig, ID: "config." + id, Severity: sev, Title: title, Detail: detail, Hint: hint})
	}
	switch strings.ToLower(cfg.Mode) {
	case "direct":
		add(Warn, "mode-direct", "Mode is direct", "Every connection bypasses rules and proxies",
			"clash-vr-tui mode rule")
	case "global":
		add(Info, "mode-global", "Mode is global", "Every connection uses the node selected in GLOBAL; rules are ignored",
			"clash-vr-tui mode rule to route by rules again")
	}
	if cfg.AllowLan {
		bind := cfg.BindAddress
		if bind == "" || bind == "*" {
			bind = "* (all interfaces)"
		}
		switch {
		case isLoopbackHost(cfg.BindAddress):
			add(Info, "allow-lan", "allow-lan is on but ports bind to loopback",
				"bind-address "+cfg.BindAddress+" keeps the inbound ports local", "")
		case len(cfg.Authentication) > 0:
			add(Info, "allow-lan", "allow-lan is on with inbound authentication",
				"Inbound ports listen on "+bind+"; LAN clients need credentials", "")
		default:
			add(Warn, "allow-lan", "allow-lan is on with no authentication",
				"Inbound ports listen on "+bind+", so any device on the network can use this proxy",
				"Turn off Allow LAN in Clash Verge Rev (clash-vr-tui config set lan off) unless you share the proxy on purpose")
		}
	}
	if strings.EqualFold(cfg.LogLevel, "debug") {
		add(Warn, "log-level-debug", "log-level is debug",
			"The core logs every connection, which costs CPU and disk",
			"clash-vr-tui config set log-level warning (doctor still captures debug lines over the API)")
	}
	if !cfg.TUN.Enable {
		if toCore, known := sp.routesToCore(cfg); sp.Checked && known && !toCore {
			// Only scutil reads the real system setting; shell variables miss a
			// desktop proxy set through gsettings or KDE, so they only warrant a warning.
			severity, detail := Fail, "Apps that do not set a proxy themselves send traffic around the core"
			if sp.Source != "scutil --proxy" {
				severity, detail = Warn, "This shell's proxy variables do not point at the core; a desktop-wide proxy setting was not checked"
			}
			add(severity, "bypass", "TUN is off and the system proxy does not point at the core", detail,
				"Turn on TUN or System Proxy in Clash Verge Rev (clash-vr-tui config set tun on)")
		}
	}
	tun := "off"
	if cfg.TUN.Enable {
		tun = "on"
	}
	summary := Finding{Check: checkConfig, ID: "config.summary", Severity: OK,
		Title: fmt.Sprintf("Mode %s, mixed port %d, TUN %s, log level %s", cfg.Mode, cfg.MixedPort, tun, cfg.LogLevel)}
	if len(out) > 0 {
		summary.Severity = Info
	}
	return append([]Finding{summary}, out...)
}

func isLoopbackHost(host string) bool {
	host = strings.Trim(strings.ToLower(host), "[]")
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// proxySetting is one configured system proxy.
type proxySetting struct {
	Kind string // HTTP, HTTPS, SOCKS, ALL
	Host string
	Port int
}

func (p proxySetting) addr() string { return net.JoinHostPort(p.Host, strconv.Itoa(p.Port)) }

// sysProxy is what the OS (macOS) or the environment (Linux) says about the
// system proxy.
type sysProxy struct {
	Checked    bool
	Err        error
	Source     string
	Proxies    []proxySetting
	PAC        string
	PACTargets []string // host:port values the PAC file returns
	PACErr     error
}

// routesToCore reports whether the system proxy sends traffic to one of the
// core's inbound ports. known is false when a PAC file is set but could not
// be read, since it may well point at the core.
func (s sysProxy) routesToCore(cfg *api.Config) (toCore, known bool) {
	ports := corePorts(cfg)
	for _, p := range s.Proxies {
		if isLoopbackHost(p.Host) && ports[p.Port] {
			return true, true
		}
	}
	for _, t := range s.PACTargets {
		if hostPortToCore(t, ports) {
			return true, true
		}
	}
	if s.PAC != "" && len(s.PACTargets) == 0 {
		return false, false
	}
	return false, true
}

func corePorts(cfg *api.Config) map[int]bool {
	ports := map[int]bool{}
	if cfg == nil {
		return ports
	}
	for _, p := range []int{cfg.MixedPort, cfg.Port, cfg.SocksPort} {
		if p > 0 {
			ports[p] = true
		}
	}
	return ports
}

func hostPortToCore(hp string, ports map[int]bool) bool {
	host, port, err := net.SplitHostPort(hp)
	if err != nil {
		return false
	}
	n, _ := strconv.Atoi(port)
	return isLoopbackHost(host) && ports[n]
}

func readSystemProxy(ctx context.Context, sys System) sysProxy {
	switch sys.GOOS {
	case "darwin":
		if sys.ProxySettings == nil {
			return sysProxy{}
		}
		out, err := sys.ProxySettings(ctx)
		if err != nil {
			return sysProxy{Checked: true, Source: "scutil --proxy", Err: err}
		}
		sp := parseScutil(out)
		if sp.PAC != "" {
			sp.PACTargets, sp.PACErr = fetchPACTargets(ctx, sp.PAC)
		}
		return sp
	case "windows":
		return sysProxy{}
	default:
		return proxyFromEnv(sys.Getenv)
	}
}

// parseScutil reads the dictionary `scutil --proxy` prints.
func parseScutil(out string) sysProxy {
	kv := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, " : ")
		if ok {
			kv[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	sp := sysProxy{Checked: true, Source: "scutil --proxy"}
	for _, kind := range []string{"HTTP", "HTTPS", "SOCKS"} {
		if kv[kind+"Enable"] != "1" {
			continue
		}
		port, _ := strconv.Atoi(kv[kind+"Port"])
		sp.Proxies = append(sp.Proxies, proxySetting{Kind: kind, Host: kv[kind+"Proxy"], Port: port})
	}
	if kv["ProxyAutoConfigEnable"] == "1" {
		sp.PAC = kv["ProxyAutoConfigURLString"]
	}
	return sp
}

// proxyFromEnv reads the proxy variables Linux shells and most CLI tools use.
func proxyFromEnv(getenv func(string) string) sysProxy {
	sp := sysProxy{Checked: true, Source: "environment of this shell"}
	for _, kind := range []string{"http", "https", "all"} {
		v := getenv(kind + "_proxy")
		if v == "" {
			v = getenv(strings.ToUpper(kind) + "_PROXY")
		}
		if v == "" {
			continue
		}
		if !strings.Contains(v, "://") {
			v = "http://" + v
		}
		u, err := url.Parse(v)
		if err != nil || u.Hostname() == "" {
			continue
		}
		port, _ := strconv.Atoi(u.Port())
		sp.Proxies = append(sp.Proxies, proxySetting{Kind: strings.ToUpper(kind), Host: u.Hostname(), Port: port})
	}
	return sp
}

var pacTargetRe = regexp.MustCompile(`(?i)\b(?:PROXY|HTTPS|SOCKS5?|SOCKS4)\s+([^\s;"']+)`)

// fetchPACTargets reads a PAC file served on loopback (Clash Verge Rev serves
// its own) and returns the proxies it names. Remote PAC URLs are not fetched.
func fetchPACTargets(ctx context.Context, pacURL string) ([]string, error) {
	u, err := url.Parse(pacURL)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" || !isLoopbackHost(u.Hostname()) {
		return nil, errors.New("not a local http URL, so it was not fetched")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pacURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		// A redirect could leave loopback; read only the local response.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var targets []string
	for _, m := range pacTargetRe.FindAllStringSubmatch(string(body), -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			targets = append(targets, m[1])
		}
	}
	if len(targets) == 0 {
		return nil, errors.New("the PAC file names no proxy")
	}
	return targets, nil
}

func sysProxyFindings(sp sysProxy, cfg *api.Config) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkSysProxy, ID: "sysproxy." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	if !sp.Checked {
		return []Finding{f(Info, "unchecked", "System proxy not checked on this OS", "", "")}
	}
	if sp.Err != nil {
		return []Finding{f(Warn, "unreadable", "Could not read the system proxy", sp.Source+": "+sp.Err.Error(), "")}
	}
	ports := corePorts(cfg)
	note := ""
	if sp.Source == "environment of this shell" {
		note = "Read from this shell's environment; desktop apps may use other settings"
	}
	var out []Finding
	var toCore, elsewhere []string
	for _, p := range sp.Proxies {
		label := p.Kind + " " + p.addr()
		if isLoopbackHost(p.Host) && ports[p.Port] {
			toCore = append(toCore, label)
		} else {
			elsewhere = append(elsewhere, label)
		}
	}
	if len(toCore) > 0 {
		out = append(out, f(OK, "core", "System proxy points at the core", joinLines(strings.Join(toCore, ", "), note), ""))
	}
	if len(elsewhere) > 0 {
		out = append(out, f(Warn, "elsewhere", "System proxy points somewhere other than the core",
			joinLines(strings.Join(elsewhere, ", ")+portsNote(cfg), note),
			"Another proxy app may own the system proxy; toggle System Proxy in Clash Verge Rev or quit the other app"))
	}
	if sp.PAC != "" {
		pac := sp.PAC
		if u, err := url.Parse(pac); err != nil || !isLoopbackHost(u.Hostname()) {
			pac = styles.RedactURLs(pac)
		}
		switch {
		case sp.PACErr != nil:
			out = append(out, f(Info, "pac", "Proxy auto-config (PAC) is on", pac+"\ncould not read it: "+sp.PACErr.Error(), ""))
		default:
			sends := "sends traffic to " + strings.Join(sp.PACTargets, ", ")
			coreHit := false
			for _, t := range sp.PACTargets {
				coreHit = coreHit || hostPortToCore(t, ports)
			}
			if coreHit {
				out = append(out, f(OK, "pac", "Proxy auto-config (PAC) points at the core", pac+"\n"+sends, ""))
			} else {
				out = append(out, f(Warn, "pac", "Proxy auto-config (PAC) does not point at the core", pac+"\n"+sends+portsNote(cfg),
					"Toggle System Proxy in Clash Verge Rev so it regenerates the PAC file"))
			}
		}
	}
	if len(out) == 0 {
		detail := "No HTTP, HTTPS, SOCKS or PAC proxy is set"
		if cfg != nil && cfg.TUN.Enable {
			detail += "; TUN carries the traffic instead"
		}
		out = append(out, f(Info, "off", "System proxy is off", joinLines(detail, note), ""))
	}
	return out
}

func portsNote(cfg *api.Config) string {
	if cfg == nil {
		return ""
	}
	return fmt.Sprintf(" (core mixed port is %d)", cfg.MixedPort)
}

func joinLines(lines ...string) string {
	var kept []string
	for _, l := range lines {
		if l != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

const egressTimeout = 5 * time.Second

// egressFindings fetches the test URL through the mixed port, the same way
// an app using the system proxy would.
func egressFindings(ctx context.Context, cfg *api.Config, testURL string) []Finding {
	f := func(sev Severity, id, title, detail, hint string) []Finding {
		return []Finding{{Check: checkEgress, ID: "egress." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}}
	}
	if cfg == nil {
		return f(Info, "skipped", "Egress not tested: runtime config unknown", "", "")
	}
	if cfg.MixedPort == 0 {
		return f(Info, "skipped", "Egress not tested: the core has no mixed port", "", "")
	}
	proxyAddr := "127.0.0.1:" + strconv.Itoa(cfg.MixedPort)
	transport := &http.Transport{Proxy: http.ProxyURL(&url.URL{Scheme: "http", Host: proxyAddr}), DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: egressTimeout}
	if _, err := url.ParseRequestURI(testURL); err != nil {
		return f(Warn, "bad-url", "Egress not tested: invalid test URL", err.Error(), "Fix --url or test-url in the config file")
	}
	// One retry keeps a single dropped request from reading as a broken setup.
	var resp *http.Response
	var err error
	var elapsed time.Duration
	for attempt := 0; attempt < 2 && ctx.Err() == nil; attempt++ {
		var req *http.Request
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)
		if err != nil {
			return f(Warn, "bad-url", "Egress not tested: invalid test URL", err.Error(), "Fix --url or test-url in the config file")
		}
		start := time.Now()
		resp, err = client.Do(req)
		elapsed = time.Since(start).Round(time.Millisecond)
		if err == nil {
			break
		}
	}
	where := fmt.Sprintf("GET %s through %s", testURL, proxyAddr)
	if err != nil {
		return f(Fail, "failed", "No response through the mixed port (2 attempts)", where+"\n"+err.Error(),
			"Check the selected nodes under Groups below, or run doctor --probe to test every group")
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	detail := fmt.Sprintf("%s returned %d in %s", where, resp.StatusCode, elapsed)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return f(Warn, "status", fmt.Sprintf("Test URL answered %d through the mixed port", resp.StatusCode), detail,
			"The path works but the test URL refused the request; try --url with another endpoint")
	}
	return f(OK, "ok", fmt.Sprintf("Internet reachable through the mixed port in %s", elapsed), detail, "")
}

var rcodeNames = map[int]string{1: "FORMERR", 2: "SERVFAIL", 3: "NXDOMAIN", 4: "NOTIMP", 5: "REFUSED"}

func dnsCheck(c *api.Client, testURL string) []Finding {
	u, err := url.Parse(testURL)
	host := ""
	if err == nil {
		host = u.Hostname()
	}
	if host == "" || net.ParseIP(host) != nil {
		return []Finding{{Check: checkDNS, ID: "dns.skipped", Severity: Info, Title: "DNS not tested: the test URL has no host name"}}
	}
	res, err := c.QueryDNS(host, "A")
	if err != nil || res == nil || res.Status != 0 || len(res.Answer) == 0 {
		res, err = c.QueryDNS(host, "A") // one retry, as for egress
	}
	return dnsFindings(host, res, err)
}

func dnsFindings(host string, res *api.DNSResult, err error) []Finding {
	f := func(sev Severity, id, title, detail, hint string) []Finding {
		return []Finding{{Check: checkDNS, ID: "dns." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}}
	}
	hint := "Check the dns section of the profile (nameserver, fallback) and that the DNS-out group has a working node"
	if err != nil {
		var se *api.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return f(Info, "unsupported", "DNS not tested: the core has no /dns/query endpoint", "", "")
		}
		return f(Fail, "error", "The core's DNS could not resolve "+host, err.Error(), hint)
	}
	if res.Status != 0 {
		name := rcodeNames[res.Status]
		if name == "" {
			name = "rcode " + strconv.Itoa(res.Status)
		}
		return f(Fail, "rcode", "The core's DNS answered "+name+" for "+host, "", hint)
	}
	var addrs []string
	for _, a := range res.Answer {
		if a.Type == 1 || a.Type == 28 {
			addrs = append(addrs, a.Data)
		}
	}
	if len(addrs) == 0 {
		return f(Fail, "empty", "The core's DNS returned no address for "+host, "", hint)
	}
	if len(addrs) > 3 {
		addrs = append(addrs[:3], "...")
	}
	return f(OK, "ok", "The core resolves "+host, strings.Join(addrs, ", "), "")
}
