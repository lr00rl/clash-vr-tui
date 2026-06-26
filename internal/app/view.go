package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

func (m Model) View() string {
	if !m.ready {
		return styles.StatusTitle.Render(" clash-vr-tui") + "\n" +
			styles.Subtle.Render(" starting controller session") + "\n" +
			styles.Faint.Render(" endpoint: "+m.client.SocketPath())
	}

	// Top status bar
	top := m.statusbar.View()

	// Sidebar
	side := m.sidebar.View()

	// Active page content
	var content string
	switch m.activePage {
	case messages.PageHome:
		content = m.home.View()
	case messages.PageProxies:
		content = m.proxies.View()
	case messages.PageConnections:
		content = m.connections.View()
	case messages.PageRules:
		content = m.rules.View()
	case messages.PageLogs:
		content = m.logs.View()
	case messages.PageSettings:
		content = m.settings.View()
	}

	sidebarW := styles.SidebarWidth(m.width)
	contentTotalW := max(m.width-sidebarW, 1)
	contentInnerW := max(contentTotalW-2, 1)
	contentH := max(m.height-2, 1)
	content = constrainView(content, contentInnerW, contentH)
	contentStyled := styles.ContentStyle.
		Width(contentInnerW).
		Height(contentH).
		Render(content)

	// Join sidebar + content
	middle := lipgloss.JoinHorizontal(lipgloss.Top, side, contentStyled)
	middle = constrainView(middle, m.width, contentH)

	// Bottom help bar
	bottom := m.helpbar.View(m.activePage)

	// Compose full layout
	view := lipgloss.JoinVertical(lipgloss.Left, top, middle, bottom)

	// Overlay on top if active
	if m.overlay.IsActive() {
		return m.overlay.View()
	}

	return constrainView(view, max(m.width-1, 1), m.height)
}

func constrainView(s string, width, height int) string {
	if height <= 0 || width <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for i, line := range lines {
		if lipgloss.Width(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}
