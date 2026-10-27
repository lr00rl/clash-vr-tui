package overlay

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/styles"
)

// Type identifies the overlay kind.
type Type int

const (
	TypeNone Type = iota
	TypeHelp
	TypeDetail
	TypeConfirm
)

type Model struct {
	Active  Type
	Title   string
	Content string
	width   int
	height  int
	offset  int

	// For confirm overlays
	OnConfirm tea.Cmd
}

func New() Model {
	return Model{Active: TypeNone}
}

func (m Model) SetSize(w, h int) Model {
	m.width = w
	m.height = h
	return m
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if m.Active == TypeNone {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.Active == TypeHelp {
			lines := strings.Split(m.Content, "\n")
			pageRows := m.bodyRows()
			maxOffset := max(len(lines)-pageRows, 0)
			switch msg.String() {
			case "j", "down":
				m.offset = min(m.offset+1, maxOffset)
				return m, nil
			case "k", "up":
				m.offset = max(m.offset-1, 0)
				return m, nil
			case "pgdown", "ctrl+d":
				m.offset = min(m.offset+pageRows, maxOffset)
				return m, nil
			case "pgup", "ctrl+u":
				m.offset = max(m.offset-pageRows, 0)
				return m, nil
			case "g", "home":
				m.offset = 0
				return m, nil
			case "G", "end":
				m.offset = maxOffset
				return m, nil
			}
		}
		switch msg.String() {
		case "esc", "q":
			m.Active = TypeNone
			return m, nil
		case "y", "Y":
			if m.Active == TypeConfirm && m.OnConfirm != nil {
				cmd := m.OnConfirm
				m.Active = TypeNone
				m.OnConfirm = nil
				return m, cmd
			}
		case "n", "N":
			if m.Active == TypeConfirm {
				m.Active = TypeNone
				return m, nil
			}
		}
	}
	return m, nil
}

func (m Model) IsActive() bool {
	return m.Active != TypeNone
}

func (m Model) View() string {
	if m.Active == TypeNone {
		return ""
	}

	maxW := m.width * 3 / 4
	if maxW < 36 {
		maxW = 36
	}
	if maxW > m.width-4 {
		maxW = max(m.width-4, 20)
	}
	maxH := m.height * 3 / 4
	if maxH < 10 {
		maxH = 10
	}
	if maxH > m.height-2 {
		maxH = max(m.height-2, 6)
	}

	meta := "Esc close"
	if m.Active == TypeHelp {
		meta = "j/k scroll  Esc close"
	}
	title := styles.PageHeader(m.Title, meta, maxW-4)

	var body string
	switch m.Active {
	case TypeConfirm:
		body = m.Content + "\n\n" + styles.KeyHint("y", "confirm") + "  " + styles.KeyHint("n", "cancel")
	case TypeHelp:
		lines := strings.Split(m.Content, "\n")
		rows := m.bodyRows()
		maxOffset := max(len(lines)-rows, 0)
		offset := min(m.offset, maxOffset)
		end := min(offset+rows, len(lines))
		body = strings.Join(lines[offset:end], "\n")
		body += "\n" + styles.Faint.Render(helpPosition(offset, end, len(lines)))
	default:
		body = m.Content
	}

	// Detail and confirmation overlays remain bounded without introducing a
	// second navigation mode. Help has its own scrollable viewport above.
	lines := strings.Split(body, "\n")
	if m.Active != TypeHelp && len(lines) > maxH-4 {
		lines = append(lines[:maxH-5], "...")
	}
	body = strings.Join(lines, "\n")

	content := lipgloss.JoinVertical(lipgloss.Left, title, styles.Divider(maxW-4), body)

	box := styles.OverlayStyle.
		MaxWidth(maxW).
		MaxHeight(maxH).
		Render(content)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}

func (m Model) bodyRows() int {
	maxH := m.height * 3 / 4
	if maxH < 10 {
		maxH = 10
	}
	if maxH > m.height-2 {
		maxH = max(m.height-2, 6)
	}
	return max(maxH-5, 1)
}

func helpPosition(offset, end, total int) string {
	if total == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d-%d/%d", offset+1, end, total)
}

// ShowHelp shows the global help overlay.
func (m Model) ShowHelp() Model {
	m.Active = TypeHelp
	m.Title = "Keyboard Shortcuts"
	m.offset = 0
	m.Content = strings.Join([]string{
		"Navigation",
		"  Tab / Shift+Tab    next / previous page",
		"  1-6                jump to page",
		"  q / Ctrl+C         quit",
		"  ?                  show this help",
		"  r                  refresh current page",
		"  R                  restart mihomo core",
		"  Esc                close filter / overlay",
		"",
		"Home",
		"  t    toggle TUN mode",
		"  m    cycle mode: rule -> global -> direct",
		"",
		"Proxies",
		"  left/right or h/l  switch groups / nodes",
		"  up/down or j/k     move selection",
		"  Ctrl+d / Ctrl+u    half-page down / up",
		"  g or gg / G        top / bottom",
		"  Enter              select node / focus nodes",
		"  d / D              test group or selected node",
		"  T                  HTTP / TCP / ICMP probe mode",
		"  u                  unpin URLTest/Fallback back to auto",
		"  o                  sort default / name / delay",
		"  /                  filter name or delay<200 / delay>500 / delay=timeout",
		"",
		"Connections",
		"  / filter   Enter detail   x close   X close all   s sort",
		"",
		"Rules",
		"  / filter   g top   G bottom",
		"",
		"Logs",
		"  space freeze view   l level   c clear   / filter",
		"",
		"Config",
		"  Enter / Space   toggle or edit selected setting",
	}, "\n")
	return m
}

// ShowDetail shows a detail overlay.
func (m Model) ShowDetail(title, content string) Model {
	m.Active = TypeDetail
	m.Title = title
	m.Content = content
	m.offset = 0
	return m
}

// ShowConfirm shows a confirmation overlay.
func (m Model) ShowConfirm(title, content string, onConfirm tea.Cmd) Model {
	m.Active = TypeConfirm
	m.Title = title
	m.Content = content
	m.OnConfirm = onConfirm
	m.offset = 0
	return m
}
