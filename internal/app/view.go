package app

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

func (m Model) View() string {
	if !m.ready {
		return "  Starting Clash Verge TUI...\n  Connecting to mihomo at " + m.client.SocketPath() + "..."
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
	case messages.PageSettings:
		content = m.settings.View()
	}

	contentW := m.width - 16
	contentStyled := styles.ContentStyle.
		Width(contentW).
		Height(m.height - 2).
		Render(content)

	// Join sidebar + content
	middle := lipgloss.JoinHorizontal(lipgloss.Top, side, contentStyled)

	// Bottom help bar
	bottom := m.helpbar.View(m.activePage)

	// Compose full layout
	view := lipgloss.JoinVertical(lipgloss.Left, top, middle, bottom)

	// Overlay on top if active
	if m.overlay.IsActive() {
		return m.overlay.View()
	}

	return view
}
