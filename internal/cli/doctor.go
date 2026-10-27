package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/doctor"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

// doctorSystem is the machine doctor reads; tests replace it.
var doctorSystem = doctor.LocalSystem

const doctorDefaultLogs = 10 * time.Second

// cmdDoctor runs every check, prints the findings, optionally writes a
// bundle, and fails (exit 1) when any finding is a fail. Ctrl+C ends the log
// capture early and still reports.
func cmdDoctor(c *api.Client, opts options) error {
	logsDur := opts.logsDur
	if logsDur < 0 {
		logsDur = doctorDefaultLogs
	}
	level := opts.level
	if level == "" {
		level = "debug"
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// The first Ctrl+C ends the capture and prints what was gathered; restoring
	// default handling lets a second one exit even if the controller hangs.
	go func() {
		<-ctx.Done()
		stop()
	}()
	if logsDur > 0 && !opts.json {
		fmt.Fprintf(os.Stderr, "Running checks and capturing %s of %s logs (Ctrl+C to stop early)...\n", logsDur, level)
	}
	report := doctor.Run(ctx, doctor.Inputs{
		Client:   c,
		Source:   opts.resolved.Source,
		TestURL:  opts.url,
		LogsDur:  logsDur,
		LogLevel: level,
		Probe:    opts.probe,
		ProbeMS:  opts.timeout,
		System:   doctorSystem(),
	})

	bundle := ""
	var bundleErr error
	if opts.out != "" {
		bundle, bundleErr = writeBundle(opts.out, report)
	}
	if opts.json {
		if err := printJSON(report); err != nil {
			return err
		}
	} else {
		printReport(os.Stdout, report, useColor(os.Stdout))
	}
	if bundleErr != nil {
		return fmt.Errorf("write bundle: %w", bundleErr)
	}
	if bundle != "" {
		// Keep stdout pure JSON under --json.
		w := os.Stdout
		if opts.json {
			w = os.Stderr
		}
		fmt.Fprintf(w, "Bundle: %s\n", bundle)
	}
	if report.Summary.Fail > 0 {
		return fmt.Errorf("doctor found %d failing check(s)", report.Summary.Fail)
	}
	return nil
}

func severityLabel(s doctor.Severity) string {
	if s == doctor.Fail {
		return "FAIL"
	}
	return string(s)
}

func severityColor(s doctor.Severity) string {
	switch s {
	case doctor.OK:
		return "\x1b[32m"
	case doctor.Warn:
		return "\x1b[33m"
	case doctor.Fail:
		return "\x1b[31m"
	}
	return "\x1b[36m"
}

// detailLines splits a multi-line detail and strips control characters from
// each line, since details carry controller-supplied text.
func detailLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = styles.SafeText(l)
	}
	return lines
}

func reportHeader(r doctor.Report) string {
	core := "core unreachable"
	if r.Version != "" {
		core = "mihomo " + r.Version
	}
	where := r.Endpoint
	if r.Source != "" {
		where += " (" + r.Source + ")"
	}
	return styles.SafeText(core + " at " + where)
}

func summaryLine(c doctor.Counts) string {
	return fmt.Sprintf("%d ok, %d info, %d warn, %d fail", c.OK, c.Info, c.Warn, c.Fail)
}

// printReport writes findings grouped by check, in check order.
func printReport(w io.Writer, r doctor.Report, color bool) {
	fmt.Fprintf(w, "clash-vr-tui doctor: %s\n", reportHeader(r))
	group := ""
	for _, f := range r.Findings {
		if f.Check != group {
			group = f.Check
			fmt.Fprintf(w, "\n%s\n", doctor.CheckTitle(group))
		}
		label := fmt.Sprintf("%-4s", severityLabel(f.Severity))
		if color {
			label = severityColor(f.Severity) + label + ansiReset
		}
		fmt.Fprintf(w, "  %s  %s\n", label, styles.SafeText(f.Title))
		for _, l := range detailLines(f.Detail) {
			fmt.Fprintf(w, "        %s\n", l)
		}
		if f.Hint != "" {
			fmt.Fprintf(w, "        -> %s\n", styles.SafeText(f.Hint))
		}
	}
	fmt.Fprintf(w, "\n%s\n", summaryLine(r.Summary))
}

var mdEscaper = strings.NewReplacer(`\`, `\\`, "`", "\\`", "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`,
	"<", `\<`, ">", `\>`, "#", `\#`, "|", `\|`)

func md(s string) string { return mdEscaper.Replace(styles.SafeText(s)) }

// renderMarkdown is the report.md form of printReport.
func renderMarkdown(w io.Writer, r doctor.Report) {
	fmt.Fprintf(w, "# clash-vr-tui doctor\n\n")
	fmt.Fprintf(w, "- Time: %s\n", r.Time.Format("2006-01-02 15:04:05 MST"))
	fmt.Fprintf(w, "- Controller: %s\n", md(reportHeader(r)))
	fmt.Fprintf(w, "- Summary: %s\n", summaryLine(r.Summary))
	group := ""
	for _, f := range r.Findings {
		if f.Check != group {
			group = f.Check
			fmt.Fprintf(w, "\n## %s\n\n", doctor.CheckTitle(group))
		}
		fmt.Fprintf(w, "- `%s` %s\n", severityLabel(f.Severity), md(f.Title))
		for _, l := range detailLines(f.Detail) {
			fmt.Fprintf(w, "  - %s\n", md(l))
		}
		if f.Hint != "" {
			fmt.Fprintf(w, "  - Next: %s\n", md(f.Hint))
		}
	}
}

// writeBundle saves the report and its raw material under
// DIR/doctor-<timestamp>/. It never writes profile files, subscription URLs,
// or credentials: the runtime config arrives already filtered to an
// allowlist, and nothing here reads Clash Verge Rev files.
func writeBundle(parent string, r doctor.Report) (string, error) {
	dir := filepath.Join(parent, "doctor-"+r.Time.Format("20060102-150405"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	var md strings.Builder
	renderMarkdown(&md, r)
	files := map[string][]byte{"report.md": []byte(md.String())}

	add := func(name string, v any) error {
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		files[name] = append(data, '\n')
		return nil
	}
	conns := r.Connections
	if conns == nil {
		conns = &api.ConnectionsSnapshot{}
	}
	cfg := r.RuntimeConfig
	if cfg == nil {
		cfg = map[string]json.RawMessage{}
	}
	if err := add("report.json", r); err != nil {
		return "", err
	}
	if err := add("connections.json", conns); err != nil {
		return "", err
	}
	if err := add("runtime-config.json", cfg); err != nil {
		return "", err
	}
	var logs strings.Builder
	for _, l := range r.Logs {
		line, err := json.Marshal(l)
		if err != nil {
			return "", fmt.Errorf("logs.ndjson: %w", err)
		}
		logs.Write(line)
		logs.WriteByte('\n')
	}
	files["logs.ndjson"] = []byte(logs.String())

	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}
