package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// ProxyProvider is one entry of GET /providers/proxies. mihomo also lists
// every proxy group here with VehicleType "Compatible"; those are not
// subscriptions.
type ProxyProvider struct {
	Name             string            `json:"name"`
	VehicleType      string            `json:"vehicleType"`
	Proxies          []Proxy           `json:"proxies"`
	UpdatedAt        time.Time         `json:"updatedAt"`
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`
}

// IsGroup reports whether the entry is a proxy group mirrored as a provider.
func (p ProxyProvider) IsGroup() bool { return p.VehicleType == "Compatible" }

// SubscriptionInfo is parsed by mihomo from the subscription-userinfo header.
// Byte counts are totals; Expire is a Unix timestamp, 0 when unknown.
type SubscriptionInfo struct {
	Upload   int64 `json:"Upload"`
	Download int64 `json:"Download"`
	Total    int64 `json:"Total"`
	Expire   int64 `json:"Expire"`
}

// RuleProvider is one entry of GET /providers/rules.
type RuleProvider struct {
	Name        string    `json:"name"`
	Behavior    string    `json:"behavior"`
	Format      string    `json:"format"`
	VehicleType string    `json:"vehicleType"`
	RuleCount   int       `json:"ruleCount"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// GetProxyProviders returns proxy providers keyed by name.
func (c *Client) GetProxyProviders() (map[string]ProxyProvider, error) {
	var resp struct {
		Providers map[string]ProxyProvider `json:"providers"`
	}
	if err := c.getJSON("/providers/proxies", &resp); err != nil {
		return nil, err
	}
	return resp.Providers, nil
}

// GetRuleProviders returns rule providers keyed by name.
func (c *Client) GetRuleProviders() (map[string]RuleProvider, error) {
	var resp struct {
		Providers map[string]RuleProvider `json:"providers"`
	}
	if err := c.getJSON("/providers/rules", &resp); err != nil {
		return nil, err
	}
	return resp.Providers, nil
}

// UpdateProxyProvider re-fetches a proxy provider (PUT /providers/proxies/{name}).
func (c *Client) UpdateProxyProvider(name string) error {
	_, err := c.put("/providers/proxies/"+url.PathEscape(name), nil)
	return err
}

// UpdateRuleProvider re-fetches a rule provider (PUT /providers/rules/{name}).
func (c *Client) UpdateRuleProvider(name string) error {
	_, err := c.put("/providers/rules/"+url.PathEscape(name), nil)
	return err
}

// DNSAnswer is one answer record of GET /dns/query.
type DNSAnswer struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

// DNSResult is the core's resolver reply; Status is the DNS rcode.
type DNSResult struct {
	Status int         `json:"Status"`
	Answer []DNSAnswer `json:"Answer"`
}

// QueryDNS resolves name through the core's DNS (GET /dns/query).
func (c *Client) QueryDNS(name, qtype string) (*DNSResult, error) {
	var result DNSResult
	path := "/dns/query?name=" + url.QueryEscape(name) + "&type=" + url.QueryEscape(qtype)
	if err := c.getJSON(path, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) getJSON(path string, v any) error {
	data, err := c.get(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal %s: %w", path, err)
	}
	return nil
}
