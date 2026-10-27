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

func TestRedactURLsDropsTokens(t *testing.T) {
	cases := map[string]string{
		"Get \"https://sub.example.com/api/v1/client/subscribe?token=abc\": EOF": "Get \"https://sub.example.com/***\": EOF",
		"fetch https://sub.example.com?token=abc failed":                         "fetch https://sub.example.com/*** failed",
		"https://user:pw@host.example/#frag.":                                    "https://host.example/***.",
		"see http://127.0.0.1:9090/ now":                                         "see http://127.0.0.1:9090/ now",
	}
	for in, want := range cases {
		if got := RedactURLs(in); got != want {
			t.Errorf("RedactURLs(%q) = %q, want %q", in, got, want)
		}
	}
}
