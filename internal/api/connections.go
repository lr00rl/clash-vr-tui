package api

import (
	"encoding/json"
	"fmt"
)

// GetConnections returns the current connections snapshot.
func (c *Client) GetConnections() (*ConnectionsSnapshot, error) {
	data, err := c.get("/connections")
	if err != nil {
		return nil, err
	}
	var snap ConnectionsSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal connections: %w", err)
	}
	return &snap, nil
}

// CloseConnection closes a single connection by ID.
func (c *Client) CloseConnection(id string) error {
	return c.delete("/connections/" + id)
}

// CloseAllConnections closes all active connections.
func (c *Client) CloseAllConnections() error {
	return c.delete("/connections")
}
