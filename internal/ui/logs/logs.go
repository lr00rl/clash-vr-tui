package logs

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/styles"
)

const maxEntries = 2000

// levelOrder lists the minimum-level filters cycled with 'l' ("" = all).
var levelOrder = []string{"", "info", "warning", "error"}

type Model struct {
	entries   []api.LogEntry
	level     string // minimum level filter ("" = all)
	filter    string
	filtering bool
	paused    bool
	follow    bool // auto-scroll to newest
	cursor    int
	offset    int
	width     int
	height    int
}

func New() Model {
	return Model{follow: true}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

// Filtering reports whether the user is typing in the filter input.
func (m Model) Filtering() bool { return m.filtering }

// Add appends a log entry to the ring buffer (called from the root model as the
// /logs stream pushes lines). Dropped while paused.
func (m Model) Add(e api.LogEntry) Model {
	if m.paused {
		return m
	}
	m.entries = append(m.entries, e)
	if len(m.entries) > maxEntries {
		m.entries = m.entries[len(m.entries)-maxEntries:]
	}
	if m.follow {
		m.cursor = len(m.visibleEntries()) - 1
		m.adjustViewport()
	}
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.filtering {
			return m.handleFilterKey(msg), nil
		}
		return m.handleKey(msg), nil
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) Model {
	visible := m.visibleEntries()
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, max(len(visible)-1, 0))
		m.follow = m.cursor >= len(visible)-1
		m.adjustViewport()
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
		m.follow = false
		m.adjustViewport()
	case "g":
		m.cursor = 0
		m.offset = 0
		m.follow = false
	case "G":
		m.cursor = max(len(visible)-1, 0)
		m.follow = true
		m.adjustViewport()
	case " ":
		m.paused = !m.paused
	case "c":
		m.entries = nil
		m.cursor = 0
		m.offset = 0
		m.follow = true
	case "l":
		m.level = nextLevel(m.level)
		m.cursor = max(len(m.visibleEntries())-1, 0)
		m.follow = true
		m.adjustViewport()
	case "/":
		m.filtering = true
		m.filter = ""
	}
	return m
}

func (m Model) handleFilterKey(msg tea.KeyMsg) Model {
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
	m.cursor = max(len(m.visibleEntries())-1, 0)
	m.follow = true
	m.adjustViewport()
	return m
}

func (m *Model) adjustViewport() {
	maxRows := m.rows()
	visible := m.visibleEntries()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+maxRows {
		m.offset = m.cursor - maxRows + 1
	}
	m.offset = clamp(m.offset, 0, max(len(visible)-maxRows, 0))
}

func (m Model) rows() int {
	return max(m.height-4, 1)
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	var b strings.Builder
	visible := m.visibleEntries()
	maxRows := m.rows()
	offset := clamp(m.offset, 0, max(len(visible)-maxRows, 0))
	end := min(offset+maxRows, len(visible))

	state := "live"
	if m.paused {
		state = styles.DelaySlow.Render("paused")
	}
	lvl := m.level
	if lvl == "" {
		lvl = "all"
	}
	pos := ""
	if len(visible) > 0 {
		pos = fmt.Sprintf("  %d-%d/%d", offset+1, end, len(visible))
	}
	header := fmt.Sprintf("Logs [%s]  level: %s  %s%s", state, lvl, "space pause · l level · c clear · / filter", pos)
	b.WriteString(header + "\n")
	if m.filtering {
		b.WriteString(styles.FilterPrompt.Render("Filter: ") + m.filter + "█\n")
	} else if m.filter != "" {
		b.WriteString(styles.FilterPrompt.Render("Filter: ") + m.filter + "\n")
	}
	b.WriteString(strings.Repeat("─", max(m.width-2, 0)) + "\n")

	for i := offset; i < end; i++ {
		e := visible[i]
		tag := levelTag(e.Type)
		payload := truncate(e.Payload, max(m.width-12, 8))
		line := tag + " " + payload
		if i == m.cursor {
			b.WriteString(styles.TableRowSelected.Render("❯ " + line))
		} else {
			b.WriteString("  " + line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) visibleEntries() []api.LogEntry {
	if m.level == "" && m.filter == "" {
		return m.entries
	}
	f := strings.ToLower(m.filter)
	minRank := levelRank(m.level)
	result := make([]api.LogEntry, 0, len(m.entries))
	for _, e := range m.entries {
		if m.level != "" && levelRank(e.Type) < minRank {
			continue
		}
		if f != "" && !strings.Contains(strings.ToLower(e.Payload), f) {
			continue
		}
		result = append(result, e)
	}
	return result
}

func levelTag(t string) string {
	label := strings.ToUpper(t)
	if len(label) > 4 {
		label = label[:4]
	}
	label = fmt.Sprintf("%-4s", label)
	switch strings.ToLower(t) {
	case "error", "err":
		return styles.DelayBad.Render(label)
	case "warning", "warn":
		return styles.DelaySlow.Render(label)
	case "info":
		return styles.DelayFast.Render(label)
	default:
		return styles.DelayNone.Render(label)
	}
}

func levelRank(level string) int {
	switch strings.ToLower(level) {
	case "debug":
		return 0
	case "info":
		return 1
	case "warning", "warn":
		return 2
	case "error", "err":
		return 3
	default:
		return 1
	}
}

func nextLevel(cur string) string {
	for i, l := range levelOrder {
		if l == cur {
			return levelOrder[(i+1)%len(levelOrder)]
		}
	}
	return ""
}

func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= w {
		return s
	}
	return runewidth.Truncate(s, w, "…")
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
