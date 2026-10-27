package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func TestCommandAliases(t *testing.T) {
	cases := map[string]string{"list-socket": "discover", "sockets": "discover", "log": "logs", "autodebug": "doctor", "groups": "proxies"}
	for alias, want := range cases {
		c, ok := findCommand(alias)
		if !ok || c.names[0] != want {
			t.Errorf("%s resolves to %v, want %s", alias, c.names, want)
		}
	}
	if !IsCommand("help") || IsCommand("nope") {
		t.Fatal("IsCommand mismatch")
	}
}

func TestParseLevelAndDuration(t *testing.T) {
	if l, err := parseLevel("WARN"); err != nil || l != "warning" {
		t.Fatalf("parseLevel(WARN) = %q, %v", l, err)
	}
	if _, err := parseLevel("verbose"); err == nil {
		t.Fatal("unknown level should fail")
	}
	for in, want := range map[string]time.Duration{"30": 30 * time.Second, "2m": 2 * time.Minute, "0": 0} {
		if d, err := parseDuration("--for", in); err != nil || d != want {
			t.Errorf("parseDuration(%q) = %v, %v", in, d, err)
		}
	}
	if _, err := parseDuration("--for", "-5s"); err == nil {
		t.Fatal("negative duration should fail")
	}
}

// A stream that delivers entries and then drops reports that it connected,
// so cmdLogs reconnects instead of failing.
func TestStreamLogsOnceReportsEntriesThenDrop(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("level"); got != "warning" {
			t.Errorf("level query = %q", got)
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		_ = conn.WriteJSON(map[string]string{"type": "warning", "payload": "dial timeout to a.example"})
		_ = conn.WriteJSON(map[string]string{"type": "error", "payload": "dns failed for b.example"})
		conn.Close()
	}))
	defer srv.Close()
	c := api.NewWith(api.Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})

	var got []string
	connected, err := streamLogsOnce(context.Background(), c, "warning", func(e api.LogEntry) { got = append(got, e.Payload) })
	if !connected || err == nil {
		t.Fatalf("connected=%v err=%v, want a connected stream that ended with an error", connected, err)
	}
	if len(got) != 2 {
		t.Fatalf("entries = %v", got)
	}
}

func TestStreamLogsOnceFailsWithoutConnecting(t *testing.T) {
	c := api.NewWith(api.Endpoint{Server: "127.0.0.1:1"})
	connected, err := streamLogsOnce(context.Background(), c, "info", func(api.LogEntry) {})
	if connected || err == nil {
		t.Fatalf("connected=%v err=%v, want a dial failure", connected, err)
	}
}
