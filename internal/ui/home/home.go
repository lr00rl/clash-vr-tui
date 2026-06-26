package home

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	client      *api.Client
	config      *api.Config
	version     *api.VersionInfo
	ruleCount   int
	memory      int64
	activeConns int
	startTime   time.Time
	width       int
	height      int
	err         error
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

// SetStats updates live core stats fed from the connections stream.
func (m Model) SetStats(memory int64, activeConns int) Model {
	m.memory = memory
	m.activeConns = activeConns
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

	width := max(m.width-2, 20)
	online := m.version != nil && m.config != nil
	meta := "waiting for mihomo"
	if online {
		meta = "controller online"
	}

	header := styles.PageHeader("Core Cockpit", meta, width)
	controls := m.renderControls(width)
	core := m.renderCore(width)
	system := m.renderSystem(width)

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		styles.Divider(width),
		"",
		controls,
		"",
		core,
		"",
		system,
	)

	if m.err != nil {
		errMsg := styles.ErrorLine(m.err, width)
		content = lipgloss.JoinVertical(lipgloss.Left, header, styles.Divider(width), "", errMsg, "", controls, "", core, "", system)
	}

	return content
}

func (m Model) renderControls(width int) string {
	var tun, mode, lan, state string
	if m.config != nil {
		tun = renderToggle(m.config.TUN.Enable)
		lan = renderToggle(m.config.AllowLan)
		mode = renderMode(m.config.Mode)
	} else {
		tun, mode, lan = dash(), dash(), dash()
	}
	if m.version != nil && m.config != nil {
		state = styles.StateBadge("ONLINE", "ok")
	} else {
		state = styles.StateBadge("OFFLINE", "bad")
	}

	body := strings.Join([]string{
		state + "  " + mode,
		"tun " + tun + "   lan " + lan,
		styles.Faint.Render("t toggle TUN  m cycle mode  R restart core"),
	}, "\n")
	return panel("Session Controls", body, width)
}

func (m Model) renderCore(width int) string {
	coreVer := valueOr(func() string { return m.version.Version }, m.version != nil)
	status := styles.DelayBad.Render("disconnected")
	if m.version != nil && m.config != nil {
		status = styles.DelayFast.Render("connected")
	}
	var proxyAddr, mixedPort, mode2 string
	if m.config != nil {
		proxyAddr = fmt.Sprintf("127.0.0.1:%d", m.config.MixedPort)
		mixedPort = fmt.Sprintf("%d", m.config.MixedPort)
		mode2 = m.config.Mode
	} else {
		proxyAddr, mixedPort, mode2 = "--", "--", "--"
	}

	body := strings.Builder{}
	body.WriteString(infoRow("Status", status))
	body.WriteString(infoRow("Core", coreVer))
	body.WriteString(infoRow("Mode", mode2))
	body.WriteString(infoRow("Proxy", proxyAddr))
	body.WriteString(infoRow("Mixed Port", mixedPort))
	body.WriteString(infoRow("Memory", formatBytes(m.memory)))
	body.WriteString(infoRow("Connections", fmt.Sprintf("%d active", m.activeConns)))
	body.WriteString(infoRow("Rules", fmt.Sprintf("%d loaded", m.ruleCount)))
	body.WriteString(infoRow("Session", formatDuration(time.Since(m.startTime))))
	return panel("Runtime", strings.TrimRight(body.String(), "\n"), width)
}

func (m Model) renderSystem(width int) string {
	body := strings.Builder{}
	body.WriteString(infoRow("Host", fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)))
	body.WriteString(infoRow("Endpoint", m.client.SocketPath()))
	body.WriteString(infoRow("Go", runtime.Version()))
	body.WriteString(styles.Faint.Render("Use CLI subcommands for scripts: status, proxies, nodes, switch, test, mode, restart."))
	return panel("Environment", strings.TrimRight(body.String(), "\n"), width)
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
		return styles.ToggleOn.Render("ON")
	}
	return styles.ToggleOff.Render("OFF")
}

func renderMode(mode string) string {
	modes := []string{"rule", "global", "direct"}
	var parts []string
	for _, item := range modes {
		parts = append(parts, styles.Badge(strings.ToUpper(item), item == mode))
	}
	return strings.Join(parts, " ")
}

func dash() string { return styles.ToggleOff.Render("--") }

func valueOr(get func() string, ok bool) string {
	if ok {
		return get()
	}
	return "--"
}

func infoRow(label, value string) string {
	return styles.MetricLabel.Render(styles.PadRight(label, 13)) + " " + value + "\n"
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

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1f GB", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func panel(title, body string, width int) string {
	inner := max(width-4, 1)
	return styles.PanelStyle.Width(inner).Render(
		styles.PanelTitle(title) + "\n" + body,
	)
}
