package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	websocketHandshakeTimeout = 10 * time.Second
	liveStreamReadTimeout     = 15 * time.Second
	maxWebSocketMessage       = 16 << 20
)

// StreamTraffic opens a WebSocket to /traffic and sends TrafficData on the channel.
// Blocks until context is cancelled or an error occurs.
func (c *Client) StreamTraffic(ctx context.Context, ch chan<- TrafficData) error {
	conn, err := c.dialWS(ctx, "/traffic")
	if err != nil {
		return fmt.Errorf("ws /traffic: %w", err)
	}
	defer conn.Close()
	defer close(ch)
	stopCancelWatch := closeOnCancel(ctx, conn)
	defer stopCancelWatch()
	conn.SetReadLimit(maxWebSocketMessage)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_ = conn.SetReadDeadline(time.Now().Add(liveStreamReadTimeout))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("read traffic: %w", err)
			}
			var data TrafficData
			if err := json.Unmarshal(msg, &data); err != nil {
				continue
			}
			select {
			case ch <- data:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// StreamConnections opens a WebSocket to /connections and sends snapshots on the channel.
func (c *Client) StreamConnections(ctx context.Context, ch chan<- ConnectionsSnapshot) error {
	conn, err := c.dialWS(ctx, "/connections")
	if err != nil {
		return fmt.Errorf("ws /connections: %w", err)
	}
	defer conn.Close()
	defer close(ch)
	stopCancelWatch := closeOnCancel(ctx, conn)
	defer stopCancelWatch()
	conn.SetReadLimit(maxWebSocketMessage)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_ = conn.SetReadDeadline(time.Now().Add(liveStreamReadTimeout))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("read connections: %w", err)
			}
			var snap ConnectionsSnapshot
			if err := json.Unmarshal(msg, &snap); err != nil {
				continue
			}
			select {
			case ch <- snap:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// StreamLogs opens a WebSocket to /logs and sends log entries on the channel.
// level is the minimum level (debug returns everything for client-side filtering).
func (c *Client) StreamLogs(ctx context.Context, level string, ch chan<- LogEntry) error {
	if level == "" {
		level = "debug"
	}
	conn, err := c.dialWS(ctx, "/logs?level="+level)
	if err != nil {
		return fmt.Errorf("ws /logs: %w", err)
	}
	defer conn.Close()
	defer close(ch)
	stopCancelWatch := closeOnCancel(ctx, conn)
	defer stopCancelWatch()
	conn.SetReadLimit(maxWebSocketMessage)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("read logs: %w", err)
			}
			var entry LogEntry
			if err := json.Unmarshal(msg, &entry); err != nil {
				continue
			}
			select {
			case ch <- entry:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func (c *Client) dialWS(ctx context.Context, path string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{HandshakeTimeout: websocketHandshakeTimeout}
	if c.isUnix {
		sock := c.addr
		dialer.NetDialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialControl(ctx, sock)
		}
	}
	var header http.Header
	if c.secret != "" {
		header = http.Header{"Authorization": []string{"Bearer " + c.secret}}
	}
	conn, _, err := dialer.DialContext(ctx, c.wsBase+path, header)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func closeOnCancel(ctx context.Context, conn *websocket.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}
