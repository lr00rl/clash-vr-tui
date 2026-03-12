package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"time"
)

// Client communicates with mihomo core over a Unix socket.
type Client struct {
	http       *http.Client
	socketPath string
	baseURL    string
}

// NewClient creates a Client connected to the mihomo Unix socket.
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
			if runtime.GOOS == "windows" {
				return net.Dial("unix", socketPath)
			}
			return net.Dial("unix", socketPath)
		},
		MaxIdleConns:       10,
		IdleConnTimeout:    30 * time.Second,
	}

	return &Client{
		http: &http.Client{
			Transport: transport,
			Timeout:   10 * time.Second,
		},
		socketPath: socketPath,
		baseURL:    "http://localhost",
	}
}

// DefaultSocketPath returns the platform-appropriate socket path.
func DefaultSocketPath() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\verge-mihomo`
	}
	return "/tmp/verge/verge-mihomo.sock"
}

// SocketPath returns the configured socket path.
func (c *Client) SocketPath() string {
	return c.socketPath
}

func (c *Client) get(path string) ([]byte, error) {
	resp, err := c.http.Get(c.baseURL + path)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("GET %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *Client) put(path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPut, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PUT %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PUT %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *Client) patch(path string, payload interface{}) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("PATCH %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("PATCH %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *Client) delete(path string) error {
	req, err := http.NewRequest(http.MethodDelete, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DELETE %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return nil
}
