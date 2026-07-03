package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDelayResultParse(t *testing.T) {
	var ok DelayResult
	if err := json.Unmarshal([]byte(`{"delay":45}`), &ok); err != nil {
		t.Fatal(err)
	}
	if ok.Delay != 45 || ok.Message != "" {
		t.Errorf("delay parse = %+v, want delay 45", ok)
	}

	var to DelayResult
	if err := json.Unmarshal([]byte(`{"message":"Timeout"}`), &to); err != nil {
		t.Fatal(err)
	}
	if to.Delay != 0 || to.Message != "Timeout" {
		t.Errorf("timeout parse = %+v, want delay 0 / message Timeout", to)
	}
}

func TestGroupParseTestURLAndFixed(t *testing.T) {
	data := `{"name":"Auto","type":"URLTest","now":"N1","all":["N1","N2"],"testUrl":"http://x/204","fixed":"N1"}`
	var g Group
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		t.Fatal(err)
	}
	if g.TestURL != "http://x/204" {
		t.Errorf("TestURL = %q", g.TestURL)
	}
	if g.Fixed != "N1" {
		t.Errorf("Fixed = %q", g.Fixed)
	}
	if len(g.All) != 2 || g.Now != "N1" {
		t.Errorf("group fields = %+v", g)
	}
}

func TestNewWithSelectsTransport(t *testing.T) {
	unix := NewWith(Endpoint{Socket: "/tmp/s.sock"})
	if !unix.isUnix || unix.baseURL != "http://localhost" || unix.SocketPath() != "/tmp/s.sock" {
		t.Errorf("unix client misconfigured: %+v", unix)
	}
	tcp := NewWith(Endpoint{Server: "127.0.0.1:9090", Secret: "x"})
	if tcp.isUnix || tcp.baseURL != "http://127.0.0.1:9090" || tcp.wsBase != "ws://127.0.0.1:9090" {
		t.Errorf("tcp client misconfigured: %+v", tcp)
	}
	if tcp.secret != "x" {
		t.Errorf("secret not set")
	}
}

func TestClientAttachesBearerSecretToMutatingRequests(t *testing.T) {
	seen := map[string]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sek" {
			t.Fatalf("%s Authorization = %q, want Bearer sek", r.Method, got)
		}
		seen[r.Method] = true
		switch r.Method {
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()

	c := NewWith(Endpoint{Server: strings.TrimPrefix(srv.URL, "http://"), Secret: "sek"})
	if _, err := c.put("/x", map[string]string{"name": "node"}); err != nil {
		t.Fatalf("put: %v", err)
	}
	if _, err := c.patch("/x", map[string]string{"mode": "rule"}); err != nil {
		t.Fatalf("patch: %v", err)
	}
	if err := c.delete("/x"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.post("/x", nil); err != nil {
		t.Fatalf("post: %v", err)
	}

	for _, method := range []string{http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodPost} {
		if !seen[method] {
			t.Fatalf("method %s was not exercised; seen=%v", method, seen)
		}
	}
}

func TestOrderGroupsByNameUsesGlobalOrderAndAppendsRemainder(t *testing.T) {
	groups := []Group{
		{Name: "OuterLand"},
		{Name: "Auto(lowlatency)"},
		{Name: "Linkedin"},
		{Name: "for-test-ip"},
		{Name: "GLOBAL"},
		{Name: "GitHub|Microsoft|Telegram"},
		{Name: "openjobs-vpn"},
		{Name: "Europe"},
		{Name: "AI-CF-Suite"},
		{Name: "InnerLand"},
		{Name: "Polymarket"},
		{Name: "Apple"},
		{Name: "US-CN2-GIA"},
	}
	order := []string{
		"for-test-ip",
		"Polymarket",
		"AI-CF-Suite",
		"Linkedin",
		"Apple",
		"GitHub|Microsoft|Telegram",
		"US-CN2-GIA",
		"Europe",
		"openjobs-vpn",
		"InnerLand",
		"OuterLand",
		"Auto(lowlatency)",
		"Reject",
	}

	got := orderGroupsByName(groups, order)
	want := []string{
		"for-test-ip",
		"Polymarket",
		"AI-CF-Suite",
		"Linkedin",
		"Apple",
		"GitHub|Microsoft|Telegram",
		"US-CN2-GIA",
		"Europe",
		"openjobs-vpn",
		"InnerLand",
		"OuterLand",
		"Auto(lowlatency)",
		"GLOBAL",
	}
	if len(got) != len(want) {
		t.Fatalf("ordered groups len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Fatalf("ordered groups[%d] = %q, want %q; full = %+v", i, got[i].Name, name, got)
		}
	}
}
