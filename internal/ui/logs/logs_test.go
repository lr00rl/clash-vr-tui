package logs

import (
	"testing"

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

func TestPausedDropsEntries(t *testing.T) {
	m := New().SetSize(80, 20)
	m.paused = true
	m = m.Add(api.LogEntry{Type: "info", Payload: "ignored"})
	if len(m.entries) != 0 {
		t.Errorf("paused should drop entries, got %d", len(m.entries))
	}
}
