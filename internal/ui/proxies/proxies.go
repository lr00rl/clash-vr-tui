package proxies

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/probe"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

// probeTimeout bounds a single TCP/ICMP test; probeConcurrency caps parallel
// pings when testing a whole group.
const (
	probeTimeout     = 3 * time.Second
	probeConcurrency = 32
)

type SortMode int

const (
	SortDefault SortMode = iota
	SortByName
	SortByDelay
)

const (
	focusGroups = "groups"
	focusNodes  = "nodes"

	proxyPanelGap      = 1
	groupPanelMinWidth = 28
	groupPanelMaxWidth = 42
	// panelChromeWidth is border plus padding. lipgloss counts padding inside
	// Width, so a box is sized width-panelBorderWidth to fill its slot.
	panelChromeWidth = 4
	panelBorderWidth = 2
	// Below twoPanelMinWidth the node column would cut names to a few cells,
	// so the page shows one panel at a time.
	twoPanelMinWidth  = 78
	panelChromeHeight = 2

	nodeTableHeaderHeight = 2
	nodeTableBottomSlack  = 1
)

type groupState struct {
	group  api.Group
	delays map[probe.Mode]map[string]messages.DelayResult
}

type visibleGroup struct {
	index int
	nodes []string
}

type Model struct {
	client      *api.Client
	groups      []groupState
	groupCursor int
	nodeCursor  int
	groupOffset int
	nodeOffset  int
	filter      string
	filtering   bool
	sortMode    SortMode
	testMode    probe.Mode
	endpoints   *probe.Endpoints
	epErr       error
	mode        string
	focus       string
	width       int
	height      int
	err         error
	// testURL is the configured default for groups without their own testUrl.
	testURL string
	// inflight maps a probe key to the newest request for it; a reply whose
	// RequestID differs was superseded and is dropped.
	inflight    map[string]uint64
	lastRequest uint64
	testSummary string
}

func New(client *api.Client) Model {
	return NewWithTestURL(client, "")
}

func NewWithTestURL(client *api.Client, testURL string) Model {
	return Model{
		client:    client,
		testURL:   testURL,
		sortMode:  SortDefault,
		testMode:  probe.ModeHTTP,
		endpoints: probe.NewEndpoints(),
		focus:     focusGroups,
		inflight:  make(map[string]uint64),
	}
}

// probeKey identifies one test target: a whole group when node is empty.
func probeKey(group string, mode probe.Mode, node string) string {
	return group + "\x00" + mode.String() + "\x00" + node
}

func (m *Model) track(key string) uint64 {
	if m.inflight == nil {
		m.inflight = make(map[string]uint64)
	}
	m.lastRequest++
	m.inflight[key] = m.lastRequest
	return m.lastRequest
}

// settle reports whether a reply is the newest request for key and, if so,
// marks that key as no longer in flight.
func (m *Model) settle(key string, id uint64) bool {
	if m.inflight[key] != id {
		return false
	}
	delete(m.inflight, key)
	return true
}

func (g *groupState) modeDelays(mode probe.Mode) map[string]messages.DelayResult {
	if g.delays == nil {
		g.delays = make(map[probe.Mode]map[string]messages.DelayResult)
	}
	if g.delays[mode] == nil {
		g.delays[mode] = make(map[string]messages.DelayResult)
	}
	return g.delays[mode]
}

func (m *Model) groupByName(name string) *groupState {
	for i := range m.groups {
		if m.groups[i].group.Name == name {
			return &m.groups[i]
		}
	}
	return nil
}

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

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchGroups(), m.fetchConfig())
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	m.adjustOffsets()
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	var cmd tea.Cmd

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
		if !m.settle(probeKey(msg.Group, msg.Mode, ""), msg.RequestID) {
			break
		}
		m.testSummary = summarizeGroupTest(msg.Group, msg.Mode, msg.Results)
		if g := m.groupByName(msg.Group); g != nil {
			dst := g.modeDelays(msg.Mode)
			for node, r := range msg.Results {
				// A single-node test started since then owns that row.
				if _, busy := m.inflight[probeKey(msg.Group, msg.Mode, node)]; busy {
					continue
				}
				dst[node] = r
			}
		}
	case messages.ProxyDelayMsg:
		if !m.settle(probeKey(msg.Group, msg.Mode, msg.Name), msg.RequestID) {
			break
		}
		m.testSummary = fmt.Sprintf("%s %s: %s", strings.ToUpper(msg.Mode.String()), styles.SafeText(msg.Name), describeDelay(msg.Result))
		if g := m.groupByName(msg.Group); g != nil {
			g.modeDelays(msg.Mode)[msg.Name] = msg.Result
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
			m, cmd = m.handleFilterKey(msg)
		} else {
			m, cmd = m.handleKey(msg)
		}
	}

	m.clampSelection()
	m.adjustOffsets()
	return m, cmd
}

func (m Model) handleKey(msg tea.KeyMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "left", "h", "esc":
		m.focus = focusGroups
	case "right", "l":
		if len(m.visibleGroups()) > 0 {
			m.focus = focusNodes
		}
	case "j", "down":
		if m.focus == focusGroups {
			m.groupCursor++
			m.nodeCursor = 0
		} else {
			m.moveNodeVertical(1)
		}
	case "k", "up":
		if m.focus == focusGroups {
			m.groupCursor--
			m.nodeCursor = 0
		} else {
			m.moveNodeVertical(-1)
		}
	case "ctrl+d":
		m.moveFocusedPage(1)
	case "ctrl+u":
		m.moveFocusedPage(-1)
	case "g":
		if m.focus == focusGroups {
			m.groupCursor = 0
		} else {
			m.nodeCursor = 0
		}
	case "G":
		if m.focus == focusGroups {
			m.groupCursor = max(len(m.visibleGroups())-1, 0)
		} else {
			m.nodeCursor = max(len(m.currentNodes())-1, 0)
		}
	case "enter":
		if m.focus == focusGroups {
			m.focus = focusNodes
			return m, nil
		}
		group := m.selectedGroup()
		nodes := m.currentNodes()
		if group == nil || m.nodeCursor >= len(nodes) {
			return m, nil
		}
		return m, m.selectProxy(group.Name, nodes[m.nodeCursor])
	case "d":
		group := m.selectedGroup()
		if group == nil {
			return m, nil
		}
		if m.focus == focusGroups {
			return m.startGroupTest(*group)
		}
		nodes := m.currentNodes()
		if m.nodeCursor < len(nodes) {
			return m.startNodeTest(*group, nodes[m.nodeCursor])
		}
	case "D":
		group := m.selectedGroup()
		nodes := m.currentNodes()
		if group != nil && m.nodeCursor < len(nodes) {
			return m.startNodeTest(*group, nodes[m.nodeCursor])
		}
	case "T":
		m.testMode = m.testMode.Next()
		m.ensureEndpoints()
	case "u":
		group := m.selectedGroup()
		if group != nil && (group.Type == "URLTest" || group.Type == "Fallback") {
			return m, m.unfixGroup(group.Name)
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

// ensureEndpoints lazily loads node server endpoints from the running config the
// first time a TCP/ICMP test mode is selected.
func (m *Model) ensureEndpoints() {
	if !m.testMode.NeedsEndpoints() {
		return
	}
	if m.endpoints == nil {
		m.endpoints = probe.NewEndpoints()
	}
	if m.endpoints.Len() == 0 {
		m.epErr = m.endpoints.Load("")
	}
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

	m.groupCursor = 0
	m.nodeCursor = 0
	m.groupOffset = 0
	m.nodeOffset = 0
	return m, nil
}

func (m Model) Filtering() bool {
	return m.filtering
}

func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	headerLines := m.renderHeaderLines()
	bodyHeight := m.proxyBodyHeight()
	groupW, nodeW := m.panelWidths()

	if m.singlePanel() {
		panel := m.renderGroupsPanel(m.usableWidth(), bodyHeight)
		if m.focus == focusNodes {
			panel = m.renderNodesPanel(m.usableWidth(), bodyHeight)
		}
		return strings.Join(append(headerLines, panel), "\n")
	}
	left := m.renderGroupsPanel(groupW, bodyHeight)
	right := m.renderNodesPanel(nodeW, bodyHeight)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", proxyPanelGap), right)
	return strings.Join(append(headerLines, body), "\n")
}

func (m Model) renderHeaderLines() []string {
	modes := []string{"rule", "global", "direct"}
	tabs := make([]string, 0, len(modes))
	for _, mode := range modes {
		// A narrow pane keeps only the active mode; the others are not keys here.
		if m.singlePanel() && mode != m.mode {
			continue
		}
		label := strings.ToUpper(mode)
		tabs = append(tabs, styles.Badge(label, mode == m.mode))
	}

	visible := m.visibleGroups()
	nodeCount := len(m.currentNodes())
	meta := fmt.Sprintf("%d groups  %d nodes  sort %s", len(visible), nodeCount, strings.ToLower(m.sortMode.String()))
	testInfo := styles.Badge("PROBE "+strings.ToUpper(m.testMode.String()), true)
	if m.testMode.NeedsEndpoints() {
		if m.epErr != nil {
			testInfo += " " + styles.StateBadge("CONFIG?", "bad")
		} else if m.endpoints != nil {
			testInfo += " " + styles.Faint.Render(fmt.Sprintf("%d endpoints", m.endpoints.Len()))
		}
	}

	testInfo += "  " + styles.Faint.Render(m.focusHint())
	if n := len(m.inflight); n > 0 {
		testInfo += "  " + styles.StateBadge(fmt.Sprintf("TESTING %d", n), "warn")
	} else if m.testSummary != "" {
		testInfo += "  " + styles.Faint.Render(m.testSummary)
	}

	lines := []string{
		styles.PageHeader("Proxy Matrix", meta, max(m.width-2, 20)),
		ansi.Truncate(strings.Join(tabs, " ")+"  "+testInfo, m.usableWidth(), ""),
		styles.Divider(max(m.width-2, 1)),
	}
	if m.filtering {
		lines = append(lines, styles.FilterLine("Filter", m.filter, true))
	} else if m.filter != "" {
		lines = append(lines, styles.FilterLine("Filter", m.filter, false))
	}
	return lines
}

func (m Model) renderGroupsPanel(width, height int) string {
	visible := m.visibleGroups()
	bodyH := proxyPanelContentHeight(height)
	lines := make([]string, 0, bodyH+1)

	title := fmt.Sprintf(" Groups %d ", len(visible))
	if m.focus == focusGroups {
		title = styles.ProxyGroupHeaderSelected.Render(title)
	} else {
		title = styles.ProxyGroupHeader.Render(title)
	}
	lines = append(lines, title)

	rows := groupRowsForPanelHeight(height)
	start := min(m.groupOffset, max(len(visible)-rows, 0))
	end := min(start+rows, len(visible))
	innerW := max(width-panelChromeWidth, 1)
	rowW := max(innerW-1, 1)

	for i := start; i < end; i++ {
		g := m.groups[visible[i].index].group
		prefix := "  "
		if i == m.groupCursor {
			prefix = "▸ "
		}

		typeW := 8
		countW := 4
		nameW := max(rowW-lipgloss.Width(prefix)-typeW-countW-2, 8)
		raw := prefix +
			styles.PadRight(g.Name, nameW) + " " +
			styles.PadRight(g.Type, typeW) +
			styles.PadLeft(fmt.Sprintf("%d", len(g.All)), countW)
		if i == m.groupCursor {
			lines = append(lines, styles.TableRowSelected.Width(rowW).Render(raw))
		} else {
			lines = append(lines, styles.TableRow.Width(rowW).Render(raw))
		}
	}

	for len(lines) < bodyH {
		lines = append(lines, "")
	}

	box := styles.ProxyGroupBox
	if m.focus == focusGroups {
		box = styles.ProxyGroupBoxSelected
	}
	return box.Width(max(width-panelBorderWidth, 1)).Height(bodyH).Render(strings.Join(lines, "\n"))
}

func (m Model) renderNodesPanel(width, height int) string {
	bodyH := proxyPanelContentHeight(height)
	group := m.selectedGroup()
	if group == nil {
		box := styles.ProxyGroupBox
		if m.focus == focusNodes {
			box = styles.ProxyGroupBoxSelected
		}
		return box.Width(max(width-panelBorderWidth, 1)).Height(bodyH).Render(
			styles.PanelTitle("Nodes") + "\n" + styles.EmptyState("No proxy group", "Refresh after the core is available.", width-panelChromeWidth),
		)
	}

	nodes := m.currentNodes()
	titleText := fmt.Sprintf(" %s  %s  now %s ", styles.SafeText(group.Name), styles.SafeText(group.Type), truncateDisplayWidth(styles.SafeText(group.Now), max(width-28, 8)))
	title := styles.ProxyGroupHeader.Render(titleText)
	if m.focus == focusNodes {
		title = styles.ProxyGroupHeaderSelected.Render(titleText)
	}

	content := title + "\n" + m.renderNodeTable(width-panelChromeWidth, nodeTableHeightForPanelHeight(height))

	box := styles.ProxyGroupBox
	if m.focus == focusNodes {
		box = styles.ProxyGroupBoxSelected
	}

	if len(nodes) == 0 {
		content = title + "\n" + styles.EmptyState("No nodes match the current filter.", "Try a name or delay predicate such as delay<200.", width-panelChromeWidth)
	}
	return box.Width(max(width-panelBorderWidth, 1)).Height(bodyH).Render(content)
}

func (m Model) renderNodeTable(innerWidth, bodyHeight int) string {
	nodes := m.currentNodes()
	if len(nodes) == 0 {
		return ""
	}

	tableW := max(innerWidth-6, 24)
	rows := nodeRowsForTableHeight(bodyHeight)
	start := clamp(m.nodeOffset, 0, max(len(nodes)-rows, 0))
	end := min(start+rows, len(nodes))

	delayW := 9
	stateW := 9
	prefixW := 2
	nameW := max(tableW-prefixW-delayW-stateW-3, 8)
	colHeader := strings.Repeat(" ", prefixW) +
		styles.PadRight("Node", nameW) + " " +
		styles.PadLeft("Delay", delayW) + " " +
		styles.PadRight("State", stateW)

	lines := []string{styles.TableHeader.Render(colHeader), styles.Divider(tableW)}
	for i := start; i < end; i++ {
		lines = append(lines, m.renderNodeRow(nodes[i], i, nameW, delayW, stateW, tableW))
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return strings.Join(lines[:bodyHeight], "\n")
}

func (m Model) renderNodeRow(name string, idx, nameW, delayW, stateW, innerWidth int) string {
	group := m.selectedGroup()
	delayText := styles.DelayNone.Render("--")
	current := ""
	fixed := ""
	if group != nil {
		r, tested := m.selectedGroupState().delays[m.testMode][name]
		switch {
		case m.testing(group.Name, name):
			delayText = styles.Faint.Render("testing")
		case tested:
			delayText = renderDelay(r)
		}
		current = group.Now
		fixed = group.Fixed
	}

	prefix := "  "
	if m.focus == focusNodes && idx == m.nodeCursor {
		prefix = "▸ "
	}

	state := styles.Faint.Render("ready")
	if name == current {
		state = styles.DelayFast.Render("active")
	}
	if name == fixed && fixed != "" {
		state = styles.DelaySlow.Render("pinned")
	}
	line := prefix +
		styles.PadRight(name, nameW) + " " +
		lipgloss.NewStyle().Width(delayW).Align(lipgloss.Right).Render(delayText) + " " +
		lipgloss.NewStyle().Width(stateW).Render(state)

	if m.focus == focusNodes && idx == m.nodeCursor {
		return styles.TableRowSelected.Width(innerWidth).Render(line)
	}
	if name == current {
		return styles.DelayFast.Render(line)
	}
	return styles.TableRow.Render(line)
}

func (m *Model) rebuildGroups(groups []api.Group) {
	old := make(map[string]groupState)
	for _, g := range m.groups {
		old[g.group.Name] = g
	}

	m.groups = make([]groupState, 0, len(groups))
	for _, g := range groups {
		if prev, ok := old[g.Name]; ok {
			m.groups = append(m.groups, groupState{
				group:  g,
				delays: prev.delays,
			})
			continue
		}
		m.groups = append(m.groups, groupState{group: g})
	}
}

func (m Model) selectedGroupState() *groupState {
	visible := m.visibleGroups()
	if len(visible) == 0 {
		return nil
	}
	if m.groupCursor >= len(visible) {
		return &m.groups[visible[len(visible)-1].index]
	}
	return &m.groups[visible[m.groupCursor].index]
}

func (m Model) selectedGroup() *api.Group {
	state := m.selectedGroupState()
	if state == nil {
		return nil
	}
	return &state.group
}

func (m Model) currentNodes() []string {
	visible := m.visibleGroups()
	if len(visible) == 0 {
		return nil
	}
	idx := min(m.groupCursor, len(visible)-1)
	return visible[idx].nodes
}

func (m Model) sortedNodes(gi int) []string {
	nodes := make([]string, len(m.groups[gi].group.All))
	copy(nodes, m.groups[gi].group.All)

	switch m.sortMode {
	case SortByName:
		sort.Slice(nodes, func(i, j int) bool {
			return strings.ToLower(nodes[i]) < strings.ToLower(nodes[j])
		})
	case SortByDelay:
		delays := m.groups[gi].delays[m.testMode]
		sort.Slice(nodes, func(i, j int) bool {
			di := getDelay(nodes[i], delays)
			dj := getDelay(nodes[j], delays)
			if di <= 0 && dj <= 0 {
				return strings.ToLower(nodes[i]) < strings.ToLower(nodes[j])
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

func (m Model) visibleGroups() []visibleGroup {
	filter := strings.ToLower(m.filter)
	result := make([]visibleGroup, 0, len(m.groups))

	for i, g := range m.groups {
		if g.group.Hidden || g.group.Name == "GLOBAL" {
			continue
		}

		nodes := m.sortedNodes(i)
		if filter != "" {
			delayQ := isDelayQuery(filter)
			filtered := make([]string, 0, len(nodes))
			for _, name := range nodes {
				if matchNode(name, getDelay(name, g.delays[m.testMode]), filter) {
					filtered = append(filtered, name)
				}
			}
			if delayQ {
				if len(filtered) == 0 {
					continue
				}
			} else {
				groupText := strings.ToLower(g.group.Name + " " + g.group.Now + " " + g.group.Type)
				if len(filtered) == 0 && !strings.Contains(groupText, filter) {
					continue
				}
			}
			nodes = filtered
		}

		result = append(result, visibleGroup{index: i, nodes: nodes})
	}
	return result
}

func (m *Model) moveNodeVertical(delta int) {
	nodes := m.currentNodes()
	if len(nodes) == 0 {
		return
	}
	m.nodeCursor = clamp(m.nodeCursor+delta, 0, len(nodes)-1)
}

func (m *Model) moveFocusedPage(delta int) {
	if delta == 0 {
		return
	}
	if m.focus == focusGroups {
		step := max(m.groupVisibleRows()/2, 1)
		m.groupCursor += delta * step
		m.nodeCursor = 0
		return
	}

	nodes := m.currentNodes()
	if len(nodes) == 0 {
		return
	}
	step := max(m.nodeVisibleRows()/2, 1)
	m.nodeCursor = clamp(m.nodeCursor+delta*step, 0, len(nodes)-1)
}

func (m *Model) clampSelection() {
	visible := m.visibleGroups()
	m.groupCursor = clamp(m.groupCursor, 0, max(len(visible)-1, 0))
	m.nodeCursor = clamp(m.nodeCursor, 0, max(len(m.currentNodes())-1, 0))
	if len(visible) == 0 {
		m.focus = focusGroups
	}
}

func (m *Model) adjustOffsets() {
	groupRows := m.groupVisibleRows()
	if m.groupCursor < m.groupOffset {
		m.groupOffset = m.groupCursor
	}
	if m.groupCursor >= m.groupOffset+groupRows {
		m.groupOffset = m.groupCursor - groupRows + 1
	}

	nodes := m.currentNodes()
	if len(nodes) == 0 {
		m.nodeOffset = 0
		return
	}

	rowsPerPage := m.nodeVisibleRows()
	if m.nodeCursor < m.nodeOffset {
		m.nodeOffset = m.nodeCursor
	}
	if m.nodeCursor >= m.nodeOffset+rowsPerPage {
		m.nodeOffset = m.nodeCursor - rowsPerPage + 1
	}
	maxOffset := max(len(nodes)-rowsPerPage, 0)
	m.nodeOffset = clamp(m.nodeOffset, 0, maxOffset)
}

func (m Model) proxyBodyHeight() int {
	return max(m.height-len(m.renderHeaderLines()), 1)
}

func (m Model) groupVisibleRows() int {
	return groupRowsForPanelHeight(m.proxyBodyHeight())
}

func (m Model) nodeVisibleRows() int {
	return nodeRowsForPanelHeight(m.proxyBodyHeight())
}

func proxyPanelContentHeight(panelHeight int) int {
	return max(panelHeight-panelChromeHeight, 1)
}

func groupRowsForPanelHeight(panelHeight int) int {
	return max(proxyPanelContentHeight(panelHeight)-1, 1)
}

func nodeTableHeightForPanelHeight(panelHeight int) int {
	return max(proxyPanelContentHeight(panelHeight)-1, 1)
}

func nodeRowsForPanelHeight(panelHeight int) int {
	return nodeRowsForTableHeight(nodeTableHeightForPanelHeight(panelHeight))
}

func nodeRowsForTableHeight(tableHeight int) int {
	return max(tableHeight-nodeTableHeaderHeight-nodeTableBottomSlack, 1)
}

// usableWidth is the page width minus the root content padding, matching the
// width-2 the header and other pages render to.
func (m Model) usableWidth() int {
	return max(m.width-2, 1)
}

func (m Model) singlePanel() bool {
	return m.usableWidth() < twoPanelMinWidth
}

func (m Model) focusHint() string {
	if m.singlePanel() && m.focus == focusNodes {
		return "Esc groups"
	}
	return "focus " + m.focus
}

func (m Model) panelWidths() (int, int) {
	available := max(m.usableWidth()-proxyPanelGap, groupPanelMinWidth+20)
	groupW := available / 3
	groupW = clamp(groupW, groupPanelMinWidth, groupPanelMaxWidth)
	nodeW := max(available-groupW, 24)
	return groupW, nodeW
}

// getDelay returns the measured delay, or 0 for an untested, timed-out,
// skipped, or failed node, so sorting and delay filters treat them alike.
func getDelay(name string, delays map[string]messages.DelayResult) int {
	if r, ok := delays[name]; ok && r.Status == messages.DelayOK {
		return r.Delay
	}
	return 0
}

func (m Model) testing(group, node string) bool {
	_, whole := m.inflight[probeKey(group, m.testMode, "")]
	_, single := m.inflight[probeKey(group, m.testMode, node)]
	return whole || single
}

// renderDelay labels every outcome in text so the column does not rely on
// color alone.
func renderDelay(r messages.DelayResult) string {
	switch r.Status {
	case messages.DelayOK:
		return styles.DelayStyle(r.Delay).Render(styles.FormatDelay(r.Delay))
	case messages.DelayTimeout:
		return styles.DelayBad.Render("timeout")
	case messages.DelaySkipped:
		return styles.DelayNone.Render("skipped")
	default:
		return styles.DelayBad.Render("error")
	}
}

func describeDelay(r messages.DelayResult) string {
	switch r.Status {
	case messages.DelayOK:
		return styles.FormatDelay(r.Delay)
	case messages.DelaySkipped:
		return "skipped (" + r.Reason + ")"
	default:
		return string(r.Status) + " (" + r.Reason + ")"
	}
}

// summarizeGroupTest counts every requested node by outcome so a probe that
// skipped or failed nodes never reads as a clean pass.
func summarizeGroupTest(group string, mode probe.Mode, results map[string]messages.DelayResult) string {
	prefix := strings.ToUpper(mode.String()) + " " + styles.SafeText(group) + ": "
	if len(results) == 0 {
		return prefix + "no nodes to test"
	}
	counts := map[messages.DelayStatus]int{}
	for _, r := range results {
		counts[r.Status]++
	}
	parts := []string{fmt.Sprintf("%d/%d ok", counts[messages.DelayOK], len(results))}
	for _, s := range []messages.DelayStatus{messages.DelayTimeout, messages.DelayError, messages.DelaySkipped} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	return prefix + strings.Join(parts, ", ")
}

// isDelayQuery reports whether a filter is a delay predicate (delay<N, delay>N,
// delay=timeout) rather than a plain substring search.
func isDelayQuery(filter string) bool {
	_, _, ok := parseDelayQuery(filter)
	return ok
}

// matchNode reports whether a node matches the filter — either a delay predicate
// against its measured delay, or a case-insensitive substring of its name.
func matchNode(name string, delay int, filter string) bool {
	if op, threshold, ok := parseDelayQuery(filter); ok {
		switch op {
		case "<":
			return delay > 0 && delay < threshold
		case ">":
			return delay > 0 && delay > threshold
		case "timeout":
			return delay <= 0
		}
		return false
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(filter))
}

func parseDelayQuery(f string) (op string, threshold int, ok bool) {
	f = strings.TrimSpace(strings.ToLower(f))
	if !strings.HasPrefix(f, "delay") {
		return "", 0, false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(f, "delay"))
	switch {
	case strings.HasPrefix(rest, "<"):
		if n, err := strconv.Atoi(strings.TrimSpace(rest[1:])); err == nil {
			return "<", n, true
		}
	case strings.HasPrefix(rest, ">"):
		if n, err := strconv.Atoi(strings.TrimSpace(rest[1:])); err == nil {
			return ">", n, true
		}
	case strings.HasPrefix(rest, "=timeout"), strings.HasPrefix(rest, "=0"):
		return "timeout", 0, true
	}
	return "", 0, false
}

func truncateDisplayWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	if width <= 2 {
		return strings.Repeat(".", width)
	}
	return runewidth.Truncate(s, width, "..")
}

func clamp(v, low, high int) int {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
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

func (m Model) startGroupTest(group api.Group) (Model, tea.Cmd) {
	id := m.track(probeKey(group.Name, m.testMode, ""))
	return m, m.testGroup(group, id)
}

func (m Model) startNodeTest(group api.Group, node string) (Model, tea.Cmd) {
	id := m.track(probeKey(group.Name, m.testMode, node))
	return m, m.testNode(group, node, id)
}

// groupTestURL prefers the group's own testUrl and falls back to the configured
// default; an empty result lets the client use its built-in URL.
func (m Model) groupTestURL(group api.Group) string {
	if group.TestURL != "" {
		return group.TestURL
	}
	return m.testURL
}

// testGroup tests every node in a group using the active test mode and returns
// an outcome for each requested node.
func (m Model) testGroup(group api.Group, id uint64) tea.Cmd {
	mode := m.testMode
	if mode.NeedsEndpoints() {
		return m.testGroupProbe(group, id)
	}
	client, name, testURL := m.client, group.Name, m.groupTestURL(group)
	nodes := append([]string(nil), group.All...)
	return func() tea.Msg {
		delays, err := client.TestGroupDelay(name, testURL, 0)
		return messages.GroupDelayMsg{Group: name, Mode: mode, RequestID: id, Results: groupHTTPResults(nodes, delays, err), Err: err}
	}
}

// groupHTTPResults maps mihomo's group delay reply onto every requested node.
// mihomo lists only nodes that answered, so a missing node timed out.
func groupHTTPResults(nodes []string, delays api.GroupDelayResult, err error) map[string]messages.DelayResult {
	results := make(map[string]messages.DelayResult, len(nodes))
	for _, n := range nodes {
		d, ok := delays[n]
		switch {
		case err != nil:
			results[n] = delayFailure(err)
		case ok && d > 0:
			results[n] = messages.DelayResult{Delay: d, Status: messages.DelayOK}
		default:
			results[n] = messages.DelayResult{Status: messages.DelayTimeout, Reason: "no reply within the test timeout"}
		}
	}
	return results
}

// testNode tests a single node using the active test mode.
func (m Model) testNode(group api.Group, name string, id uint64) tea.Cmd {
	mode, ep, client := m.testMode, m.endpoints, m.client
	groupName, testURL := group.Name, m.groupTestURL(group)
	if mode.NeedsEndpoints() {
		return func() tea.Msg {
			return messages.ProxyDelayMsg{Group: groupName, Name: name, Mode: mode, RequestID: id, Result: probeNode(ep, mode, name)}
		}
	}
	return func() tea.Msg {
		msg := messages.ProxyDelayMsg{Group: groupName, Name: name, Mode: mode, RequestID: id}
		result, err := client.TestProxyDelay(name, testURL, 0)
		switch {
		case err != nil:
			msg.Result, msg.Err = delayFailure(err), err
		case result == nil || result.Delay <= 0:
			msg.Result = messages.DelayResult{Status: messages.DelayTimeout, Reason: "no reply within the test timeout"}
		default:
			msg.Result = messages.DelayResult{Delay: result.Delay, Status: messages.DelayOK}
		}
		return msg
	}
}

// testGroupProbe runs TCP/ICMP pings across all leaf nodes of a group with
// bounded concurrency (large groups can hold 140+ nodes). Nodes without a known
// server, such as sub-groups, are reported as skipped rather than dropped.
func (m Model) testGroupProbe(group api.Group, id uint64) tea.Cmd {
	mode, ep := m.testMode, m.endpoints
	nodes := append([]string(nil), group.All...)
	name := group.Name
	return func() tea.Msg {
		results := make(map[string]messages.DelayResult, len(nodes))
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, probeConcurrency)
		for _, n := range nodes {
			wg.Add(1)
			sem <- struct{}{}
			go func(n string) {
				defer wg.Done()
				defer func() { <-sem }()
				r := probeNode(ep, mode, n)
				mu.Lock()
				results[n] = r
				mu.Unlock()
			}(n)
		}
		wg.Wait()
		return messages.GroupDelayMsg{Group: name, Mode: mode, RequestID: id, Results: results}
	}
}

func probeNode(ep *probe.Endpoints, mode probe.Mode, name string) messages.DelayResult {
	if ep == nil {
		return messages.DelayResult{Status: messages.DelayError, Reason: "node endpoints not loaded"}
	}
	e, ok := ep.Lookup(name)
	if !ok {
		return messages.DelayResult{Status: messages.DelaySkipped, Reason: "no server address (sub-group or unknown node)"}
	}
	var d int
	var err error
	switch mode {
	case probe.ModeTCP:
		d, err = probe.TCPPing(e.Server, e.Port, probeTimeout)
	case probe.ModeICMP:
		d, err = probe.ICMPPing(e.Server, probeTimeout)
	default:
		err = fmt.Errorf("%s is not a probe mode", mode)
	}
	if err != nil {
		return delayFailure(err)
	}
	return messages.DelayResult{Delay: d, Status: messages.DelayOK}
}

// delayFailure separates a timeout (504 from mihomo, or a network deadline)
// from any other failure.
func delayFailure(err error) messages.DelayResult {
	reason := styles.SafeText(err.Error())
	var se *api.StatusError
	var ne net.Error
	if (errors.As(err, &se) && se.Code == http.StatusGatewayTimeout) ||
		(errors.As(err, &ne) && ne.Timeout()) || errors.Is(err, probe.ErrTimeout) {
		return messages.DelayResult{Status: messages.DelayTimeout, Reason: reason}
	}
	return messages.DelayResult{Status: messages.DelayError, Reason: reason}
}

// unfixGroup clears a URLTest/Fallback group's fixed node, then refetches.
func (m Model) unfixGroup(name string) tea.Cmd {
	return func() tea.Msg {
		if err := m.client.UnfixProxy(name); err != nil {
			return messages.ProxySelectedMsg{Group: name, Err: err}
		}
		groups, gerr := m.client.GetGroups()
		return messages.GroupsMsg{Groups: groups, Err: gerr}
	}
}
