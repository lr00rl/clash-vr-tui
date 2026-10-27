package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// Real lines captured from the development machine.
const (
	dialLineProxy  = `[TCP] dial AI-CF-Suite (match DomainSuffix/chatgpt.com) 198.18.0.1:64161(codex) --> chatgpt.com:443 error: eb-wee.dmit.roobli.org:57289 connect error: dial tcp 1.2.3.4:57289: i/o timeout`
	dialLineDirect = `[TCP] dial DIRECT 127.0.0.1:64163 --> nezha.roobli.org:443 error: dial tcp 151.244.134.20:443: i/o timeout`
)

func find(t *testing.T, fs []Finding, id string) Finding {
	t.Helper()
	for _, f := range fs {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no finding %s in %+v", id, fs)
	return Finding{}
}

func findTitled(t *testing.T, fs []Finding, id, titlePart string) Finding {
	t.Helper()
	for _, f := range fs {
		if f.ID == id && strings.Contains(f.Title, titlePart) {
			return f
		}
	}
	t.Fatalf("no finding %s with title containing %q in %+v", id, titlePart, fs)
	return Finding{}
}

func hasID(fs []Finding, id string) bool {
	for _, f := range fs {
		if f.ID == id {
			return true
		}
	}
	return false
}

func TestConfigFindingsSeverity(t *testing.T) {
	toCore := sysProxy{Checked: true, Proxies: []proxySetting{{Kind: "HTTP", Host: "127.0.0.1", Port: 7897}}}
	off := sysProxy{Checked: true}
	cases := []struct {
		name string
		cfg  api.Config
		sp   sysProxy
		id   string
		want Severity
	}{
		{"direct mode", api.Config{Mode: "direct", TUN: api.TUNConfig{Enable: true}}, off, "config.mode-direct", Warn},
		{"lan without auth", api.Config{Mode: "rule", AllowLan: true, BindAddress: "*", TUN: api.TUNConfig{Enable: true}}, off, "config.allow-lan", Warn},
		{"lan with auth", api.Config{Mode: "rule", AllowLan: true, Authentication: []string{""}, TUN: api.TUNConfig{Enable: true}}, off, "config.allow-lan", Info},
		{"lan on loopback", api.Config{Mode: "rule", AllowLan: true, BindAddress: "127.0.0.1", TUN: api.TUNConfig{Enable: true}}, off, "config.allow-lan", Info},
		{"debug log level", api.Config{Mode: "rule", LogLevel: "debug", TUN: api.TUNConfig{Enable: true}}, off, "config.log-level-debug", Warn},
		{"tun off and no system proxy", api.Config{Mode: "rule", MixedPort: 7897}, off, "config.bypass", Fail},
		{"clean", api.Config{Mode: "rule", MixedPort: 7897}, toCore, "config.summary", OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := find(t, configFindings(&tc.cfg, tc.sp), tc.id)
			if got.Severity != tc.want {
				t.Fatalf("%s severity = %s, want %s", tc.id, got.Severity, tc.want)
			}
		})
	}

	// TUN off is not a bypass when the system proxy, or a PAC file that could
	// not be read, may route to the core, or when the OS was not checked.
	cfg := api.Config{Mode: "rule", MixedPort: 7897}
	for name, sp := range map[string]sysProxy{
		"proxy to core":  toCore,
		"unreadable pac": {Checked: true, PAC: "http://127.0.0.1:1/pac"},
		"unchecked os":   {},
	} {
		if hasID(configFindings(&cfg, sp), "config.bypass") {
			t.Errorf("%s: unexpected bypass fail", name)
		}
	}
}

func TestSystemProxyFromScutilAndPAC(t *testing.T) {
	pac := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`function FindProxyForURL(url, host) { return "PROXY 127.0.0.1:7897; SOCKS5 127.0.0.1:7897; DIRECT;"; }`))
	}))
	defer pac.Close()
	scutil := "<dictionary> {\n  HTTPEnable : 0\n  HTTPSEnable : 0\n  ProxyAutoConfigEnable : 1\n  ProxyAutoConfigURLString : " +
		pac.URL + "/commands/pac\n  SOCKSEnable : 0\n}\n"
	sys := System{GOOS: "darwin", ProxySettings: func(context.Context) (string, error) { return scutil, nil }}
	sp := readSystemProxy(t.Context(), sys)
	cfg := &api.Config{MixedPort: 7897}
	if toCore, known := sp.routesToCore(cfg); !toCore || !known {
		t.Fatalf("routesToCore = %v, %v; targets %v err %v", toCore, known, sp.PACTargets, sp.PACErr)
	}
	if f := find(t, sysProxyFindings(sp, cfg), "sysproxy.pac"); f.Severity != OK {
		t.Fatalf("pac finding = %+v", f)
	}

	elsewhere := parseScutil("<dictionary> {\n  HTTPEnable : 1\n  HTTPPort : 7890\n  HTTPProxy : 127.0.0.1\n  SOCKSEnable : 1\n  SOCKSPort : 7897\n  SOCKSProxy : 127.0.0.1\n}")
	fs := sysProxyFindings(elsewhere, cfg)
	if find(t, fs, "sysproxy.elsewhere").Severity != Warn || find(t, fs, "sysproxy.core").Severity != OK {
		t.Fatalf("findings = %+v", fs)
	}
}

func TestProxyFromEnvAcceptsEitherCase(t *testing.T) {
	env := map[string]string{"HTTPS_PROXY": "http://127.0.0.1:7897", "all_proxy": "socks5://localhost:7898"}
	sp := proxyFromEnv(func(k string) string { return env[k] })
	if len(sp.Proxies) != 2 || sp.Proxies[0].Kind != "HTTPS" || sp.Proxies[0].Port != 7897 || sp.Proxies[1].Host != "localhost" {
		t.Fatalf("proxies = %+v", sp.Proxies)
	}
}

func history(delays ...int) []api.DelayHistory {
	h := make([]api.DelayHistory, len(delays))
	for i, d := range delays {
		h[i].Delay = d
	}
	return h
}

func TestGroupFindingsSeverity(t *testing.T) {
	proxies := map[string]api.Proxy{
		"Proxy":  {Type: "Selector", All: []string{"Auto", "NodeA", "NodeB"}, Now: "Auto"},
		"Auto":   {Type: "URLTest", All: []string{"NodeA", "NodeB"}, Now: "NodeA"},
		"Dead":   {Type: "Selector", All: []string{"NodeC", "NodeD", "NodeE"}, Now: "NodeC"},
		"Fresh":  {Type: "Selector", All: []string{"NodeE", "NodeB"}, Now: "NodeE"},
		"Ads":    {Type: "Selector", All: []string{"REJECT", "DIRECT"}, Now: "REJECT"},
		"Good":   {Type: "Selector", All: []string{"NodeB"}, Now: "NodeB"},
		"NodeA":  {Type: "Vless", History: history(120, 0)},
		"NodeB":  {Type: "Vless", History: history(90)},
		"NodeC":  {Type: "Trojan", History: history(0)},
		"NodeD":  {Type: "Trojan", History: history(0)},
		"NodeE":  {Type: "Trojan"},
		"REJECT": {Type: "Reject"},
		"DIRECT": {Type: "Direct"},
	}
	var groups []api.Group
	for _, n := range []string{"Proxy", "Auto", "Dead", "Fresh", "Ads", "Good"} {
		p := proxies[n]
		groups = append(groups, api.Group{Name: n, Type: p.Type, All: p.All, Now: p.Now})
	}
	groups = append(groups, api.Group{Name: "GLOBAL", All: []string{"Proxy"}, Now: "Proxy"}, api.Group{Name: "Hid", Hidden: true})
	fs := groupFindings(visibleGroups(groups), proxies, false)

	if f := find(t, fs, "groups.all-failed"); f.Severity != Fail || !strings.Contains(f.Title, "Dead") || !strings.Contains(f.Detail, "2 of 3") {
		t.Errorf("all-failed = %+v", f)
	}
	if f := find(t, fs, "groups.leaf-failed"); f.Severity != Warn || !strings.Contains(f.Title, "NodeA") || f.Detail != "Selected by Proxy, Auto" {
		t.Errorf("leaf-failed = %+v", f)
	}
	if f := find(t, fs, "groups.untested"); f.Severity != Info || f.Detail != "Fresh" {
		t.Errorf("untested = %+v", f)
	}
	if f := find(t, fs, "groups.ok"); f.Severity != OK || !strings.HasPrefix(f.Title, "2 of 6") {
		t.Errorf("ok = %+v", f)
	}
}

func TestProviderFindingsExpiryUsageAndStaleness(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	const gb = int64(1 << 30)
	nodes := func(n int) []api.Proxy { return make([]api.Proxy, n) }
	pp := map[string]api.ProxyProvider{
		"Proxy": {VehicleType: "Compatible", Proxies: nodes(3)},
		"subA": {VehicleType: "HTTP", Proxies: nodes(10), UpdatedAt: now.Add(-time.Hour),
			SubscriptionInfo: &api.SubscriptionInfo{Upload: 10 * gb, Download: 85 * gb, Total: 100 * gb, Expire: now.Add(30 * 24 * time.Hour).Unix()}},
		"subB": {VehicleType: "HTTP", Proxies: nodes(5), UpdatedAt: now.Add(-10 * 24 * time.Hour),
			SubscriptionInfo: &api.SubscriptionInfo{Expire: now.Add(-24 * time.Hour).Unix()}},
		"subC":  {VehicleType: "HTTP", UpdatedAt: now.Add(-time.Hour)},
		"local": {VehicleType: "File", Proxies: nodes(3), UpdatedAt: now.Add(-30 * 24 * time.Hour)},
		"subD": {VehicleType: "HTTP", Proxies: nodes(2), UpdatedAt: now.Add(-time.Hour),
			SubscriptionInfo: &api.SubscriptionInfo{Upload: 40, Download: 60, Total: 100, Expire: now.Add(3 * 24 * time.Hour).Unix()}},
		"subE": {VehicleType: "HTTP", Proxies: nodes(4), UpdatedAt: now.Add(-time.Hour),
			SubscriptionInfo: &api.SubscriptionInfo{Download: 50 * gb, Total: 100 * gb, Expire: now.Add(60 * 24 * time.Hour).Unix()}},
	}
	rp := map[string]api.RuleProvider{
		"r1": {VehicleType: "HTTP", RuleCount: 100, UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		"r2": {VehicleType: "HTTP", RuleCount: 0, UpdatedAt: now},
		"r3": {VehicleType: "File", RuleCount: 5},
	}
	fs := providerFindings(pp, rp, now)
	checks := []struct {
		id, name string
		want     Severity
	}{
		{"providers.quota", "subA", Warn},
		{"providers.stale", "subB", Warn},
		{"providers.expired", "subB", Fail},
		{"providers.empty", "subC", Fail},
		{"providers.ok", "local", OK},
		{"providers.quota", "subD", Fail},
		{"providers.expiring", "subD", Warn},
		{"providers.ok", "subE", OK},
		{"providers.rules-stale", "rule set", Warn},
		{"providers.rules-empty", "rule set", Warn},
	}
	for _, c := range checks {
		if f := findTitled(t, fs, c.id, c.name); f.Severity != c.want {
			t.Errorf("%s %s severity = %s, want %s", c.id, c.name, f.Severity, c.want)
		}
	}
	if f := findTitled(t, fs, "providers.quota", "subA"); f.Detail != "95% of 100.0 GB used" {
		t.Errorf("subA usage detail = %q", f.Detail)
	}
	if f := findTitled(t, fs, "providers.ok", "subE"); !strings.Contains(f.Detail, "50% of 100.0 GB used") || !strings.Contains(f.Detail, "expires 2026-12-26") {
		t.Errorf("subE detail = %q", f.Detail)
	}
	for _, f := range fs {
		if strings.Contains(f.Title, "Proxy") {
			t.Errorf("group mirrored as a provider was checked: %+v", f)
		}
	}
	if f := find(t, fs, "providers.rules-stale"); f.Detail != "r1" {
		t.Errorf("rules-stale detail = %q", f.Detail)
	}
	if fs := providerFindings(map[string]api.ProxyProvider{"default": {VehicleType: "Compatible"}}, nil, now); find(t, fs, "providers.none").Severity != Info {
		t.Errorf("no providers = %+v", fs)
	}
}

func TestRoutingFindingsMatchShareAndTops(t *testing.T) {
	conn := func(host, ip, rule string, bytes int64, chains ...string) api.Connection {
		return api.Connection{Metadata: api.ConnMetadata{Host: host, DstIP: ip}, Rule: rule, Download: bytes, Chains: chains}
	}
	snap := &api.ConnectionsSnapshot{Connections: []api.Connection{
		conn("a.com", "", "Match", 10, "nodeX", "Proxy"),
		conn("b.com", "", "MATCH", 20, "nodeX", "Proxy"),
		conn("", "1.2.3.4", "match", 5, "nodeY", "Proxy"),
		conn("big.com", "", "DomainSuffix", 5000, "DIRECT"),
	}}
	fs := routingFindings(snap)
	m := find(t, fs, "routing.match")
	if !strings.HasPrefix(m.Title, "3 of 4 connections (75%) fell through to MATCH, sent to Proxy") {
		t.Errorf("match title = %q", m.Title)
	}
	if !strings.Contains(m.Detail, "1.2.3.4 (1)") || strings.Contains(m.Detail, "big.com") {
		t.Errorf("match detail = %q", m.Detail)
	}
	if top := find(t, fs, "routing.top-hosts"); !strings.HasPrefix(top.Detail, "big.com") {
		t.Errorf("top hosts = %q", top.Detail)
	}
	if nodes := find(t, fs, "routing.top-nodes"); !strings.HasPrefix(nodes.Detail, "nodeX  2 conn(s)") {
		t.Errorf("top nodes = %q", nodes.Detail)
	}
	if fs := routingFindings(&api.ConnectionsSnapshot{Connections: snap.Connections[3:]}); find(t, fs, "routing.match").Severity != OK {
		t.Errorf("no MATCH traffic = %+v", fs)
	}
}

func TestNormalizePattern(t *testing.T) {
	cases := map[string]string{
		dialLineProxy:  "[TCP] dial AI-CF-Suite (match DomainSuffix/chatgpt.com) <ip>:<port>(<proc>) --> chatgpt.com:<port> error: eb-wee.dmit.roobli.org:<port> connect error: dial tcp <ip>:<port>: i/o timeout",
		dialLineDirect: "[TCP] dial DIRECT <ip>:<port> --> nezha.roobli.org:<port> error: dial tcp <ip>:<port>: i/o timeout",
		"dial tcp [2001:db8::1]:443: connect: network is unreachable": "dial tcp <ip>:<port>: connect: network is unreachable",
		"server connection error: hyper::Error(Parse(Method))":        "server connection error: hyper::Error(Parse(Method))",
		"retried 3 times for fe80::1 and 10.0.0.1":                    "retried <n> times for <ip> and <ip>",
	}
	for in, want := range cases {
		if got := normalizePattern(in); got != want {
			t.Errorf("normalizePattern(%q)\n got %q\nwant %q", in, got, want)
		}
	}
	// Lines that differ only in addresses and ports share a pattern.
	other := strings.NewReplacer("198.18.0.1:64161", "198.18.0.1:50000", "57289", "443", "1.2.3.4", "5.6.7.8").Replace(dialLineProxy)
	if normalizePattern(other) != normalizePattern(dialLineProxy) {
		t.Errorf("variants did not normalize alike:\n%s\n%s", normalizePattern(other), normalizePattern(dialLineProxy))
	}
}

func TestParseDialError(t *testing.T) {
	cases := []struct{ line, outbound, host string }{
		{dialLineProxy, "AI-CF-Suite", "chatgpt.com"},
		{dialLineDirect, "DIRECT", "nezha.roobli.org"},
		{"[UDP] dial JP Tokyo 01 (match Match/) 192.168.1.2:5000(Google Chrome Helper) --> [2001:db8::1]:443 error: timeout", "JP Tokyo 01", "2001:db8::1"},
	}
	for _, c := range cases {
		ob, host, ok := parseDialError(c.line)
		if !ok || ob != c.outbound || host != c.host {
			t.Errorf("parseDialError(%q) = %q, %q, %v", c.line, ob, host, ok)
		}
	}
	if _, _, ok := parseDialError("[Provider] sub pull error: dial tcp 1.2.3.4:443: i/o timeout"); ok {
		t.Error("a non-dial line parsed as a failed dial")
	}
}

func TestLogFindingsGroupsPatternsAndDials(t *testing.T) {
	variant := strings.ReplaceAll(dialLineProxy, "64161", "50001")
	lines := []LogLine{
		{Level: "debug", Payload: "[TCP] 198.18.0.1:1 --> a.com:443 match Match using Proxy"},
		{Level: "info", Payload: "[TCP] ok"},
		{Level: "warning", Payload: dialLineProxy},
		{Level: "warning", Payload: variant},
		{Level: "warning", Payload: dialLineDirect},
		{Level: "error", Payload: "dns resolve a.example failed"},
	}
	fs := logFindings(lines, 0, captureEnd{connected: true}, 10*time.Second, "debug")
	if s := find(t, fs, "logs.summary"); s.Severity != Info || s.Detail != "debug 1, info 1, warning 3, error 1" {
		t.Errorf("summary = %+v", s)
	}
	p := find(t, fs, "logs.patterns")
	if p.Severity != Warn || !strings.Contains(p.Title, "3 pattern(s)") || !strings.HasPrefix(p.Detail, "2x warning: [TCP] dial AI-CF-Suite") {
		t.Errorf("patterns = %+v", p)
	}
	d := find(t, fs, "logs.dial-failures")
	want := "by outbound: AI-CF-Suite (2), DIRECT (1)\nby destination: chatgpt.com (2), nezha.roobli.org (1)"
	if d.Detail != want {
		t.Errorf("dial failures detail = %q", d.Detail)
	}

	quiet := logFindings(lines[:2], 0, captureEnd{connected: true}, time.Second, "debug")
	if len(quiet) != 1 || quiet[0].Severity != OK {
		t.Errorf("quiet capture = %+v", quiet)
	}
	if fs := logFindings(nil, 0, captureEnd{}, time.Second, "debug"); fs[0].ID != "logs.unavailable" || fs[0].Severity != Warn {
		t.Errorf("failed stream = %+v", fs)
	}
}

func TestRedactURLs(t *testing.T) {
	in := `[Provider] sub pull error: Get "https://user:pw@sub.example.com/api/v1/client/subscribe?token=SECRET": EOF; see https://example.org/`
	got := redactURLs(in)
	if strings.Contains(got, "SECRET") || strings.Contains(got, "pw@") || strings.Contains(got, "subscribe") {
		t.Fatalf("redactURLs left a secret: %q", got)
	}
	if !strings.Contains(got, `"https://sub.example.com/***"`) || !strings.Contains(got, "https://example.org/") {
		t.Fatalf("redactURLs = %q", got)
	}
	// A real Verge line: the URL sits in parentheses followed by a colon.
	verge := "error sending request for url (http://substore.example.org:8080/***/api/file/***): client error (Connect)"
	if got := redactURLs(verge); got != "error sending request for url (http://substore.example.org:8080/***): client error (Connect)" {
		t.Fatalf("redactURLs(verge line) = %q", got)
	}
}

func TestVergeLogFinding(t *testing.T) {
	data := strings.Join([]string{
		"[2026-10-27 00:22:40.713] ERROR server connection error: hyper::Error(Parse(Method))",
		"[2026-10-27 00:31:16.792] ERROR server connection error: hyper::Error(Parse(Method))",
		"[2026-10-26 11:55:00.100] WARN [Config] update failed (http://sub.example.com/api/file/token123): os error 51",
		"  a continuation line without a timestamp",
		"[2026-10-27 00:30:00.000] INFO started",
	}, "\n")
	recent := vergeLogFinding("logs/latest.log", data, time.Date(2026, 10, 27, 0, 40, 0, 0, time.UTC))
	if recent.Severity != Warn || recent.Title != "logs/latest.log: 2 ERROR and 1 WARN line(s) in the last 64 KB" {
		t.Errorf("recent = %+v", recent)
	}
	if !strings.HasPrefix(recent.Detail, "2x ERROR server connection error: hyper::Error(Parse(Method)) (last 10-27 00:31)") {
		t.Errorf("detail = %q", recent.Detail)
	}
	if strings.Contains(recent.Detail, "token123") || strings.Contains(recent.Detail, "2026-10-26 11:55") {
		t.Errorf("detail kept a URL path or a timestamp: %q", recent.Detail)
	}
	if !strings.Contains(recent.Hint, "PAC") {
		t.Errorf("hint = %q", recent.Hint)
	}
	if old := vergeLogFinding("x", data, time.Date(2026, 10, 27, 3, 0, 0, 0, time.UTC)); old.Severity != Info {
		t.Errorf("errors older than an hour = %s, want info", old.Severity)
	}
	if clean := vergeLogFinding("x", "[2026-10-27 00:30:00.000] INFO started\n", time.Now()); clean.Severity != OK {
		t.Errorf("clean log = %+v", clean)
	}
}

func TestVergeFindingsReadsProfileAndNewestServiceLog(t *testing.T) {
	dir := "Users/u/Library/Application Support/" + vergeID + "/"
	t0 := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	fsys := fstest.MapFS{
		dir + "profiles.yaml": {Data: []byte("current: abc\nitems:\n- uid: Merge\n  type: merge\n  name: null\n" +
			"- uid: abc\n  type: remote\n  name: work\n  url: https://sub.example.com/secret-token\n")},
		dir + "logs/latest.log":              {Data: []byte("[2026-10-27 00:10:00.000] INFO ok\n")},
		dir + "service-logs/service/old.log": {Data: []byte("[2026-10-26 00:10:00.000] ERROR old\n"), ModTime: t0.Add(-time.Hour)},
		dir + "service-logs/service/new.log": {Data: []byte("[2026-10-27 00:10:00.000] ERROR core crashed\n"), ModTime: t0},
	}
	sys := System{GOOS: "darwin", Home: "/Users/u", FS: fsys, Now: func() time.Time { return t0.Add(20 * time.Minute) }}
	fs := vergeFindings(sys, "/var/run/clash-verge-service/users/501/verge-mihomo.sock")
	found := find(t, fs, "verge.found")
	if !strings.Contains(found.Detail, "service mode") || !strings.Contains(found.Detail, "current profile: work (remote)") {
		t.Errorf("found = %+v", found)
	}
	if len(fs) != 3 || fs[1].Title != "logs/latest.log: no ERROR or WARN lines in the last 64 KB" ||
		!strings.HasPrefix(fs[2].Title, "service-logs/service/new.log: 1 ERROR") || fs[2].Severity != Warn {
		t.Errorf("log findings = %+v", fs)
	}
	for _, f := range fs {
		if strings.Contains(f.Detail+f.Title, "secret-token") {
			t.Errorf("profile URL leaked: %+v", f)
		}
	}
	if fs := vergeFindings(System{GOOS: "linux", Home: "/home/u", FS: fstest.MapFS{}}, ""); fs[0].ID != "verge.absent" {
		t.Errorf("absent = %+v", fs)
	}
}

func TestExeName(t *testing.T) {
	cases := map[string]string{
		"/Applications/ClashX Meta.app/Contents/MacOS/ClashX Meta":                                     "ClashX Meta",
		"/Library/Application Support/clash-verge-service/cores/verge-mihomo -d /Library/x -f /x.yaml": "verge-mihomo",
		"Contents/Library/LaunchServices/app.clashmac.helper":                                          "app.clashmac.helper",
		"/usr/bin/vim /home/u/clash-config.yaml":                                                       "vim",
		"vim /home/u/clash-config.yaml":                                                                "vim",
		"/usr/bin/python3 script.py clash":                                                             "python3",
		"/bin/zsh -c clash-vr-tui doctor":                                                              "zsh",
	}
	for in, want := range cases {
		if got := exeName(in); got != want {
			t.Errorf("exeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConflictFindings(t *testing.T) {
	endpoint := "/var/run/clash-verge-service/users/501/verge-mihomo.sock"
	own := `  492 Contents/Library/LaunchServices/app.clashmac.helper
  600 /Applications/Clash Verge.app/Contents/MacOS/clash-verge
  601 /Library/PrivilegedHelperTools/io.github.clash-verge-rev.clash-verge-rev.service.bundle/Contents/MacOS/clash-verge-service
57129 /Library/Application Support/clash-verge-service/cores/verge-mihomo -d /Library/Application Support/clash-verge-service/users/501/runtime -ext-ctl-unix ` + endpoint + `
  702 /var/folders/xx/go-build1/b001/exe/clash-vr-tui doctor
  703 vim /home/u/clash-config.yaml
  704 ugrep -iE clash|mihomo
`
	sysWith := func(ps string) System {
		return System{GOOS: "darwin", PS: func(context.Context) (string, error) { return ps, nil }}
	}
	fs := conflictFindings(t.Context(), sysWith(own), endpoint)
	if hasID(fs, "conflicts.running") || find(t, fs, "conflicts.helpers").Detail != "app.clashmac.helper (pid 492)" {
		t.Errorf("own setup = %+v", fs)
	}

	extra := own + "  700 /Applications/ClashX Meta.app/Contents/MacOS/ClashX Meta\n  701 /usr/local/bin/mihomo -d /etc/mihomo\n"
	f := find(t, conflictFindings(t.Context(), sysWith(extra), endpoint), "conflicts.running")
	if f.Severity != Warn || f.Detail != "ClashX Meta (pid 700)\nmihomo (pid 701): another core" {
		t.Errorf("conflicts = %+v", f)
	}

	clean := "57129 /usr/local/bin/mihomo -d /etc/mihomo\n  600 /Applications/Clash Verge.app/Contents/MacOS/clash-verge\n"
	if fs := conflictFindings(t.Context(), sysWith(clean), "127.0.0.1:9090"); find(t, fs, "conflicts.none").Severity != OK {
		t.Errorf("single core = %+v", fs)
	}
}
