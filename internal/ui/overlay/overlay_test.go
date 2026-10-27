package overlay

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHelpOverlayScrollsToLaterSections(t *testing.T) {
	m := New().SetSize(72, 24).ShowHelp()
	initial := m.View()
	if strings.Contains(initial, "Config") {
		t.Fatal("small initial help unexpectedly contains final section; test needs scrolling content")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	view := updated.View()
	if !strings.Contains(view, "Config") {
		t.Fatalf("help did not reveal final section after End: %q", view)
	}
	if !strings.Contains(view, "/") {
		t.Fatalf("help does not show a position indicator: %q", view)
	}
}

func TestShowHelpResetsScrollPosition(t *testing.T) {
	m := New().SetSize(72, 24).ShowHelp()
	m.offset = 10
	m = m.ShowHelp()
	if m.offset != 0 {
		t.Fatalf("ShowHelp offset = %d, want 0", m.offset)
	}
}
