package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	previous := userConfigDir
	userConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userConfigDir = previous })
	for _, name := range []string{
		"CLASH_VR_TUI_SERVER", "CLASH_VR_TUI_SOCKET", "CLASH_VR_TUI_SECRET",
		"CLASH_VR_TUI_SECRET_FILE", "CLASH_VR_TUI_CONFIG_PATH", "CLASH_VR_TUI_TEST_URL",
	} {
		t.Setenv(name, "")
	}
}

func writeConfig(t *testing.T, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(Path()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveFlagPrecedence(t *testing.T) {
	isolateConfig(t)
	t.Setenv("CLASH_VR_TUI_SERVER", "10.0.0.1:9090")
	t.Setenv("CLASH_VR_TUI_SECRET", "env-secret")
	writeConfig(t, "server: 10.0.0.2:9090\nsecret: file-secret\n")

	r, err := Resolve(Flags{Server: "1.2.3.4:9090", Secret: "flag-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint.Server != "1.2.3.4:9090" || r.Endpoint.Socket != "" || r.Endpoint.Secret != "flag-secret" {
		t.Errorf("server flag profile: got %+v", r.Endpoint)
	}

	r, err = Resolve(Flags{Socket: "/tmp/x.sock"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint.Socket != "/tmp/x.sock" || r.Endpoint.Server != "" || r.Endpoint.Secret != "" {
		t.Errorf("flag socket inherited lower-tier values: %+v", r.Endpoint)
	}
}

func TestResolveEnvTierKeepsCredentialWithEndpoint(t *testing.T) {
	isolateConfig(t)
	t.Setenv("CLASH_VR_TUI_SERVER", "10.0.0.1:9090")
	t.Setenv("CLASH_VR_TUI_SECRET", "env-secret")
	writeConfig(t, "server: 10.0.0.2:9090\nsecret: file-secret\n")

	r, err := Resolve(Flags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint.Server != "10.0.0.1:9090" || r.Endpoint.Secret != "env-secret" {
		t.Errorf("env profile not used as a unit: %+v", r.Endpoint)
	}
}

func TestResolveConfigPathAndTestURLPrecedence(t *testing.T) {
	isolateConfig(t)
	t.Setenv("CLASH_VR_TUI_CONFIG_PATH", "/etc/mihomo/config.yaml")
	t.Setenv("CLASH_VR_TUI_TEST_URL", "https://example.com/generate_204")
	r, err := Resolve(Flags{})
	if err != nil {
		t.Fatal(err)
	}
	if r.ConfigPath != "/etc/mihomo/config.yaml" || r.TestURL != "https://example.com/generate_204" {
		t.Fatalf("env auxiliary config not used: %+v", r)
	}

	r, err = Resolve(Flags{ConfigPath: "/tmp/core.yaml", TestURL: "https://probe.local/204"})
	if err != nil {
		t.Fatal(err)
	}
	if r.ConfigPath != "/tmp/core.yaml" || r.TestURL != "https://probe.local/204" {
		t.Fatalf("flag auxiliary config not used: %+v", r)
	}
}

func TestResolveRejectsMalformedConfig(t *testing.T) {
	isolateConfig(t)
	writeConfig(t, "server: [unterminated\n")

	_, err := Resolve(Flags{})
	if err == nil || !strings.Contains(err.Error(), Path()) {
		t.Fatalf("Resolve error = %v, want config path", err)
	}
}

func TestResolveRejectsUnknownConfigField(t *testing.T) {
	isolateConfig(t)
	writeConfig(t, "servre: 127.0.0.1:9090\n")

	_, err := Resolve(Flags{})
	if err == nil || !strings.Contains(err.Error(), "servre") {
		t.Fatalf("Resolve error = %v, want unknown field", err)
	}
}

func TestResolveSecretFile(t *testing.T) {
	isolateConfig(t)
	secretPath := filepath.Join(t.TempDir(), "controller.secret")
	if err := os.WriteFile(secretPath, []byte("file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	r, err := Resolve(Flags{Server: "127.0.0.1:9090", SecretFile: secretPath})
	if err != nil {
		t.Fatal(err)
	}
	if r.Endpoint.Secret != "file-secret" {
		t.Fatalf("secret = %q", r.Endpoint.Secret)
	}
}

func TestResolveRejectsAmbiguousSecret(t *testing.T) {
	isolateConfig(t)
	_, err := Resolve(Flags{Server: "127.0.0.1:9090", Secret: "inline", SecretFile: "/tmp/secret"})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("Resolve error = %v", err)
	}
}

func TestResolveRejectsMixedOrOrphanedProfiles(t *testing.T) {
	isolateConfig(t)
	tests := []Flags{
		{Server: "127.0.0.1:9090", Socket: "/tmp/mihomo.sock"},
		{Secret: "orphan"},
		{SecretFile: "/tmp/orphan"},
	}
	for _, flags := range tests {
		if _, err := Resolve(flags); err == nil {
			t.Errorf("Resolve(%+v) succeeded", flags)
		}
	}
}

func TestFirst(t *testing.T) {
	if first("", "", "c") != "c" || first("a", "b") != "a" || first("", "") != "" {
		t.Error("first did not return the first non-empty value")
	}
}

func TestLoadAcceptsEmptyAndCommentOnlyFiles(t *testing.T) {
	for _, data := range []string{"", "# only a comment\n"} {
		isolateConfig(t)
		writeConfig(t, data)
		if _, err := Load(); err != nil {
			t.Fatalf("Load(%q) = %v, want nil", data, err)
		}
	}
}
