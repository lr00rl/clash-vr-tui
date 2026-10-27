package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
)

// fakeController accepts PATCH /configs and serves GET /configs. When ignore is
// set it accepts the patch but keeps the old value, like a core that cannot
// apply the change.
type fakeController struct {
	mu      sync.Mutex
	cfg     api.Config
	ignore  bool
	patches int
}

func (f *fakeController) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.Method {
	case http.MethodPatch:
		f.patches++
		var p api.ConfigPatch
		_ = json.NewDecoder(r.Body).Decode(&p)
		if !f.ignore && p.AllowLan != nil {
			f.cfg.AllowLan = *p.AllowLan
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		_ = json.NewEncoder(w).Encode(f.cfg)
	}
}

func controllerModel(t *testing.T, f *fakeController) Model {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	m := NewModel(api.NewWith(api.Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")}), "")
	m.statusbar = m.statusbar.SetWidth(160)
	return m
}

// drive runs cmd and feeds its message back into the model, the way the Bubble
// Tea runtime would. Status-clearing ticks are skipped.
func drive(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case messages.ConfigPatchedMsg:
		return step(t, m, msg)
	case tea.BatchMsg:
		for _, c := range msg {
			m = drive(t, m, c)
		}
	}
	return m
}

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestEnablingLANNeedsConfirmation(t *testing.T) {
	ctl := &fakeController{}
	m := controllerModel(t, ctl)
	on := true
	change := messages.ConfigChangeMsg{Label: "Allow LAN on", Patch: api.ConfigPatch{AllowLan: &on}}

	m = step(t, m, change)
	if !m.overlay.IsActive() || !strings.Contains(m.overlay.Content, "local network") {
		t.Fatalf("enabling LAN should open a confirmation explaining the impact")
	}
	next, cmd := m.Update(key("n"))
	m = drive(t, next.(Model), cmd)
	if ctl.patches != 0 || m.overlay.IsActive() {
		t.Fatalf("cancel must not send a PATCH: patches=%d overlay=%v", ctl.patches, m.overlay.IsActive())
	}

	m = step(t, m, change)
	next, cmd = m.Update(key("y"))
	m = drive(t, next.(Model), cmd)
	if ctl.patches != 1 {
		t.Fatalf("confirm should send exactly one PATCH, got %d", ctl.patches)
	}
	if view := m.statusbar.View(); !strings.Contains(view, "Applied Allow LAN on") {
		t.Fatalf("status bar should report the read-back success:\n%s", view)
	}
}

func TestIgnoredPatchIsReportedFromReadBack(t *testing.T) {
	ctl := &fakeController{ignore: true}
	m := controllerModel(t, ctl)
	off := false
	ctl.cfg.AllowLan = true

	// Turning LAN off is low impact, so it applies without a prompt.
	next, cmd := m.Update(messages.ConfigChangeMsg{Label: "Allow LAN off", Patch: api.ConfigPatch{AllowLan: &off}})
	m = next.(Model)
	if m.overlay.IsActive() {
		t.Fatalf("disabling LAN should not need confirmation")
	}
	m = drive(t, m, cmd)
	if view := m.statusbar.View(); !strings.Contains(view, "did not keep Allow LAN off") {
		t.Fatalf("status bar should flag the ignored change:\n%s", view)
	}
	m.settings = m.settings.SetSize(100, 30)
	for _, line := range strings.Split(m.settings.View(), "\n") {
		if strings.Contains(line, "Allow LAN") && !strings.Contains(line, "ON") {
			t.Fatalf("Config page should show the read-back value ON, got %q", line)
		}
	}
}

func TestConfigChangeWarningPolicy(t *testing.T) {
	on, off := true, false
	direct, global, rule := "direct", "global", "rule"
	cases := []struct {
		name    string
		patch   api.ConfigPatch
		confirm bool
	}{
		{"lan on", api.ConfigPatch{AllowLan: &on}, true},
		{"lan off", api.ConfigPatch{AllowLan: &off}, false},
		{"tun off", api.ConfigPatch{TUN: &api.TUNConfig{Enable: false}}, true},
		{"tun on", api.ConfigPatch{TUN: &api.TUNConfig{Enable: true}}, false},
		{"direct", api.ConfigPatch{Mode: &direct}, true},
		{"global", api.ConfigPatch{Mode: &global}, true},
		{"rule", api.ConfigPatch{Mode: &rule}, false},
	}
	for _, tc := range cases {
		if got := configChangeWarning(tc.patch) != ""; got != tc.confirm {
			t.Errorf("%s: confirm = %v, want %v", tc.name, got, tc.confirm)
		}
	}
}
