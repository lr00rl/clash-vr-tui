package app

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/cdcd/clash-vr-tui/internal/messages"
)

// isQuit returns true if the key should quit the app.
func isQuit(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+c", "q":
		return true
	}
	return false
}

// pageForNumberKey maps number keys 1-N to pages for quick jumping.
func pageForNumberKey(msg tea.KeyMsg) (messages.Page, bool) {
	pages := messages.Pages()
	switch msg.String() {
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(msg.String()[0] - '1')
		if idx >= 0 && idx < len(pages) {
			return pages[idx], true
		}
	}
	return messages.PageHome, false
}
