package proxies

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	groupPanelMinWidth = 34
	groupPanelMaxWidth = 48
	panelChromeWidth   = 4
	panelChromeHeight  = 2
	cardOuterMinWidth  = 24
	cardOuterMaxWidth  = 30
	cardChromeWidth    = 4
	cardOuterHeight    = 4
	cardRowGap         = 1
	cardColGap         = 2
	groupRowMargin     = 2
)

type groupState struct {
	group  api.Group
	delays map[string]int
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
}

func New(client *api.Client) Model {
	return Model{
		client:    client,
		sortMode:  SortDefault,
		testMode:  probe.ModeHTTP,
		endpoints: probe.NewEndpoints(),
		focus:     focusGroups,
	}
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
	case "left", "h":
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
			return m, m.testGroup(*group)
		}
		nodes := m.currentNodes()
		if m.nodeCursor < len(nodes) {
			return m, m.testNode(nodes[m.nodeCursor])
		}
	case "D":
		nodes := m.currentNodes()
		if m.nodeCursor < len(nodes) {
			return m, m.testNode(nodes[m.nodeCursor])
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
	bodyHeight := max(m.height-len(headerLines), 1)
	groupW, nodeW := m.panelWidths()

	left := m.renderGroupsPanel(groupW, bodyHeight)
	right := m.renderNodesPanel(nodeW, bodyHeight)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", proxyPanelGap), right)
	return strings.Join(append(headerLines, body), "\n")
}

func (m Model) renderHeaderLines() []string {
	modes := []string{"rule", "global", "direct"}
	tabs := make([]string, 0, len(modes))
	for _, mode := range modes {
		label := fmt.Sprintf("[%s%s]", strings.ToUpper(mode[:1]), mode[1:])
		if mode == m.mode {
			tabs = append(tabs, styles.ModeActive.Render(label))
		} else {
			tabs = append(tabs, styles.ModeInactive.Render(label))
		}
	}

	testInfo := fmt.Sprintf("[Test: %s]", m.testMode)
	if m.testMode.NeedsEndpoints() {
		if m.epErr != nil {
			testInfo += styles.DelayBad.Render(" config?")
		} else if m.endpoints != nil {
			testInfo += styles.DelayNone.Render(fmt.Sprintf(" %d nodes", m.endpoints.Len()))
		}
	}

	lines := []string{
		"Mode: " + strings.Join(tabs, "  ") + fmt.Sprintf("    [Sort: %s]  %s  [Focus: %s]", m.sortMode.String(), testInfo, m.focus),
		strings.Repeat("─", max(m.width-2, 0)),
	}
	if m.filtering {
		lines = append(lines, styles.FilterPrompt.Render("Filter: ")+m.filter+"█")
	} else if m.filter != "" {
		lines = append(lines, styles.FilterPrompt.Render("Filter: ")+m.filter)
	}
	return lines
}

func (m Model) renderGroupsPanel(width, height int) string {
	visible := m.visibleGroups()
	bodyH := max(height-panelChromeHeight, 1)
	lines := make([]string, 0, bodyH+1)

	title := fmt.Sprintf("Groups (%d)", len(visible))
	if m.focus == focusGroups {
		title = styles.ProxyGroupHeaderSelected.Render(title)
	} else {
		title = styles.ProxyGroupHeader.Render(title)
	}
	lines = append(lines, title)

	start := min(m.groupOffset, max(len(visible)-bodyH, 0))
	end := min(start+bodyH, len(visible))

	for i := start; i < end; i++ {
		g := m.groups[visible[i].index].group
		prefix := "  "
		if i == m.groupCursor {
			prefix = "• "
		}

		now := truncateDisplayWidth(g.Now, max(width-groupRowMargin-18, 8))
		line := prefix + truncateDisplayWidth(g.Name, max(width-20, 12))
		meta := fmt.Sprintf("  %s  %d", g.Type, len(g.All))
		raw := truncateDisplayWidth(line+meta, width-panelChromeWidth)
		if i == m.groupCursor {
			lines = append(lines, styles.TableRowSelected.Width(max(width-panelChromeWidth, 1)).Render(raw))
		} else {
			lines = append(lines, styles.TableRow.Width(max(width-panelChromeWidth, 1)).Render(raw))
		}
		if i == m.groupCursor {
			lines = append(lines, styles.DelayNone.Render("  now: "+now))
		}
	}

	for len(lines) < bodyH+1 {
		lines = append(lines, "")
	}

	box := styles.ProxyGroupBox
	if m.focus == focusGroups {
		box = styles.ProxyGroupBoxSelected
	}
	return box.Width(max(width-panelChromeWidth, 1)).Height(bodyH).Render(strings.Join(lines, "\n"))
}

func (m Model) renderNodesPanel(width, height int) string {
	bodyH := max(height-panelChromeHeight, 1)
	group := m.selectedGroup()
	if group == nil {
		box := styles.ProxyGroupBox
		if m.focus == focusNodes {
			box = styles.ProxyGroupBoxSelected
		}
		return box.Width(max(width-panelChromeWidth, 1)).Height(bodyH).Render("Nodes\n\nNo proxy group")
	}

	nodes := m.currentNodes()
	titleText := fmt.Sprintf("%s  [%s]  now: %s", group.Name, group.Type, truncateDisplayWidth(group.Now, max(width-28, 8)))
	title := styles.ProxyGroupHeader.Render(titleText)
	if m.focus == focusNodes {
		title = styles.ProxyGroupHeaderSelected.Render(titleText)
	}

	gridBodyH := max(bodyH-1, 1)
	gridLines := m.renderNodeGrid(width-panelChromeWidth, gridBodyH)
	content := title
	if len(gridLines) > 0 {
		content += "\n" + strings.Join(gridLines, "\n")
	}

	box := styles.ProxyGroupBox
	if m.focus == focusNodes {
		box = styles.ProxyGroupBoxSelected
	}

	if len(nodes) == 0 {
		content = title + "\n\nNo nodes match the current filter."
	}
	return box.Width(max(width-panelChromeWidth, 1)).Height(bodyH).Render(content)
}

func (m Model) renderNodeGrid(innerWidth, bodyHeight int) []string {
	nodes := m.currentNodes()
	if len(nodes) == 0 {
		return nil
	}

	cols, cardOuterW := gridLayout(innerWidth)
	rowsPerPage := max((bodyHeight+cardRowGap)/(cardOuterHeight+cardRowGap), 1)
	startRow := min(m.nodeOffset, max((len(nodes)+cols-1)/cols-rowsPerPage, 0))
	startIdx := startRow * cols

	lines := make([]string, 0, bodyHeight)
	for row := 0; row < rowsPerPage; row++ {
		rowStart := startIdx + row*cols
		if rowStart >= len(nodes) {
			break
		}

		cards := make([]string, 0, cols)
		for col := 0; col < cols; col++ {
			idx := rowStart + col
			if idx >= len(nodes) {
				break
			}
			cards = append(cards, m.renderProxyCard(nodes[idx], idx, cardOuterW))
		}

		lines = append(lines, splitLines(joinCardRow(cards))...)
		if row < rowsPerPage-1 {
			lines = append(lines, "")
		}
	}

	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}
	return lines[:bodyHeight]
}

func (m Model) renderProxyCard(name string, idx, outerWidth int) string {
	group := m.selectedGroup()
	delay := 0
	current := ""
	fixed := ""
	if group != nil {
		delay = getDelay(name, m.selectedGroupState().delays)
		current = group.Now
		fixed = group.Fixed
	}

	innerWidth := max(outerWidth-cardChromeWidth, 8)
	nameLine := truncateDisplayWidth(name, innerWidth)
	status := "standby"
	if name == current {
		status = "active"
	}
	if name == fixed && fixed != "" {
		status = "pinned"
	}
	delayLine := truncateDisplayWidth(status, max(innerWidth-10, 4)) + padLeft(styles.DelayStyle(delay).Render(styles.FormatDelay(delay)), 10)

	cardStyle := styles.ProxyCard
	if name == current {
		cardStyle = styles.ProxyCardCurrent
	}
	if m.focus == focusNodes && idx == m.nodeCursor {
		cardStyle = styles.ProxyCardSelected
	}

	return cardStyle.Width(innerWidth).Height(2).Render(nameLine + "\n" + delayLine)
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
		m.groups = append(m.groups, groupState{
			group:  g,
			delays: make(map[string]int),
		})
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
		delays := m.groups[gi].delays
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
		if g.group.Hidden {
			continue
		}

		nodes := m.sortedNodes(i)
		if filter != "" {
			delayQ := isDelayQuery(filter)
			filtered := make([]string, 0, len(nodes))
			for _, name := range nodes {
				if matchNode(name, getDelay(name, g.delays), filter) {
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
	cols, _ := gridLayout(m.nodeGridWidth())
	m.nodeCursor = clamp(m.nodeCursor+delta*cols, 0, len(nodes)-1)
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
	groupBodyHeight := max(m.height-len(m.renderHeaderLines())-panelChromeHeight, 1)
	if m.groupCursor < m.groupOffset {
		m.groupOffset = m.groupCursor
	}
	if m.groupCursor >= m.groupOffset+max(groupBodyHeight/2, 1) {
		m.groupOffset = m.groupCursor - max(groupBodyHeight/2, 1) + 1
	}

	nodes := m.currentNodes()
	if len(nodes) == 0 {
		m.nodeOffset = 0
		return
	}

	cols, _ := gridLayout(m.nodeGridWidth())
	rowsPerPage := max((max(m.height-len(m.renderHeaderLines())-panelChromeHeight-1, 1)+cardRowGap)/(cardOuterHeight+cardRowGap), 1)
	cursorRow := m.nodeCursor / cols
	if cursorRow < m.nodeOffset {
		m.nodeOffset = cursorRow
	}
	if cursorRow >= m.nodeOffset+rowsPerPage {
		m.nodeOffset = cursorRow - rowsPerPage + 1
	}
	maxRowOffset := max((len(nodes)+cols-1)/cols-rowsPerPage, 0)
	m.nodeOffset = clamp(m.nodeOffset, 0, maxRowOffset)
}

func (m Model) panelWidths() (int, int) {
	available := max(m.width-proxyPanelGap, groupPanelMinWidth+20)
	groupW := available / 3
	groupW = clamp(groupW, groupPanelMinWidth, groupPanelMaxWidth)
	nodeW := max(available-groupW, 24)
	return groupW, nodeW
}

func (m Model) nodeGridWidth() int {
	_, nodeW := m.panelWidths()
	return max(nodeW-panelChromeWidth, 1)
}

func gridLayout(innerWidth int) (int, int) {
	available := max(innerWidth, cardOuterMinWidth)
	maxCols := max((available+cardColGap)/(cardOuterMinWidth+cardColGap), 1)
	for cols := maxCols; cols >= 1; cols-- {
		cardW := (available - (cols-1)*cardColGap) / cols
		if cardW < cardOuterMinWidth {
			continue
		}
		cardW = min(cardW, cardOuterMaxWidth)
		return cols, cardW
	}
	return 1, min(available, cardOuterMaxWidth)
}

func joinCardRow(cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	if len(cards) == 1 {
		return cards[0]
	}
	parts := make([]string, 0, len(cards)*2-1)
	gap := strings.Repeat(" ", cardColGap)
	for i, card := range cards {
		if i > 0 {
			parts = append(parts, gap)
		}
		parts = append(parts, card)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func splitLines(s string) []string {
	if s == "" {
		return []string{""}
	}
	return strings.Split(s, "\n")
}

func getDelay(name string, delays map[string]int) int {
	if delays == nil {
		return 0
	}
	return delays[name]
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

func padLeft(s string, width int) string {
	padding := max(width-lipgloss.Width(s), 0)
	return strings.Repeat(" ", padding) + s
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

// testGroup tests every node in a group using the active test mode.
func (m Model) testGroup(group api.Group) tea.Cmd {
	if m.testMode.NeedsEndpoints() {
		return m.testGroupProbe(group)
	}
	name, testURL := group.Name, group.TestURL
	return func() tea.Msg {
		result, err := m.client.TestGroupDelay(name, testURL, 0)
		return messages.GroupDelayMsg{Group: name, Result: result, Err: err}
	}
}

// testNode tests a single node using the active test mode.
func (m Model) testNode(name string) tea.Cmd {
	mode, ep := m.testMode, m.endpoints
	testURL := ""
	if g := m.selectedGroup(); g != nil {
		testURL = g.TestURL
	}
	if mode.NeedsEndpoints() {
		return func() tea.Msg {
			d, err := probePing(ep, mode, name)
			return messages.ProxyDelayMsg{Name: name, Delay: d, Err: err}
		}
	}
	return func() tea.Msg {
		result, err := m.client.TestProxyDelay(name, testURL, 0)
		delay := 0
		if result != nil {
			delay = result.Delay
		}
		return messages.ProxyDelayMsg{Name: name, Delay: delay, Err: err}
	}
}

// testGroupProbe runs TCP/ICMP pings across all leaf nodes of a group with
// bounded concurrency (large groups can hold 140+ nodes).
func (m Model) testGroupProbe(group api.Group) tea.Cmd {
	mode, ep := m.testMode, m.endpoints
	nodes := append([]string(nil), group.All...)
	name := group.Name
	return func() tea.Msg {
		result := make(api.GroupDelayResult, len(nodes))
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, probeConcurrency)
		for _, n := range nodes {
			if _, ok := ep.Lookup(n); !ok {
				continue // sub-group or unknown server; skip
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(n string) {
				defer wg.Done()
				defer func() { <-sem }()
				d, _ := probePing(ep, mode, n)
				mu.Lock()
				result[n] = d
				mu.Unlock()
			}(n)
		}
		wg.Wait()
		return messages.GroupDelayMsg{Group: name, Result: result}
	}
}

func probePing(ep *probe.Endpoints, mode probe.Mode, name string) (int, error) {
	if ep == nil {
		return 0, fmt.Errorf("no endpoints loaded")
	}
	e, ok := ep.Lookup(name)
	if !ok {
		return 0, fmt.Errorf("unknown server for %s", name)
	}
	switch mode {
	case probe.ModeTCP:
		return probe.TCPPing(e.Server, e.Port, probeTimeout)
	case probe.ModeICMP:
		return probe.ICMPPing(e.Server, probeTimeout)
	}
	return 0, fmt.Errorf("not a probe mode")
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
