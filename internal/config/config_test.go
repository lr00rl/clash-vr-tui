package config

import "testing"

func TestResolveFlagPrecedence(t *testing.T) {
	// Flag tier wins over everything.
	r := Resolve(Flags{Server: "1.2.3.4:9090", Secret: "sek"})
	if r.Endpoint.Server != "1.2.3.4:9090" || r.Endpoint.Socket != "" {
		t.Errorf("server flag: got %+v, want TCP server", r.Endpoint)
	}
	if r.Endpoint.Secret != "sek" {
		t.Errorf("secret flag not applied: %q", r.Endpoint.Secret)
	}

	// --socket wins over a (hypothetical) file-level server: flag tier is checked
	// first and Socket is set, so Server stays empty.
	r = Resolve(Flags{Socket: "/tmp/x.sock"})
	if r.Endpoint.Socket != "/tmp/x.sock" || r.Endpoint.Server != "" {
		t.Errorf("socket flag: got %+v, want unix socket", r.Endpoint)
	}
}

func TestResolveEnvTier(t *testing.T) {
	t.Setenv("CLASH_VR_TUI_SERVER", "10.0.0.1:9090")
	r := Resolve(Flags{}) // no flags -> env tier
	if r.Endpoint.Server != "10.0.0.1:9090" {
		t.Errorf("env server not used: %+v", r.Endpoint)
	}
	// A socket flag still beats the env server (higher tier).
	r = Resolve(Flags{Socket: "/tmp/y.sock"})
	if r.Endpoint.Socket != "/tmp/y.sock" || r.Endpoint.Server != "" {
		t.Errorf("socket flag should beat env server: %+v", r.Endpoint)
	}
}

func TestFirst(t *testing.T) {
	if first("", "", "c") != "c" {
		t.Error("first should return first non-empty")
	}
	if first("a", "b") != "a" {
		t.Error("first should return a")
	}
	if first("", "") != "" {
		t.Error("first of all-empty should be empty")
	}
}
