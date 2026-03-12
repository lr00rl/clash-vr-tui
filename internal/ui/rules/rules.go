package rules

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	client    *api.Client
	rules     []api.Rule
	cursor    int
	offset    int // viewport scroll offset
	filter    string
	filtering bool
	width     int
	height    int
	err       error
}

func New(client *api.Client) Model {
	return Model{client: client}
}

func (m Model) Init() tea.Cmd {
	return m.fetchRules()
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.RulesMsg:
		if msg.Err != nil {
			m.err = msg.Err
		} else if msg.Rules != nil {
			m.rules = msg.Rules.Rules
		}
	case tea.KeyMsg:
		if m.filtering {
			return m.handleFilterKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	visible := m.visibleRules()
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(visible)-1, 0))
		m.adjustViewport()
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
		m.adjustViewport()
	case "g":
		m.cursor = 0
		m.offset = 0
	case "G":
		m.cursor = max(len(visible)-1, 0)
		m.adjustViewport()
	case "/":
		m.filtering = true
		m.filter = ""
	case "r":
		return m, m.fetchRules()
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filter = ""
	case "enter":
		m.filtering = false
	case "backspace":
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
		}
	default:
		if len(msg.String()) == 1 {
			m.filter += msg.String()
		}
	}
	m.cursor = 0
	m.offset = 0
	return m, nil
}

func (m *Model) adjustViewport() {
	maxRows := m.height - 5
	if maxRows < 1 {
		maxRows = 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+maxRows {
		m.offset = m.cursor - maxRows + 1
	}
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	visible := m.visibleRules()

	// Header
	b.WriteString(fmt.Sprintf("Rules (%d)", len(visible)))
	if m.filtering {
		b.WriteString("   " + styles.FilterPrompt.Render("Filter: ") + m.filter + "█")
	} else if m.filter != "" {
		b.WriteString("   " + styles.FilterPrompt.Render("Filter: ") + m.filter)
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2) + "\n")

	// Column headers
	colHeader := fmt.Sprintf("%-6s %-18s %-28s %s", "#", "Type", "Payload", "Proxy")
	b.WriteString(styles.TableHeader.Render(colHeader) + "\n")
	b.WriteString(strings.Repeat("─", m.width-2) + "\n")

	// Rows
	maxRows := m.height - 5
	if maxRows < 1 {
		maxRows = 1
	}

	end := m.offset + maxRows
	if end > len(visible) {
		end = len(visible)
	}

	for i := m.offset; i < end; i++ {
		rule := visible[i]

		payload := rule.Payload
		if len(payload) > 26 {
			payload = payload[:26] + ".."
		}
		proxy := rule.Proxy
		if len(proxy) > 14 {
			proxy = proxy[:14] + ".."
		}

		line := fmt.Sprintf("%-6d %-18s %-28s %s", i+1, rule.Type, payload, proxy)

		if i == m.cursor {
			b.WriteString(styles.TableRowSelected.Render("❯ " + line))
		} else {
			b.WriteString(styles.TableRow.Render("  " + line))
		}
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString("\n" + styles.DelayBad.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return b.String()
}

func (m Model) visibleRules() []api.Rule {
	if m.filter == "" {
		return m.rules
	}
	f := strings.ToLower(m.filter)
	var result []api.Rule
	for _, r := range m.rules {
		if strings.Contains(strings.ToLower(r.Payload), f) ||
			strings.Contains(strings.ToLower(r.Type), f) ||
			strings.Contains(strings.ToLower(r.Proxy), f) {
			result = append(result, r)
		}
	}
	return result
}

func (m Model) fetchRules() tea.Cmd {
	return func() tea.Msg {
		rules, err := m.client.GetRules()
		return messages.RulesMsg{Rules: rules, Err: err}
	}
}
