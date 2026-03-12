package helpbar

import (
	"strings"

	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Binding struct {
	Key  string
	Desc string
}

type Model struct {
	width int
}

func New() Model {
	return Model{}
}

func (m Model) SetWidth(w int) Model {
	m.width = w
	return m
}

func (m Model) View(page messages.Page) string {
	bindings := globalBindings()
	bindings = append(bindings, pageBindings(page)...)

	var parts []string
	for _, b := range bindings {
		parts = append(parts,
			styles.HelpKey.Render(b.Key)+styles.HelpDesc.Render(":"+b.Desc),
		)
	}

	content := strings.Join(parts, "  ")
	return styles.HelpBar.Width(m.width).Render(content)
}

func globalBindings() []Binding {
	return []Binding{
		{Key: "q", Desc: "Quit"},
		{Key: "Tab", Desc: "Next"},
		{Key: "?", Desc: "Help"},
		{Key: "r", Desc: "Refresh"},
	}
}

func pageBindings(page messages.Page) []Binding {
	switch page {
	case messages.PageHome:
		return []Binding{
			{Key: "t", Desc: "TUN"},
			{Key: "m", Desc: "Mode"},
		}
	case messages.PageProxies:
		return []Binding{
			{Key: "←→", Desc: "Panel"},
			{Key: "↑↓", Desc: "Move"},
			{Key: "Enter", Desc: "Select"},
			{Key: "d", Desc: "Test"},
			{Key: "o", Desc: "Sort"},
			{Key: "/", Desc: "Filter"},
		}
	case messages.PageConnections:
		return []Binding{
			{Key: "/", Desc: "Filter"},
			{Key: "Enter", Desc: "Detail"},
			{Key: "x", Desc: "Close"},
			{Key: "s", Desc: "Sort"},
		}
	case messages.PageRules:
		return []Binding{
			{Key: "/", Desc: "Filter"},
			{Key: "g", Desc: "Top"},
			{Key: "G", Desc: "Bottom"},
		}
	case messages.PageSettings:
		return []Binding{
			{Key: "Enter", Desc: "Toggle"},
		}
	}
	return nil
}
