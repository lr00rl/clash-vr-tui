package statusbar

import (
	"fmt"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	Upload      int64
	Download    int64
	Memory      int64
	status      string
	statusErr   bool
	streamState string
	streamLast  time.Time
	width       int
}

func New() Model {
	return Model{streamState: "connecting"}
}

func (m Model) SetStreamState(state string, last time.Time) Model {
	m.streamState = state
	m.streamLast = last
	return m
}

func (m Model) SetWidth(w int) Model {
	m.width = w
	return m
}

// SetStatus sets a transient status message shown in the status bar.
func (m Model) SetStatus(text string, isErr bool) Model {
	m.status = text
	m.statusErr = isErr
	return m
}

var (
	statusInfoStyle = lipgloss.NewStyle().Foreground(styles.Secondary).Bold(true)
	statusErrStyle  = lipgloss.NewStyle().Foreground(styles.Danger).Bold(true)
	memStyle        = lipgloss.NewStyle().Foreground(styles.TextFaint)
)

func (m Model) View() string {
	innerW := max(m.width-2, 1)
	title := styles.StatusTitle.Render(" clash-vr-tui")

	up := styles.TrafficUp.Render(fmt.Sprintf("UP %s", formatSpeed(m.Upload)))
	down := styles.TrafficDown.Render(fmt.Sprintf("DOWN %s", formatSpeed(m.Download)))
	right := fmt.Sprintf("%s  %s", down, up)
	state := styles.StateBadge("CONNECTING", "warn")
	switch m.streamState {
	case "live":
		state = styles.StateBadge("LIVE", "ok")
	case "reconnecting":
		state = styles.StateBadge("STALE", "warn")
	}
	if m.streamState != "live" && !m.streamLast.IsZero() {
		state += styles.Faint.Render(" " + m.streamLast.Format("15:04:05"))
	}
	right = state + "  " + right
	// StatusBar pads one cell on each side inside innerW.
	avail := innerW - 2
	if mem := "  " + memStyle.Render("MEM "+formatBytes(m.Memory)); m.Memory > 0 &&
		lipgloss.Width(title)+lipgloss.Width(right)+lipgloss.Width(mem)+1 <= avail {
		right += mem
	}

	left := title
	const sep = 3 // " • "
	// Truncate raw text (before styling) so ANSI codes stay intact.
	if maxMid := avail - lipgloss.Width(title) - lipgloss.Width(right) - sep - 1; m.status != "" && maxMid > 1 {
		text := truncate(styles.SafeText(m.status), maxMid)
		mid := statusInfoStyle.Render(text)
		if m.statusErr {
			mid = statusErrStyle.Render(text)
		}
		left = title + " " + styles.Faint.Render("•") + " " + mid
	}

	gap := max(avail-lipgloss.Width(left)-lipgloss.Width(right), 1)
	body := ansi.Truncate(left+lipgloss.NewStyle().Width(gap).Render("")+right, avail, "")

	return styles.StatusBar.Width(innerW).Render(body)
}

func truncate(s string, maxLen int) string {
	if lipgloss.Width(s) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return ""
	}
	return string([]rune(s)[:maxLen-1]) + "…"
}

func formatBytes(b int64) string {
	switch {
	case b >= 1024*1024*1024:
		return fmt.Sprintf("%.1fG", float64(b)/(1024*1024*1024))
	case b >= 1024*1024:
		return fmt.Sprintf("%.0fM", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.0fK", float64(b)/1024)
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatSpeed(bytesPerSec int64) string {
	switch {
	case bytesPerSec >= 1024*1024:
		return fmt.Sprintf("%.1f MB/s", float64(bytesPerSec)/(1024*1024))
	case bytesPerSec >= 1024:
		return fmt.Sprintf("%.1f KB/s", float64(bytesPerSec)/1024)
	default:
		return fmt.Sprintf("%d B/s", bytesPerSec)
	}
}
