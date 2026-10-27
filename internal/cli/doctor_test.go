package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/doctor"
)

func stubDoctorSystem(t *testing.T) {
	t.Helper()
	orig := doctorSystem
	doctorSystem = func() doctor.System {
		return doctor.System{GOOS: "linux", Getenv: func(string) string { return "" }, Now: time.Now}
	}
	t.Cleanup(func() { doctorSystem = orig })
}

// The bundle keeps the runtime config and logs but never the inbound
// credentials or a subscription token that appeared in a log line.
func TestDoctorBundleOmitsCredentials(t *testing.T) {
	stubDoctorSystem(t)
	upgrader := websocket.Upgrader{}
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			fmt.Fprint(w, `{"version":"v1.19.32","meta":true}`)
		case "/configs":
			fmt.Fprint(w, `{"mixed-port":0,"mode":"rule","allow-lan":true,"bind-address":"*","tun":{"enable":true},
				"authentication":["alice:hunter2"],"ss-config":"ss://aes-128-gcm:supersecret@:8388"}`)
		case "/connections":
			fmt.Fprint(w, `{"connections":[{"id":"1","metadata":{"host":"a.com"},"rule":"Match","chains":["A","Proxy"]}]}`)
		case "/logs":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteJSON(api.LogEntry{Type: "warning", Payload: `[Provider] sub pull error: Get "https://sub.example.com/s?token=SECRETTOKEN": EOF`})
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	})
	r := doctor.Run(context.Background(), doctor.Inputs{Client: client, LogsDur: 300 * time.Millisecond, System: doctorSystem()})
	if len(r.Logs) != 1 || r.Connections == nil {
		t.Fatalf("nothing collected: logs=%d conns=%v", len(r.Logs), r.Connections)
	}

	dir, err := writeBundle(t.TempDir(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(dir), "doctor-"+r.Time.Format("20060102-")) {
		t.Errorf("bundle dir = %s", dir)
	}
	for _, name := range []string{"report.md", "report.json", "logs.ndjson", "connections.json", "runtime-config.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, secret := range []string{"hunter2", "alice", "supersecret", "SECRETTOKEN", `"authentication"`, "ss-config"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("%s contains %q", name, secret)
			}
		}
	}
	cfg, _ := os.ReadFile(filepath.Join(dir, "runtime-config.json"))
	if !bytes.Contains(cfg, []byte(`"inbound-auth-set": true`)) || !bytes.Contains(cfg, []byte(`"allow-lan": true`)) {
		t.Errorf("runtime-config.json = %s", cfg)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "report.md"))
	if !bytes.Contains(md, []byte("## Runtime config")) || !bytes.Contains(md, []byte("- `info` allow-lan is on with inbound authentication")) {
		t.Errorf("report.md = %s", md)
	}
}

func TestPrintReportGroupsFindings(t *testing.T) {
	r := doctor.Report{
		Endpoint: "/tmp/m.sock", Source: "flag", Version: "v1",
		Findings: []doctor.Finding{
			{Check: "controller", Severity: doctor.OK, Title: "mihomo v1 answers"},
			{Check: "config", Severity: doctor.Fail, Title: "TUN is off\x1b[31m", Detail: "line one\nline two", Hint: "turn it on"},
			{Check: "config", Severity: doctor.Warn, Title: "allow-lan is on"},
		},
		Summary: doctor.Counts{OK: 1, Warn: 1, Fail: 1},
	}
	var b bytes.Buffer
	printReport(&b, r, false)
	want := `clash-vr-tui doctor: mihomo v1 at /tmp/m.sock (flag)

Controller
  ok    mihomo v1 answers

Runtime config
  FAIL  TUN is off
        line one
        line two
        -> turn it on
  warn  allow-lan is on

1 ok, 0 info, 1 warn, 1 fail
`
	if b.String() != want {
		t.Fatalf("printReport:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestCmdDoctorFailsWhenAFindingFails(t *testing.T) {
	stubDoctorSystem(t)
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	stdout := os.Stdout
	os.Stdout = devnull
	defer func() { os.Stdout = stdout }()

	err = cmdDoctor(api.NewWith(api.Endpoint{Server: "127.0.0.1:1"}), options{logsDur: 0, json: true})
	if err == nil || !strings.Contains(err.Error(), "1 failing check") || errors.As(err, new(usageError)) {
		t.Fatalf("cmdDoctor error = %v, want a runtime failure for the unreachable controller", err)
	}
}
