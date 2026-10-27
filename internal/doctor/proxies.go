package doctor

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// probeConcurrency bounds how many group delay tests run at once.
const probeConcurrency = 4

// builtinTypes are outbounds that are not remote nodes; a group pointing at
// one has nothing to delay-test.
var builtinTypes = map[string]bool{"Direct": true, "Reject": true, "RejectDrop": true, "Pass": true, "Compatible": true, "Dns": true}

func groupsCheck(ctx context.Context, in Inputs) []Finding {
	groups, err := in.Client.GetGroups()
	if err != nil {
		return []Finding{{Check: checkGroups, ID: "groups.unreadable", Severity: Warn, Title: "Could not read proxy groups", Detail: err.Error()}}
	}
	groups = visibleGroups(groups)
	var probeErrs map[string]error
	if in.Probe {
		probeErrs = probeGroups(ctx, in.Client, groups, in.TestURL, in.ProbeMS)
	}
	proxies, err := in.Client.GetProxies()
	if err != nil {
		return []Finding{{Check: checkGroups, ID: "groups.unreadable", Severity: Warn, Title: "Could not read delay history", Detail: err.Error()}}
	}
	out := groupFindings(groups, proxies.Proxies, in.Probe)
	if n := len(probeErrs); n > 0 {
		var lines []string
		for _, g := range groups {
			if err := probeErrs[g.Name]; err != nil {
				lines = append(lines, g.Name+": "+err.Error())
			}
		}
		out = append(out, Finding{Check: checkGroups, ID: "groups.probe-errors", Severity: Info,
			Title:  fmt.Sprintf("%d group delay test(s) returned an error", n),
			Detail: strings.Join(lines, "\n"),
			Hint:   "mihomo answers an error when every node in the group timed out; the findings above already count them as failed"})
	}
	return out
}

func visibleGroups(groups []api.Group) []api.Group {
	out := make([]api.Group, 0, len(groups))
	for _, g := range groups {
		if !g.Hidden && g.Name != "GLOBAL" {
			out = append(out, g)
		}
	}
	return out
}

// probeGroups runs a fresh delay test on each group. mihomo records every
// result in /proxies history, which groupFindings then reads.
func probeGroups(ctx context.Context, c *api.Client, groups []api.Group, testURL string, timeoutMS int) map[string]error {
	var mu sync.Mutex
	errs := map[string]error{}
	sem := make(chan struct{}, probeConcurrency)
	var wg sync.WaitGroup
	for _, g := range groups {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return errs
		}
		wg.Add(1)
		go func(name string) {
			defer func() { <-sem; wg.Done() }()
			if _, err := c.TestGroupDelay(name, testURL, timeoutMS); err != nil {
				mu.Lock()
				errs[name] = err
				mu.Unlock()
			}
		}(g.Name)
	}
	wg.Wait()
	return errs
}

func lastDelay(p api.Proxy) (delay int, tested bool) {
	if len(p.History) == 0 {
		return 0, false
	}
	return p.History[len(p.History)-1].Delay, true
}

// leaf follows a group's selection through nested groups to the node that
// carries traffic. It returns "" when the chain ends in a group with no
// selection (LoadBalance, Relay) or loops.
func leaf(name string, proxies map[string]api.Proxy) string {
	for range 16 {
		p, ok := proxies[name]
		if !ok || len(p.All) == 0 {
			return name
		}
		if p.Now == "" {
			return ""
		}
		name = p.Now
	}
	return ""
}

// groupFindings applies the delay-history rules: a group whose tested nodes
// all failed is a fail, a selected node that failed its last test is a warn,
// and a selected node with no history is reported as untested.
func groupFindings(groups []api.Group, proxies map[string]api.Proxy, probed bool) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkGroups, ID: "groups." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	var out []Finding
	failedLeaf := map[string][]string{} // leaf node -> groups selecting it
	var leafOrder, untested []string
	healthy := 0
	for _, g := range groups {
		// Every member resolved to its leaf; built-ins are not tested.
		seen := map[string]bool{}
		tested, alive := 0, 0
		for _, m := range g.All {
			l := leaf(m, proxies)
			if l == "" || seen[l] || builtinTypes[proxies[l].Type] {
				continue
			}
			seen[l] = true
			if d, ok := lastDelay(proxies[l]); ok {
				tested++
				if d > 0 {
					alive++
				}
			}
		}
		if tested > 0 && alive == 0 {
			out = append(out, f(Fail, "all-failed", fmt.Sprintf("Every tested node in %s failed", g.Name),
				fmt.Sprintf("%d of %d nodes tested, none answered", tested, len(seen)),
				"clash-vr-tui test '"+g.Name+"'; if all still fail, update the subscription (clash-vr-tui providers update) or check the network"))
			continue
		}
		l := leaf(g.Name, proxies)
		if l == "" || builtinTypes[proxies[l].Type] {
			healthy++
			continue
		}
		d, ok := lastDelay(proxies[l])
		switch {
		case !ok:
			untested = append(untested, g.Name)
		case d == 0:
			if _, seen := failedLeaf[l]; !seen {
				leafOrder = append(leafOrder, l)
			}
			failedLeaf[l] = append(failedLeaf[l], g.Name)
		default:
			healthy++
		}
	}
	for _, l := range leafOrder {
		gs := failedLeaf[l]
		out = append(out, f(Warn, "leaf-failed", fmt.Sprintf("%s failed its last delay test", l),
			"Selected by "+strings.Join(gs, ", "),
			"clash-vr-tui test '"+gs[0]+"', then switch to a working node (or unpin a URLTest/Fallback group)"))
	}
	if len(untested) > 0 {
		hint := "Run doctor --probe, or clash-vr-tui test <group>"
		if probed {
			hint = ""
		}
		out = append(out, f(Info, "untested", fmt.Sprintf("%d group(s) select a node with no delay history", len(untested)),
			listSome(untested, 8), hint))
	}
	if healthy > 0 {
		out = append([]Finding{f(OK, "ok", fmt.Sprintf("%d of %d groups select a node that passed its last test or a built-in outbound", healthy, len(groups)), "", "")}, out...)
	}
	if len(groups) == 0 {
		out = append(out, f(Info, "none", "No visible proxy groups", "", ""))
	}
	return out
}

func listSome(items []string, n int) string {
	if len(items) <= n {
		return strings.Join(items, ", ")
	}
	return strings.Join(items[:n], ", ") + fmt.Sprintf(" and %d more", len(items)-n)
}

func providersCheck(c *api.Client, now time.Time) []Finding {
	pp, err := c.GetProxyProviders()
	if err != nil {
		return []Finding{{Check: checkProviders, ID: "providers.unreadable", Severity: Warn, Title: "Could not read proxy providers", Detail: err.Error()}}
	}
	rp, err := c.GetRuleProviders()
	if err != nil {
		return append(providerFindings(pp, nil, now), Finding{Check: checkProviders, ID: "providers.rules-unreadable",
			Severity: Warn, Title: "Could not read rule providers", Detail: err.Error()})
	}
	return providerFindings(pp, rp, now)
}

const (
	staleAfter  = 7 * 24 * time.Hour
	expiringIn  = 7 * 24 * time.Hour
	usageWarnAt = 0.9
)

// providerFindings checks subscriptions (expiry, usage, freshness, node
// count) and HTTP rule sets (freshness, rule count). Groups that mihomo
// mirrors as "Compatible" providers are skipped.
func providerFindings(pp map[string]api.ProxyProvider, rp map[string]api.RuleProvider, now time.Time) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkProviders, ID: "providers." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	var out []Finding
	names := make([]string, 0, len(pp))
	for name, p := range pp {
		if !p.IsGroup() {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		p := pp[name]
		var problems []Finding
		var facts []string
		facts = append(facts, fmt.Sprintf("%d nodes", len(p.Proxies)))
		if len(p.Proxies) == 0 {
			problems = append(problems, f(Fail, "empty", "Provider "+name+" has no nodes", "",
				"clash-vr-tui providers update '"+name+"'; if it stays empty, check the subscription URL in Clash Verge Rev"))
		}
		if strings.EqualFold(p.VehicleType, "HTTP") {
			switch {
			case p.UpdatedAt.IsZero():
				problems = append(problems, f(Warn, "stale", "Provider "+name+" was never updated", "",
					"clash-vr-tui providers update '"+name+"'"))
			case now.Sub(p.UpdatedAt) > staleAfter:
				problems = append(problems, f(Warn, "stale", "Provider "+name+" was not updated for "+ago(now, p.UpdatedAt),
					"last update "+p.UpdatedAt.Local().Format("2006-01-02 15:04"), "clash-vr-tui providers update '"+name+"'"))
			default:
				facts = append(facts, "updated "+ago(now, p.UpdatedAt)+" ago")
			}
		}
		if s := p.SubscriptionInfo; s != nil {
			if s.Total > 0 {
				used := float64(s.Upload+s.Download) / float64(s.Total)
				usage := fmt.Sprintf("%.0f%% of %s used", math.Floor(used*100), formatBytes(s.Total))
				switch {
				case used >= 1:
					problems = append(problems, f(Fail, "quota", "Provider "+name+" has used its whole traffic quota", usage,
						"Renew or upgrade the plan with the provider"))
				case used > usageWarnAt:
					problems = append(problems, f(Warn, "quota", "Provider "+name+" is close to its traffic quota", usage,
						"Renew or upgrade the plan before it runs out"))
				default:
					facts = append(facts, usage)
				}
			}
			if s.Expire > 0 {
				exp := time.Unix(s.Expire, 0)
				date := exp.Local().Format("2006-01-02")
				switch {
				case !exp.After(now):
					problems = append(problems, f(Fail, "expired", "Subscription "+name+" expired on "+date, "",
						"Renew the subscription, then clash-vr-tui providers update '"+name+"'"))
				case exp.Sub(now) < expiringIn:
					problems = append(problems, f(Warn, "expiring", "Subscription "+name+" expires on "+date, "in "+ago(exp, now),
						"Renew the subscription soon"))
				default:
					facts = append(facts, "expires "+date)
				}
			}
		}
		if len(problems) == 0 {
			out = append(out, f(OK, "ok", "Provider "+name, strings.Join(facts, ", "), ""))
		}
		out = append(out, problems...)
	}
	if len(names) == 0 {
		out = append(out, f(Info, "none", "No proxy providers; the nodes come from the profile itself", "", ""))
	}

	var stale, empty []string
	for name, p := range rp {
		if !strings.EqualFold(p.VehicleType, "HTTP") {
			continue
		}
		if p.RuleCount == 0 {
			empty = append(empty, name)
		} else if p.UpdatedAt.IsZero() || now.Sub(p.UpdatedAt) > staleAfter {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	sort.Strings(empty)
	if len(empty) > 0 {
		out = append(out, f(Warn, "rules-empty", fmt.Sprintf("%d rule set(s) have no rules", len(empty)), listSome(empty, 8),
			"Their download likely failed; clash-vr-tui providers update, then check the logs"))
	}
	if len(stale) > 0 {
		out = append(out, f(Warn, "rules-stale", fmt.Sprintf("%d rule set(s) not updated for 7 days", len(stale)), listSome(stale, 8),
			"clash-vr-tui providers update"))
	}
	return out
}

// ago renders the span between two times coarsely: 3d, 5h, 12m.
func ago(later, earlier time.Time) string {
	d := later.Sub(earlier)
	switch {
	case d >= 48*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1<<40:
		return fmt.Sprintf("%.1f TB", float64(b)/(1<<40))
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

type tally struct {
	name  string
	count int
	bytes int64
}

// topN sorts by count (or bytes when byBytes), then name, and keeps n.
func topN(m map[string]*tally, n int, byBytes bool) []tally {
	out := make([]tally, 0, len(m))
	for _, t := range m {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if byBytes && a.bytes != b.bytes {
			return a.bytes > b.bytes
		}
		if a.count != b.count {
			return a.count > b.count
		}
		return a.name < b.name
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func connHost(c api.Connection) string {
	if c.Metadata.Host != "" {
		return c.Metadata.Host
	}
	return c.Metadata.DstIP
}

// routingFindings summarizes current connections: how many fell through to
// the final MATCH rule (candidates for new rules), the busiest hosts, and
// the exit nodes in use.
func routingFindings(snap *api.ConnectionsSnapshot) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkRouting, ID: "routing." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	conns := snap.Connections
	if len(conns) == 0 {
		return []Finding{f(Info, "idle", "No active connections to analyze", "", "")}
	}
	matchHosts := map[string]*tally{}
	matchPolicy := map[string]*tally{}
	hosts := map[string]*tally{}
	nodes := map[string]*tally{}
	matched := 0
	bump := func(m map[string]*tally, key string, bytes int64) {
		if key == "" {
			return
		}
		t := m[key]
		if t == nil {
			t = &tally{name: key}
			m[key] = t
		}
		t.count++
		t.bytes += bytes
	}
	for _, c := range conns {
		host := connHost(c)
		traffic := c.Upload + c.Download
		bump(hosts, host, traffic)
		if len(c.Chains) > 0 {
			bump(nodes, c.Chains[0], traffic)
		}
		if strings.EqualFold(c.Rule, "Match") {
			matched++
			bump(matchHosts, host, traffic)
			if len(c.Chains) > 0 {
				bump(matchPolicy, c.Chains[len(c.Chains)-1], 0)
			}
		}
	}

	var out []Finding
	if matched == 0 {
		out = append(out, f(OK, "match", fmt.Sprintf("None of %d connections fell through to MATCH", len(conns)), "", ""))
	} else {
		var policies []string
		for _, t := range topN(matchPolicy, 3, false) {
			policies = append(policies, t.name)
		}
		var lines []string
		for _, t := range topN(matchHosts, 5, false) {
			lines = append(lines, fmt.Sprintf("%s (%d)", t.name, t.count))
		}
		out = append(out, f(Info, "match",
			fmt.Sprintf("%d of %d connections (%.0f%%) fell through to MATCH, sent to %s", matched, len(conns),
				100*float64(matched)/float64(len(conns)), strings.Join(policies, ", ")),
			"Top hosts there: "+strings.Join(lines, ", "),
			"Add rules for hosts that should not take the MATCH policy"))
	}
	var lines []string
	for _, t := range topN(hosts, 5, true) {
		lines = append(lines, fmt.Sprintf("%s  %s in %d conn(s)", t.name, formatBytes(t.bytes), t.count))
	}
	out = append(out, f(Info, "top-hosts", "Top hosts by traffic", strings.Join(lines, "\n"), ""))
	lines = nil
	for _, t := range topN(nodes, 5, false) {
		lines = append(lines, fmt.Sprintf("%s  %d conn(s), %s", t.name, t.count, formatBytes(t.bytes)))
	}
	out = append(out, f(Info, "top-nodes", "Top exit nodes", strings.Join(lines, "\n"), ""))
	return out
}
