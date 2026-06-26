package connections

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/messages"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type SortField int

const (
	SortByTime SortField = iota
	SortByDLSpeed
	SortByULSpeed
)

func (s SortField) String() string {
	switch s {
	case SortByDLSpeed:
		return "DL"
	case SortByULSpeed:
		return "UL"
	default:
		return "Time"
	}
}

type connEntry struct {
	conn    api.Connection
	dlSpeed int64
	ulSpeed int64
}

type Model struct {
	client    *api.Client
	conns     []connEntry
	prevSnap  map[string]api.Connection
	totalDL   int64
	totalUL   int64
	cursor    int
	offset    int    // viewport scroll offset
	selID     string // stable selection keyed by connection ID
	filter    string
	filtering bool
	sortField SortField
	width     int
	height    int
	err       error
}

func New(client *api.Client) Model {
	return Model{
		client:   client,
		prevSnap: make(map[string]api.Connection),
	}
}

func (m Model) Init() tea.Cmd {
	return nil // data comes from WebSocket via root model
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// Filtering returns whether user is typing in filter input.
func (m Model) Filtering() bool {
	return m.filtering
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case messages.ConnectionsMsg:
		m.updateConnections(msg.Data)
	case messages.ConnClosedMsg:
		if msg.Err != nil {
			m.err = msg.Err
		}
	case messages.AllConnsClosedMsg:
		if msg.Err != nil {
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
	visible := m.visibleConns()
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(visible)-1, 0))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = max(len(visible)-1, 0)
	case "/":
		m.filtering = true
		m.filter = ""
	case "s":
		m.sortField = (m.sortField + 1) % 3
	case "x":
		if m.cursor < len(visible) {
			id := visible[m.cursor].conn.ID
			return m, m.closeConn(id)
		}
	case "enter":
		return m, nil
	}
	// Anchor the stable selection to whatever the cursor now points at, and keep
	// the viewport in sync.
	if m.cursor < len(visible) {
		m.selID = visible[m.cursor].conn.ID
	}
	m.adjustViewport()
	return m, nil
}

// adjustViewport keeps the cursor row within the visible window.
func (m *Model) adjustViewport() {
	maxRows := m.rows()
	visible := m.visibleConns()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+maxRows {
		m.offset = m.cursor - maxRows + 1
	}
	maxOffset := max(len(visible)-maxRows, 0)
	m.offset = clamp(m.offset, 0, maxOffset)
}

// rows returns how many connection rows fit in the current viewport.
func (m Model) rows() int {
	maxRows := m.height - 5
	if maxRows < 1 {
		maxRows = 1
	}
	return maxRows
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
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
	// Filter changed: re-anchor to the top of the new result set.
	m.cursor = 0
	m.offset = 0
	if visible := m.visibleConns(); len(visible) > 0 {
		m.selID = visible[0].conn.ID
	} else {
		m.selID = ""
	}
	return m, nil
}

func (m *Model) updateConnections(snap api.ConnectionsSnapshot) {
	m.totalDL = snap.DownloadTotal
	m.totalUL = snap.UploadTotal

	entries := make([]connEntry, len(snap.Connections))
	for i, c := range snap.Connections {
		var dl, ul int64
		if prev, ok := m.prevSnap[c.ID]; ok {
			dl = c.Download - prev.Download
			ul = c.Upload - prev.Upload
		}
		entries[i] = connEntry{conn: c, dlSpeed: dl, ulSpeed: ul}
	}

	m.prevSnap = make(map[string]api.Connection, len(snap.Connections))
	for _, c := range snap.Connections {
		m.prevSnap[c.ID] = c
	}

	m.sortEntries(entries)
	m.conns = entries

	// Re-anchor the cursor to the previously selected connection by ID so it does
	// not jump around as the list re-sorts each tick.
	visible := m.visibleConns()
	if m.selID != "" {
		found := false
		for i, e := range visible {
			if e.conn.ID == m.selID {
				m.cursor = i
				found = true
				break
			}
		}
		if !found {
			m.cursor = clamp(m.cursor, 0, max(len(visible)-1, 0))
			if m.cursor < len(visible) {
				m.selID = visible[m.cursor].conn.ID
			} else {
				m.selID = ""
			}
		}
	} else if len(visible) > 0 {
		m.cursor = clamp(m.cursor, 0, len(visible)-1)
		m.selID = visible[m.cursor].conn.ID
	}
	m.adjustViewport()
}

func (m Model) sortEntries(entries []connEntry) {
	switch m.sortField {
	case SortByDLSpeed:
		sort.Slice(entries, func(i, j int) bool { return entries[i].dlSpeed > entries[j].dlSpeed })
	case SortByULSpeed:
		sort.Slice(entries, func(i, j int) bool { return entries[i].ulSpeed > entries[j].ulSpeed })
	default:
		sort.Slice(entries, func(i, j int) bool { return entries[i].conn.Start > entries[j].conn.Start })
	}
}

// columnWidths computes adaptive column widths based on available width.
// Returns: hostW, dlW, ulW, chainW, ruleW
func (m Model) columnWidths() (int, int, int, int, int) {
	avail := m.width - 4 // 2 for prefix "❯ ", 2 padding

	// Fixed minimum widths for speed columns
	dlW := 8
	ulW := 8

	// Remaining space split between host, chain, rule
	remaining := avail - dlW - ulW - 4 // 4 for column gaps

	// Proportional split: host 50%, chain 25%, rule 25%
	hostW := remaining * 50 / 100
	chainW := remaining * 25 / 100
	ruleW := remaining - hostW - chainW

	// Clamp
	if hostW < 16 {
		hostW = 16
	}
	if hostW > 60 {
		hostW = 60
	}
	if chainW < 8 {
		chainW = 8
	}
	if chainW > 30 {
		chainW = 30
	}
	if ruleW < 6 {
		ruleW = 6
	}
	if ruleW > 20 {
		ruleW = 20
	}

	return hostW, dlW, ulW, chainW, ruleW
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}

	var b strings.Builder
	hostW, dlW, ulW, chainW, ruleW := m.columnWidths()

	// Header
	visible := m.visibleConns()
	maxRows := m.rows()
	offset := clamp(m.offset, 0, max(len(visible)-maxRows, 0))
	end := min(offset+maxRows, len(visible))
	pos := ""
	if len(visible) > 0 {
		pos = fmt.Sprintf("  %d-%d/%d", offset+1, end, len(visible))
	}
	count := len(m.conns)
	if m.filter != "" {
		count = len(visible)
	}
	header := fmt.Sprintf("Connections (%d active)    Total ▼%s ▲%s    [Sort: %s]%s",
		count,
		formatBytes(m.totalDL),
		formatBytes(m.totalUL),
		m.sortField.String(),
		pos,
	)
	b.WriteString(header + "\n")

	// Filter
	if m.filtering {
		b.WriteString(styles.FilterPrompt.Render("Filter: ") + m.filter + "█\n")
	} else if m.filter != "" {
		b.WriteString(styles.FilterPrompt.Render("Filter: ") + m.filter + "\n")
	}

	// Column headers
	b.WriteString(strings.Repeat("─", m.width-2) + "\n")
	colFmt := fmt.Sprintf("%%-%ds %%%ds %%%ds  %%-%ds %%-%ds", hostW, dlW, ulW, chainW, ruleW)
	colHeader := fmt.Sprintf(colFmt, "Host", "DL", "UL", "Chain", "Rule")
	b.WriteString(styles.TableHeader.Render(colHeader) + "\n")
	b.WriteString(strings.Repeat("─", m.width-2) + "\n")

	// Rows (windowed by viewport offset)
	for i := offset; i < end; i++ {
		e := visible[i]

		host := e.conn.Metadata.Host
		if host == "" {
			host = e.conn.Metadata.DstIP
		}
		if e.conn.Metadata.DstPort != "" {
			host += ":" + e.conn.Metadata.DstPort
		}
		host = truncate(host, hostW)

		chain := ""
		if len(e.conn.Chains) > 0 {
			chain = e.conn.Chains[0]
		}
		chain = truncate(chain, chainW)

		rule := e.conn.Rule
		rule = truncate(rule, ruleW)

		// Build cells with display-width-aware padding so CJK hosts/chains stay
		// aligned (fmt's %-Ns counts runes, not terminal cells).
		line := padRight(host, hostW) + " " +
			padLeft(formatSpeed(e.dlSpeed), dlW) + " " +
			padLeft(formatSpeed(e.ulSpeed), ulW) + "  " +
			padRight(chain, chainW) + " " +
			padRight(rule, ruleW)

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

// SelectedConn returns a copy of the currently selected connection for detail view.
func (m Model) SelectedConn() *api.Connection {
	visible := m.visibleConns()
	if m.cursor < len(visible) {
		conn := visible[m.cursor].conn
		return &conn
	}
	return nil
}

// FormatConnDetail returns a detail string for a connection.
func FormatConnDetail(c *api.Connection) string {
	return fmt.Sprintf(
		"Host:        %s\n"+
			"Source:      %s:%s\n"+
			"Destination: %s:%s\n"+
			"Process:     %s\n"+
			"Path:        %s\n"+
			"Type:        %s\n"+
			"Network:     %s\n"+
			"Chains:      %s\n"+
			"Rule:        %s\n"+
			"Payload:     %s\n"+
			"Upload:      %s\n"+
			"Download:    %s\n"+
			"Start:       %s",
		c.Metadata.Host,
		c.Metadata.SrcIP, c.Metadata.SrcPort,
		c.Metadata.DstIP, c.Metadata.DstPort,
		c.Metadata.Process,
		c.Metadata.ProcessPath,
		c.Metadata.Type,
		c.Metadata.Network,
		strings.Join(c.Chains, " → "),
		c.Rule,
		c.RulePayload,
		formatBytes(c.Upload),
		formatBytes(c.Download),
		c.Start,
	)
}

func (m Model) visibleConns() []connEntry {
	if m.filter == "" {
		return m.conns
	}
	f := strings.ToLower(m.filter)
	var result []connEntry
	for _, e := range m.conns {
		host := strings.ToLower(e.conn.Metadata.Host)
		proc := strings.ToLower(e.conn.Metadata.Process)
		dst := strings.ToLower(e.conn.Metadata.DstIP)
		if strings.Contains(host, f) || strings.Contains(proc, f) || strings.Contains(dst, f) {
			result = append(result, e)
		}
	}
	return result
}

func (m Model) closeConn(id string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.CloseConnection(id)
		return messages.ConnClosedMsg{ID: id, Err: err}
	}
}

// padRight pads s on the right to width display columns (truncating if longer).
func padRight(s string, width int) string {
	s = truncate(s, width)
	pad := width - runewidth.StringWidth(s)
	if pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

// padLeft pads s on the left to width display columns (truncating if longer).
func padLeft(s string, width int) string {
	s = truncate(s, width)
	pad := width - runewidth.StringWidth(s)
	if pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// truncate shortens s to fit maxLen display columns, respecting multi-byte and
// wide (CJK) runes so it never cuts mid-character.
func truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxLen {
		return s
	}
	if maxLen <= 2 {
		return runewidth.Truncate(s, maxLen, "")
	}
	return runewidth.Truncate(s, maxLen, "..")
}

func formatSpeed(bytesPerSec int64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1fM", float64(bytesPerSec)/(1024*1024))
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.1fK", float64(bytesPerSec)/1024)
	case bytesPerSec > 0:
		return fmt.Sprintf("%dB", bytesPerSec)
	default:
		return "0"
	}
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1fG", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.1fM", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1fK", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}
