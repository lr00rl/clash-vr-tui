package proxies

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func TestParseDelayQuery(t *testing.T) {
	tests := []struct {
		in        string
		wantOp    string
		wantThres int
		wantOK    bool
	}{
		{"delay<200", "<", 200, true},
		{"delay>500", ">", 500, true},
		{"delay = 300", "", 0, false}, // "= 300" not supported; only =timeout/=0
		{"delay=timeout", "timeout", 0, true},
		{"delay=0", "timeout", 0, true},
		{"hk", "", 0, false},
		{"delay", "", 0, false},
		{"DELAY<100", "<", 100, true}, // case-insensitive
	}
	for _, tt := range tests {
		op, thr, ok := parseDelayQuery(tt.in)
		if op != tt.wantOp || thr != tt.wantThres || ok != tt.wantOK {
			t.Errorf("parseDelayQuery(%q) = (%q,%d,%v), want (%q,%d,%v)",
				tt.in, op, thr, ok, tt.wantOp, tt.wantThres, tt.wantOK)
		}
	}
}

func TestMatchNode(t *testing.T) {
	tests := []struct {
		name   string
		delay  int
		filter string
		want   bool
	}{
		{"JP-Tokyo", 88, "delay<200", true},
		{"JP-Tokyo", 350, "delay<200", false},
		{"JP-Tokyo", 0, "delay<200", false}, // timeout excluded from delay<
		{"JP-Tokyo", 600, "delay>500", true},
		{"JP-Tokyo", 0, "delay=timeout", true},
		{"JP-Tokyo", 88, "delay=timeout", false},
		{"JP-Tokyo", 88, "tokyo", true}, // substring, case-insensitive
		{"JP-Tokyo", 88, "osaka", false},
	}
	for _, tt := range tests {
		if got := matchNode(tt.name, tt.delay, tt.filter); got != tt.want {
			t.Errorf("matchNode(%q, %d, %q) = %v, want %v", tt.name, tt.delay, tt.filter, got, tt.want)
		}
	}
}

func TestSortModeString(t *testing.T) {
	if SortDefault.String() != "Default" || SortByName.String() != "Name" || SortByDelay.String() != "Delay" {
		t.Errorf("SortMode.String mismatch")
	}
}

func TestVisibleGroupsSkipsGlobalAndHiddenGroups(t *testing.T) {
	m := New(nil)
	m.groups = []groupState{
		{group: api.Group{Name: "for-test-ip", All: []string{"n1"}}},
		{group: api.Group{Name: "GLOBAL", All: []string{"for-test-ip"}}},
		{group: api.Group{Name: "Reject", Hidden: true, All: []string{"REJECT"}}},
		{group: api.Group{Name: "Polymarket", All: []string{"n2"}}},
	}

	visible := m.visibleGroups()
	got := make([]string, 0, len(visible))
	for _, item := range visible {
		got = append(got, m.groups[item.index].group.Name)
	}
	want := []string{"for-test-ip", "Polymarket"}
	if len(got) != len(want) {
		t.Fatalf("visible groups = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("visible groups = %v, want %v", got, want)
		}
	}
}

func TestNodeOffsetKeepsSelectedNodeVisible(t *testing.T) {
	m := proxyFixture(30, 96, 18)
	m.focus = focusNodes

	rows := m.nodeVisibleRows()
	if rows < 2 {
		t.Fatalf("node visible rows = %d, want at least 2", rows)
	}

	m.nodeCursor = rows - 1
	m.adjustOffsets()
	if m.nodeOffset != 0 {
		t.Fatalf("cursor at last visible row shifted offset to %d, want 0", m.nodeOffset)
	}

	m.nodeCursor = rows
	m.adjustOffsets()
	if m.nodeOffset != 1 {
		t.Fatalf("cursor just below viewport shifted offset to %d, want 1", m.nodeOffset)
	}

	for cursor := 0; cursor < len(m.currentNodes()); cursor++ {
		m.nodeCursor = cursor
		m.adjustOffsets()
		if cursor < m.nodeOffset || cursor >= m.nodeOffset+rows {
			t.Fatalf("cursor %d not visible with offset %d and rows %d", cursor, m.nodeOffset, rows)
		}
	}
}

func TestProxyVimPagingAndEdges(t *testing.T) {
	m := proxyFixture(50, 96, 22)
	m.focus = focusNodes
	step := max(m.nodeVisibleRows()/2, 1)

	m = sendProxyKey(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.nodeCursor != step {
		t.Fatalf("ctrl+d node cursor = %d, want %d", m.nodeCursor, step)
	}

	m = sendProxyKey(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.nodeCursor != 0 {
		t.Fatalf("ctrl+u node cursor = %d, want 0", m.nodeCursor)
	}

	m.nodeCursor = 17
	m = sendProxyKey(m, runeKey('g'))
	m = sendProxyKey(m, runeKey('g'))
	if m.nodeCursor != 0 {
		t.Fatalf("gg node cursor = %d, want 0", m.nodeCursor)
	}

	m = sendProxyKey(m, runeKey('G'))
	if m.nodeCursor != len(m.currentNodes())-1 {
		t.Fatalf("G node cursor = %d, want %d", m.nodeCursor, len(m.currentNodes())-1)
	}
}

func TestProxyGroupPagingUsesVisibleRows(t *testing.T) {
	m := New(nil).SetSize(96, 22)
	for i := 0; i < 40; i++ {
		m.groups = append(m.groups, groupState{
			group: api.Group{Name: fmt.Sprintf("group-%02d", i), All: []string{"node"}},
		})
	}
	m.focus = focusGroups
	step := max(m.groupVisibleRows()/2, 1)

	m = sendProxyKey(m, tea.KeyMsg{Type: tea.KeyCtrlD})
	if m.groupCursor != step {
		t.Fatalf("ctrl+d group cursor = %d, want %d", m.groupCursor, step)
	}

	m = sendProxyKey(m, tea.KeyMsg{Type: tea.KeyCtrlU})
	if m.groupCursor != 0 {
		t.Fatalf("ctrl+u group cursor = %d, want 0", m.groupCursor)
	}
}

func proxyFixture(nodes, width, height int) Model {
	names := make([]string, 0, nodes)
	for i := 0; i < nodes; i++ {
		names = append(names, fmt.Sprintf("node-%02d", i))
	}
	m := New(nil).SetSize(width, height)
	m.groups = []groupState{
		{group: api.Group{Name: "Auto", Type: "Selector", All: names}},
	}
	m.clampSelection()
	m.adjustOffsets()
	return m
}

func sendProxyKey(m Model, msg tea.KeyMsg) Model {
	next, _ := m.Update(msg)
	return next
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}
