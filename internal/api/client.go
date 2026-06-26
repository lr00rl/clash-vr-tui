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

// Endpoint describes how to reach the mihomo controller: either a Unix socket
// (clash-verge default) or a TCP external-controller (host:port + optional
// secret), which also enables SSH-tunneled and plain-mihomo setups.
type Endpoint struct {
	Socket string // Unix socket path; takes precedence if set
	Server string // host:port for an external controller
	Secret string // bearer secret for the external controller
}

// Client communicates with the mihomo core over a Unix socket or TCP.
type Client struct {
	http    *http.Client
	addr    string // socket path or host:port, for display
	secret  string
	baseURL string
	wsBase  string
	isUnix  bool
}

// NewClient creates a Client connected to a mihomo Unix socket.
func NewClient(socketPath string) *Client {
	return NewWith(Endpoint{Socket: socketPath})
}

// NewWith creates a Client from an Endpoint (Unix socket or TCP controller).
func NewWith(ep Endpoint) *Client {
	transport := &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 30 * time.Second,
	}
	c := &Client{secret: ep.Secret}
	if ep.Socket != "" {
		socketPath := ep.Socket
		transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialControl(ctx, socketPath)
		}
		c.addr = socketPath
		c.baseURL = "http://localhost"
		c.wsBase = "ws://localhost"
		c.isUnix = true
	} else {
		c.addr = ep.Server
		c.baseURL = "http://" + ep.Server
		c.wsBase = "ws://" + ep.Server
	}
	c.http = &http.Client{Transport: transport, Timeout: 10 * time.Second}
	return c
}

// DefaultSocketPath returns the platform-appropriate socket path.
func DefaultSocketPath() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\verge-mihomo`
	}
	return "/tmp/verge/verge-mihomo.sock"
}

// SocketPath returns the configured address (socket path or host:port).
func (c *Client) SocketPath() string {
	return c.addr
}

// auth attaches the bearer secret to a request, if configured.
func (c *Client) auth(req *http.Request) {
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
}

func (c *Client) get(path string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	c.auth(req)
	resp, err := c.http.Do(req)
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

// post sends a POST request with an optional JSON body (nil for none). Used for
// system operations like /restart, /configs/geo, and /cache/*/flush.
func (c *Client) post(path string, payload interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(body))
	}
	return body, nil
}
