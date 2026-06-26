// Package probe provides node latency testing beyond mihomo's HTTP delay test:
// raw TCP connect timing and ICMP echo. These need each node's server:port,
// which the mihomo API does not expose, so we locate and parse the running
// mihomo config on disk.
package probe

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Endpoint is a node's underlying server address.
type Endpoint struct {
	Server string
	Port   int
}

// Endpoints maps proxy node names to their server endpoints, parsed from the
// running mihomo config.
type Endpoints struct {
	mu      sync.RWMutex
	byName  map[string]Endpoint
	srcPath string
}

// NewEndpoints returns an empty endpoint cache.
func NewEndpoints() *Endpoints {
	return &Endpoints{byName: map[string]Endpoint{}}
}

// Lookup returns the endpoint for a node name, if known.
func (e *Endpoints) Lookup(name string) (Endpoint, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	ep, ok := e.byName[name]
	return ep, ok
}

// Len returns the number of known endpoints.
func (e *Endpoints) Len() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.byName)
}

// Source returns the config path the endpoints were loaded from.
func (e *Endpoints) Source() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.srcPath
}

// Load locates and parses the running mihomo config, populating endpoints.
// If configPath is non-empty it is used directly; otherwise it is auto-detected.
func (e *Endpoints) Load(configPath string) error {
	path := configPath
	if path == "" {
		path = locateConfig()
	}
	if path == "" {
		return fmt.Errorf("could not locate running mihomo config (set one explicitly for TCP/ICMP tests)")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}
	m, err := parseEndpoints(data)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.byName = m
	e.srcPath = path
	e.mu.Unlock()
	return nil
}

type rawConfig struct {
	Proxies []struct {
		Name   string `yaml:"name"`
		Type   string `yaml:"type"`
		Server string `yaml:"server"`
		Port   any    `yaml:"port"`
	} `yaml:"proxies"`
}

func parseEndpoints(data []byte) (map[string]Endpoint, error) {
	var cfg rawConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config yaml: %w", err)
	}
	m := make(map[string]Endpoint, len(cfg.Proxies))
	for _, p := range cfg.Proxies {
		if p.Name == "" || p.Server == "" {
			continue
		}
		m[p.Name] = Endpoint{Server: p.Server, Port: toInt(p.Port)}
	}
	return m, nil
}

func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(n))
		return i
	}
	return 0
}

var configFlagRe = regexp.MustCompile(`-f\s+(.+?\.ya?ml)(?:\s|$)`)

// locateConfig tries to find the running mihomo config: first from the mihomo
// process command line (-f flag), then common clash-verge install paths.
func locateConfig() string {
	if p := configFromProcess(); p != "" {
		return p
	}
	for _, p := range candidatePaths() {
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func configFromProcess() string {
	out, err := exec.Command("ps", "-axww", "-o", "command=").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		low := strings.ToLower(line)
		if !strings.Contains(low, "mihomo") && !strings.Contains(low, "clash") {
			continue
		}
		if !strings.Contains(line, " -f ") {
			continue
		}
		if m := configFlagRe.FindStringSubmatch(line); len(m) > 1 {
			if fileExists(m[1]) {
				return m[1]
			}
		}
	}
	return ""
}

func candidatePaths() []string {
	home, _ := os.UserHomeDir()
	const vergeDir = "io.github.clash-verge-rev.clash-verge-rev"
	var paths []string
	add := func(base string) {
		paths = append(paths,
			filepath.Join(base, "clash-verge.yaml"),
			filepath.Join(base, "config.yaml"),
		)
	}
	if home != "" {
		add(filepath.Join(home, "Library", "Application Support", vergeDir))
		add(filepath.Join(home, ".config", vergeDir))
		add(filepath.Join(home, ".local", "share", vergeDir))
		add(filepath.Join(home, ".config", "clash-verge"))
	}
	return paths
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
