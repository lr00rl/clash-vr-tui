package statusbar

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Neither a flash message nor the STALE timestamp may push the bar onto a
// second line; the root layout budgets exactly one row for it.
func TestStatusBarStaysOneLine(t *testing.T) {
	for _, width := range []int{40, 50, 60, 72, 80, 90, 120, 160} {
		for _, state := range []string{"live", "reconnecting", "connecting"} {
			for _, status := range []string{"", "Applied Allow LAN on", strings.Repeat("Config update failed: GET /configs ", 6)} {
				checkOneLine(t, width, state, status)
			}
		}
	}
}

func checkOneLine(t *testing.T, width int, state, status string) {
	t.Helper()
	m := New().SetWidth(width).SetStreamState(state, time.Now())
	m.Memory = 48 << 20
	m.Upload, m.Download = 186_400, 3_600_000
	view := m.SetStatus(status, false).View()
	if h := lipgloss.Height(view); h != 1 {
		t.Errorf("width %d %s status %q: height %d\n%s", width, state, status, h, view)
	}
	if w := lipgloss.Width(view); w > width {
		t.Errorf("width %d %s status %q: rendered width %d", width, state, status, w)
	}
}
