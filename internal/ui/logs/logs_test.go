package logs

import (
	"strings"
	"testing"
	"time"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func TestLevelRank(t *testing.T) {
	if !(levelRank("debug") < levelRank("info") &&
		levelRank("info") < levelRank("warning") &&
		levelRank("warning") < levelRank("error")) {
		t.Errorf("level ranks not ordered: debug=%d info=%d warning=%d error=%d",
			levelRank("debug"), levelRank("info"), levelRank("warning"), levelRank("error"))
	}
	if levelRank("warn") != levelRank("warning") || levelRank("err") != levelRank("error") {
		t.Errorf("level aliases not equal")
	}
}

func TestLogBadgesSeparateStreamAndViewportState(t *testing.T) {
	m := New().SetSize(80, 20)
	m = m.SetStreamState("reconnecting", time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC))
	m.paused = true
	view := m.View()
	if !strings.Contains(view, "RECONNECTING") || !strings.Contains(view, "FROZEN") {
		t.Fatalf("view does not separate stream and viewport states: %q", view)
	}
}

func TestNextLevel(t *testing.T) {
	got := []string{}
	cur := ""
	for i := 0; i < 4; i++ {
		cur = nextLevel(cur)
		got = append(got, cur)
	}
	want := []string{"info", "warning", "error", ""}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("nextLevel step %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAddRingBufferAndLevelFilter(t *testing.T) {
	m := New()
	m = m.SetSize(80, 20)
	for i := 0; i < maxEntries+50; i++ {
		m = m.Add(api.LogEntry{Type: "debug", Payload: "x"})
	}
	if len(m.entries) != maxEntries {
		t.Errorf("ring buffer = %d, want capped at %d", len(m.entries), maxEntries)
	}
	m = m.Add(api.LogEntry{Type: "error", Payload: "boom"})
	m.level = "warning"
	vis := m.visibleEntries()
	for _, e := range vis {
		if levelRank(e.Type) < levelRank("warning") {
			t.Errorf("level filter leaked %q", e.Type)
		}
	}
	if len(vis) != 1 {
		t.Errorf("expected only the error entry at warning filter, got %d", len(vis))
	}
}

func TestPausedKeepsCollectingWithoutMovingViewport(t *testing.T) {
	m := New().SetSize(80, 20)
	m = m.Add(api.LogEntry{Type: "info", Payload: "visible"})
	wantCursor, wantOffset := m.cursor, m.offset
	m.paused = true
	m = m.Add(api.LogEntry{Type: "info", Payload: "buffered"})
	if len(m.entries) != 2 {
		t.Errorf("frozen view should retain incoming entries, got %d", len(m.entries))
	}
	if m.cursor != wantCursor || m.offset != wantOffset {
		t.Errorf("frozen viewport moved to cursor=%d offset=%d, want %d/%d", m.cursor, m.offset, wantCursor, wantOffset)
	}
	if m.frozenNew != 1 {
		t.Errorf("frozen new count = %d, want 1", m.frozenNew)
	}
}

func TestRingBufferCountsDroppedEntries(t *testing.T) {
	m := New().SetSize(80, 20)
	for i := 0; i < maxEntries+3; i++ {
		m = m.Add(api.LogEntry{Type: "info", Payload: "line"})
	}
	if len(m.entries) != maxEntries || m.dropped != 3 {
		t.Fatalf("buffer=%d dropped=%d, want %d/3", len(m.entries), m.dropped, maxEntries)
	}
}

func TestLogViewSanitizesControllerText(t *testing.T) {
	m := New().SetSize(80, 20)
	m = m.Add(api.LogEntry{Type: "info", Payload: "safe\x1b]52;c;bad\a tail"})
	view := m.View()
	if strings.Contains(view, "\x1b") || strings.Contains(view, "\a") {
		t.Fatalf("view contains terminal control sequence: %q", view)
	}
}
