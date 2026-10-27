package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/gorilla/websocket"

	"github.com/cdcd/clash-vr-tui/internal/api"
	"github.com/cdcd/clash-vr-tui/internal/discover"
)

const secretLogLine = `[Provider] sub pull error: Get "https://sub.example.com/api?token=SECRETTOKEN": EOF`

// fakeController serves the endpoints doctor reads. mixedPort is where the
// egress check sends its request.
func fakeController(t *testing.T, mixedPort int) *api.Client {
	t.Helper()
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version":
			fmt.Fprint(w, `{"version":"v1.19.32","meta":true}`)
		case "/configs":
			fmt.Fprintf(w, `{"mixed-port":%d,"mode":"rule","allow-lan":false,"log-level":"info","tun":{"enable":false},
				"authentication":["alice:hunter2"],"ss-config":"ss://aes-128-gcm:supersecret@:8388"}`, mixedPort)
		case "/group":
			fmt.Fprint(w, `{"proxies":[{"name":"Proxy","type":"Selector","now":"A","all":["A"]}]}`)
		case "/proxies":
			fmt.Fprint(w, `{"proxies":{"Proxy":{"name":"Proxy","type":"Selector","now":"A","all":["A"]},
				"A":{"name":"A","type":"Vless","history":[{"delay":80}]}}}`)
		case "/providers/proxies", "/providers/rules":
			fmt.Fprint(w, `{"providers":{}}`)
		case "/connections":
			fmt.Fprint(w, `{"connections":[{"id":"1","metadata":{"host":"a.com"},"rule":"Match","chains":["A","Proxy"]}]}`)
		case "/dns/query":
			fmt.Fprint(w, `{"Status":0,"Answer":[{"name":"doctor.test.","type":1,"TTL":60,"data":"1.2.3.4"}]}`)
		case "/logs":
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.WriteJSON(api.LogEntry{Type: "warning", Payload: dialLineProxy})
			_ = conn.WriteJSON(api.LogEntry{Type: "info", Payload: secretLogLine})
			// Hold the stream open until the client goes away.
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return api.NewWith(api.Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})
}

// egressProxy answers every proxied request with 204, as a forward proxy
// reaching the test URL would.
func egressProxy(t *testing.T) int {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	var port int
	fmt.Sscanf(srv.URL[strings.LastIndexByte(srv.URL, ':')+1:], "%d", &port)
	return port
}

func testSystem() System {
	return System{
		GOOS:     "linux",
		Home:     "/home/u",
		FS:       fstest.MapFS{},
		Getenv:   func(string) string { return "" },
		Now:      time.Now,
		PS:       func(context.Context) (string, error) { return "", nil },
		Discover: func(context.Context) []discover.Candidate { return nil },
	}
}

func TestRunAgainstFakeController(t *testing.T) {
	client := fakeController(t, egressProxy(t))
	r := Run(t.Context(), Inputs{
		Client:  client,
		Source:  "flag",
		TestURL: "http://doctor.test/generate_204",
		LogsDur: 300 * time.Millisecond,
		System:  testSystem(),
	})

	if r.Version != "v1.19.32" || r.Source != "flag" {
		t.Fatalf("header = %q %q", r.Version, r.Source)
	}
	order := map[string]int{}
	for i, c := range Checks {
		order[c.ID] = i
	}
	for i := 1; i < len(r.Findings); i++ {
		if order[r.Findings[i].Check] < order[r.Findings[i-1].Check] {
			t.Fatalf("findings out of check order at %d: %s after %s", i, r.Findings[i].Check, r.Findings[i-1].Check)
		}
	}
	for id, want := range map[string]Severity{
		"controller.ok": OK, "config.bypass": Fail, "sysproxy.off": Info, "egress.ok": OK, "dns.ok": OK,
		"groups.ok": OK, "providers.none": Info, "routing.match": Info, "logs.patterns": Warn, "verge.absent": Info,
		"conflicts.none": OK,
	} {
		if got := find(t, r.Findings, id).Severity; got != want {
			t.Errorf("%s = %s, want %s", id, got, want)
		}
	}
	if r.Summary.Fail != 1 {
		t.Errorf("summary = %+v", r.Summary)
	}
	if len(r.Logs) != 2 || r.Connections == nil || len(r.Connections.Connections) != 1 {
		t.Fatalf("collected logs=%d connections=%v", len(r.Logs), r.Connections)
	}
	if strings.Contains(r.Logs[1].Payload, "SECRETTOKEN") {
		t.Errorf("log payload kept the subscription token: %q", r.Logs[1].Payload)
	}

	if _, ok := r.RuntimeConfig["mixed-port"]; !ok {
		t.Errorf("runtime config lost mixed-port: %v", r.RuntimeConfig)
	}
	if string(r.RuntimeConfig["inbound-auth-set"]) != "true" {
		t.Errorf("inbound-auth-set = %s", r.RuntimeConfig["inbound-auth-set"])
	}
	for _, k := range []string{"authentication", "ss-config"} {
		if _, ok := r.RuntimeConfig[k]; ok {
			t.Errorf("runtime config kept %s", k)
		}
	}
	data, _ := json.Marshal(r)
	if s := string(data); strings.Contains(s, "hunter2") || strings.Contains(s, "supersecret") {
		t.Errorf("report JSON carries a credential: %s", s)
	}
}

func TestRunWithUnreachableController(t *testing.T) {
	sys := testSystem()
	sys.Discover = func(context.Context) []discover.Candidate {
		return []discover.Candidate{{Kind: discover.KindSocket, Address: "/tmp/other.sock", State: discover.StateOK, Version: "v1", Sources: []string{"known path"}}}
	}
	r := Run(t.Context(), Inputs{Client: api.NewWith(api.Endpoint{Server: "127.0.0.1:1"}), LogsDur: time.Second, System: sys})
	if f := find(t, r.Findings, "controller.unreachable"); f.Severity != Fail {
		t.Errorf("controller = %+v", f)
	}
	for _, id := range []string{"config.skipped", "egress.skipped", "dns.skipped", "groups.skipped", "logs.skipped"} {
		if find(t, r.Findings, id).Severity != Info {
			t.Errorf("%s not reported as skipped", id)
		}
	}
	if f := find(t, r.Findings, "controller.multiple"); !strings.Contains(f.Detail, "/tmp/other.sock (v1), from known path") {
		t.Errorf("multiple = %+v", f)
	}
	if !hasID(r.Findings, "sysproxy.off") || !hasID(r.Findings, "conflicts.none") {
		t.Errorf("local checks did not run: %+v", r.Findings)
	}
}

// Probes test every visible group but never more than probeConcurrency at once.
func TestProbeGroupsBoundsConcurrency(t *testing.T) {
	var mu sync.Mutex
	active, peak, calls := 0, 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		active++
		calls++
		peak = max(peak, active)
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		if strings.HasPrefix(r.URL.Path, "/group/dead") {
			http.Error(w, `{"message":"all proxies timeout"}`, http.StatusGatewayTimeout)
			return
		}
		fmt.Fprint(w, `{"A":80}`)
	}))
	defer srv.Close()
	client := api.NewWith(api.Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})
	groups := []api.Group{{Name: "dead"}}
	for i := range 9 {
		groups = append(groups, api.Group{Name: fmt.Sprintf("g%d", i)})
	}
	errs := probeGroups(t.Context(), client, groups, "", 1000)
	if calls != 10 || peak > probeConcurrency || peak < 2 {
		t.Fatalf("calls=%d peak=%d, want 10 calls with at most %d at once", calls, peak, probeConcurrency)
	}
	if len(errs) != 1 || errs["dead"] == nil {
		t.Fatalf("errs = %v", errs)
	}
}

// Ctrl+C during the log capture still yields a full report with the lines
// captured so far, not an unfinished logs check.
func TestRunReportsCapturedLogsWhenCancelled(t *testing.T) {
	client := fakeController(t, 0)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(300*time.Millisecond, cancel)
	r := Run(ctx, Inputs{Client: client, LogsDur: time.Minute, System: testSystem()})
	s := find(t, r.Findings, "logs.summary")
	if !strings.Contains(s.Detail, "capture stopped early") || len(r.Logs) != 2 || hasID(r.Findings, "logs.unfinished") {
		t.Fatalf("logs summary = %+v, captured %d", s, len(r.Logs))
	}
}

// A capture that the caller cancels reports an early stop, not an error.
func TestCaptureLogsStopsEarlyOnCancel(t *testing.T) {
	client := fakeController(t, 0)
	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)
	lines, dropped, end := captureLogs(ctx, client, "debug", time.Minute, time.Now)
	if !end.connected || !end.early || end.err != nil || dropped != 0 || len(lines) != 2 {
		t.Fatalf("lines=%d dropped=%d end=%+v", len(lines), dropped, end)
	}
}
