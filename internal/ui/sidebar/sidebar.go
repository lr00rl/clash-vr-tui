package sidebar

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	Active messages.Page
	pages  []messages.Page
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

func (m Model) View() string {
	var items []string
	for _, p := range m.pages {
		label := " " + p.String()
		if p == m.Active {
			items = append(items, styles.SidebarActive.Render("❯"+label))
		} else {
			items = append(items, styles.SidebarItem.Render(" "+label))
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, items...)

	return styles.SidebarStyle.
		Height(m.height).
		Render(content)
}
