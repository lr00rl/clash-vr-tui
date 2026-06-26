// Package config resolves how to reach the mihomo controller from (in
// precedence order) command-line flags, environment variables, an optional
// config file, and built-in defaults.
package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// File is the on-disk config (~/.config/clash-vr-tui/config.yaml).
type File struct {
	Socket     string `yaml:"socket"`
	Server     string `yaml:"server"`
	Secret     string `yaml:"secret"`
	ConfigPath string `yaml:"config-path"`
	TestURL    string `yaml:"test-url"`
}

// Flags holds command-line overrides (empty string means unset).
type Flags struct {
	Socket string
	Server string
	Secret string
}

// Resolved is the final connection configuration.
type Resolved struct {
	Endpoint   api.Endpoint
	ConfigPath string
	TestURL    string
}

// Path returns the config file path.
func Path() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "clash-vr-tui", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clash-vr-tui", "config.yaml")
}

// Load reads the config file, returning a zero File if it does not exist.
func Load() File {
	var f File
	data, err := os.ReadFile(Path())
	if err != nil {
		return f
	}
	_ = yaml.Unmarshal(data, &f)
	return f
}

// Resolve merges flags > env > file > defaults into a connection config.
func Resolve(flags Flags) Resolved {
	f := Load()
	socket := first(flags.Socket, os.Getenv("CLASH_VR_TUI_SOCKET"), f.Socket)
	server := first(flags.Server, os.Getenv("CLASH_VR_TUI_SERVER"), f.Server)
	secret := first(flags.Secret, os.Getenv("CLASH_VR_TUI_SECRET"), f.Secret)

	ep := api.Endpoint{Secret: secret}
	if server != "" {
		ep.Server = server
	} else {
		ep.Socket = first(socket, api.DefaultSocketPath())
	}
	return Resolved{
		Endpoint:   ep,
		ConfigPath: f.ConfigPath,
		TestURL:    f.TestURL,
	}
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
