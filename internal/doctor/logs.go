package doctor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// maxLogLines caps what a long capture keeps in memory; later lines are
// counted as dropped.
const maxLogLines = 100_000

// captureEnd says how a log capture stopped.
type captureEnd struct {
	connected bool
	early     bool  // the caller cancelled before the duration elapsed
	err       error // a stream error other than reaching the duration
	elapsed   time.Duration
}

// captureLogs streams /logs at level for d. StreamLogs closes its channel
// only after a successful dial, so a dial failure shows up on errCh alone.
func captureLogs(ctx context.Context, c *api.Client, level string, d time.Duration, now func() time.Time) ([]LogLine, int, captureEnd) {
	start := now()
	capCtx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	ch := make(chan api.LogEntry, 256)
	errCh := make(chan error, 1)
	go func() { errCh <- c.StreamLogs(capCtx, level, ch) }()

	var lines []LogLine
	dropped := 0
	keep := func(e api.LogEntry) {
		if len(lines) >= maxLogLines {
			dropped++
			return
		}
		lines = append(lines, LogLine{Time: now(), Level: e.Type, Payload: redactURLs(e.Payload)})
	}
	end := captureEnd{}
	var streamErr error
loop:
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				end.connected = true
				streamErr = <-errCh
				break loop
			}
			keep(e)
		case streamErr = <-errCh:
			// When the dial succeeded, close(ch) happened before this send,
			// so draining reaches the closed state; otherwise it never does.
			for {
				select {
				case e, ok := <-ch:
					if !ok {
						end.connected = true
						break loop
					}
					keep(e)
				default:
					break loop
				}
			}
		}
	}
	end.elapsed = now().Sub(start)
	switch {
	case ctx.Err() != nil:
		end.early = true
	case streamErr != nil && !errors.Is(streamErr, context.DeadlineExceeded) && !errors.Is(streamErr, context.Canceled):
		end.err = streamErr
	}
	return lines, dropped, end
}

func logFindings(lines []LogLine, dropped int, end captureEnd, d time.Duration, level string) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkLogs, ID: "logs." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	if !end.connected {
		msg := "unknown error"
		if end.err != nil {
			msg = end.err.Error()
		}
		return []Finding{f(Warn, "unavailable", "Could not stream core logs", msg, "")}
	}
	counts := map[string]int{}
	for _, l := range lines {
		counts[l.Level]++
	}
	var parts []string
	for _, lv := range []string{"debug", "info", "warning", "error"} {
		if counts[lv] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", lv, counts[lv]))
		}
	}
	span := d.String()
	var notes []string
	if end.early {
		span = end.elapsed.Round(time.Second).String()
		notes = append(notes, "capture stopped early")
	}
	if end.err != nil {
		notes = append(notes, "stream ended: "+end.err.Error())
	}
	if dropped > 0 {
		notes = append(notes, fmt.Sprintf("kept the first %d lines, dropped %d", len(lines), dropped))
	}
	problems := counts["warning"] + counts["error"]
	sev := OK
	if problems > 0 {
		sev = Info
	}
	summary := fmt.Sprintf("%d line(s) in %s at level %s", len(lines), span, level)
	detail := strings.Join(parts, ", ")
	if len(lines) == 0 {
		detail = "no log lines arrived; the core may be idle"
	}
	out := []Finding{f(sev, "summary", summary, joinLines(detail, strings.Join(notes, "; ")), "")}
	if problems == 0 {
		return out
	}

	patterns := groupPatterns(lines)
	var pl []string
	for _, p := range patterns[:min(5, len(patterns))] {
		pl = append(pl, fmt.Sprintf("%dx %s: %s", p.count, p.level, p.pattern))
	}
	out = append(out, f(Warn, "patterns",
		fmt.Sprintf("%d warning and %d error line(s) in %d pattern(s)", counts["warning"], counts["error"], len(patterns)),
		strings.Join(pl, "\n"), "Watch them live with clash-vr-tui logs --level warning; --out keeps every line in logs.ndjson"))

	outbounds, dests := map[string]*tally{}, map[string]*tally{}
	for _, l := range lines {
		if l.Level != "warning" && l.Level != "error" {
			continue
		}
		if ob, dst, ok := parseDialError(l.Payload); ok {
			bumpTally(outbounds, ob)
			bumpTally(dests, dst)
		}
	}
	if len(outbounds) > 0 {
		out = append(out, f(Info, "dial-failures", "Failed dials",
			"by outbound: "+tallyList(topN(outbounds, 5, false))+"\nby destination: "+tallyList(topN(dests, 5, false)),
			"Test a failing group with clash-vr-tui test <group>; DIRECT failures mean the host needs a proxy rule"))
	}
	return out
}

func bumpTally(m map[string]*tally, key string) {
	t := m[key]
	if t == nil {
		t = &tally{name: key}
		m[key] = t
	}
	t.count++
}

func tallyList(ts []tally) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = fmt.Sprintf("%s (%d)", t.name, t.count)
	}
	return strings.Join(parts, ", ")
}

type pattern struct {
	level   string
	pattern string
	count   int
	example string
}

// groupPatterns groups warning and error lines by normalized payload,
// most frequent first.
func groupPatterns(lines []LogLine) []pattern {
	idx := map[string]int{}
	var out []pattern
	for _, l := range lines {
		if l.Level != "warning" && l.Level != "error" {
			continue
		}
		p := normalizePattern(l.Payload)
		key := l.Level + "\x00" + p
		if i, ok := idx[key]; ok {
			out[i].count++
			continue
		}
		idx[key] = len(out)
		out = append(out, pattern{level: l.Level, pattern: p, count: 1, example: l.Payload})
	}
	sortPatterns(out)
	return out
}

func sortPatterns(ps []pattern) {
	// Stable insertion keeps first-seen order among equal counts.
	for i := 1; i < len(ps); i++ {
		for j := i; j > 0 && ps[j].count > ps[j-1].count; j-- {
			ps[j], ps[j-1] = ps[j-1], ps[j]
		}
	}
}

var (
	ipv4Re     = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}\b`)
	ipv6Re     = regexp.MustCompile(`\[?[0-9A-Fa-f]{0,4}(?::[0-9A-Fa-f]{0,4}){2,7}(?:%[0-9A-Za-z]+)?\]?`)
	portProcRe = regexp.MustCompile(`:\d+\([^)]*\)`)
	portRe     = regexp.MustCompile(`:\d+\b`)
	digitsRe   = regexp.MustCompile(`\d+`)
)

// normalizePattern masks what varies between otherwise identical log lines:
// IP addresses, ports, the process name after a source port, and other digit
// runs.
func normalizePattern(s string) string {
	s = ipv4Re.ReplaceAllString(s, "<ip>")
	s = maskIPv6(s)
	s = portProcRe.ReplaceAllString(s, ":<port>(<proc>)")
	s = portRe.ReplaceAllString(s, ":<port>")
	return digitsRe.ReplaceAllString(s, "<n>")
}

// maskIPv6 replaces IPv6 addresses. RE2 has no lookbehind, so a candidate
// glued to a word character (the "::E" in "hyper::Error") is left alone,
// as is anything net.ParseIP rejects, such as a clock time.
func maskIPv6(s string) string {
	locs := ipv6Re.FindAllStringIndex(s, -1)
	if locs == nil {
		return s
	}
	var b strings.Builder
	last := 0
	for _, l := range locs {
		if l[0] > 0 && isWordByte(s[l[0]-1]) || l[1] < len(s) && isWordByte(s[l[1]]) {
			continue
		}
		host := strings.Trim(s[l[0]:l[1]], "[]")
		if i := strings.IndexByte(host, '%'); i >= 0 {
			host = host[:i]
		}
		if net.ParseIP(host) == nil {
			continue
		}
		b.WriteString(s[last:l[0]])
		b.WriteString("<ip>")
		last = l[1]
	}
	b.WriteString(s[last:])
	return b.String()
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

var urlRe = regexp.MustCompile(`(?i)\b((?:https?|wss?)://)([^/\s"'<>()]+)([^\s"'<>()]*)`)

// redactURLs keeps the scheme and host of each URL and drops credentials,
// path and query. A provider fetch error carries the subscription URL, whose
// path or query is usually the account token.
func redactURLs(s string) string {
	return urlRe.ReplaceAllStringFunc(s, func(m string) string {
		// Punctuation that ends a sentence is not part of the URL.
		trimmed := strings.TrimRight(m, ".,;:")
		parts := urlRe.FindStringSubmatch(trimmed)
		if parts == nil {
			return m
		}
		host := parts[2]
		if i := strings.LastIndexByte(host, '@'); i >= 0 {
			host = host[i+1:]
		}
		rest := parts[3]
		if rest != "" && rest != "/" {
			rest = "/***"
		}
		return parts[1] + host + rest + m[len(trimmed):]
	})
}

// dialErrRe matches mihomo's failed-dial lines:
//
//	[TCP] dial PROXY (match TYPE/PAYLOAD) SRC:PORT(PROCESS) --> HOST:PORT error: ...
//	[TCP] dial PROXY SRC:PORT --> HOST:PORT error: ...
//
// The outbound name may contain spaces, so it runs up to the optional match
// clause or the source address.
var dialErrRe = regexp.MustCompile(`^\[(?:TCP|UDP)\] dial (.+?) (?:\(match [^)]*\) )?\S+:\d+(?:\([^)]*\))? --> (\S+) error`)

// parseDialError returns the outbound and destination host of a failed dial.
func parseDialError(payload string) (outbound, host string, ok bool) {
	m := dialErrRe.FindStringSubmatch(payload)
	if m == nil {
		return "", "", false
	}
	host = m[2]
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	return m[1], host, true
}
