package api

import (
	"encoding/json"
	"fmt"
)

// GetRules returns all rules.
func (c *Client) GetRules() (*RulesResponse, error) {
	data, err := c.get("/rules")
	if err != nil {
		return nil, err
	}
	var resp RulesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal rules: %w", err)
	}
	return &resp, nil
}
