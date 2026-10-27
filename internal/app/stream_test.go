package app

import (
	"errors"
	"strings"
	"testing"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func step(t *testing.T, m Model, msg any) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

// A dropped stream must stop the status bar from claiming LIVE, and a reply
// from a stream generation that was already replaced must not revive it.
func TestStreamDisconnectShowsStale(t *testing.T) {
	m := NewModel(api.NewWith(api.Endpoint{Socket: "/tmp/test.sock"}), "")
	m.statusbar = m.statusbar.SetWidth(140)
	trafficCh := make(chan api.TrafficData)
	connsCh := make(chan api.ConnectionsSnapshot)
	cancelled := 0
	cancel := func() { cancelled++ }

	m = step(t, m, trafficStarted{gen: 1, cancel: cancel, ch: trafficCh, first: api.TrafficData{Up: 10, Down: 20}})
	m = step(t, m, connsStarted{gen: 1, cancel: cancel, ch: connsCh})
	if view := m.statusbar.View(); !strings.Contains(view, "LIVE") {
		t.Fatalf("both streams up but status bar not LIVE:\n%s", view)
	}

	m = step(t, m, trafficDown{gen: 1, err: errors.New("socket closed")})
	if view := m.statusbar.View(); strings.Contains(view, "LIVE") || !strings.Contains(view, "STALE") {
		t.Fatalf("traffic stream down but status bar not STALE:\n%s", view)
	}
	if cancelled != 1 {
		t.Fatalf("dropped stream not cancelled before redial: cancelled = %d", cancelled)
	}

	m = step(t, m, trafficReconnect{gen: 1})
	if m.trafficGen != 2 {
		t.Fatalf("reconnect did not start a new generation: gen = %d", m.trafficGen)
	}
	m = step(t, m, trafficTick{gen: 1, ch: trafficCh, data: api.TrafficData{Up: 99}})
	if m.trafficHealth.state == "live" || m.statusbar.Upload == 99 {
		t.Fatalf("tick from replaced stream was applied: %+v upload=%d", m.trafficHealth, m.statusbar.Upload)
	}

	m = step(t, m, trafficStarted{gen: 2, cancel: cancel, ch: trafficCh})
	if view := m.statusbar.View(); !strings.Contains(view, "LIVE") {
		t.Fatalf("reconnected stream should restore LIVE:\n%s", view)
	}
}
