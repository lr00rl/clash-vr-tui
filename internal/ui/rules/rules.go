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

// Filtering reports whether the user is typing in the filter input.
func (m Model) Filtering() bool {
	return m.filtering
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
	maxRows := m.rows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+maxRows {
		m.offset = m.cursor - maxRows + 1
	}
}

func (m Model) rows() int {
	maxRows := m.height - 6
	if m.filtering || m.filter != "" {
		maxRows--
	}
	if maxRows < 1 {
		maxRows = 1
	}
	return maxRows
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	width := max(m.width-2, 20)

	visible := m.visibleRules()
	meta := fmt.Sprintf("%d shown  %d total", len(visible), len(m.rules))

	// Header
	b.WriteString(styles.PageHeader("Rules", meta, width))
	b.WriteString("\n")
	if m.filtering {
		b.WriteString(styles.FilterLine("Filter", m.filter, true))
		b.WriteString("\n")
	} else if m.filter != "" {
		b.WriteString(styles.FilterLine("Filter", m.filter, false))
		b.WriteString("\n")
	}
	b.WriteString(styles.Divider(width) + "\n")

	// Column headers
	idxW, typeW, payloadW, proxyW := m.columnWidths(width)
	colHeader := styles.PadRight("#", idxW) + " " +
		styles.PadRight("Type", typeW) + " " +
		styles.PadRight("Payload", payloadW) + " " +
		styles.PadRight("Proxy", proxyW)
	b.WriteString(styles.TableHeader.Render(colHeader) + "\n")
	b.WriteString(styles.Divider(width) + "\n")

	if len(visible) == 0 {
		b.WriteString(styles.EmptyState("No matching rules", "Clear the filter or search by type, payload, or proxy.", width))
		return b.String()
	}

	// Rows
	maxRows := m.rows()

	end := m.offset + maxRows
	if end > len(visible) {
		end = len(visible)
	}

	for i := m.offset; i < end; i++ {
		rule := visible[i]

		line := styles.PadRight(fmt.Sprintf("%d", i+1), idxW) + " " +
			styles.PadRight(rule.Type, typeW) + " " +
			styles.PadRight(rule.Payload, payloadW) + " " +
			styles.PadRight(rule.Proxy, proxyW)

		if i == m.cursor {
			b.WriteString(styles.TableRowSelected.Width(width).Render("▸ " + line))
		} else {
			b.WriteString(styles.TableRow.Render("  " + line))
		}
		b.WriteString("\n")
	}

	if m.err != nil {
		b.WriteString("\n" + styles.ErrorLine(m.err, width))
	}

	return b.String()
}

func (m Model) columnWidths(width int) (int, int, int, int) {
	tableW := max(width-2, 12)
	idxW := min(5, max(tableW/10, 3))
	typeW := min(18, max(tableW/5, 5))
	proxyW := min(18, max(tableW/5, 5))
	payloadW := tableW - idxW - typeW - proxyW - 3
	for payloadW < 4 && (proxyW > 5 || typeW > 5) {
		if proxyW > 5 {
			proxyW--
		}
		if payloadW = tableW - idxW - typeW - proxyW - 3; payloadW >= 4 {
			break
		}
		if typeW > 5 {
			typeW--
		}
		payloadW = tableW - idxW - typeW - proxyW - 3
	}
	payloadW = max(payloadW, 4)
	return idxW, typeW, payloadW, proxyW
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
