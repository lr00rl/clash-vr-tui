package doctor

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/styles"
	"gopkg.in/yaml.v3"
)

const vergeID = "io.github.clash-verge-rev.clash-verge-rev"

// vergeLogTail is how much of each Verge log is read.
const vergeLogTail = 64 << 10

// vergeDirs lists where Clash Verge Rev keeps its app directory.
func vergeDirs(sys System) []string {
	switch sys.GOOS {
	case "darwin":
		return []string{filepath.Join(sys.Home, "Library", "Application Support", vergeID)}
	case "windows":
		if appData := sys.Getenv("APPDATA"); appData != "" {
			return []string{filepath.Join(appData, vergeID)}
		}
		return nil
	default:
		return []string{filepath.Join(sys.Home, ".local", "share", vergeID), filepath.Join(sys.Home, ".config", vergeID)}
	}
}

func vergeFindings(sys System, endpoint string) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkVerge, ID: "verge." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	if sys.FS == nil || sys.Home == "" {
		return nil
	}
	dir := ""
	for _, d := range vergeDirs(sys) {
		if info, err := fs.Stat(sys.FS, fsPath(d)); err == nil && info.IsDir() {
			dir = d
			break
		}
	}
	if dir == "" {
		return []Finding{f(Info, "absent", "Clash Verge Rev app directory not found", strings.Join(vergeDirs(sys), "\n"), "")}
	}

	var facts []string
	switch {
	case strings.Contains(endpoint, "clash-verge-service"):
		facts = append(facts, "service mode: the core runs as root under clash-verge-service")
	case strings.Contains(endpoint, "verge"):
		facts = append(facts, "user mode: the core runs as your user")
	}
	if name, err := currentProfile(sys.FS, fsPath(filepath.Join(dir, "profiles.yaml"))); err == nil && name != "" {
		facts = append(facts, "current profile: "+name)
	}
	facts = append(facts, "app directory: "+shortHome(dir, sys.Home))
	out := []Finding{f(OK, "found", "Clash Verge Rev found", strings.Join(facts, "\n"), "")}

	// Log labels are relative to the app directory printed above.
	root := fsPath(dir) + "/"
	logs := []string{root + "logs/latest.log"}
	if svc := newestFile(sys.FS, root+"service-logs"); svc != "" {
		logs = append(logs, svc)
	}
	for _, name := range logs {
		data, err := readTail(sys.FS, name, vergeLogTail)
		if err != nil {
			continue
		}
		out = append(out, vergeLogFinding(strings.TrimPrefix(name, root), string(data), sys.Now()))
	}
	return out
}

// currentProfile returns the name of the profile profiles.yaml marks
// current. Only uid, name and type are read; the file also holds
// subscription URLs, which never leave this function.
func currentProfile(fsys fs.FS, name string) (string, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", err
	}
	var doc struct {
		Current string `yaml:"current"`
		Items   []struct {
			UID  string `yaml:"uid"`
			Name string `yaml:"name"`
			Type string `yaml:"type"`
		} `yaml:"items"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	for _, it := range doc.Items {
		if it.UID == doc.Current {
			label := it.Name
			if label == "" {
				label = it.UID
			}
			if it.Type != "" {
				label += " (" + it.Type + ")"
			}
			return label, nil
		}
	}
	return "", nil
}

// newestFile returns the fs name of the most recently modified regular file
// under dir (one level of subdirectories deep), or "" when there is none.
func newestFile(fsys fs.FS, dir string) string {
	var best string
	var bestTime time.Time
	_ = fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Count(strings.TrimPrefix(p, dir), "/") > 1 {
				return fs.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if best == "" || info.ModTime().After(bestTime) {
			best, bestTime = p, info.ModTime()
		}
		return nil
	})
	return best
}

func shortHome(p, home string) string {
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// vergeLineRe matches "[2026-10-27 00:22:40.713] ERROR message".
var vergeLineRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?)\]\s+(ERROR|WARN)\s+(.*)$`)

// vergeHints explains Verge log patterns whose cause is known.
var vergeHints = []struct{ contains, hint string }{
	{"hyper::Error(Parse(Method))", "Verge's local HTTP server (it serves the PAC file) received bytes that are not an HTTP request, " +
		"such as TLS or SOCKS sent to that port. It is noise unless it is constant; then look for an app configured with the PAC port as its proxy"},
}

type vergePattern struct {
	level   string
	message string
	count   int
	last    time.Time
}

// vergeLogFinding groups ERROR and WARN lines of a Verge log tail by pattern.
// Recent errors (the last hour) are a warn; older ones are context.
func vergeLogFinding(label, data string, now time.Time) Finding {
	f := Finding{Check: checkVerge, ID: "verge.log", Title: label}
	idx := map[string]int{}
	var pats []vergePattern
	errs, warns := 0, 0
	recentErr := false
	for _, line := range strings.Split(data, "\n") {
		m := vergeLineRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		ts, err := time.ParseInLocation("2006-01-02 15:04:05", m[1], now.Location())
		if err != nil {
			continue
		}
		if m[2] == "ERROR" {
			errs++
			if now.Sub(ts) <= time.Hour {
				recentErr = true
			}
		} else {
			warns++
		}
		msg := normalizePattern(styles.RedactURLs(m[3]))
		key := m[2] + "\x00" + msg
		i, ok := idx[key]
		if !ok {
			i = len(pats)
			idx[key] = i
			pats = append(pats, vergePattern{level: m[2], message: msg})
		}
		pats[i].count++
		if ts.After(pats[i].last) {
			pats[i].last = ts
		}
	}
	if errs+warns == 0 {
		f.Severity = OK
		f.Title = label + ": no ERROR or WARN lines in the last 64 KB"
		return f
	}
	for i := 1; i < len(pats); i++ {
		for j := i; j > 0 && pats[j].count > pats[j-1].count; j-- {
			pats[j], pats[j-1] = pats[j-1], pats[j]
		}
	}
	f.Severity = Info
	if recentErr {
		f.Severity = Warn
	}
	f.Title = fmt.Sprintf("%s: %d ERROR and %d WARN line(s) in the last 64 KB", label, errs, warns)
	var lines []string
	var hints []string
	for _, p := range pats[:min(5, len(pats))] {
		lines = append(lines, fmt.Sprintf("%dx %s %s (last %s)", p.count, p.level, p.message, p.last.Format("01-02 15:04")))
		for _, h := range vergeHints {
			if strings.Contains(p.message, h.contains) && !contains(hints, h.hint) {
				hints = append(hints, h.hint)
			}
		}
	}
	f.Detail = strings.Join(lines, "\n")
	f.Hint = strings.Join(hints, "; ")
	return f
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// process is one ps line reduced to what conflict detection needs.
type process struct {
	pid     string
	name    string // executable name, e.g. "ClashX Meta" or "verge-mihomo"
	command string
}

// parsePS reads "pid command" lines.
func parsePS(out string) []process {
	var procs []process
	for _, line := range strings.Split(out, "\n") {
		pid, cmd, ok := strings.Cut(strings.TrimSpace(line), " ")
		cmd = strings.TrimSpace(cmd)
		if !ok || cmd == "" {
			continue
		}
		procs = append(procs, process{pid: pid, name: exeName(cmd), command: cmd})
	}
	return procs
}

// exeName extracts the executable name from a command line. An absolute
// path may contain spaces ("/Library/Application Support/..."), so it runs
// until the first " -flag" or " /path" argument. App bundles are named by
// the binary under Contents/MacOS, which may contain spaces ("ClashX Meta").
func exeName(cmd string) string {
	exe := cmd
	if i := strings.Index(exe, " -"); i > 0 {
		exe = exe[:i]
	}
	if !strings.HasPrefix(exe, "/") {
		return path.Base(strings.Fields(exe)[0])
	}
	if i := strings.Index(exe, " /"); i > 0 {
		exe = exe[:i]
	}
	if i := strings.Index(exe, "/Contents/MacOS/"); i >= 0 {
		return strings.TrimSpace(exe[i+len("/Contents/MacOS/"):])
	}
	return strings.Fields(path.Base(exe))[0]
}

// ownClients are processes that belong to the setup doctor examines.
var ownClients = map[string]bool{"clash-verge": true, "clash verge": true, "clash-verge-service": true, "clash-vr-tui": true}

func isCore(name string) bool {
	n := strings.ToLower(name)
	for _, p := range []string{"mihomo", "verge-mihomo", "clash-meta", "clash.meta", "flclashcore", "clash-linux", "clash-darwin", "clash-windows"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return n == "clash" || n == "clash-premium"
}

func conflictFindings(ctx context.Context, sys System, endpoint string) []Finding {
	f := func(sev Severity, id, title, detail, hint string) Finding {
		return Finding{Check: checkConflicts, ID: "conflicts." + id, Severity: sev, Title: title, Detail: detail, Hint: hint}
	}
	if sys.PS == nil {
		return nil
	}
	if sys.GOOS == "windows" {
		return []Finding{f(Info, "unchecked", "Process list not checked on Windows", "", "")}
	}
	out, err := sys.PS(ctx)
	if err != nil {
		return []Finding{f(Info, "unchecked", "Could not list processes", err.Error(), "")}
	}
	var cores, clients, helpers []process
	for _, p := range parsePS(out) {
		n := strings.ToLower(p.name)
		if !strings.Contains(n, "clash") && !strings.Contains(n, "mihomo") || ownClients[n] {
			continue
		}
		switch {
		case strings.Contains(n, "helper"):
			helpers = append(helpers, p)
		case isCore(p.name):
			cores = append(cores, p)
		default:
			clients = append(clients, p)
		}
	}
	// With several cores, the one whose command line names the endpoint is
	// ours. A single core is assumed to be ours.
	var extra []process
	extraNote := ": another core"
	if len(cores) > 1 {
		var mine []process
		for _, c := range cores {
			if endpoint != "" && strings.Contains(c.command, endpoint) {
				mine = append(mine, c)
			} else {
				extra = append(extra, c)
			}
		}
		if len(mine) == 0 {
			extra, extraNote = cores, ": one of several cores (could not tell which serves the controller)"
		}
	}
	label := func(p process) string { return fmt.Sprintf("%s (pid %s)", p.name, p.pid) }
	var res []Finding
	if len(clients) > 0 || len(extra) > 0 {
		var lines []string
		for _, p := range clients {
			lines = append(lines, label(p))
		}
		for _, p := range extra {
			lines = append(lines, label(p)+extraNote)
		}
		res = append(res, f(Warn, "running", "Other Clash clients are running", strings.Join(lines, "\n"),
			"Two clients fight over the system proxy, TUN routes and ports; quit the one you do not use"))
	}
	if len(helpers) > 0 {
		var lines []string
		for _, p := range helpers {
			lines = append(lines, label(p))
		}
		res = append(res, f(Info, "helpers", "Helper of another Clash client is running", strings.Join(lines, "\n"),
			"It stays idle unless its app runs; uninstall that app if you no longer use it"))
	}
	if len(res) == 0 {
		res = append(res, f(OK, "none", "No other Clash clients running", "", ""))
	}
	return res
}
