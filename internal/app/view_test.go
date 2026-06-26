package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
)

func TestViewFitsPane(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{name: "reported tmux pane", width: 159, height: 64},
		{name: "narrow ssh pane", width: 90, height: 28},
		{name: "small ssh pane", width: 72, height: 24},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, page := range messages.Pages() {
				m := NewModel(api.NewWith(api.Endpoint{Socket: "/tmp/test.sock"}))
				m.ready = true
				m.width = tt.width
				m.height = tt.height
				m.activePage = page
				m.sidebar.Active = page
				m = m.updateSizes()

				view := m.View()
				if got := lipgloss.Height(view); got > tt.height {
					t.Fatalf("%s height = %d, want <= %d", page, got, tt.height)
				}
				if got := maxLineWidth(view); got > tt.width {
					t.Fatalf("%s max line width = %d, want <= %d", page, got, tt.width)
				}
			}
		})
	}
}

func maxLineWidth(s string) int {
	maxWidth := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}
