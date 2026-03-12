package home

import (
	"fmt"
	"runtime"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	client    *api.Client
	config    *api.Config
	version   *api.VersionInfo
	ruleCount int
	startTime time.Time
	width     int
	height    int
	err       error
}

func New(client *api.Client) Model {
	return Model{
		client:    client,
		startTime: time.Now(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchConfig(),
		m.fetchVersion(),
		m.fetchRuleCount(),
	)
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ConfigMsg:
		m.config = msg.Config
		m.err = msg.Err
	case messages.VersionMsg:
		m.version = msg.Version
		if msg.Err != nil {
			m.err = msg.Err
		}
	case messages.RulesMsg:
		if msg.Err == nil && msg.Rules != nil {
			m.ruleCount = len(msg.Rules.Rules)
		}
	case messages.ConfigPatchedMsg:
		if msg.Err == nil {
			return m, m.fetchConfig()
		}
		m.err = msg.Err
	case tea.KeyMsg:
		switch msg.String() {
		case "t":
			if m.config != nil {
				enabled := !m.config.TUN.Enable
				return m, m.patchConfig(api.ConfigPatch{
					TUN: &api.TUNConfig{Enable: enabled},
				})
			}
		case "m":
			if m.config != nil {
				next := cycleMode(m.config.Mode)
				return m, m.patchConfig(api.ConfigPatch{Mode: &next})
			}
		case "r":
			return m, tea.Batch(m.fetchConfig(), m.fetchVersion(), m.fetchRuleCount())
		}
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

	// Quick Controls
	var tun, mode string
	if m.config != nil {
		tun = renderToggle(m.config.TUN.Enable)
		mode = styles.ModeActive.Render(fmt.Sprintf("[%s]", m.config.Mode))
	} else {
		tun = styles.ToggleOff.Render("[--]")
		mode = styles.ToggleOff.Render("[--]")
	}

	controls := styles.SectionBorder.Width(boxW).Render(
		styles.SectionTitle.Render("Quick Controls") + "\n" +
			fmt.Sprintf(" TUN %s   Mode %s", tun, mode),
	)

	// Clash Info
	var coreVer, proxyAddr, mixedPort, uptime, rules string
	if m.version != nil {
		coreVer = m.version.Version
	} else {
		coreVer = "--"
	}
	if m.config != nil {
		proxyAddr = fmt.Sprintf("127.0.0.1:%d", m.config.MixedPort)
		mixedPort = fmt.Sprintf("%d", m.config.MixedPort)
	} else {
		proxyAddr = "--"
		mixedPort = "--"
	}
	uptime = formatDuration(time.Since(m.startTime))
	rules = fmt.Sprintf("%d", m.ruleCount)

	clashInfo := styles.SectionBorder.Width(boxW).Render(
		styles.SectionTitle.Render("Clash Info") + "\n" +
			infoRow("Core Version", coreVer) +
			infoRow("Proxy Addr", proxyAddr) +
			infoRow("Mixed Port", mixedPort) +
			infoRow("Uptime", uptime) +
			infoRow("Rules Count", rules),
	)

	// System Info
	osInfo := fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
	sysInfo := styles.SectionBorder.Width(boxW).Render(
		styles.SectionTitle.Render("System Info") + "\n" +
			infoRow("OS", osInfo) +
			infoRow("Go Version", runtime.Version()),
	)

	content := lipgloss.JoinVertical(lipgloss.Left, controls, "", clashInfo, "", sysInfo)

	if m.err != nil {
		errMsg := styles.DelayBad.Render(fmt.Sprintf("Error: %v", m.err))
		content = lipgloss.JoinVertical(lipgloss.Left, errMsg, "", content)
	}

	return content
}

func (m Model) fetchConfig() tea.Cmd {
	return func() tea.Msg {
		cfg, err := m.client.GetConfig()
		return messages.ConfigMsg{Config: cfg, Err: err}
	}
}

func (m Model) fetchVersion() tea.Cmd {
	return func() tea.Msg {
		ver, err := m.client.GetVersion()
		return messages.VersionMsg{Version: ver, Err: err}
	}
}

func (m Model) fetchRuleCount() tea.Cmd {
	return func() tea.Msg {
		rules, err := m.client.GetRules()
		return messages.RulesMsg{Rules: rules, Err: err}
	}
}

func (m Model) patchConfig(patch api.ConfigPatch) tea.Cmd {
	return func() tea.Msg {
		err := m.client.PatchConfig(patch)
		return messages.ConfigPatchedMsg{Err: err}
	}
}

func renderToggle(on bool) string {
	if on {
		return styles.ToggleOn.Render("[ON ]")
	}
	return styles.ToggleOff.Render("[OFF]")
}

func infoRow(label, value string) string {
	return fmt.Sprintf(" %-16s %s\n", label, value)
}

func cycleMode(current string) string {
	switch current {
	case "rule":
		return "global"
	case "global":
		return "direct"
	default:
		return "rule"
	}
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%dh %dm %ds", h, m, s)
}
