package proxies

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/probe"
)

func realisticModel(width, height int) Model {
	var hk []string
	for i := 1; i <= 18; i++ {
		hk = append(hk, fmt.Sprintf("HK-HongKong-%02d", i))
	}
	m := New(nil).SetSize(width, height)
	m.groups = []groupState{
		{group: api.Group{Name: "Proxy", Type: "Selector", Now: "Auto", All: append([]string{"Auto", "HK"}, hk...)}},
		{group: api.Group{Name: "Auto", Type: "URLTest", Now: hk[0], All: hk}},
		{group: api.Group{Name: "Streaming", Type: "Fallback", Now: "HK", All: []string{"HK", "DIRECT"}}},
	}
	m.clampSelection()
	m.adjustOffsets()
	return m
}

// Rows must never wrap: a wrapped row pushes the panel past the pane and cuts
// off the bottom border and the help bar in a narrow SSH window. The root pads
// the content area by one cell on each side, so a page may use width-2.
func TestProxyViewFitsNarrowPanes(t *testing.T) {
	for _, width := range []int{56, 60, 64, 70, 76, 80, 82, 100, 140} {
		for _, focus := range []string{focusGroups, focusNodes} {
			m := realisticModel(width, 20)
			m.focus = focus
			m.testSummary = "HTTP Proxy: 17/20 ok, 3 timeout"
			m.track(probeKey("Proxy", probe.ModeHTTP, ""))
			view := m.View()
			if h := lipgloss.Height(view); h > 20 {
				t.Errorf("width %d focus %s: height %d > 20\n%s", width, focus, h, view)
			}
			for _, line := range strings.Split(view, "\n") {
				if w := lipgloss.Width(line); w > width-2 {
					t.Errorf("width %d focus %s: line width %d > %d: %q", width, focus, w, width-2, line)
					break
				}
			}
		}
	}
}

// In a narrow pane the page shows one panel at a time: groups first, then
// Enter opens the nodes of the selected group and h or Esc goes back.
func TestNarrowProxyFlowShowsOnePanel(t *testing.T) {
	m := realisticModel(60, 20)
	if v := m.View(); !strings.Contains(v, "Groups 3") || strings.Contains(v, "HK-HongKong-01") {
		t.Fatalf("narrow groups view should list groups only:\n%s", v)
	}
	m = sendProxyKey(m, runeKey('j'))
	m = sendProxyKey(m, keyEnter())
	v := m.View()
	if !strings.Contains(v, "HK-HongKong-01") || strings.Contains(v, "Groups 3") {
		t.Fatalf("Enter should open the nodes of Auto in a single panel:\n%s", v)
	}
	if !strings.Contains(v, "Auto") {
		t.Fatalf("node panel should name its group:\n%s", v)
	}
	m = sendProxyKey(m, keyEsc())
	if m.focus != focusGroups || !strings.Contains(m.View(), "Groups 3") {
		t.Fatalf("Esc should return to the group list, focus = %s", m.focus)
	}
}

func keyEnter() tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyEsc() tea.KeyMsg   { return tea.KeyMsg{Type: tea.KeyEsc} }
