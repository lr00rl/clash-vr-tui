package settings

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func TestWideConfigKeepsRowsInReadableColumn(t *testing.T) {
	m := New(api.NewWith(api.Endpoint{Socket: "/tmp/test.sock"})).SetSize(140, 30)
	m.config = &api.Config{Mode: "rule", MixedPort: 7897, LogLevel: "info"}
	view := m.View()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "TUN Mode") && lipgloss.Width(line) > 72 {
			t.Fatalf("setting row width = %d, want <= 72: %q", lipgloss.Width(line), line)
		}
	}
}
