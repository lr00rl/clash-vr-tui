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

func (c *Client) dialWS(path string) (*websocket.Conn, error) {
	dialer := websocket.Dialer{
		NetDialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", c.socketPath)
		},
		Proxy: http.ProxyFromEnvironment,
	}
	conn, _, err := dialer.Dial("ws://localhost"+path, nil)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
