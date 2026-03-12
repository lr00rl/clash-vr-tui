package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/ui/connections"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m = m.updateSizes()
		return m, nil

	case tea.KeyMsg:
		// Overlay intercepts all keys when active
		if m.overlay.IsActive() {
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		// Global keys
		if isQuit(msg) && !m.isPageFiltering() {
			m.cancelAll()
			return m, tea.Quit
		}

		if msg.String() == "?" {
			m.overlay = m.overlay.ShowHelp()
			return m, nil
		}

		// Navigation keys go to sidebar
		if isNavigationKey(msg) {
			var cmd tea.Cmd
			m.sidebar, cmd = m.sidebar.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			return m, tea.Batch(cmds...)
		}

		// Connection detail enter key
		if msg.String() == "enter" && m.activePage == messages.PageConnections {
			if c := m.connections.SelectedConn(); c != nil {
				conn := *c // copy to avoid pointer into temporary slice
				detail := connections.FormatConnDetail(&conn)
				m.overlay = m.overlay.ShowDetail("Connection Detail", detail)
				return m, nil
			}
		}

		// Close all connections with X
		if msg.String() == "X" && m.activePage == messages.PageConnections {
			client := m.client
			m.overlay = m.overlay.ShowConfirm(
				"Close All Connections",
				"Are you sure you want to close all connections?",
				func() tea.Msg {
					err := client.CloseAllConnections()
					return messages.AllConnsClosedMsg{Err: err}
				},
			)
			return m, nil
		}

		// Delegate to active page
		cmd := m.updateActivePage(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case messages.SwitchPageMsg:
		m.activePage = msg.Page
		m.sidebar.Active = msg.Page
		cmd := m.initPage(msg.Page)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	// WebSocket stream messages - store cancel functions
	case trafficStarted:
		m.trafficCancel = msg.cancel
		m.statusbar.Upload = msg.first.Up
		m.statusbar.Download = msg.first.Down
		cmds = append(cmds, waitForTraffic(msg.ch))
	case trafficTick:
		m.statusbar.Upload = msg.data.Up
		m.statusbar.Download = msg.data.Down
		cmds = append(cmds, waitForTraffic(msg.ch))
	case connsStarted:
		m.connsCancel = msg.cancel
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(messages.ConnectionsMsg{Data: msg.first})
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, waitForConns(msg.ch))
	case connsTick:
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(messages.ConnectionsMsg{Data: msg.data})
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, waitForConns(msg.ch))

	default:
		cmd := m.routeDataMsg(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) updateActivePage(msg tea.KeyMsg) tea.Cmd {
	switch m.activePage {
	case messages.PageHome:
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		return cmd
	case messages.PageProxies:
		var cmd tea.Cmd
		m.proxies, cmd = m.proxies.Update(msg)
		return cmd
	case messages.PageConnections:
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(msg)
		return cmd
	case messages.PageRules:
		var cmd tea.Cmd
		m.rules, cmd = m.rules.Update(msg)
		return cmd
	case messages.PageSettings:
		var cmd tea.Cmd
		m.settings, cmd = m.settings.Update(msg)
		return cmd
	}
	return nil
}

func (m *Model) routeDataMsg(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg.(type) {
	case messages.ConfigMsg:
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.proxies, cmd = m.proxies.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.settings, cmd = m.settings.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.VersionMsg:
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.GroupsMsg:
		var cmd tea.Cmd
		m.proxies, cmd = m.proxies.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.RulesMsg:
		var cmd tea.Cmd
		m.rules, cmd = m.rules.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.home, cmd = m.home.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.GroupDelayMsg, messages.ProxyDelayMsg, messages.ProxySelectedMsg:
		var cmd tea.Cmd
		m.proxies, cmd = m.proxies.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.ConfigPatchedMsg:
		var cmd tea.Cmd
		m.home, cmd = m.home.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		m.settings, cmd = m.settings.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.ConnClosedMsg, messages.AllConnsClosedMsg:
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.ErrMsg:
		// Errors from WS streams are non-fatal; silently ignore
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

func (m Model) initPage(page messages.Page) tea.Cmd {
	switch page {
	case messages.PageHome:
		return m.home.Init()
	case messages.PageProxies:
		return m.proxies.Init()
	case messages.PageRules:
		return m.rules.Init()
	case messages.PageSettings:
		return m.settings.Init()
	}
	return nil
}

func (m Model) updateSizes() Model {
	sidebarW := 16
	contentW := m.width - sidebarW
	contentH := m.height - 2 // status bar + help bar

	m.sidebar = m.sidebar.SetHeight(contentH)
	m.statusbar = m.statusbar.SetWidth(m.width)
	m.helpbar = m.helpbar.SetWidth(m.width)
	m.overlay = m.overlay.SetSize(m.width, m.height)

	m.home = m.home.SetSize(contentW, contentH)
	m.proxies = m.proxies.SetSize(contentW, contentH)
	m.connections = m.connections.SetSize(contentW, contentH)
	m.rules = m.rules.SetSize(contentW, contentH)
	m.settings = m.settings.SetSize(contentW, contentH)

	return m
}

func isNavigationKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab", "shift+tab":
		return true
	}
	return false
}

func (m Model) isPageFiltering() bool {
	return false
}
