package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// GetProxies returns all proxies.
func (c *Client) GetProxies() (*ProxiesResponse, error) {
	data, err := c.get("/proxies")
	if err != nil {
		return nil, err
	}
	var resp ProxiesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal proxies: %w", err)
	}
	return &resp, nil
}

// GetGroups returns all proxy groups with their nodes.
func (c *Client) GetGroups() ([]Group, error) {
	groups, err := c.getGroups()
	if err != nil {
		return nil, err
	}
	order, err := c.getGlobalGroupOrder()
	if err != nil || len(order) == 0 {
		return groups, nil
	}
	return orderGroupsByName(groups, order), nil
}

func (c *Client) getGroups() ([]Group, error) {
	data, err := c.get("/group")
	if err != nil {
		return nil, err
	}
	var resp GroupsResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal groups: %w", err)
	}
	return resp.Proxies, nil
}

func (c *Client) getGlobalGroupOrder() ([]string, error) {
	resp, err := c.GetProxies()
	if err != nil {
		return nil, err
	}
	global, ok := resp.Proxies["GLOBAL"]
	if !ok {
		return nil, nil
	}
	return global.All, nil
}

func orderGroupsByName(groups []Group, order []string) []Group {
	byName := make(map[string]Group, len(groups))
	for _, g := range groups {
		byName[g.Name] = g
	}

	seen := make(map[string]bool, len(groups))
	result := make([]Group, 0, len(groups))
	for _, name := range order {
		g, ok := byName[name]
		if !ok || seen[name] {
			continue
		}
		result = append(result, g)
		seen[name] = true
	}

	for _, g := range groups {
		if seen[g.Name] {
			continue
		}
		result = append(result, g)
	}
	return result
}

// GetProxy returns a single proxy by name.
func (c *Client) GetProxy(name string) (*Proxy, error) {
	data, err := c.get("/proxies/" + url.PathEscape(name))
	if err != nil {
		return nil, err
	}
	var proxy Proxy
	if err := json.Unmarshal(data, &proxy); err != nil {
		return nil, fmt.Errorf("unmarshal proxy: %w", err)
	}
	return &proxy, nil
}

// SelectProxy selects a proxy node within a group.
func (c *Client) SelectProxy(group, proxy string) error {
	payload := map[string]string{"name": proxy}
	_, err := c.put("/proxies/"+url.PathEscape(group), payload)
	return err
}

// UnfixProxy clears a fixed selection for a URLTest/Fallback group, restoring
// automatic selection (DELETE /proxies/{group}).
func (c *Client) UnfixProxy(group string) error {
	return c.delete("/proxies/" + url.PathEscape(group))
}

// TestGroupDelay tests delay for all nodes in a group.
func (c *Client) TestGroupDelay(group string, testURL string, timeout int) (GroupDelayResult, error) {
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}
	if timeout == 0 {
		timeout = 5000
	}
	path := fmt.Sprintf("/group/%s/delay?url=%s&timeout=%d",
		url.PathEscape(group),
		url.QueryEscape(testURL),
		timeout,
	)
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result GroupDelayResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal group delay: %w", err)
	}
	return result, nil
}

// TestProxyDelay tests delay for a single proxy node.
func (c *Client) TestProxyDelay(name string, testURL string, timeout int) (*DelayResult, error) {
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}
	if timeout == 0 {
		timeout = 5000
	}
	path := fmt.Sprintf("/proxies/%s/delay?url=%s&timeout=%d",
		url.PathEscape(name),
		url.QueryEscape(testURL),
		timeout,
	)
	data, err := c.get(path)
	if err != nil {
		return nil, err
	}
	var result DelayResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("unmarshal delay: %w", err)
	}
	return &result, nil
}
