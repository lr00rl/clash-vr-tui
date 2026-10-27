package home

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func TestOfflineViewExplainsSocketRecovery(t *testing.T) {
	m := New(api.NewWith(api.Endpoint{Socket: "/tmp/missing.sock"})).SetSize(90, 28)
	m.err = errors.New("dial unix /tmp/missing.sock: no such file or directory")
	view := m.View()
	if !strings.Contains(view, "--socket PATH") || !strings.Contains(view, "r retry") {
		t.Fatalf("offline view lacks recovery guidance: %q", view)
	}
}

func TestConnectedRESTWithReconnectingStreamsIsDegraded(t *testing.T) {
	m := New(api.NewWith(api.Endpoint{Socket: "/tmp/test.sock"})).SetSize(90, 28)
	m.version = &api.VersionInfo{Version: "1.0"}
	m.config = &api.Config{Mode: "rule"}
	m = m.SetStreamState("reconnecting", time.Date(2026, 10, 26, 12, 30, 0, 0, time.UTC))
	view := m.View()
	if !strings.Contains(view, "DEGRADED") || strings.Contains(view, "ONLINE") {
		t.Fatalf("reconnecting view does not show degraded state: %q", view)
	}
}

func TestOfflineHintClassifiesAuthentication(t *testing.T) {
	hint := offlineHint(errors.New("GET /version: status 401: unauthorized"))
	if !strings.Contains(hint, "secret") {
		t.Fatalf("authentication hint = %q", hint)
	}
}
