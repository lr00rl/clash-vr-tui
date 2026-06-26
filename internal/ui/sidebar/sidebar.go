package sidebar

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	Active messages.Page
	pages  []messages.Page
	width  int
	height int
}

func New() Model {
	return Model{
		Active: messages.PageHome,
		pages:  messages.Pages(),
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			next := int(m.Active) + 1
			if next >= len(m.pages) {
				next = 0
			}
			m.Active = m.pages[next]
			return m, func() tea.Msg { return messages.SwitchPageMsg{Page: m.Active} }
		case "shift+tab":
			prev := int(m.Active) - 1
			if prev < 0 {
				prev = len(m.pages) - 1
			}
			m.Active = m.pages[prev]
			return m, func() tea.Msg { return messages.SwitchPageMsg{Page: m.Active} }
		}
	}
	return m, nil
}

func (m Model) SetHeight(h int) Model {
	m.height = h
	return m
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) View() string {
	width := m.width
	if width == 0 {
		width = 16
	}
	innerW := max(width-2, 1)
	innerH := max(m.height-2, 1)
	compact := width <= 12

	var items []string
	for i, p := range m.pages {
		label := p.String()
		if compact {
			label = shortName(p)
		}
		line := styles.Faint.Render(fmt.Sprintf("%d", i+1)) + " " + label
		if p == m.Active {
			items = append(items, styles.SidebarActive.Width(innerW).Render("▸ "+line))
		} else {
			items = append(items, styles.SidebarItem.Width(innerW).Render("  "+line))
		}
	}

	brand := styles.StatusTitle.Render("clash")
	subtitle := styles.Faint.Render("verge tui")
	if compact {
		brand = styles.StatusTitle.Render("cvr")
		subtitle = styles.Faint.Render("tui")
	}

	top := lipgloss.JoinVertical(lipgloss.Left,
		brand,
		subtitle,
		"",
		lipgloss.JoinVertical(lipgloss.Left, items...),
	)

	footer := styles.Faint.Render("Tab nav")
	if compact {
		footer = styles.Faint.Render("Tab")
	}

	content := top
	if innerH > lipgloss.Height(top)+1 {
		content = lipgloss.JoinVertical(lipgloss.Left,
			top,
			lipgloss.NewStyle().Height(max(innerH-lipgloss.Height(top)-1, 0)).Render(""),
			footer,
		)
	}

	return styles.SidebarStyle.
		Width(innerW).
		Height(innerH).
		Render(content)
}

func shortName(p messages.Page) string {
	switch p {
	case messages.PageHome:
		return "Home"
	case messages.PageProxies:
		return "Proxy"
	case messages.PageConnections:
		return "Conns"
	case messages.PageRules:
		return "Rules"
	case messages.PageLogs:
		return "Logs"
	case messages.PageSettings:
		return "Cfg"
	default:
		return "?"
	}
}
