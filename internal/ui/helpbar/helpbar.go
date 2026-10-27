package helpbar

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

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
	bindings := prioritizedBindings(page)
	innerW := max(m.width-2, 1)

	var parts []string
	for _, b := range bindings {
		parts = append(parts, styles.KeyHint(b.Key, b.Desc))
	}

	content := joinWithin(parts, innerW)
	return styles.HelpBar.Width(innerW).Render(content)
}

// prioritizedBindings keeps the current task discoverable when an SSH pane is
// too narrow to show every shortcut. The full list remains available with ?.
func prioritizedBindings(page messages.Page) []Binding {
	pageKeys := pageBindings(page)
	primaryCount := min(2, len(pageKeys))
	bindings := append([]Binding(nil), pageKeys[:primaryCount]...)
	bindings = append(bindings,
		Binding{Key: "?", Desc: "more"},
	)
	bindings = append(bindings, pageKeys[primaryCount:]...)
	bindings = append(bindings,
		Binding{Key: "Tab", Desc: "page"},
		Binding{Key: "r", Desc: "refresh"},
		Binding{Key: "q", Desc: "quit"},
		Binding{Key: "R", Desc: "restart"},
	)
	return bindings
}

func pageBindings(page messages.Page) []Binding {
	switch page {
	case messages.PageHome:
		return []Binding{
			{Key: "t", Desc: "TUN"},
			{Key: "m", Desc: "mode"},
		}
	case messages.PageProxies:
		return []Binding{
			{Key: "←→", Desc: "panel"},
			{Key: "C-d/u", Desc: "page"},
			{Key: "gg/G", Desc: "edge"},
			{Key: "Enter", Desc: "select"},
			{Key: "d", Desc: "test"},
			{Key: "T", Desc: "probe"},
			{Key: "o", Desc: "sort"},
			{Key: "u", Desc: "auto"},
			{Key: "/", Desc: "filter"},
		}
	case messages.PageConnections:
		return []Binding{
			{Key: "/", Desc: "filter"},
			{Key: "Enter", Desc: "detail"},
			{Key: "x", Desc: "close"},
			{Key: "X", Desc: "close all"},
			{Key: "s", Desc: "sort"},
		}
	case messages.PageRules:
		return []Binding{
			{Key: "/", Desc: "filter"},
			{Key: "g", Desc: "top"},
			{Key: "G", Desc: "bottom"},
		}
	case messages.PageLogs:
		return []Binding{
			{Key: "space", Desc: "freeze"},
			{Key: "l", Desc: "level"},
			{Key: "c", Desc: "clear"},
			{Key: "/", Desc: "filter"},
		}
	case messages.PageSettings:
		return []Binding{
			{Key: "Enter", Desc: "edit"},
		}
	}
	return nil
}

func joinWithin(parts []string, width int) string {
	for len(parts) > 0 {
		content := strings.Join(parts, "  ")
		if lipgloss.Width(content) <= width {
			return content
		}
		parts = parts[:len(parts)-1]
	}
	return ""
}
