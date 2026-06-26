package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
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
		// Overlay intercepts all keys when active.
		if m.overlay.IsActive() {
			var cmd tea.Cmd
			m.overlay, cmd = m.overlay.Update(msg)
			return m, cmd
		}

		// Ctrl+C always quits, even while a filter input is active.
		if msg.String() == "ctrl+c" {
			m.cancelAll()
			return m, tea.Quit
		}

		// While a page's filter input is active, every other key belongs to the
		// page so the user can type freely (e.g. 'q' in a search term).
		if !m.isPageFiltering() {
			if isQuit(msg) {
				m.cancelAll()
				return m, tea.Quit
			}
			if msg.String() == "?" {
				m.overlay = m.overlay.ShowHelp()
				return m, nil
			}
			if isNavigationKey(msg) {
				var cmd tea.Cmd
				m.sidebar, cmd = m.sidebar.Update(msg)
				return m, cmd
			}
			if page, ok := pageForNumberKey(msg); ok {
				return m, func() tea.Msg { return messages.SwitchPageMsg{Page: page} }
			}
			// Restart core (global, with confirmation).
			if msg.String() == "R" {
				client := m.client
				m.overlay = m.overlay.ShowConfirm(
					"Restart mihomo Core",
					"Restart the core now? Active connections will drop briefly.",
					func() tea.Msg {
						return messages.CoreRestartMsg{Err: client.RestartCore()}
					},
				)
				return m, nil
			}
			// Connection detail (Enter) is handled at root so it can build the
			// overlay from the selected connection.
			if msg.String() == "enter" && m.activePage == messages.PageConnections {
				if c := m.connections.SelectedConn(); c != nil {
					conn := *c // copy to avoid pointer into temporary slice
					detail := connections.FormatConnDetail(&conn)
					m.overlay = m.overlay.ShowDetail("Connection Detail", detail)
					return m, nil
				}
			}
			// Close all connections with X (confirm overlay).
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
		}

		// Delegate to the active page.
		if cmd := m.updateActivePage(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}

	case messages.SwitchPageMsg:
		m.activePage = msg.Page
		m.sidebar.Active = msg.Page
		if cmd := m.initPage(msg.Page); cmd != nil {
			cmds = append(cmds, cmd)
		}
		// Lazily start the logs stream the first time the Logs page is opened.
		if msg.Page == messages.PageLogs && !m.logsStarted {
			m.logsStarted = true
			cmds = append(cmds, m.startLogsStream())
		}

	// --- WebSocket stream lifecycle ---
	case trafficStarted:
		m.trafficCancel = msg.cancel
		m.statusbar.Upload = msg.first.Up
		m.statusbar.Download = msg.first.Down
		cmds = append(cmds, waitForTraffic(msg.ch))
	case trafficTick:
		m.statusbar.Upload = msg.data.Up
		m.statusbar.Download = msg.data.Down
		cmds = append(cmds, waitForTraffic(msg.ch))
	case trafficDown:
		cmds = append(cmds, tea.Tick(reconnectDelay, func(time.Time) tea.Msg { return trafficReconnect{} }))
	case trafficReconnect:
		cmds = append(cmds, m.startTrafficStream())

	case connsStarted:
		m.connsCancel = msg.cancel
		m.statusbar.Memory = msg.first.Memory
		m.home = m.home.SetStats(msg.first.Memory, len(msg.first.Connections))
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(messages.ConnectionsMsg{Data: msg.first})
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, waitForConns(msg.ch))
	case connsTick:
		m.statusbar.Memory = msg.data.Memory
		m.home = m.home.SetStats(msg.data.Memory, len(msg.data.Connections))
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(messages.ConnectionsMsg{Data: msg.data})
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		cmds = append(cmds, waitForConns(msg.ch))
	case connsDown:
		cmds = append(cmds, tea.Tick(reconnectDelay, func(time.Time) tea.Msg { return connsReconnect{} }))
	case connsReconnect:
		cmds = append(cmds, m.startConnectionsStream())

	case logsStarted:
		m.logsCancel = msg.cancel
		m.logs = m.logs.Add(msg.first)
		cmds = append(cmds, waitForLogs(msg.ch))
	case logsTick:
		m.logs = m.logs.Add(msg.entry)
		cmds = append(cmds, waitForLogs(msg.ch))
	case logsDown:
		cmds = append(cmds, tea.Tick(reconnectDelay, func(time.Time) tea.Msg { return logsReconnect{} }))
	case logsReconnect:
		if m.logsStarted {
			cmds = append(cmds, m.startLogsStream())
		}

	case clearStatus:
		if msg.seq == m.statusSeq {
			m.statusbar = m.statusbar.SetStatus("", false)
		}

	default:
		if cmd := m.routeDataMsg(msg); cmd != nil {
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
	case messages.PageLogs:
		var cmd tea.Cmd
		m.logs, cmd = m.logs.Update(msg)
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

	switch msg := msg.(type) {
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
	case messages.GroupDelayMsg:
		var cmd tea.Cmd
		m.proxies, cmd = m.proxies.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.Err != nil {
			cmds = append(cmds, m.flash("Delay test failed: "+msg.Err.Error(), true))
		}
	case messages.ProxyDelayMsg:
		var cmd tea.Cmd
		m.proxies, cmd = m.proxies.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	case messages.ProxySelectedMsg:
		var cmd tea.Cmd
		m.proxies, cmd = m.proxies.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.Err != nil {
			cmds = append(cmds, m.flash("Select failed: "+msg.Err.Error(), true))
		} else {
			cmds = append(cmds, m.flash("Switched "+msg.Group+" → "+msg.Proxy, false))
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
		if msg.Err != nil {
			cmds = append(cmds, m.flash("Config update failed: "+msg.Err.Error(), true))
		}
	case messages.ConnClosedMsg:
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.Err != nil {
			cmds = append(cmds, m.flash("Close failed: "+msg.Err.Error(), true))
		}
	case messages.AllConnsClosedMsg:
		var cmd tea.Cmd
		m.connections, cmd = m.connections.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		if msg.Err != nil {
			cmds = append(cmds, m.flash("Close all failed: "+msg.Err.Error(), true))
		} else {
			cmds = append(cmds, m.flash("All connections closed", false))
		}
	case messages.CoreRestartMsg:
		if msg.Err != nil {
			cmds = append(cmds, m.flash("Restart failed: "+msg.Err.Error(), true))
		} else {
			cmds = append(cmds, m.flash("Core restarting… reconnecting", false))
			// Re-fetch page data once the core is back up. Streams auto-reconnect.
			cmds = append(cmds, tea.Tick(reconnectDelay, func(time.Time) tea.Msg {
				return refetchAll{}
			}))
		}
	case refetchAll:
		cmds = append(cmds,
			m.home.Init(),
			m.proxies.Init(),
			m.rules.Init(),
			m.settings.Init(),
		)
	case messages.ErrMsg:
		if msg.Err != nil {
			cmds = append(cmds, m.flash(msg.Err.Error(), true))
		}
	}

	if len(cmds) > 0 {
		return tea.Batch(cmds...)
	}
	return nil
}

// refetchAll triggers a refresh of all pages' data (used after a core restart).
type refetchAll struct{}

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
	sidebarW := styles.SidebarWidth(m.width)
	contentTotalW := max(m.width-sidebarW, 1)
	contentInnerW := max(contentTotalW-2, 1)
	contentH := m.height - 2 // status bar + help bar

	m.sidebar = m.sidebar.SetSize(sidebarW, contentH)
	m.statusbar = m.statusbar.SetWidth(m.width)
	m.helpbar = m.helpbar.SetWidth(m.width)
	m.overlay = m.overlay.SetSize(m.width, m.height)

	m.home = m.home.SetSize(contentInnerW, contentH)
	m.proxies = m.proxies.SetSize(contentInnerW, contentH)
	m.connections = m.connections.SetSize(contentInnerW, contentH)
	m.rules = m.rules.SetSize(contentInnerW, contentH)
	m.logs = m.logs.SetSize(contentInnerW, contentH)
	m.settings = m.settings.SetSize(contentInnerW, contentH)

	return m
}

func isNavigationKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab", "shift+tab":
		return true
	}
	return false
}

// isPageFiltering reports whether the active page has a filter input open, so
// the root model can stop intercepting global keys and let the page consume them.
func (m Model) isPageFiltering() bool {
	switch m.activePage {
	case messages.PageProxies:
		return m.proxies.Filtering()
	case messages.PageConnections:
		return m.connections.Filtering()
	case messages.PageRules:
		return m.rules.Filtering()
	case messages.PageLogs:
		return m.logs.Filtering()
	}
	return false
}
