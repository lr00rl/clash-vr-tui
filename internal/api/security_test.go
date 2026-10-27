package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint Endpoint
		wantErr  string
	}{
		{name: "unix socket", endpoint: Endpoint{Socket: "/tmp/mihomo.sock", Secret: "sek"}},
		{name: "loopback ipv4 with secret", endpoint: Endpoint{Server: "127.0.0.1:9090", Secret: "sek"}},
		{name: "loopback ipv6 with secret", endpoint: Endpoint{Server: "[::1]:9090", Secret: "sek"}},
		{name: "localhost with secret", endpoint: Endpoint{Server: "localhost:9090", Secret: "sek"}},
		{name: "remote without secret", endpoint: Endpoint{Server: "controller.example:9090"}},
		{name: "remote with secret", endpoint: Endpoint{Server: "controller.example:9090", Secret: "sek"}, wantErr: "refusing to send controller secret"},
		{name: "missing server", endpoint: Endpoint{}, wantErr: "server is empty"},
		{name: "missing port", endpoint: Endpoint{Server: "localhost"}, wantErr: "want host:port"},
		{name: "invalid port", endpoint: Endpoint{Server: "localhost:70000"}, wantErr: "invalid controller server port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEndpoint(tt.endpoint)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateEndpoint() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateEndpoint() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestRESTResponseBodyLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBody+1)))
	}))
	defer srv.Close()

	c := NewWith(Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})
	_, err := c.get("/large")
	if err == nil || !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("get large response error = %v", err)
	}
}

func TestWebSocketDialHonorsContextCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	release := make(chan struct{})
	defer close(release)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		<-release
	}()

	c := NewWith(Endpoint{Server: listener.Addr().String()})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = c.dialWS(ctx, "/traffic")
	if err == nil {
		t.Fatal("dialWS returned no error for a stalled handshake")
	}
	if elapsed := time.Since(started); elapsed < 20*time.Millisecond || elapsed > time.Second {
		t.Fatalf("dialWS context deadline took %v", elapsed)
	}
}

func TestLogsStreamMayIdleUntilCancelled(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-time.After(250 * time.Millisecond)
	}))
	defer srv.Close()

	c := NewWith(Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := c.StreamLogs(ctx, "debug", make(chan LogEntry, 1))
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) < 80*time.Millisecond {
		t.Fatalf("StreamLogs returned before cancellation: elapsed=%v err=%v", time.Since(started), err)
	}
}

func TestWebSocketMessageLimit(t *testing.T) {
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(strings.Repeat("x", maxWebSocketMessage+1)))
	}))
	defer srv.Close()

	c := NewWith(Endpoint{Server: strings.TrimPrefix(srv.URL, "http://")})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.StreamLogs(ctx, "debug", make(chan LogEntry, 1))
	if err == nil || !strings.Contains(err.Error(), "read limit") {
		t.Fatalf("StreamLogs oversized message error = %v", err)
	}
}
