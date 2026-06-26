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

	title := styles.PageHeader(m.Title, "Esc close", maxW-4)

	var body string
	switch m.Active {
	case TypeConfirm:
		body = m.Content + "\n\n" + styles.KeyHint("y", "confirm") + "  " + styles.KeyHint("n", "cancel")
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

// ShowHelp shows the global help overlay.
func (m Model) ShowHelp() Model {
	m.Active = TypeHelp
	m.Title = "Keyboard Shortcuts"
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
		"  space pause   l level   c clear   / filter",
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
