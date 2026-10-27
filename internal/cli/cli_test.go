package cli

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cdcd/clash-vr-tui/internal/api"
)

func testClient(t *testing.T, handler http.HandlerFunc) *api.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return api.NewWith(api.Endpoint{Server: strings.TrimPrefix(server.URL, "http://")})
}

func TestCmdStatusFailsWhenARequiredSectionFails(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			_, _ = w.Write([]byte(`{"version":"mock","meta":true}`))
		case "/configs":
			http.Error(w, "simulated config failure", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	})

	err := cmdStatus(client, options{json: true})
	if err == nil || !strings.Contains(err.Error(), "get runtime config") {
		t.Fatalf("cmdStatus error = %v, want required section failure", err)
	}
}

func TestCmdStatusFetchesEveryRequiredSection(t *testing.T) {
	seen := map[string]bool{}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = true
		responses := map[string]string{
			"/version":     `{"version":"mock","meta":true}`,
			"/configs":     `{"mode":"rule","mixed-port":7890,"log-level":"info","tun":{"enable":true}}`,
			"/connections": `{"downloadTotal":1,"uploadTotal":2,"connections":[]}`,
			"/rules":       `{"rules":[]}`,
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
		_, _ = fmt.Fprint(w, body)
	})

	if err := cmdStatus(client, options{json: true}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/version", "/configs", "/connections", "/rules"} {
		if !seen[path] {
			t.Errorf("required endpoint %s was not fetched", path)
		}
	}
}

func TestCmdNodesFailsWhenDelayHistoryIsUnavailable(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/proxies/group":
			_, _ = w.Write([]byte(`{"name":"group","type":"Selector","now":"node","all":["node"]}`))
		case "/proxies":
			http.Error(w, "simulated history failure", http.StatusInternalServerError)
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	})

	err := cmdNodes(client, options{args: []string{"group"}, json: true})
	if err == nil || !strings.Contains(err.Error(), "get proxy delay history") {
		t.Fatalf("cmdNodes error = %v, want history failure", err)
	}
}

func TestParseTimeoutRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "nope", "0", "-1", "600001"} {
		if _, err := parseTimeout(value); err == nil {
			t.Errorf("parseTimeout(%q) succeeded", value)
		}
	}
	if got, err := parseTimeout("5000"); err != nil || got != 5000 {
		t.Fatalf("parseTimeout valid = (%d, %v)", got, err)
	}
}

func TestFlagValueAcceptsLeadingDash(t *testing.T) {
	argv := []string{"status", "--secret", "-Ab3_secret"}
	value, next, err := flagValue(argv, 1, "--secret")
	if err != nil || value != "-Ab3_secret" || next != 2 {
		t.Fatalf("flagValue = %q, %d, %v", value, next, err)
	}
	if _, _, err := flagValue(argv[:2], 1, "--secret"); err == nil {
		t.Fatal("a flag at the end of argv should still require a value")
	}
}
