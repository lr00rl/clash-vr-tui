package api

import (
	"encoding/json"
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
