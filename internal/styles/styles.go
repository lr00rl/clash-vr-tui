package styles

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

var (
	// Palette: warm terminal cockpit, tuned for SSH and truecolor terminals.
	Ink        = lipgloss.Color("#151511")
	InkRaised  = lipgloss.Color("#1F201A")
	Panel      = lipgloss.Color("#24251E")
	PanelSoft  = lipgloss.Color("#2B2B23")
	Border     = lipgloss.Color("#4C4B3E")
	BorderSoft = lipgloss.Color("#35362D")
	Accent     = lipgloss.Color("#D8A657")
	AccentSoft = lipgloss.Color("#8E6F35")
	Secondary  = lipgloss.Color("#6AA3A0")
	Success    = lipgloss.Color("#80B85D")
	Warning    = lipgloss.Color("#D7B84F")
	Danger     = lipgloss.Color("#D66A5A")
	Muted      = lipgloss.Color("#85816F")
	Text       = lipgloss.Color("#EAE1C8")
	TextSubtle = lipgloss.Color("#BFB69C")
	TextFaint  = lipgloss.Color("#8F8978")
	BgDark     = InkRaised
	BgSidebar  = Ink
	Primary    = Accent

	// Base text
	Strong = lipgloss.NewStyle().
		Foreground(Text).
		Bold(true)

	Subtle = lipgloss.NewStyle().
		Foreground(TextSubtle)

	Faint = lipgloss.NewStyle().
		Foreground(TextFaint)

	DividerStyle = lipgloss.NewStyle().
			Foreground(BorderSoft)

	// Sidebar
	SidebarStyle = lipgloss.NewStyle().
			Background(BgSidebar).
			Foreground(Text).
			Padding(1, 1)

	SidebarItem = lipgloss.NewStyle().
			Foreground(TextFaint).
			Padding(0, 1)

	SidebarActive = lipgloss.NewStyle().
			Foreground(Text).
			Background(PanelSoft).
			Bold(true).
			Padding(0, 1)

	// Status bar
	StatusBar = lipgloss.NewStyle().
			Foreground(Text).
			Background(Ink).
			Padding(0, 1)

	StatusTitle = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true)

	TrafficUp = lipgloss.NewStyle().
			Foreground(Success)

	TrafficDown = lipgloss.NewStyle().
			Foreground(Secondary)

	// Help bar
	HelpBar = lipgloss.NewStyle().
		Foreground(TextFaint).
		Background(Ink).
		Padding(0, 1)

	HelpKey = lipgloss.NewStyle().
		Foreground(Accent).
		Bold(true)

	HelpDesc = lipgloss.NewStyle().
			Foreground(TextFaint)

	// Content area
	ContentStyle = lipgloss.NewStyle().
			Foreground(Text).
			Background(InkRaised).
			Padding(0, 1)

	// Section boxes
	SectionTitle = lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true)

	SectionBorder = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(Border).
			Padding(0, 1)

	// Table
	TableHeader = lipgloss.NewStyle().
			Foreground(TextSubtle).
			Bold(true)

	TableRow = lipgloss.NewStyle().
			Foreground(Text)

	TableRowSelected = lipgloss.NewStyle().
				Foreground(Text).
				Background(PanelSoft).
				Bold(true)

	// Delay colors
	DelayFast = lipgloss.NewStyle().Foreground(Success) // <200ms
	DelaySlow = lipgloss.NewStyle().Foreground(Warning) // 200-500ms
	DelayBad  = lipgloss.NewStyle().Foreground(Danger)  // >=500ms
	DelayNone = lipgloss.NewStyle().Foreground(Muted)   // timeout/unknown

	// Toggle
	ToggleOn  = lipgloss.NewStyle().Foreground(Success).Bold(true)
	ToggleOff = lipgloss.NewStyle().Foreground(Muted)

	// Overlay
	OverlayStyle = lipgloss.NewStyle().
			Border(lipgloss.ThickBorder()).
			BorderForeground(Accent).
			Padding(1, 2).
			Background(Panel).
			Foreground(Text)

	// Group header
	GroupExpanded  = lipgloss.NewStyle().Foreground(Text).Bold(true)
	GroupCollapsed = lipgloss.NewStyle().Foreground(Muted).Bold(true)

	ProxyGroupBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(Border).
			Padding(0, 1).
			Foreground(Text)

	ProxyGroupBoxSelected = ProxyGroupBox.
				BorderForeground(Primary)

	ProxyGroupHeader = lipgloss.NewStyle().
				Foreground(Text).
				Bold(true)

	ProxyGroupHeaderSelected = ProxyGroupHeader.
					Foreground(Primary)

	ProxyCard = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(BorderSoft).
			Padding(0, 1).
			Foreground(Text)

	ProxyCardCurrent = ProxyCard.
				BorderForeground(Success)

	ProxyCardSelected = ProxyCard.
				BorderForeground(Primary).
				Background(BgDark).
				Bold(true)

	// Mode tabs
	ModeActive   = lipgloss.NewStyle().Foreground(Ink).Background(Accent).Bold(true).Padding(0, 1)
	ModeInactive = lipgloss.NewStyle().Foreground(TextFaint).Background(Panel).Padding(0, 1)

	// Filter input
	FilterPrompt = lipgloss.NewStyle().Foreground(Accent).Bold(true)

	PageHeading = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true)

	PageMeta = lipgloss.NewStyle().
			Foreground(TextFaint)

	PanelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(Border).
			Foreground(Text).
			Padding(0, 1)

	PanelFocused = PanelStyle.
			BorderForeground(Accent)

	PanelFlat = lipgloss.NewStyle().
			Foreground(Text).
			Background(InkRaised)

	Chip = lipgloss.NewStyle().
		Foreground(TextSubtle).
		Background(PanelSoft).
		Padding(0, 1)

	ChipActive = lipgloss.NewStyle().
			Foreground(Ink).
			Background(Accent).
			Bold(true).
			Padding(0, 1)

	ChipWarn = lipgloss.NewStyle().
			Foreground(Ink).
			Background(Warning).
			Bold(true).
			Padding(0, 1)

	ChipBad = lipgloss.NewStyle().
		Foreground(Ink).
		Background(Danger).
		Bold(true).
		Padding(0, 1)

	MetricLabel = lipgloss.NewStyle().
			Foreground(TextFaint)

	MetricValue = lipgloss.NewStyle().
			Foreground(Text).
			Bold(true)
)

// SidebarWidth adapts navigation density for narrow SSH windows.
func SidebarWidth(total int) int {
	switch {
	case total < 78:
		return 12
	case total < 110:
		return 16
	default:
		return 20
	}
}

// PageHeader renders a compact page title with right-aligned metadata.
func PageHeader(title, meta string, width int) string {
	width = max(width, 1)
	title = Fit(title, max(width/2, 8))
	meta = Fit(meta, max(width-lipgloss.Width(title)-2, 0))

	left := PageHeading.Render(title)
	right := PageMeta.Render(meta)
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// Divider renders a width-aware horizontal rule.
func Divider(width int) string {
	width = max(width, 1)
	return DividerStyle.Render(strings.Repeat("─", width))
}

// PanelTitle gives panels a consistent title treatment.
func PanelTitle(title string) string {
	return SectionTitle.Render(" " + title + " ")
}

// Badge renders a compact state label.
func Badge(label string, active bool) string {
	if active {
		return ChipActive.Render(label)
	}
	return Chip.Render(label)
}

// StateBadge renders semantic status labels.
func StateBadge(label string, kind string) string {
	switch kind {
	case "ok", "good":
		return ToggleOn.Render(label)
	case "warn":
		return ChipWarn.Render(label)
	case "bad", "error":
		return ChipBad.Render(label)
	default:
		return Chip.Render(label)
	}
}

// KeyHint renders a keybinding pair.
func KeyHint(key, desc string) string {
	return HelpKey.Render(key) + HelpDesc.Render(" "+desc)
}

// FilterLine renders an active or retained filter.
func FilterLine(label, value string, active bool) string {
	cursor := ""
	if active {
		cursor = "█"
	}
	return FilterPrompt.Render(label+": ") + value + cursor
}

// EmptyState is intentionally quiet: useful over SSH where wasted rows hurt.
func EmptyState(title, hint string, width int) string {
	title = Fit(title, max(width, 1))
	if hint == "" {
		return Faint.Render(title)
	}
	return Faint.Render(title) + "\n" + HelpDesc.Render(Fit(hint, max(width, 1)))
}

// ErrorLine renders a consistent one-line error.
func ErrorLine(err error, width int) string {
	if err == nil {
		return ""
	}
	return DelayBad.Render("error ") + Fit(err.Error(), max(width-6, 1))
}

func Fit(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	if width <= 2 {
		return runewidth.Truncate(s, width, "")
	}
	return runewidth.Truncate(s, width, "..")
}

func PadRight(s string, width int) string {
	s = Fit(s, width)
	pad := width - runewidth.StringWidth(s)
	if pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}

func PadLeft(s string, width int) string {
	s = Fit(s, width)
	pad := width - runewidth.StringWidth(s)
	if pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

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
