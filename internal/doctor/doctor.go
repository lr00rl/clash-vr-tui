// Package doctor checks a mihomo setup from the controller to the internet and
// explains each problem with a next step. It gathers data and returns
// findings; printing and saving them is the caller's job.
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// DefaultTestURL is what egress and DNS checks use when no test URL is set.
const DefaultTestURL = "https://www.gstatic.com/generate_204"

// Severity grades a finding.
type Severity string

const (
	OK   Severity = "ok"
	Info Severity = "info"
	Warn Severity = "warn"
	Fail Severity = "fail"
)

// Finding is one result of a check. Detail may span several lines.
type Finding struct {
	Check    string   `json:"check"`
	ID       string   `json:"id"`
	Severity Severity `json:"severity"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail,omitempty"`
	Hint     string   `json:"hint,omitempty"`
}

// Check names one check; Checks lists them in report order.
type Check struct {
	ID    string
	Title string
}

const (
	checkController = "controller"
	checkConfig     = "config"
	checkSysProxy   = "sysproxy"
	checkEgress     = "egress"
	checkDNS        = "dns"
	checkGroups     = "groups"
	checkProviders  = "providers"
	checkRouting    = "routing"
	checkLogs       = "logs"
	checkVerge      = "verge"
	checkConflicts  = "conflicts"
)

// Checks is the order findings are reported in.
var Checks = []Check{
	{checkController, "Controller"},
	{checkConfig, "Runtime config"},
	{checkSysProxy, "System proxy"},
	{checkEgress, "Egress"},
	{checkDNS, "DNS"},
	{checkGroups, "Groups"},
	{checkProviders, "Providers"},
	{checkRouting, "Routing"},
	{checkLogs, "Core logs"},
	{checkVerge, "Clash Verge Rev"},
	{checkConflicts, "Conflicts"},
}

// CheckTitle returns the display title of a check id.
func CheckTitle(id string) string {
	for _, c := range Checks {
		if c.ID == id {
			return c.Title
		}
	}
	return id
}

// LogLine is one captured core log entry.
type LogLine struct {
	Time    time.Time `json:"time"`
	Level   string    `json:"level"`
	Payload string    `json:"payload"`
}

// Counts tallies findings by severity.
type Counts struct {
	OK   int `json:"ok"`
	Info int `json:"info"`
	Warn int `json:"warn"`
	Fail int `json:"fail"`
}

// Report is the result of Run. The fields tagged "-" are raw material for a
// bundle and stay out of the JSON report.
type Report struct {
	Time       time.Time `json:"time"`
	DurationMS int64     `json:"duration_ms"`
	Endpoint   string    `json:"endpoint"`
	Source     string    `json:"source,omitempty"`
	Version    string    `json:"version,omitempty"`
	Findings   []Finding `json:"findings"`
	Summary    Counts    `json:"summary"`

	Logs        []LogLine                `json:"-"`
	Connections *api.ConnectionsSnapshot `json:"-"`
	// RuntimeConfig holds only allowlisted, credential-free /configs fields.
	RuntimeConfig map[string]json.RawMessage `json:"-"`
}

// Inputs is everything Run needs. System is injected so tests never touch the
// real machine.
type Inputs struct {
	Client   *api.Client
	Source   string        // how the endpoint was chosen, for display
	TestURL  string        // egress and DNS target; DefaultTestURL when empty
	LogsDur  time.Duration // log capture length; 0 skips it
	LogLevel string        // /logs level; "debug" when empty
	Probe    bool          // run fresh group delay tests
	ProbeMS  int           // per-test timeout for probes
	System   System
}

// Overall time beyond the log capture that Run waits for checks. Probes test
// every visible group, four at a time, so they get more.
const (
	checkBudget = 20 * time.Second
	probeBudget = 60 * time.Second
	resultGrace = 2 * time.Second
)

// outcome is what a concurrent check hands back: its findings plus an
// optional step that copies collected data into the report. The step runs on
// Run's goroutine, so a check that misses the deadline never races it.
type outcome struct {
	findings []Finding
	apply    func(*Report)
}

type task struct {
	check string
	run   func(ctx context.Context) outcome
}

// Run executes every check and returns the findings in Checks order.
func Run(ctx context.Context, in Inputs) Report {
	if in.TestURL == "" {
		in.TestURL = DefaultTestURL
	}
	if in.LogLevel == "" {
		in.LogLevel = "debug"
	}
	sys := in.System
	start := sys.Now()
	budget := in.LogsDur + checkBudget
	if in.Probe {
		budget += probeBudget
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	r := Report{Time: start, Endpoint: in.Client.SocketPath(), Source: in.Source}
	byCheck := map[string][]Finding{}
	add := func(fs ...Finding) {
		for _, f := range fs {
			byCheck[f.Check] = append(byCheck[f.Check], f)
		}
	}

	// Phase 1: the controller and its runtime config. Most checks read them,
	// and nothing else can work when the controller does not answer.
	ver, verErr := in.Client.GetVersion()
	reachable := verErr == nil
	if reachable {
		r.Version = ver.Version
	}
	endpoint := r.Endpoint // tasks read this copy, never r, which Run keeps writing
	add(controllerFindings(endpoint, in.Source, ver, verErr)...)

	var cfg *api.Config
	if reachable {
		var cfgErr error
		var raw map[string]json.RawMessage
		cfg, raw, cfgErr = fetchConfig(in.Client)
		if cfgErr != nil {
			add(Finding{Check: checkConfig, ID: "config.unreadable", Severity: Fail,
				Title: "Could not read the runtime config", Detail: cfgErr.Error(),
				Hint: "The controller answers /version but not /configs; restart the core and run doctor again"})
		}
		r.RuntimeConfig = raw
	}
	sp := readSystemProxy(ctx, sys)
	if cfg != nil {
		add(configFindings(cfg, sp)...)
	}
	add(sysProxyFindings(sp, cfg)...)

	// Phase 2: independent checks, concurrently.
	tasks := []task{
		{checkController, func(ctx context.Context) outcome {
			return outcome{findings: otherControllerFindings(ctx, in.Client, cfg, ver, sys)}
		}},
		{checkVerge, func(ctx context.Context) outcome {
			return outcome{findings: vergeFindings(sys, endpoint)}
		}},
		{checkConflicts, func(ctx context.Context) outcome {
			return outcome{findings: conflictFindings(ctx, sys, endpoint)}
		}},
	}
	if reachable {
		tasks = append(tasks,
			task{checkEgress, func(ctx context.Context) outcome {
				return outcome{findings: egressFindings(ctx, cfg, in.TestURL)}
			}},
			task{checkDNS, func(ctx context.Context) outcome {
				return outcome{findings: dnsCheck(in.Client, in.TestURL)}
			}},
			task{checkGroups, func(ctx context.Context) outcome {
				return outcome{findings: groupsCheck(ctx, in)}
			}},
			task{checkProviders, func(ctx context.Context) outcome {
				return outcome{findings: providersCheck(in.Client, sys.Now())}
			}},
			task{checkRouting, func(ctx context.Context) outcome {
				snap, err := in.Client.GetConnections()
				if err != nil {
					return outcome{findings: []Finding{{Check: checkRouting, ID: "routing.unreadable", Severity: Warn,
						Title: "Could not read connections", Detail: err.Error()}}}
				}
				return outcome{findings: routingFindings(snap), apply: func(r *Report) { r.Connections = snap }}
			}},
			task{checkLogs, func(ctx context.Context) outcome {
				if in.LogsDur <= 0 {
					return outcome{findings: []Finding{{Check: checkLogs, ID: "logs.skipped", Severity: Info,
						Title: "Log capture skipped (--logs 0)"}}}
				}
				lines, dropped, end := captureLogs(ctx, in.Client, in.LogLevel, in.LogsDur, sys.Now)
				return outcome{findings: logFindings(lines, dropped, end, in.LogsDur, in.LogLevel),
					apply: func(r *Report) { r.Logs = lines }}
			}},
		)
	} else {
		for _, id := range []string{checkConfig, checkEgress, checkDNS, checkGroups, checkProviders, checkRouting, checkLogs} {
			add(Finding{Check: id, ID: id + ".skipped", Severity: Info, Title: "Skipped because the controller is unreachable"})
		}
	}

	results := make([]chan outcome, len(tasks))
	for i, t := range tasks {
		ch := make(chan outcome, 1)
		results[i] = ch
		go func() { ch <- t.run(ctx) }()
	}
	// Once ctx ends (deadline or Ctrl+C), checks that watch it, such as the
	// log capture, still need a moment to hand back what they collected, so
	// the remaining results share one short grace period.
	var grace <-chan time.Time
	for i, ch := range results {
		var o outcome
		select {
		case o = <-ch:
		case <-ctx.Done():
			if grace == nil {
				grace = time.After(resultGrace)
			}
			select {
			case o = <-ch:
			case <-grace:
				o.findings = []Finding{{Check: tasks[i].check, ID: tasks[i].check + ".unfinished", Severity: Warn,
					Title:  "Check did not finish in time",
					Detail: fmt.Sprintf("stopped after %s: %v", sys.Now().Sub(start).Round(time.Second), context.Cause(ctx))}}
			}
		}
		add(o.findings...)
		if o.apply != nil {
			o.apply(&r)
		}
	}

	for _, c := range Checks {
		r.Findings = append(r.Findings, byCheck[c.ID]...)
	}
	for _, f := range r.Findings {
		switch f.Severity {
		case OK:
			r.Summary.OK++
		case Info:
			r.Summary.Info++
		case Warn:
			r.Summary.Warn++
		case Fail:
			r.Summary.Fail++
		}
	}
	r.DurationMS = sys.Now().Sub(start).Milliseconds()
	return r
}

// runtimeConfigKeys are the /configs fields a bundle may keep. It is an
// allowlist because the endpoint also returns inbound credentials
// (authentication, ss-config, vmess-config, tuic-server) and may add more.
var runtimeConfigKeys = map[string]bool{
	"port": true, "socks-port": true, "redir-port": true, "tproxy-port": true, "mixed-port": true,
	"tun": true, "allow-lan": true, "bind-address": true, "lan-allowed-ips": true,
	"lan-disallowed-ips": true, "skip-auth-prefixes": true, "inbound-tfo": true, "inbound-mptcp": true,
	"mode": true, "unified-delay": true, "log-level": true, "ipv6": true, "interface-name": true,
	"routing-mark": true, "geo-auto-update": true, "geo-update-interval": true, "geodata-mode": true,
	"geodata-loader": true, "geosite-matcher": true, "tcp-concurrent": true, "find-process-mode": true,
	"sniffing": true, "global-ua": true, "etag-support": true, "keep-alive-idle": true,
	"keep-alive-interval": true, "disable-keep-alive": true,
}

// fetchConfig reads /configs once into the typed config and a filtered copy
// for the bundle. The filtered copy records only whether inbound
// authentication is set.
func fetchConfig(c *api.Client) (*api.Config, map[string]json.RawMessage, error) {
	body, err := c.GetConfigRaw()
	if err != nil {
		return nil, nil, err
	}
	var cfg api.Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return nil, nil, fmt.Errorf("unmarshal /configs: %w", err)
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(body, &all); err != nil {
		return nil, nil, fmt.Errorf("unmarshal /configs: %w", err)
	}
	kept := make(map[string]json.RawMessage, len(runtimeConfigKeys)+1)
	for k, v := range all {
		if runtimeConfigKeys[k] {
			kept[k] = v
		}
	}
	kept["inbound-auth-set"] = json.RawMessage(fmt.Sprint(len(cfg.Authentication) > 0))
	// Only the number of credentials matters from here on; blank the values
	// so nothing downstream can leak them.
	for i := range cfg.Authentication {
		cfg.Authentication[i] = ""
	}
	return &cfg, kept, nil
}
