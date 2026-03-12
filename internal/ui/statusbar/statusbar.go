package statusbar

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/cdcd/clash-vr-tui/internal/styles"
)

type Model struct {
	Upload   int64
	Download int64
	width    int
}

func New() Model {
	return Model{}
}

func (m Model) SetWidth(w int) Model {
	m.width = w
	return m
}

func (m Model) View() string {
	title := styles.StatusTitle.Render(" Clash Verge TUI")
	up := styles.TrafficUp.Render(fmt.Sprintf("▲ %s", formatSpeed(m.Upload)))
	down := styles.TrafficDown.Render(fmt.Sprintf("▼ %s", formatSpeed(m.Download)))
	traffic := fmt.Sprintf("%s  %s", up, down)

	gap := m.width - lipgloss.Width(title) - lipgloss.Width(traffic) - 2
	if gap < 1 {
		gap = 1
	}
	padding := lipgloss.NewStyle().Width(gap).Render("")

	return styles.StatusBar.Width(m.width).Render(
		title + padding + traffic,
	)
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
