package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// captureStdout runs f and returns what it printed.
func captureStdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	runErr := f()
	w.Close()
	os.Stdout = orig
	return <-done, runErr
}

func TestConfigSetFailsWhenCoreDoesNotKeepValue(t *testing.T) {
	var mu sync.Mutex
	tun := true
	keep := false
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.Method {
		case http.MethodPatch:
			var p map[string]map[string]bool
			_ = json.NewDecoder(r.Body).Decode(&p)
			if keep {
				tun = p["tun"]["enable"]
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"tun": map[string]any{"enable": tun}, "authentication": []string{"u:secretpw"}})
		}
	})

	if err := setConfig(client, "tun", "off"); err == nil || !strings.Contains(err.Error(), "did not keep") {
		t.Fatalf("ignored patch should fail, got %v", err)
	}
	keep = true
	if _, err := captureStdout(t, func() error { return setConfig(client, "tun", "off") }); err != nil {
		t.Fatalf("kept patch should succeed: %v", err)
	}
	if err := setConfig(client, "tun", "maybe"); err == nil {
		t.Fatal("bad value should be a usage error")
	}

	out, err := captureStdout(t, func() error { return cmdConfig(client, options{json: true}) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "secretpw") || !strings.Contains(out, `"inbound_auth": true`) {
		t.Fatalf("config output must report auth presence without credentials:\n%s", out)
	}
}

func TestProvidersListAndUpdate(t *testing.T) {
	now = func() time.Time { return time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { now = time.Now })
	var mu sync.Mutex
	var updated []string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodPut:
			updated = append(updated, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/providers/proxies":
			_, _ = w.Write([]byte(`{"providers":{
				"Proxy":{"name":"Proxy","vehicleType":"Compatible","proxies":[{"name":"a"}]},
				"sub":{"name":"sub","vehicleType":"HTTP","proxies":[{"name":"a"},{"name":"b"}],
					"updatedAt":"2026-10-25T12:00:00Z",
					"subscriptionInfo":{"Upload":1073741824,"Download":1073741824,"Total":10737418240,"Expire":1793102400}}}}`))
		case r.URL.Path == "/providers/rules":
			_, _ = w.Write([]byte(`{"providers":{"ads":{"name":"ads","behavior":"Domain","vehicleType":"HTTP","ruleCount":12,"updatedAt":"2026-10-27T11:00:00Z"},
				"local":{"name":"local","behavior":"Classical","vehicleType":"File","ruleCount":3}}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	})

	out, err := captureStdout(t, func() error { return cmdProviders(client, options{}) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sub", "2.0 GB / 10.0 GB (20%)", "2d ago", "(in ", "ads", "1h ago"} {
		if !strings.Contains(out, want) {
			t.Errorf("providers output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Compatible") {
		t.Errorf("mirrored groups must not be listed as providers:\n%s", out)
	}

	if _, err := captureStdout(t, func() error { return cmdProviders(client, options{args: []string{"update"}}) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(updated, ",") != "/providers/rules/ads,/providers/proxies/sub" {
		t.Fatalf("update all should hit only HTTP providers, got %v", updated)
	}
	if err := cmdProviders(client, options{args: []string{"update", "nope"}}); err == nil {
		t.Fatal("unknown provider should fail")
	}
}

func TestCloseByHost(t *testing.T) {
	var mu sync.Mutex
	var closed []string
	client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodDelete {
			closed = append(closed, strings.TrimPrefix(r.URL.Path, "/connections/"))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = w.Write([]byte(`{"connections":[
			{"id":"1","metadata":{"host":"api.github.com"}},
			{"id":"2","metadata":{"host":"example.org"}},
			{"id":"3","metadata":{"host":"","destinationIP":"140.82.112.3"}}]}`))
	})
	if _, err := captureStdout(t, func() error { return cmdClose(client, options{host: "GitHub"}) }); err != nil {
		t.Fatal(err)
	}
	if strings.Join(closed, ",") != "1" {
		t.Fatalf("closed %v, want only the github connection", closed)
	}
	if err := cmdClose(client, options{}); err == nil {
		t.Fatal("close without targets should be a usage error")
	}
}
