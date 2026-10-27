package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/config"
	"github.com/cdcd/clash-vr-tui/internal/discover"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

func cmdDiscover(_ *api.Client, opts options) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*discover.ProbeTimeout)
	defer cancel()
	cands := discover.Find(ctx)
	best, found := discover.Best(cands)

	if opts.json {
		out := map[string]any{"candidates": cands}
		if found {
			out["selected"] = best.Address
		}
		if src := explicitSource(opts); src != "" {
			out["explicit"] = src
		}
		if err := printJSON(out); err != nil {
			return err
		}
	} else {
		printCandidates(cands, best, found)
		if src := explicitSource(opts); src != "" {
			fmt.Printf("\nnote: %s names the controller explicitly, so auto-detect is not used.\n", src)
		} else if found {
			fmt.Println("\n* is what clash-vr-tui uses without flags. Pin a controller with --socket/--server or in")
			fmt.Printf("  %s\n", config.Path())
		}
	}
	if len(cands) == 0 {
		return errors.New("no controller candidates found in running processes, config files, or Clash Verge Rev paths")
	}
	if !found {
		return errors.New("no candidate answered GET /version")
	}
	return nil
}

func printCandidates(cands []discover.Candidate, best discover.Candidate, found bool) {
	if len(cands) == 0 {
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "  ADDRESS\tKIND\tSTATE\tVERSION\tSOURCE")
	for _, c := range cands {
		mark := " "
		if found && c.Address == best.Address {
			mark = "*"
		}
		source := c.Sources[0]
		if n := len(c.Sources) - 1; n > 0 {
			source += fmt.Sprintf(" (+%d)", n)
		}
		version := c.Version
		if version == "" {
			version = "-"
		}
		fmt.Fprintf(w, "%s %s\t%s\t%s\t%s\t%s\n", mark, styles.SafeText(c.Address), c.Kind, c.State, styles.SafeText(version), styles.SafeText(source))
	}
	w.Flush()
	for _, c := range cands {
		if c.State != discover.StateOK && c.Err != "" {
			fmt.Printf("  %s: %s\n", styles.SafeText(c.Address), styles.SafeText(c.Err))
		}
	}
}

// explicitSource names the tier that pins the controller, or "" when the
// endpoint would be auto-detected.
func explicitSource(opts options) string {
	switch {
	case opts.socket != "" || opts.server != "":
		return "a --socket/--server flag"
	case os.Getenv("CLASH_VR_TUI_SOCKET") != "" || os.Getenv("CLASH_VR_TUI_SERVER") != "":
		return "CLASH_VR_TUI_SOCKET/CLASH_VR_TUI_SERVER"
	}
	if f, err := config.Load(); err == nil && (f.Socket != "" || f.Server != "") {
		return "the config file " + config.Path()
	}
	return ""
}

// endpointLabel describes the resolved endpoint for status output.
func endpointLabel(r config.Resolved) string {
	addr := r.Endpoint.Socket
	if addr == "" {
		addr = r.Endpoint.Server
	}
	if r.Source == "" {
		return addr
	}
	return strings.TrimSpace(addr + "  (" + r.Source + ")")
}
