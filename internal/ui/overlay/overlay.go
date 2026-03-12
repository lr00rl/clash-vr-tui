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
		"  1-5                Jump to page",
		"  q / Ctrl+C         Quit",
		"  ?                  Toggle this help",
		"  r                  Refresh current page",
		"",
		"Home",
		"  s    Toggle system proxy",
		"  t    Toggle TUN mode",
		"  m    Cycle mode (rule → global → direct)",
		"",
		"Proxies",
		"  Space   Expand/collapse group",
		"  Enter   Select proxy node",
		"  d       Delay test group",
		"  D       Delay test single node",
		"  o       Cycle sort (default → name → delay)",
		"  /       Filter nodes",
		"",
		"Connections",
		"  /       Filter",
		"  Enter   Show detail",
		"  x       Close connection",
		"  X       Close all (confirm)",
		"  s       Cycle sort",
		"",
		"Rules",
		"  /    Filter",
		"  g    Go to top",
		"  G    Go to bottom",
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
