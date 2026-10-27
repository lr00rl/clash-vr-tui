package styles

import (
	"strings"
	"testing"
)

func TestSafeTextRemovesTerminalCommandsAndControls(t *testing.T) {
	input := "A\x1b[31mB\x1b[0m\x1b]8;;https://bad.example\aC\x1b]8;;\a\x1b]52;c;Y2xpcA==\a\r\n\tD\u009bE"
	got := SafeText(input)
	if strings.ContainsAny(got, "\x1b\a\r\n\t\u009b") {
		t.Fatalf("unsafe control remains in %q", got)
	}
	if strings.Contains(got, "bad.example") || strings.Contains(got, "Y2xpcA") {
		t.Fatalf("OSC payload remains in %q", got)
	}
	if !strings.Contains(got, "ABC") || !strings.HasSuffix(got, "D E") {
		t.Fatalf("printable text was lost: %q", got)
	}
}

func TestSafeTextPreservesUnicode(t *testing.T) {
	const input = "香港节点 ✓"
	if got := SafeText(input); got != input {
		t.Fatalf("SafeText(%q) = %q", input, got)
	}
}
