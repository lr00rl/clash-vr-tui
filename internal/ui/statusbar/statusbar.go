package statusbar

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	Upload    int64
	Download  int64
	Memory    int64
	status    string
	statusErr bool
	width     int
}

func New() Model {
	return Model{}
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
	statusInfoStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	statusErrStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	memStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
)

func (m Model) View() string {
	title := styles.StatusTitle.Render(" Clash Verge TUI")

	up := styles.TrafficUp.Render(fmt.Sprintf("▲ %s", formatSpeed(m.Upload)))
	down := styles.TrafficDown.Render(fmt.Sprintf("▼ %s", formatSpeed(m.Download)))
	right := fmt.Sprintf("%s  %s", up, down)
	if m.Memory > 0 {
		right += "  " + memStyle.Render("MEM "+formatBytes(m.Memory))
	}

	mid := ""
	if m.status != "" {
		// Truncate raw text (before styling) so ANSI codes stay intact.
		maxMid := max(m.width-lipgloss.Width(title)-lipgloss.Width(right)-4, 0)
		text := truncate(m.status, maxMid)
		if m.statusErr {
			mid = statusErrStyle.Render(text)
		} else {
			mid = statusInfoStyle.Render(text)
		}
	}

	gap := max(m.width-lipgloss.Width(title)-lipgloss.Width(mid)-lipgloss.Width(right)-3, 1)

	var body string
	if mid == "" {
		padding := lipgloss.NewStyle().Width(gap + 1).Render("")
		body = title + padding + right
	} else {
		left := lipgloss.NewStyle().Width(gap).Render("")
		body = title + " " + mid + left + right
	}

	return styles.StatusBar.Width(m.width).Render(body)
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
