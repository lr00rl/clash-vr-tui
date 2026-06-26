package probe

import "testing"

func TestParseEndpoints(t *testing.T) {
	yaml := `
proxies:
  - name: "[cd]-tokyo"
    type: vless
    server: tokyo.example.com
    port: 443
  - {name: ss-node, type: ss, server: 1.2.3.4, port: "8388"}
  - name: no-server
    type: direct
proxy-groups:
  - name: G
`
	m, err := parseEndpoints([]byte(yaml))
	if err != nil {
		t.Fatalf("parseEndpoints: %v", err)
	}
	if len(m) != 2 {
		t.Fatalf("want 2 endpoints (server-less skipped), got %d: %+v", len(m), m)
	}
	if got := m["[cd]-tokyo"]; got.Server != "tokyo.example.com" || got.Port != 443 {
		t.Errorf("tokyo endpoint = %+v, want tokyo.example.com:443", got)
	}
	if got := m["ss-node"]; got.Server != "1.2.3.4" || got.Port != 8388 {
		t.Errorf("ss-node endpoint = %+v, want 1.2.3.4:8388 (string port)", got)
	}
	if _, ok := m["no-server"]; ok {
		t.Errorf("no-server should be skipped (no server field)")
	}
}

func TestConfigFlagRe(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"/usr/bin/verge-mihomo -d /etc -f /etc/clash/clash-verge.yaml -ext-ctl x", "/etc/clash/clash-verge.yaml"},
		{`/App/verge-mihomo -f /Users/x/Application Support/io.x/clash-verge.yaml`, "/Users/x/Application Support/io.x/clash-verge.yaml"},
		{"mihomo --config /tmp/c.yml", ""}, // no -f flag
	}
	for _, tt := range tests {
		m := configFlagRe.FindStringSubmatch(tt.line)
		got := ""
		if len(m) > 1 {
			got = m[1]
		}
		if got != tt.want {
			t.Errorf("configFlagRe(%q) = %q, want %q", tt.line, got, tt.want)
		}
	}
}

func TestToInt(t *testing.T) {
	cases := map[any]int{443: 443, "8388": 8388, int64(80): 80, float64(123): 123, "bad": 0, nil: 0}
	for in, want := range cases {
		if got := toInt(in); got != want {
			t.Errorf("toInt(%v) = %d, want %d", in, got, want)
		}
	}
}

func TestModeCycle(t *testing.T) {
	if ModeHTTP.Next() != ModeTCP || ModeTCP.Next() != ModeICMP || ModeICMP.Next() != ModeHTTP {
		t.Errorf("mode cycle broken")
	}
	if ModeHTTP.NeedsEndpoints() || !ModeTCP.NeedsEndpoints() || !ModeICMP.NeedsEndpoints() {
		t.Errorf("NeedsEndpoints wrong")
	}
}
