package app

import tea "github.com/charmbracelet/bubbletea"

// isGlobalKey returns true if the key should be handled at root level.
func isGlobalKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+c", "q":
		return true
	case "?":
		return true
	case "tab", "shift+tab", "1", "2", "3", "4", "5":
		return true
	}
	return false
}

// isQuit returns true if the key should quit the app.
func isQuit(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "ctrl+c":
		return true
	case "q":
		return true
	}
	return false
}
