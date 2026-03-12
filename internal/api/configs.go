package api

import (
	"encoding/json"
	"fmt"
)

// GetConfig returns the current runtime config.
func (c *Client) GetConfig() (*Config, error) {
	data, err := c.get("/configs")
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &cfg, nil
}

// PatchConfig updates runtime config fields.
func (c *Client) PatchConfig(patch ConfigPatch) error {
	_, err := c.patch("/configs", patch)
	return err
}

// GetVersion returns mihomo version info.
func (c *Client) GetVersion() (*VersionInfo, error) {
	data, err := c.get("/version")
	if err != nil {
		return nil, err
	}
	var ver VersionInfo
	if err := json.Unmarshal(data, &ver); err != nil {
		return nil, fmt.Errorf("unmarshal version: %w", err)
	}
	return &ver, nil
}
