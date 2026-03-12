package settings

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type settingItem struct {
	label    string
	kind     string // "toggle", "select", "info"
	getBool  func(*api.Config) bool
	getStr   func(*api.Config) string
}

var logLevels = []string{"debug", "info", "warning", "error", "silent"}

type Model struct {
	client          *api.Client
	config          *api.Config
	items           []settingItem
	cursor          int
	selectingLevel  bool
	levelCursor     int
	width           int
	height          int
	err             error
}

func New(client *api.Client) Model {
	items := []settingItem{
		{
			label:   "TUN Mode",
			kind:    "toggle",
			getBool: func(c *api.Config) bool { return c.TUN.Enable },
		},
		{
			label:   "Allow LAN",
			kind:    "toggle",
			getBool: func(c *api.Config) bool { return c.AllowLan },
		},
		{
			label:  "Log Level",
			kind:   "select",
			getStr: func(c *api.Config) string { return c.LogLevel },
		},
		{
			label:  "Mixed Port",
			kind:   "info",
			getStr: func(c *api.Config) string { return fmt.Sprintf("%d", c.MixedPort) },
		},
		{
			label:  "Mode",
			kind:   "info",
			getStr: func(c *api.Config) string { return c.Mode },
		},
	}

	return Model{
		client: client,
		items:  items,
	}
}

func (m Model) Init() tea.Cmd {
	return m.fetchConfig()
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ConfigMsg:
		if msg.Err == nil {
			m.config = msg.Config
		} else {
			m.err = msg.Err
		}
	case messages.ConfigPatchedMsg:
		if msg.Err == nil {
			return m, m.fetchConfig()
		}
		m.err = msg.Err
	case tea.KeyMsg:
		if m.selectingLevel {
			return m.handleLevelSelect(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, len(m.items)-1)
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "enter", " ":
		return m.activateItem()
	case "r":
		return m, m.fetchConfig()
	}
	return m, nil
}

func (m Model) handleLevelSelect(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.levelCursor = min(m.levelCursor+1, len(logLevels)-1)
	case "k", "up":
		m.levelCursor = max(m.levelCursor-1, 0)
	case "enter":
		level := logLevels[m.levelCursor]
		m.selectingLevel = false
		return m, m.patchConfig(api.ConfigPatch{LogLevel: &level})
	case "esc":
		m.selectingLevel = false
	}
	return m, nil
}

func (m Model) activateItem() (Model, tea.Cmd) {
	if m.config == nil || m.cursor >= len(m.items) {
		return m, nil
	}

	item := m.items[m.cursor]
	switch item.kind {
	case "toggle":
		return m.toggleItem(item)
	case "select":
		if item.label == "Log Level" {
			m.selectingLevel = true
			// Set level cursor to current level
			for i, l := range logLevels {
				if l == m.config.LogLevel {
					m.levelCursor = i
					break
				}
			}
		}
	}
	return m, nil
}

func (m Model) toggleItem(item settingItem) (Model, tea.Cmd) {
	if m.config == nil {
		return m, nil
	}

	switch item.label {
	case "TUN Mode":
		enabled := !m.config.TUN.Enable
		return m, m.patchConfig(api.ConfigPatch{
			TUN: &api.TUNConfig{Enable: enabled},
		})
	case "Allow LAN":
		enabled := !m.config.AllowLan
		return m, m.patchConfig(api.ConfigPatch{AllowLan: &enabled})
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	boxW := m.width - 2
	if boxW < 20 {
		boxW = 20
	}

	var b strings.Builder

	// System section
	b.WriteString(styles.SectionTitle.Render("System") + "\n")
	b.WriteString(strings.Repeat("─", boxW) + "\n")

	for i, item := range m.items {
		prefix := "  "
		if i == m.cursor {
			prefix = "❯ "
		}

		var value string
		if m.config == nil {
			value = "--"
		} else {
			switch item.kind {
			case "toggle":
				if item.getBool(m.config) {
					value = styles.ToggleOn.Render("[ON ]")
				} else {
					value = styles.ToggleOff.Render("[OFF]")
				}
			case "select":
				value = item.getStr(m.config)
			case "info":
				value = styles.HelpDesc.Render(item.getStr(m.config))
			}
		}

		line := fmt.Sprintf("%s%-20s %s", prefix, item.label, value)
		if i == m.cursor {
			b.WriteString(styles.TableRowSelected.Render(line))
		} else {
			b.WriteString(styles.TableRow.Render(line))
		}
		b.WriteString("\n")
	}

	// Log level selector overlay
	if m.selectingLevel {
		b.WriteString("\n" + styles.SectionTitle.Render("Select Log Level") + "\n")
		for i, level := range logLevels {
			prefix := "  "
			if i == m.levelCursor {
				prefix = "❯ "
			}
			current := ""
			if m.config != nil && m.config.LogLevel == level {
				current = " (current)"
			}
			b.WriteString(fmt.Sprintf("%s%s%s\n", prefix, level, current))
		}
	}

	if m.err != nil {
		b.WriteString("\n" + styles.DelayBad.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return b.String()
}

func (m Model) fetchConfig() tea.Cmd {
	return func() tea.Msg {
		cfg, err := m.client.GetConfig()
		return messages.ConfigMsg{Config: cfg, Err: err}
	}
}

func (m Model) patchConfig(patch api.ConfigPatch) tea.Cmd {
	return func() tea.Msg {
		err := m.client.PatchConfig(patch)
		return messages.ConfigPatchedMsg{Err: err}
	}
}
