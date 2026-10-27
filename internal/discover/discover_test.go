package discover

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeSources(ps string, files map[string]string, existing ...string) sources {
	exists := map[string]bool{}
	for _, p := range existing {
		exists[p] = true
	}
	return sources{
		goos:   "darwin",
		uid:    501,
		home:   "/Users/me",
		tmpDir: "/var/folders/xx/T",
		ps:     func() (string, error) { return ps, nil },
		readFile: func(p string) ([]byte, error) {
			if data, ok := files[p]; ok {
				return []byte(data), nil
			}
			return nil, os.ErrNotExist
		},
		exists: func(p string) (bool, error) { return exists[p], nil },
	}
}

// The Clash Verge Rev service on macOS starts the core with paths that contain
// spaces and overrides the socket its generated config names.
const vergeServicePS = `/Applications/Clash Verge.app/Contents/MacOS/clash-verge
/Library/Application Support/clash-verge-service/cores/verge-mihomo -d /Library/Application Support/clash-verge-service/users/501/runtime -f /Library/Application Support/clash-verge-service/users/501/runtime/config.yaml -ext-ctl-unix /var/run/clash-verge-service/users/501/verge-mihomo.sock
/usr/local/bin/clash-vr-tui -socket /tmp/x.sock`

func TestGatherPrefersProcessFlagsOverConfig(t *testing.T) {
	serviceSock := "/var/run/clash-verge-service/users/501/verge-mihomo.sock"
	vergeCfg := "/Users/me/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev/clash-verge.yaml"
	staleSock := "/var/folders/xx/T/verge-mihomo.sock"
	src := fakeSources(vergeServicePS,
		map[string]string{vergeCfg: "external-controller: ''\nexternal-controller-unix: " + staleSock + "\nsecret: s3\n"},
		serviceSock, staleSock)

	got := gather(src)
	if len(got) != 2 {
		t.Fatalf("want service socket and the config socket, got %+v", got)
	}
	if got[0].Address != serviceSock || got[0].Kind != KindSocket {
		t.Fatalf("first candidate = %+v, want the process socket", got[0])
	}
	if !strings.Contains(got[0].Sources[0], "verge-mihomo -ext-ctl-unix") {
		t.Fatalf("source = %v", got[0].Sources)
	}
	// The same path is also a known path; it must merge, not duplicate.
	if len(got[0].Sources) < 2 {
		t.Fatalf("known path should merge into the process candidate: %v", got[0].Sources)
	}
	if got[1].Address != staleSock || !strings.HasPrefix(got[1].Sources[0], "config ~/Library/") {
		t.Fatalf("second candidate = %+v", got[1])
	}
}

func TestGatherDropsMissingSocketsKeepsTCP(t *testing.T) {
	cfg := "/Users/me/.config/mihomo/config.yaml"
	src := fakeSources("", map[string]string{cfg: "external-controller: 0.0.0.0:9090\nsecret: pw\nexternal-controller-unix: mihomo.sock\n"})
	got := gather(src)
	if len(got) != 1 {
		t.Fatalf("missing sockets should be dropped, got %+v", got)
	}
	if got[0].Kind != KindTCP || got[0].Address != "127.0.0.1:9090" || got[0].Secret != "pw" {
		t.Fatalf("tcp candidate = %+v", got[0])
	}
}

func TestFromConfigResolvesRelativeSocket(t *testing.T) {
	got := fromConfig([]byte("external-controller-unix: mihomo.sock\n"), "/etc/mihomo", "config")
	if len(got) != 1 || got[0].Address != "/etc/mihomo/mihomo.sock" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeTCP(t *testing.T) {
	cases := map[string]string{
		":9090": "127.0.0.1:9090", "0.0.0.0:9097": "127.0.0.1:9097", "[::]:9090": "127.0.0.1:9090",
		"*:9090": "127.0.0.1:9090", "127.0.0.1:9090": "127.0.0.1:9090", "192.168.1.2:9090": "192.168.1.2:9090",
		"": "", "9090": "",
	}
	for in, want := range cases {
		if got := normalizeTCP(in); got != want {
			t.Errorf("normalizeTCP(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestProcessSecretAppliesToTCP(t *testing.T) {
	ps := "/usr/bin/mihomo -d /etc/mihomo -ext-ctl 0.0.0.0:9090 -secret abc"
	got := fromProcesses(ps, fakeSources("", nil))
	if len(got) != 1 || got[0].Address != "127.0.0.1:9090" || got[0].Secret != "abc" {
		t.Fatalf("got %+v", got)
	}
}

func versionServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"version": "v1.19.32", "meta": true})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestProbeClassifiesOutcomes(t *testing.T) {
	ok := versionServer(t, http.StatusOK)
	auth := versionServer(t, http.StatusUnauthorized)
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	refusedAddr := closed.Addr().String()
	closed.Close()

	sockDir, err := os.MkdirTemp("", "cvt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(sockDir) })
	sock := filepath.Join(sockDir, "m.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	unixSrv := &http.Server{Handler: ok.Config.Handler}
	go unixSrv.Serve(ln)
	t.Cleanup(func() { unixSrv.Close() })

	cands := []Candidate{
		{Kind: KindTCP, Address: strings.TrimPrefix(ok.URL, "http://")},
		{Kind: KindTCP, Address: strings.TrimPrefix(auth.URL, "http://")},
		{Kind: KindTCP, Address: refusedAddr},
		{Kind: KindTCP, Address: "192.168.77.1:9090", Secret: "pw"},
		{Kind: KindSocket, Address: sock},
	}
	probeAll(context.Background(), cands)
	want := []State{StateOK, StateUnauthorized, StateRefused, StateRejected, StateOK}
	for i, c := range cands {
		if c.State != want[i] {
			t.Errorf("%s: state %s (%s), want %s", c.Address, c.State, c.Err, want[i])
		}
	}
	if cands[0].Version != "v1.19.32" {
		t.Errorf("version = %q", cands[0].Version)
	}
	best, found := Best(cands)
	if !found || best.Address != cands[0].Address {
		t.Errorf("Best = %+v, %v", best, found)
	}
}

func TestClassifyPermission(t *testing.T) {
	if got := classify(&os.PathError{Op: "dial", Path: "/x", Err: os.ErrPermission}); got != StateDenied {
		t.Fatalf("got %s", got)
	}
	if got := classify(errors.New("weird")); got != StateError {
		t.Fatalf("got %s", got)
	}
}
