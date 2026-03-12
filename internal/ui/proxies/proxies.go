package proxies

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

// SortMode defines how proxy nodes are sorted within groups.
type SortMode int

const (
	SortDefault SortMode = iota
	SortByName
	SortByDelay
)

func (s SortMode) String() string {
	switch s {
	case SortByName:
		return "Name"
	case SortByDelay:
		return "Delay"
	default:
		return "Default"
	}
}

type groupState struct {
	group    api.Group
	expanded bool
	delays   map[string]int // proxy name -> delay ms (0 = untested, -1 = timeout)
}

type Model struct {
	client   *api.Client
	groups   []groupState
	cursor   int // flat cursor position across all visible items
	filter   string
	filtering bool
	sortMode SortMode
	mode     string // current clash mode
	width    int
	height   int
	err      error
}

func New(client *api.Client) Model {
	return Model{
		client:   client,
		sortMode: SortDefault,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchGroups(), m.fetchConfig())
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.GroupsMsg:
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.rebuildGroups(msg.Groups)
	case messages.ConfigMsg:
		if msg.Err == nil && msg.Config != nil {
			m.mode = msg.Config.Mode
		}
	case messages.GroupDelayMsg:
		if msg.Err == nil {
			for i := range m.groups {
				if m.groups[i].group.Name == msg.Group {
					if m.groups[i].delays == nil {
						m.groups[i].delays = make(map[string]int)
					}
					for k, v := range msg.Result {
						m.groups[i].delays[k] = v
					}
					break
				}
			}
		}
	case messages.ProxyDelayMsg:
		if msg.Err == nil {
			for i := range m.groups {
				for _, name := range m.groups[i].group.All {
					if name == msg.Name {
						if m.groups[i].delays == nil {
							m.groups[i].delays = make(map[string]int)
						}
						m.groups[i].delays[msg.Name] = msg.Delay
						break
					}
				}
			}
		}
	case messages.ProxySelectedMsg:
		if msg.Err == nil {
			for i := range m.groups {
				if m.groups[i].group.Name == msg.Group {
					m.groups[i].group.Now = msg.Proxy
					break
				}
			}
		} else {
			m.err = msg.Err
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
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, m.flatLen()-1)
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(m.flatLen()-1, 0)
	case " ":
		gi := m.groupAtCursor()
		if gi >= 0 {
			m.groups[gi].expanded = !m.groups[gi].expanded
		}
	case "enter":
		gi, ni := m.nodeAtCursor()
		if gi >= 0 && ni >= 0 {
			g := m.groups[gi]
			nodes := m.sortedNodes(gi)
			if ni < len(nodes) {
				return m, m.selectProxy(g.group.Name, nodes[ni])
			}
		}
	case "d":
		gi := m.groupAtCursor()
		if gi >= 0 {
			return m, m.testGroupDelay(m.groups[gi].group.Name)
		}
	case "D":
		gi, ni := m.nodeAtCursor()
		if gi >= 0 && ni >= 0 {
			nodes := m.sortedNodes(gi)
			if ni < len(nodes) {
				return m, m.testProxyDelay(nodes[ni])
			}
		}
	case "o":
		m.sortMode = (m.sortMode + 1) % 3
	case "/":
		m.filtering = true
		m.filter = ""
	case "r":
		return m, tea.Batch(m.fetchGroups(), m.fetchConfig())
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
	return m, nil
}

// Filtering returns whether user is typing in filter input.
func (m Model) Filtering() bool {
	return m.filtering
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder

	// Mode tabs
	modes := []string{"rule", "global", "direct"}
	var tabs []string
	for _, mode := range modes {
		label := fmt.Sprintf("[%s%s]", strings.ToUpper(mode[:1]), mode[1:])
		if mode == m.mode {
			tabs = append(tabs, styles.ModeActive.Render(label))
		} else {
			tabs = append(tabs, styles.ModeInactive.Render(label))
		}
	}
	b.WriteString("Mode: " + strings.Join(tabs, "  "))
	b.WriteString(fmt.Sprintf("    [Sort: %s]", m.sortMode.String()))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", m.width-2))
	b.WriteString("\n")

	// Filter bar
	if m.filtering {
		b.WriteString(styles.FilterPrompt.Render("Filter: ") + m.filter + "█\n")
	} else if m.filter != "" {
		b.WriteString(styles.FilterPrompt.Render("Filter: ") + m.filter + "\n")
	}

	// Groups
	flatIdx := 0
	for gi := range m.groups {
		g := m.groups[gi]

		// Skip hidden groups
		if g.group.Hidden {
			continue
		}

		nodeCount := len(g.group.All)
		arrow := "▶"
		if g.expanded {
			arrow = "▼"
		}

		// Group header
		selected := flatIdx == m.cursor
		header := fmt.Sprintf("%s %s (%s) %d nodes → %s",
			arrow, g.group.Name, g.group.Type, nodeCount, g.group.Now)
		if selected {
			b.WriteString(styles.TableRowSelected.Render("❯ " + header))
		} else {
			b.WriteString(styles.GroupCollapsed.Render("  " + header))
		}
		b.WriteString("\n")
		flatIdx++

		// Expanded nodes
		if g.expanded {
			nodes := m.sortedNodes(gi)
			for _, name := range nodes {
				if m.filter != "" && !strings.Contains(
					strings.ToLower(name),
					strings.ToLower(m.filter),
				) {
					continue
				}

				isSelected := flatIdx == m.cursor
				isCurrent := name == g.group.Now

				// Delay
				delay := getDelay(name, g.delays)
				delayStr := styles.FormatDelay(delay)
				delayStyled := styles.DelayStyle(delay).Render(delayStr)

				prefix := "  "
				if isCurrent {
					prefix = "● "
				}

				// Adaptive name width: leave room for prefix(4) + delay(10)
				nameW := m.width - 20
				if nameW < 16 {
					nameW = 16
				}
				displayName := name
				if len(displayName) > nameW {
					displayName = displayName[:nameW-2] + ".."
				}

				line := fmt.Sprintf("│ %s%-*s %s", prefix, nameW, displayName, delayStyled)
				if isSelected {
					b.WriteString(styles.TableRowSelected.Render("❯ " + line))
				} else {
					b.WriteString(styles.TableRow.Render("  " + line))
				}
				b.WriteString("\n")
				flatIdx++
			}
		}
	}

	if m.err != nil {
		b.WriteString("\n" + styles.DelayBad.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	return b.String()
}

// rebuildGroups replaces group data while preserving UI state.
func (m *Model) rebuildGroups(groups []api.Group) {
	oldState := make(map[string]groupState)
	for _, g := range m.groups {
		oldState[g.group.Name] = g
	}

	m.groups = make([]groupState, 0, len(groups))
	for i, g := range groups {
		if old, ok := oldState[g.Name]; ok {
			m.groups = append(m.groups, groupState{
				group:    g,
				expanded: old.expanded,
				delays:   old.delays,
			})
		} else {
			m.groups = append(m.groups, groupState{
				group:    g,
				expanded: i == 0,
				delays:   make(map[string]int),
			})
		}
	}
}

// sortedNodes returns sorted node names for a group.
func (m Model) sortedNodes(gi int) []string {
	if gi < 0 || gi >= len(m.groups) {
		return nil
	}
	nodes := make([]string, len(m.groups[gi].group.All))
	copy(nodes, m.groups[gi].group.All)

	switch m.sortMode {
	case SortByName:
		sort.Slice(nodes, func(i, j int) bool {
			return strings.ToLower(nodes[i]) < strings.ToLower(nodes[j])
		})
	case SortByDelay:
		delays := m.groups[gi].delays
		sort.Slice(nodes, func(i, j int) bool {
			di := getDelay(nodes[i], delays)
			dj := getDelay(nodes[j], delays)
			if di <= 0 && dj <= 0 {
				return false
			}
			if di <= 0 {
				return false
			}
			if dj <= 0 {
				return true
			}
			return di < dj
		})
	}
	return nodes
}

func getDelay(name string, delays map[string]int) int {
	if delays != nil {
		if d, ok := delays[name]; ok {
			return d
		}
	}
	return 0
}

// flatLen returns total visible items (groups + expanded nodes).
func (m Model) flatLen() int {
	n := 0
	for _, g := range m.groups {
		if g.group.Hidden {
			continue
		}
		n++ // group header
		if g.expanded {
			if m.filter != "" {
				for _, name := range g.group.All {
					if strings.Contains(strings.ToLower(name), strings.ToLower(m.filter)) {
						n++
					}
				}
			} else {
				n += len(g.group.All)
			}
		}
	}
	if n == 0 {
		return 1
	}
	return n
}

// groupAtCursor returns the group index the cursor is on (or -1).
func (m Model) groupAtCursor() int {
	flatIdx := 0
	for gi := range m.groups {
		if m.groups[gi].group.Hidden {
			continue
		}
		if flatIdx == m.cursor {
			return gi
		}
		flatIdx++
		if m.groups[gi].expanded {
			count := m.visibleNodeCount(gi)
			if m.cursor >= flatIdx && m.cursor < flatIdx+count {
				return gi
			}
			flatIdx += count
		}
	}
	return -1
}

// nodeAtCursor returns (groupIndex, nodeIndex) if cursor is on a node.
func (m Model) nodeAtCursor() (int, int) {
	flatIdx := 0
	for gi := range m.groups {
		if m.groups[gi].group.Hidden {
			continue
		}
		flatIdx++ // group header
		if m.groups[gi].expanded {
			nodes := m.sortedNodes(gi)
			for ni, name := range nodes {
				if m.filter != "" && !strings.Contains(
					strings.ToLower(name), strings.ToLower(m.filter)) {
					continue
				}
				if flatIdx == m.cursor {
					return gi, ni
				}
				flatIdx++
			}
		}
	}
	return -1, -1
}

func (m Model) visibleNodeCount(gi int) int {
	if m.filter == "" {
		return len(m.groups[gi].group.All)
	}
	n := 0
	for _, name := range m.groups[gi].group.All {
		if strings.Contains(strings.ToLower(name), strings.ToLower(m.filter)) {
			n++
		}
	}
	return n
}

func (m Model) fetchGroups() tea.Cmd {
	return func() tea.Msg {
		groups, err := m.client.GetGroups()
		return messages.GroupsMsg{Groups: groups, Err: err}
	}
}

func (m Model) fetchConfig() tea.Cmd {
	return func() tea.Msg {
		cfg, err := m.client.GetConfig()
		return messages.ConfigMsg{Config: cfg, Err: err}
	}
}

func (m Model) selectProxy(group, proxy string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.SelectProxy(group, proxy)
		return messages.ProxySelectedMsg{Group: group, Proxy: proxy, Err: err}
	}
}

func (m Model) testGroupDelay(group string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.client.TestGroupDelay(group, "", 0)
		return messages.GroupDelayMsg{Group: group, Result: result, Err: err}
	}
}

func (m Model) testProxyDelay(name string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.client.TestProxyDelay(name, "", 0)
		delay := 0
		if result != nil {
			delay = result.Delay
		}
		return messages.ProxyDelayMsg{Name: name, Delay: delay, Err: err}
	}
}
