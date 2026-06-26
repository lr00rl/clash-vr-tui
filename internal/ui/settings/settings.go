package settings

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type settingItem struct {
	label   string
	help    string
	kind    string // "toggle", "select", "info"
	getBool func(*api.Config) bool
	getStr  func(*api.Config) string
}

var logLevels = []string{"debug", "info", "warning", "error", "silent"}

type Model struct {
	client         *api.Client
	config         *api.Config
	items          []settingItem
	cursor         int
	selectingLevel bool
	levelCursor    int
	width          int
	height         int
	err            error
}

func New(client *api.Client) Model {
	items := []settingItem{
		{
			label:   "TUN Mode",
			help:    "Route traffic through the core TUN stack",
			kind:    "toggle",
			getBool: func(c *api.Config) bool { return c.TUN.Enable },
		},
		{
			label:   "Allow LAN",
			help:    "Expose proxy ports to the local network",
			kind:    "toggle",
			getBool: func(c *api.Config) bool { return c.AllowLan },
		},
		{
			label:  "Log Level",
			help:   "Minimum verbosity emitted by mihomo",
			kind:   "select",
			getStr: func(c *api.Config) string { return c.LogLevel },
		},
		{
			label:  "Mixed Port",
			help:   "HTTP and SOCKS entrypoint",
			kind:   "info",
			getStr: func(c *api.Config) string { return fmt.Sprintf("%d", c.MixedPort) },
		},
		{
			label:  "Mode",
			help:   "Runtime routing mode",
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

	width := max(m.width-2, 20)

	var b strings.Builder

	b.WriteString(styles.PageHeader("Runtime Config", "Enter edit  Space toggle", width))
	b.WriteString("\n")
	b.WriteString(styles.Divider(width))
	b.WriteString("\n\n")

	for i, item := range m.items {
		var value string
		if m.config == nil {
			value = "--"
		} else {
			switch item.kind {
			case "toggle":
				if item.getBool(m.config) {
					value = styles.StateBadge("ON", "ok")
				} else {
					value = styles.StateBadge("OFF", "")
				}
			case "select":
				value = styles.Badge(strings.ToUpper(item.getStr(m.config)), true)
			case "info":
				value = styles.Subtle.Render(item.getStr(m.config))
			}
		}

		line := renderSettingRow(item, value, width, i == m.cursor)
		if i == m.cursor {
			b.WriteString(styles.TableRowSelected.Width(width).Render(line))
		} else {
			b.WriteString(styles.TableRow.Render(line))
		}
		b.WriteString("\n")
	}

	// Log level selector overlay
	if m.selectingLevel {
		b.WriteString("\n")
		b.WriteString(styles.PanelStyle.Width(max(width-4, 1)).Render(
			styles.PanelTitle("Log Level") + "\n" + m.renderLevelPicker(width-4),
		))
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString("\n" + styles.ErrorLine(m.err, width))
	}

	return b.String()
}

func renderSettingRow(item settingItem, value string, width int, selected bool) string {
	prefix := "  "
	if selected {
		prefix = "▸ "
	}
	valueW := min(14, max(width/3, 6))
	labelW := max(width-valueW-lipgloss.Width(prefix)-1, 4)
	right := lipgloss.NewStyle().Width(valueW).Align(lipgloss.Right).Render(value)
	top := prefix + styles.PadRight(item.label, labelW) + " " + right
	help := strings.Repeat(" ", lipgloss.Width(prefix)) + styles.Faint.Render(styles.Fit(item.help, labelW))
	return top + "\n" + help
}

func (m Model) renderLevelPicker(width int) string {
	var lines []string
	for i, level := range logLevels {
		prefix := "  "
		if i == m.levelCursor {
			prefix = "▸ "
		}
		current := ""
		if m.config != nil && m.config.LogLevel == level {
			current = " current"
		}
		line := prefix + styles.PadRight(level, 10) + styles.Faint.Render(current)
		if i == m.levelCursor {
			line = styles.TableRowSelected.Width(max(width, 1)).Render(line)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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
