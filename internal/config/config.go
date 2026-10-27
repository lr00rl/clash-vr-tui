// Package config resolves how to reach the mihomo controller from (in
// precedence order) command-line flags, environment variables, an optional
// config file, and built-in defaults.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

// File is the on-disk config at Path().
type File struct {
	Socket     string `yaml:"socket"`
	Server     string `yaml:"server"`
	Secret     string `yaml:"secret"`
	SecretFile string `yaml:"secret-file"`
	ConfigPath string `yaml:"config-path"`
	TestURL    string `yaml:"test-url"`
}

// Flags holds command-line overrides (empty string means unset).
type Flags struct {
	Socket     string
	Server     string
	Secret     string
	SecretFile string
	ConfigPath string
	TestURL    string
}

// Resolved is the final connection configuration.
type Resolved struct {
	Endpoint   api.Endpoint
	ConfigPath string
	TestURL    string
}

var userConfigDir = os.UserConfigDir

// Path returns the config file path.
func Path() string {
	if dir, err := userConfigDir(); err == nil {
		return filepath.Join(dir, "clash-vr-tui", "config.yaml")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "clash-vr-tui", "config.yaml")
}

// Load reads and validates the config file. A missing file is not an error.
func Load() (File, error) {
	var f File
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return f, nil
		}
		return f, fmt.Errorf("read config %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	// An empty or comment-only file has no YAML document and decodes to io.EOF.
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		return f, fmt.Errorf("parse config %s: %w", path, err)
	}
	if f.Socket != "" && f.Server != "" {
		return f, fmt.Errorf("parse config %s: socket and server are mutually exclusive", path)
	}
	if f.Socket == "" && f.Server == "" && (f.Secret != "" || f.SecretFile != "") {
		return f, fmt.Errorf("parse config %s: secret requires socket or server in the same file", path)
	}
	return f, nil
}

// Resolve merges flags > env > file > defaults into a connection config.
// Within each tier, an explicit server selects TCP over a socket; a higher tier
// fully overrides lower ones (so --socket wins over a file-level server).
func Resolve(flags Flags) (Resolved, error) {
	f, err := Load()
	if err != nil {
		return Resolved{}, err
	}
	if flags.Socket != "" && flags.Server != "" {
		return Resolved{}, fmt.Errorf("socket and server flags are mutually exclusive")
	}
	if flags.Socket == "" && flags.Server == "" && (flags.Secret != "" || flags.SecretFile != "") {
		return Resolved{}, fmt.Errorf("secret flag requires --socket or --server in the same command")
	}
	envSocket := os.Getenv("CLASH_VR_TUI_SOCKET")
	envServer := os.Getenv("CLASH_VR_TUI_SERVER")
	envSecret := os.Getenv("CLASH_VR_TUI_SECRET")
	envSecretFile := os.Getenv("CLASH_VR_TUI_SECRET_FILE")
	if flags.Socket == "" && flags.Server == "" {
		if envSocket != "" && envServer != "" {
			return Resolved{}, fmt.Errorf("CLASH_VR_TUI_SOCKET and CLASH_VR_TUI_SERVER are mutually exclusive")
		}
		if envSocket == "" && envServer == "" && (envSecret != "" || envSecretFile != "") {
			return Resolved{}, fmt.Errorf("controller secret environment variable requires CLASH_VR_TUI_SOCKET or CLASH_VR_TUI_SERVER")
		}
	}
	var ep api.Endpoint
	switch {
	case flags.Server != "":
		ep.Server = flags.Server
		ep.Secret, err = resolveSecret(flags.Secret, flags.SecretFile)
	case flags.Socket != "":
		ep.Socket = flags.Socket
		ep.Secret, err = resolveSecret(flags.Secret, flags.SecretFile)
	case envServer != "":
		ep.Server = envServer
		ep.Secret, err = resolveSecret(envSecret, envSecretFile)
	case envSocket != "":
		ep.Socket = envSocket
		ep.Secret, err = resolveSecret(envSecret, envSecretFile)
	case f.Server != "":
		ep.Server = f.Server
		ep.Secret, err = resolveSecret(f.Secret, f.SecretFile)
	case f.Socket != "":
		ep.Socket = f.Socket
		ep.Secret, err = resolveSecret(f.Secret, f.SecretFile)
	default:
		ep.Socket = api.DefaultSocketPath()
	}
	if err != nil {
		return Resolved{}, err
	}
	return Resolved{
		Endpoint:   ep,
		ConfigPath: first(flags.ConfigPath, os.Getenv("CLASH_VR_TUI_CONFIG_PATH"), f.ConfigPath),
		TestURL:    first(flags.TestURL, os.Getenv("CLASH_VR_TUI_TEST_URL"), f.TestURL),
	}, nil
}

func resolveSecret(value, path string) (string, error) {
	if value != "" && path != "" {
		return "", fmt.Errorf("secret and secret-file are mutually exclusive")
	}
	if path == "" {
		return value, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read secret file %s: %w", path, err)
	}
	secret := strings.TrimSpace(string(data))
	if secret == "" {
		return "", fmt.Errorf("secret file %s is empty", path)
	}
	return secret, nil
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
