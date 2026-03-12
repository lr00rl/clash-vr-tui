package styles

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Colors
	Primary   = lipgloss.Color("#7C3AED") // purple
	Secondary = lipgloss.Color("#06B6D4") // cyan
	Success   = lipgloss.Color("#22C55E") // green
	Warning   = lipgloss.Color("#EAB308") // yellow
	Danger    = lipgloss.Color("#EF4444") // red
	Muted     = lipgloss.Color("#6B7280") // gray
	Text      = lipgloss.Color("#E5E7EB") // light gray
	BgDark    = lipgloss.Color("#1F2937") // dark bg
	BgSidebar = lipgloss.Color("#111827") // darker bg

	// Sidebar
	SidebarStyle = lipgloss.NewStyle().
			Width(16).
			Background(BgSidebar).
			Padding(1, 1)

	SidebarItem = lipgloss.NewStyle().
			Foreground(Muted).
			Padding(0, 1)

	SidebarActive = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Padding(0, 1)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Foreground(Text).
			Background(BgDark).
			Padding(0, 1)

	StatusTitle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	TrafficUp = lipgloss.NewStyle().
			Foreground(Success)

	TrafficDown = lipgloss.NewStyle().
			Foreground(Secondary)

	// Help bar
	HelpBar = lipgloss.NewStyle().
		Foreground(Muted).
		Background(BgDark).
		Padding(0, 1)

	HelpKey = lipgloss.NewStyle().
		Foreground(Text).
		Bold(true)

	HelpDesc = lipgloss.NewStyle().
		Foreground(Muted)

	// Content area
	ContentStyle = lipgloss.NewStyle().
			Padding(0, 1)

	// Section boxes
	SectionTitle = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Padding(0, 0, 0, 1)

	SectionBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Muted).
			Padding(0, 1)

	// Table
	TableHeader = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true).
			BorderBottom(true).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(Muted)

	TableRow = lipgloss.NewStyle().
		Foreground(Text)

	TableRowSelected = lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true)

	// Delay colors
	DelayFast = lipgloss.NewStyle().Foreground(Success)   // <200ms
	DelaySlow = lipgloss.NewStyle().Foreground(Warning)   // 200-500ms
	DelayBad  = lipgloss.NewStyle().Foreground(Danger)    // >=500ms
	DelayNone = lipgloss.NewStyle().Foreground(Muted)     // timeout/unknown

	// Toggle
	ToggleOn  = lipgloss.NewStyle().Foreground(Success).Bold(true)
	ToggleOff = lipgloss.NewStyle().Foreground(Muted)

	// Overlay
	OverlayStyle = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(Primary).
			Padding(1, 2).
			Background(BgDark)

	// Group header
	GroupExpanded  = lipgloss.NewStyle().Foreground(Text).Bold(true)
	GroupCollapsed = lipgloss.NewStyle().Foreground(Muted).Bold(true)

	// Mode tabs
	ModeActive   = lipgloss.NewStyle().Foreground(Primary).Bold(true).Underline(true)
	ModeInactive = lipgloss.NewStyle().Foreground(Muted)

	// Filter input
	FilterPrompt = lipgloss.NewStyle().Foreground(Primary)
)

// DelayStyle returns the appropriate style for a delay value.
func DelayStyle(delay int) lipgloss.Style {
	switch {
	case delay <= 0:
		return DelayNone
	case delay < 200:
		return DelayFast
	case delay < 500:
		return DelaySlow
	default:
		return DelayBad
	}
}

// FormatDelay returns a formatted delay string.
func FormatDelay(delay int) string {
	if delay <= 0 {
		return "timeout"
	}
	return fmt.Sprintf("%dms", delay)
}
