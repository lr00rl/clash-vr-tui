// Package discover finds mihomo controllers on the local machine: sockets,
// Windows pipes, and TCP controllers named by running core processes, config
// files, or well-known Clash Verge Rev paths.
package discover

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// Kind is the transport of a candidate controller.
type Kind string

const (
	KindSocket Kind = "socket"
	KindPipe   Kind = "pipe"
	KindTCP    Kind = "tcp"
)

// State is the outcome of probing a candidate with GET /version.
type State string

const (
	StateOK           State = "ok"
	StateUnauthorized State = "unauthorized"
	StateRefused      State = "refused"
	StateDenied       State = "denied"
	StateTimeout      State = "timeout"
	StateRejected     State = "rejected"
	StateError        State = "error"
)

// Candidate is one place a controller may listen.
type Candidate struct {
	Kind    Kind     `json:"kind"`
	Address string   `json:"address"`
	Secret  string   `json:"-"`
	Sources []string `json:"sources"`
	State   State    `json:"state,omitempty"`
	Version string   `json:"version,omitempty"`
	Err     string   `json:"error,omitempty"`
}

// Endpoint converts the candidate into a client endpoint.
func (c Candidate) Endpoint() api.Endpoint {
	if c.Kind == KindTCP {
		return api.Endpoint{Server: c.Address, Secret: c.Secret}
	}
	return api.Endpoint{Socket: c.Address, Secret: c.Secret}
}

// ProbeTimeout bounds each GET /version probe.
const ProbeTimeout = time.Second

// Find gathers candidates in priority order (processes, config files, known
// paths), drops socket files that do not exist, and probes the rest in
// parallel.
func Find(ctx context.Context) []Candidate {
	cands := gather(systemSources())
	probeAll(ctx, cands)
	return cands
}

// Best returns the first reachable candidate.
func Best(cands []Candidate) (Candidate, bool) {
	for _, c := range cands {
		if c.State == StateOK {
			return c, true
		}
	}
	return Candidate{}, false
}

// sources is the machine state discovery reads; tests replace it.
type sources struct {
	goos     string
	uid      int
	home     string
	tmpDir   string
	ps       func() (string, error)
	readFile func(string) ([]byte, error)
	exists   func(string) (bool, error)
}

func systemSources() sources {
	home, _ := os.UserHomeDir()
	return sources{
		goos:   runtime.GOOS,
		uid:    os.Getuid(),
		home:   home,
		tmpDir: os.TempDir(),
		ps: func() (string, error) {
			if runtime.GOOS == "windows" {
				return "", nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			out, err := exec.CommandContext(ctx, "ps", "-axww", "-o", "command=").Output()
			return string(out), err
		},
		readFile: os.ReadFile,
		exists: func(p string) (bool, error) {
			_, err := os.Stat(p)
			if errors.Is(err, os.ErrNotExist) {
				return false, nil
			}
			return err == nil, err
		},
	}
}

func gather(src sources) []Candidate {
	var list candidateList
	if out, err := src.ps(); err == nil {
		for _, c := range fromProcesses(out, src) {
			list.add(c)
		}
	}
	for _, path := range ConfigPaths(src.home, src.goos) {
		data, err := src.readFile(path)
		if err != nil {
			continue
		}
		for _, c := range fromConfig(data, filepath.Dir(path), "config "+shortPath(path, src.home)) {
			list.add(c)
		}
	}
	for _, c := range knownPaths(src) {
		list.add(c)
	}

	kept := list.items[:0]
	for _, c := range list.items {
		if c.Kind == KindSocket {
			ok, err := src.exists(c.Address)
			if err != nil {
				c.State, c.Err = StateDenied, err.Error()
			} else if !ok {
				continue
			}
		}
		kept = append(kept, c)
	}
	return kept
}

// candidateList keeps first-seen order and merges sources of duplicates.
type candidateList struct{ items []Candidate }

func (l *candidateList) add(c Candidate) {
	for i := range l.items {
		if l.items[i].Address == c.Address {
			l.items[i].Sources = append(l.items[i].Sources, c.Sources...)
			if l.items[i].Secret == "" {
				l.items[i].Secret = c.Secret
			}
			return
		}
	}
	l.items = append(l.items, c)
}

var flagRe = regexp.MustCompile(`\s-([a-zA-Z][\w-]*)`)

// fromProcesses reads controller flags from ps output. Paths may contain
// spaces (macOS "Application Support"), so a flag's value runs until the
// next " -flag" token.
func fromProcesses(psOut string, src sources) []Candidate {
	var out []Candidate
	for _, line := range strings.Split(psOut, "\n") {
		line = strings.TrimSpace(line)
		locs := flagRe.FindAllStringSubmatchIndex(line, -1)
		if len(locs) == 0 {
			continue
		}
		exe := line[:locs[0][0]]
		name := strings.ToLower(filepath.Base(exe))
		if !strings.Contains(name, "mihomo") && !strings.Contains(name, "clash") {
			continue
		}
		flags := map[string]string{}
		for i, loc := range locs {
			end := len(line)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			flags[line[loc[2]:loc[3]]] = strings.TrimSpace(line[loc[1]:end])
		}
		source := "process " + filepath.Base(exe)
		var found []Candidate
		if v := flags["ext-ctl-unix"]; v != "" {
			found = append(found, Candidate{Kind: KindSocket, Address: v, Sources: []string{source + " -ext-ctl-unix"}})
		}
		if v := flags["ext-ctl-pipe"]; v != "" {
			found = append(found, Candidate{Kind: KindPipe, Address: v, Sources: []string{source + " -ext-ctl-pipe"}})
		}
		if v := normalizeTCP(flags["ext-ctl"]); v != "" {
			found = append(found, Candidate{Kind: KindTCP, Address: v, Secret: flags["secret"], Sources: []string{source + " -ext-ctl"}})
		}
		// The config the process loaded may name controllers the flags did not.
		cfg := flags["f"]
		if cfg == "" && flags["d"] != "" {
			cfg = filepath.Join(flags["d"], "config.yaml")
		}
		if cfg != "" {
			if data, err := src.readFile(cfg); err == nil {
				for _, c := range fromConfig(data, filepath.Dir(cfg), source+" config") {
					if c.Kind == KindTCP && flags["secret"] != "" {
						c.Secret = flags["secret"]
					}
					found = append(found, c)
				}
			}
		}
		out = append(out, found...)
	}
	return out
}

type controllerConfig struct {
	ExternalController     string `yaml:"external-controller"`
	ExternalControllerUnix string `yaml:"external-controller-unix"`
	ExternalControllerPipe string `yaml:"external-controller-pipe"`
	Secret                 string `yaml:"secret"`
}

// fromConfig reads controller settings from a mihomo-style config. A relative
// socket path is relative to the config directory, as mihomo resolves it.
func fromConfig(data []byte, dir, source string) []Candidate {
	var cfg controllerConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil
	}
	var out []Candidate
	if p := cfg.ExternalControllerUnix; p != "" {
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		out = append(out, Candidate{Kind: KindSocket, Address: p, Sources: []string{source}})
	}
	if p := cfg.ExternalControllerPipe; p != "" {
		out = append(out, Candidate{Kind: KindPipe, Address: p, Sources: []string{source}})
	}
	if a := normalizeTCP(cfg.ExternalController); a != "" {
		out = append(out, Candidate{Kind: KindTCP, Address: a, Secret: cfg.Secret, Sources: []string{source}})
	}
	return out
}

// normalizeTCP turns a listen address into one this machine can dial:
// ":9090", "0.0.0.0:9090", "[::]:9090" and "*:9090" become 127.0.0.1:9090.
func normalizeTCP(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	switch host {
	case "", "0.0.0.0", "::", "*":
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}

func knownPaths(src sources) []Candidate {
	if src.goos == "windows" {
		return []Candidate{
			{Kind: KindPipe, Address: `\\.\pipe\verge-mihomo`, Sources: []string{"known path (Clash Verge Rev)"}},
			{Kind: KindPipe, Address: `\\.\pipe\mihomo`, Sources: []string{"known path (mihomo)"}},
		}
	}
	paths := []struct{ path, label string }{
		{"/var/run/clash-verge-service/users/" + strconv.Itoa(src.uid) + "/verge-mihomo.sock", "known path (Clash Verge Rev service mode)"},
		{"/tmp/verge/verge-mihomo.sock", "known path (Clash Verge Rev)"},
	}
	if src.tmpDir != "" {
		paths = append(paths, struct{ path, label string }{filepath.Join(src.tmpDir, "verge-mihomo.sock"), "known path (Clash Verge Rev, user temp dir)"})
	}
	out := make([]Candidate, 0, len(paths))
	for _, p := range paths {
		out = append(out, Candidate{Kind: KindSocket, Address: p.path, Sources: []string{p.label}})
	}
	return out
}

// ConfigPaths lists mihomo-style config files in the order clients usually
// keep them: the Clash Verge Rev app directory first, then mihomo, clash and
// clash-meta locations.
func ConfigPaths(home, goos string) []string {
	const vergeDir = "io.github.clash-verge-rev.clash-verge-rev"
	var paths []string
	add := func(base string) {
		paths = append(paths, filepath.Join(base, "clash-verge.yaml"), filepath.Join(base, "config.yaml"))
	}
	if home != "" {
		if goos == "darwin" {
			add(filepath.Join(home, "Library", "Application Support", vergeDir))
		}
		add(filepath.Join(home, ".config", vergeDir))
		add(filepath.Join(home, ".local", "share", vergeDir))
		if appData := os.Getenv("APPDATA"); goos == "windows" && appData != "" {
			add(filepath.Join(appData, vergeDir))
		}
		add(filepath.Join(home, ".config", "clash-verge"))
		add(filepath.Join(home, ".config", "mihomo"))
		add(filepath.Join(home, ".config", "clash"))
		add(filepath.Join(home, ".config", "clash.meta"))
		add(filepath.Join(home, ".config", "clash-meta"))
	}
	if goos != "windows" {
		add("/etc/mihomo")
		add("/etc/clash")
		add("/etc/clash-meta")
	}
	return paths
}

func shortPath(p, home string) string {
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

func probeAll(ctx context.Context, cands []Candidate) {
	var wg sync.WaitGroup
	for i := range cands {
		if cands[i].State != "" {
			continue // already classified, e.g. a socket we may not stat
		}
		wg.Add(1)
		go func(c *Candidate) {
			defer wg.Done()
			probe(ctx, c)
		}(&cands[i])
	}
	wg.Wait()
}

func probe(ctx context.Context, c *Candidate) {
	ep := c.Endpoint()
	if err := api.ValidateEndpoint(ep); err != nil {
		c.State, c.Err = StateRejected, err.Error()
		return
	}
	done := make(chan struct{})
	var ver *api.VersionInfo
	var err error
	go func() {
		defer close(done)
		ver, err = api.NewWith(ep).WithTimeout(ProbeTimeout).GetVersion()
	}()
	select {
	case <-done:
	case <-ctx.Done():
		c.State, c.Err = StateTimeout, ctx.Err().Error()
		return
	}
	if err == nil {
		c.State, c.Version = StateOK, ver.Version
		return
	}
	c.State, c.Err = classify(err), err.Error()
}

func classify(err error) State {
	var se *api.StatusError
	var ne net.Error
	switch {
	case errors.As(err, &se) && (se.Code == 401 || se.Code == 403):
		return StateUnauthorized
	case errors.Is(err, syscall.ECONNREFUSED), errors.Is(err, os.ErrNotExist):
		return StateRefused
	case errors.Is(err, os.ErrPermission):
		return StateDenied
	case errors.As(err, &ne) && ne.Timeout():
		return StateTimeout
	}
	return StateError
}
