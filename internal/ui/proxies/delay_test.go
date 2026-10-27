package proxies

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/probe"
)

func delayModel() Model {
	m := New(nil).SetSize(120, 30)
	m.groups = []groupState{{group: api.Group{Name: "auto", Type: "Selector", All: []string{"n1", "n2"}}}}
	return m
}

func TestSupersededGroupReplyIsDropped(t *testing.T) {
	m := delayModel()
	key := probeKey("auto", probe.ModeHTTP, "")
	first := m.track(key)
	second := m.track(key)

	m, _ = m.Update(messages.GroupDelayMsg{Group: "auto", Mode: probe.ModeHTTP, RequestID: first,
		Results: map[string]messages.DelayResult{"n1": {Delay: 900, Status: messages.DelayOK}}})
	if got := getDelay("n1", m.groups[0].delays[probe.ModeHTTP]); got != 0 {
		t.Fatalf("stale reply applied: n1 = %d", got)
	}
	if len(m.inflight) != 1 {
		t.Fatalf("stale reply cleared the newer request: inflight = %v", m.inflight)
	}

	m, _ = m.Update(messages.GroupDelayMsg{Group: "auto", Mode: probe.ModeHTTP, RequestID: second,
		Results: map[string]messages.DelayResult{"n1": {Delay: 80, Status: messages.DelayOK}}})
	if got := getDelay("n1", m.groups[0].delays[probe.ModeHTTP]); got != 80 {
		t.Fatalf("current reply not applied: n1 = %d", got)
	}
	if len(m.inflight) != 0 {
		t.Fatalf("request still in flight after its reply: %v", m.inflight)
	}
}

func TestDelaysAreKeptPerMode(t *testing.T) {
	m := delayModel()
	id := m.track(probeKey("auto", probe.ModeHTTP, "n1"))
	m, _ = m.Update(messages.ProxyDelayMsg{Group: "auto", Name: "n1", Mode: probe.ModeHTTP, RequestID: id,
		Result: messages.DelayResult{Delay: 120, Status: messages.DelayOK}})

	m.testMode = probe.ModeTCP
	if got := getDelay("n1", m.groups[0].delays[m.testMode]); got != 0 {
		t.Fatalf("HTTP delay leaked into TCP view: %d", got)
	}
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, " n1 ") && strings.Contains(line, "120ms") {
			t.Fatalf("TCP row shows the HTTP measurement: %q", line)
		}
	}
}

func TestGroupHTTPResultsCoverEveryNode(t *testing.T) {
	nodes := []string{"fast", "silent"}
	got := groupHTTPResults(nodes, api.GroupDelayResult{"fast": 120}, nil)
	if got["fast"].Status != messages.DelayOK || got["fast"].Delay != 120 {
		t.Fatalf("fast = %+v", got["fast"])
	}
	if got["silent"].Status != messages.DelayTimeout {
		t.Fatalf("node missing from mihomo reply should time out, got %+v", got["silent"])
	}

	timeout := &api.StatusError{Method: "GET", Path: "/group/x/delay", Code: http.StatusGatewayTimeout}
	for _, r := range groupHTTPResults(nodes, nil, timeout) {
		if r.Status != messages.DelayTimeout {
			t.Fatalf("504 should mark nodes as timeout, got %+v", r)
		}
	}
	broken := &api.StatusError{Method: "GET", Path: "/group/x/delay", Code: http.StatusInternalServerError}
	for _, r := range groupHTTPResults(nodes, nil, broken) {
		if r.Status != messages.DelayError {
			t.Fatalf("500 should mark nodes as error, got %+v", r)
		}
	}
}

func TestProbeReportsSkippedNodes(t *testing.T) {
	r := probeNode(probe.NewEndpoints(), probe.ModeTCP, "sub-group")
	if r.Status != messages.DelaySkipped {
		t.Fatalf("node without a server should be skipped, got %+v", r)
	}
	if r := probeNode(nil, probe.ModeTCP, "n1"); r.Status != messages.DelayError {
		t.Fatalf("missing endpoints should be an error, got %+v", r)
	}
}

func TestSummaryCountsEveryOutcome(t *testing.T) {
	got := summarizeGroupTest("auto", probe.ModeTCP, map[string]messages.DelayResult{
		"a": {Delay: 50, Status: messages.DelayOK},
		"b": {Status: messages.DelayTimeout},
		"c": {Status: messages.DelaySkipped},
	})
	want := "TCP auto: 1/3 ok, 1 timeout, 1 skipped"
	if got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	if got := summarizeGroupTest("empty", probe.ModeHTTP, nil); !strings.Contains(got, "no nodes to test") {
		t.Fatalf("empty group summary = %q", got)
	}
}

func TestPendingTestIsVisible(t *testing.T) {
	m := delayModel()
	m.track(probeKey("auto", probe.ModeHTTP, ""))
	view := m.View()
	if !strings.Contains(view, "TESTING 1") || !strings.Contains(view, "testing") {
		t.Fatalf("pending group test not shown:\n%s", view)
	}
}

// The configured test-url must reach the controller request when the group
// has no testUrl of its own, and a group's own testUrl still wins.
func TestGroupTestUsesConfiguredURL(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.Query().Get("url")
		_ = json.NewEncoder(w).Encode(map[string]int{"n1": 42})
	}))
	defer srv.Close()

	client := api.NewWith(api.Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})
	m := NewWithTestURL(client, "https://probe.example/204")
	group := api.Group{Name: "auto", All: []string{"n1", "n2"}}

	msg := m.testGroup(group, 7)().(messages.GroupDelayMsg)
	if gotURL != "https://probe.example/204" {
		t.Fatalf("controller saw url=%q", gotURL)
	}
	if msg.RequestID != 7 || msg.Results["n1"].Delay != 42 || msg.Results["n2"].Status != messages.DelayTimeout {
		t.Fatalf("unexpected reply: %+v", msg)
	}

	group.TestURL = "http://group.example/204"
	m.testGroup(group, 8)()
	if gotURL != "http://group.example/204" {
		t.Fatalf("group testUrl should win, controller saw url=%q", gotURL)
	}
}
