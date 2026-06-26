package overlay

import (
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
	if maxW < 40 {
		maxW = 40
	}
	maxH := m.height * 3 / 4
	if maxH < 10 {
		maxH = 10
	}

	title := styles.SectionTitle.Render(m.Title)

	var body string
	switch m.Active {
	case TypeConfirm:
		body = m.Content + "\n\n" + styles.HelpKey.Render("[y]") + " Yes  " + styles.HelpKey.Render("[n]") + " No"
	default:
		body = m.Content
	}

	// Truncate body lines if too tall
	lines := strings.Split(body, "\n")
	if len(lines) > maxH-4 {
		lines = lines[:maxH-4]
		lines = append(lines, "...")
	}
	body = strings.Join(lines, "\n")

	content := lipgloss.JoinVertical(lipgloss.Left, title, "", body)

	box := styles.OverlayStyle.
		MaxWidth(maxW).
		MaxHeight(maxH).
		Render(content)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		box,
	)
}

// ShowHelp shows the global help overlay.
func (m Model) ShowHelp() Model {
	m.Active = TypeHelp
	m.Title = "Keyboard Shortcuts"
	m.Content = strings.Join([]string{
		"Navigation",
		"  Tab / Shift+Tab    Next / Previous page",
		"  1-5                Jump to page by number",
		"  q / Ctrl+C         Quit",
		"  ?                  Toggle this help",
		"  r                  Refresh current page",
		"  R                  Restart mihomo core",
		"  Esc                Close filter / overlay",
		"",
		"Home",
		"  t    Toggle TUN mode (core flag)",
		"  m    Cycle mode (rule → global → direct)",
		"",
		"Proxies",
		"  ← → / h l   Switch panel (groups ↔ nodes)",
		"  ↑ ↓ / j k   Move within panel",
		"  Enter       Select node / focus nodes",
		"  d           Delay test (group or node)",
		"  D           Delay test single node",
		"  T           Cycle test mode (HTTP / TCP / ICMP)",
		"  u           Unpin URLTest/Fallback group (auto)",
		"  o           Cycle sort (default → name → delay)",
		"  /           Filter (name, or delay<200 / delay>500 / delay=timeout)",
		"",
		"Connections",
		"  / Filter   Enter Detail   x Close   X Close all   s Sort",
		"",
		"Rules",
		"  / Filter   g Top   G Bottom",
		"",
		"Settings",
		"  Enter / Space   Toggle setting",
	}, "\n")
	return m
}

// ShowDetail shows a detail overlay.
func (m Model) ShowDetail(title, content string) Model {
	m.Active = TypeDetail
	m.Title = title
	m.Content = content
	return m
}

// ShowConfirm shows a confirmation overlay.
func (m Model) ShowConfirm(title, content string, onConfirm tea.Cmd) Model {
	m.Active = TypeConfirm
	m.Title = title
	m.Content = content
	m.OnConfirm = onConfirm
	return m
}
