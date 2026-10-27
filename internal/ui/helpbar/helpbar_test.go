package helpbar

import (
	"strings"
	"testing"

	"github.com/cdcd/clash-vr-tui/internal/messages"
)

func TestNarrowHelpKeepsCurrentTaskAndMore(t *testing.T) {
	tests := []struct {
		page messages.Page
		want string
	}{
		{page: messages.PageProxies, want: "panel"},
		{page: messages.PageConnections, want: "filter"},
		{page: messages.PageLogs, want: "freeze"},
		{page: messages.PageSettings, want: "edit"},
	}
	for _, tt := range tests {
		view := New().SetWidth(72).View(tt.page)
		if !strings.Contains(view, tt.want) {
			t.Errorf("%s narrow help does not contain primary action %q: %q", tt.page, tt.want, view)
		}
		if !strings.Contains(view, "more") {
			t.Errorf("%s narrow help does not keep help entry: %q", tt.page, view)
		}
	}
}
