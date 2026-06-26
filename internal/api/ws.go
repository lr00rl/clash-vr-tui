package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/websocket"
)

// StreamTraffic opens a WebSocket to /traffic and sends TrafficData on the channel.
// Blocks until context is cancelled or an error occurs.
func (c *Client) StreamTraffic(ctx context.Context, ch chan<- TrafficData) error {
	conn, err := c.dialWS("/traffic")
	if err != nil {
		return fmt.Errorf("ws /traffic: %w", err)
	}
	defer conn.Close()
	defer close(ch)
	// Closing the conn on cancellation unblocks the blocking ReadMessage below.
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
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
	conn, err := c.dialWS("/connections")
	if err != nil {
		return fmt.Errorf("ws /connections: %w", err)
	}
	defer conn.Close()
	defer close(ch)
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
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
	conn, err := c.dialWS("/logs?level=" + level)
	if err != nil {
		return fmt.Errorf("ws /logs: %w", err)
	}
	defer conn.Close()
	defer close(ch)
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			_, msg, err := conn.ReadMessage()
			if err != nil {
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

func (c *Client) dialWS(path string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment}
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
	conn, _, err := dialer.Dial(c.wsBase+path, header)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
